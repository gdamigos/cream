package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

// tokens live in .credentials.json (keychain on macos), the account in the oauthAccount block of .claude.json
type Claude struct{}

const claudeKeychainService = "Claude Code-credentials"

type claudeSecret struct {
	Credentials  string          `json:"credentials"`
	OAuthAccount json.RawMessage `json:"oauthAccount,omitempty"`
}

func (Claude) ID() string { return "claude" }

func (Claude) Name() string { return "Claude Code" }

func claudeConfigDir(env *adapter.Env) string {
	if d := env.Env("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return env.Paths.InHome(".claude")
}

func claudeCredFile(env *adapter.Env) string {
	return filepath.Join(claudeConfigDir(env), ".credentials.json")
}

func claudeStateFile(env *adapter.Env) string {
	if d := env.Env("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".claude.json")
	}
	return env.Paths.InHome(".claude.json")
}

func (c Claude) useKeychain(env *adapter.Env) bool {
	return env.OS() == "darwin" && env.Env("CLAUDE_CONFIG_DIR") == ""
}

func (c Claude) Files(env *adapter.Env) []string {
	return []string{claudeCredFile(env), claudeStateFile(env)}
}

func (c Claude) readCredentials(ctx context.Context, env *adapter.Env) (string, bool, error) {
	if c.useKeychain(env) {
		out, err := env.Run.Run(ctx, nil, nil, "/usr/bin/security", "find-generic-password", "-s", claudeKeychainService, "-w")
		if err != nil {
			if strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "exit status 44") {
				return "", false, nil
			}
			return "", false, err
		}
		return strings.TrimSpace(string(out)), true, nil
	}
	b, ok, err := fsutil.ReadFileIfExists(claudeCredFile(env))
	return string(b), ok, err
}

func (c Claude) writeCredentials(ctx context.Context, env *adapter.Env, creds string) error {
	if c.useKeychain(env) {
		user := env.Env("USER")
		if user == "" {
			user = filepath.Base(env.Paths.Home)
		}
		// security -i reads stdin so the secret stays out of argv, -X takes hex
		cmd := fmt.Sprintf("add-generic-password -U -a %q -s %q -X %s\n", user, claudeKeychainService, hex.EncodeToString([]byte(creds)))
		_, err := env.Run.Run(ctx, []byte(cmd), nil, "/usr/bin/security", "-i")
		return err
	}
	return fsutil.WriteFileAtomic(claudeCredFile(env), []byte(creds), 0o600)
}

func readJSONMap(path string) (map[string]json.RawMessage, bool, error) {
	b, ok, err := fsutil.ReadFileIfExists(path)
	if err != nil || !ok {
		return map[string]json.RawMessage{}, ok, err
	}
	m := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return m, true, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, true, nil
}

func writeJSONMap(path string, m map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, append(b, '\n'), 0o600)
}

func claudeEmail(raw json.RawMessage) string {
	var acct struct {
		Email string `json:"emailAddress"`
		Org   string `json:"organizationName"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &acct) != nil {
		return ""
	}
	return acct.Email
}

func (c Claude) Status(ctx context.Context, env *adapter.Env) adapter.Status {
	var s adapter.Status
	if _, err := env.Run.LookPath("claude"); err == nil {
		s.Installed = true
	}
	if fsutil.Exists(claudeConfigDir(env)) || fsutil.Exists(claudeStateFile(env)) {
		s.Installed = true
	}
	hasCreds := false
	if c.useKeychain(env) {
		// no -w, reading the secret could show a keychain prompt
		_, err := env.Run.Run(ctx, nil, nil, "/usr/bin/security", "find-generic-password", "-s", claudeKeychainService)
		hasCreds = err == nil
		s.Detail = "macOS Keychain"
	} else {
		hasCreds = fsutil.Exists(claudeCredFile(env))
		s.Detail = claudeCredFile(env)
	}
	m, _, err := readJSONMap(claudeStateFile(env))
	if err != nil {
		s.Warning = err.Error()
	}
	s.Account = claudeEmail(m["oauthAccount"])
	s.LoggedIn = hasCreds && s.Account != ""
	if hasCreds && s.Account == "" {
		s.LoggedIn = true
		s.Account = "(unknown account)"
	}
	return s
}

func (c Claude) Import(ctx context.Context, env *adapter.Env, p *config.Profile) (*config.Account, error) {
	creds, ok, err := c.readCredentials(ctx, env)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(creds) == "" {
		return nil, errors.New("not logged in (run `claude` and /login first)")
	}
	m, _, err := readJSONMap(claudeStateFile(env))
	if err != nil {
		return nil, err
	}
	sec := claudeSecret{Credentials: creds, OAuthAccount: m["oauthAccount"]}
	b, err := json.Marshal(sec)
	if err != nil {
		return nil, err
	}
	if err := env.Vault.Set(vault.ProfileKey(p.Name, c.ID()), b); err != nil {
		return nil, err
	}
	label := claudeEmail(sec.OAuthAccount)
	if label == "" {
		label = "claude account"
	}
	return &config.Account{Label: label, Imported: time.Now().UTC().Truncate(time.Second)}, nil
}

func (c Claude) loadSecret(env *adapter.Env, p *config.Profile) (*claudeSecret, error) {
	b, err := env.Vault.Get(vault.ProfileKey(p.Name, c.ID()))
	if errors.Is(err, vault.ErrNotFound) {
		return nil, adapter.ErrNotImported{Adapter: c.ID(), Profile: p.Name}
	}
	if err != nil {
		return nil, err
	}
	var sec claudeSecret
	if err := json.Unmarshal(b, &sec); err != nil {
		return nil, fmt.Errorf("claude: corrupt vault entry: %w", err)
	}
	return &sec, nil
}

func (c Claude) Activate(ctx context.Context, env *adapter.Env, p *config.Profile, dryRun bool) ([]adapter.Change, error) {
	sec, err := c.loadSecret(env, p)
	if err != nil {
		return nil, err
	}
	var changes []adapter.Change
	if c.useKeychain(env) {
		changes = append(changes, adapter.Change{Adapter: c.ID(), Kind: "keyring", Target: "macOS Keychain \"" + claudeKeychainService + "\"", Action: "replace"})
	} else {
		changes = append(changes, adapter.Change{Adapter: c.ID(), Kind: "file", Target: claudeCredFile(env), Action: "replace"})
	}
	changes = append(changes, adapter.Change{Adapter: c.ID(), Kind: "file", Target: claudeStateFile(env) + " (oauthAccount)", Action: "patch"})
	if dryRun {
		return changes, nil
	}
	if err := c.writeCredentials(ctx, env, sec.Credentials); err != nil {
		return nil, err
	}
	m, _, err := readJSONMap(claudeStateFile(env))
	if err != nil {
		return nil, err
	}
	if len(sec.OAuthAccount) > 0 {
		m["oauthAccount"] = sec.OAuthAccount
	} else {
		delete(m, "oauthAccount")
	}
	if err := writeJSONMap(claudeStateFile(env), m); err != nil {
		return nil, err
	}
	return changes, nil
}

func (c Claude) SessionEnv(ctx context.Context, env *adapter.Env, p *config.Profile) ([]string, func() error, error) {
	sec, err := c.loadSecret(env, p)
	if err != nil {
		return nil, nil, err
	}
	dir := env.Paths.SessionDir(p.Name, c.ID())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	cred := filepath.Join(dir, ".credentials.json")
	if err := fsutil.WriteFileAtomic(cred, []byte(sec.Credentials), 0o600); err != nil {
		return nil, nil, err
	}
	state := filepath.Join(dir, ".claude.json")
	m, existed, err := readJSONMap(state)
	if err != nil {
		return nil, nil, err
	}
	if !existed {
		if g, _, gerr := readJSONMap(claudeStateFile(env)); gerr == nil {
			for _, k := range []string{"hasCompletedOnboarding", "theme", "lastOnboardingVersion"} {
				if v, ok := g[k]; ok {
					m[k] = v
				}
			}
		}
	}
	if len(sec.OAuthAccount) > 0 {
		m["oauthAccount"] = sec.OAuthAccount
	}
	if err := writeJSONMap(state, m); err != nil {
		return nil, nil, err
	}
	settings := filepath.Join(dir, "settings.json")
	if !fsutil.Exists(settings) {
		if b, ok, _ := fsutil.ReadFileIfExists(filepath.Join(claudeConfigDir(env), "settings.json")); ok {
			_ = fsutil.WriteFileAtomic(settings, b, 0o600)
		}
	}
	finish := func() error {
		// claude refreshes tokens while running
		b, ok, err := fsutil.ReadFileIfExists(cred)
		if err != nil || !ok || len(b) == 0 {
			return err
		}
		if string(b) == sec.Credentials {
			return nil
		}
		latest, _, _ := readJSONMap(state)
		ns := claudeSecret{Credentials: string(b), OAuthAccount: latest["oauthAccount"]}
		if len(ns.OAuthAccount) == 0 {
			ns.OAuthAccount = sec.OAuthAccount
		}
		nb, err := json.Marshal(ns)
		if err != nil {
			return err
		}
		return env.Vault.Set(vault.ProfileKey(p.Name, c.ID()), nb)
	}
	return []string{"CLAUDE_CONFIG_DIR=" + dir}, finish, nil
}

func (c Claude) Verify(ctx context.Context, env *adapter.Env) (string, error) {
	s := c.Status(ctx, env)
	if !s.LoggedIn {
		return "", errors.New("not logged in")
	}
	return s.Account, nil
}

func (c Claude) Snapshot(env *adapter.Env) (adapter.Snapshot, error) {
	if !c.useKeychain(env) {
		return nil, nil
	}
	creds, ok, err := c.readCredentials(context.Background(), env)
	if err != nil || !ok {
		return nil, err
	}
	return adapter.Snapshot{"keychain": []byte(creds)}, nil
}

func (c Claude) RestoreSnapshot(env *adapter.Env, s adapter.Snapshot) error {
	if !c.useKeychain(env) {
		return nil
	}
	if v, ok := s["keychain"]; ok {
		return c.writeCredentials(context.Background(), env, string(v))
	}
	return nil
}
