package adapters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

// only works when config.toml has cli_auth_credentials_store = "file"
type Codex struct{}

func (Codex) ID() string { return "codex" }

func (Codex) Name() string { return "Codex" }

func codexHome(env *adapter.Env) string {
	if d := env.Env("CODEX_HOME"); d != "" {
		return d
	}
	return env.Paths.InHome(".codex")
}

func codexAuth(env *adapter.Env) string { return filepath.Join(codexHome(env), "auth.json") }

func (Codex) Files(env *adapter.Env) []string { return []string{codexAuth(env)} }

var codexStoreRe = regexp.MustCompile(`(?m)^\s*cli_auth_credentials_store\s*=\s*"([a-z]+)"`)

func codexStore(env *adapter.Env) string {
	b, ok, _ := fsutil.ReadFileIfExists(filepath.Join(codexHome(env), "config.toml"))
	if !ok {
		return ""
	}
	if m := codexStoreRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// email from the id_token claims, decoded but not verified
func codexLabel(authJSON []byte) string {
	var a struct {
		APIKey *string `json:"OPENAI_API_KEY"`
		Tokens *struct {
			IDToken string `json:"id_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(authJSON, &a) != nil {
		return ""
	}
	if a.Tokens != nil && a.Tokens.IDToken != "" {
		parts := strings.Split(a.Tokens.IDToken, ".")
		if len(parts) >= 2 {
			payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
			if err == nil {
				var claims struct {
					Email string `json:"email"`
				}
				if json.Unmarshal(payload, &claims) == nil && claims.Email != "" {
					return claims.Email
				}
			}
		}
		return "chatgpt account"
	}
	if a.APIKey != nil && *a.APIKey != "" {
		return "API key"
	}
	return ""
}

func (c Codex) Status(ctx context.Context, env *adapter.Env) adapter.Status {
	var s adapter.Status
	if _, err := env.Run.LookPath("codex"); err == nil {
		s.Installed = true
	}
	if fsutil.Exists(codexHome(env)) {
		s.Installed = true
	}
	s.Detail = codexAuth(env)
	b, ok, err := fsutil.ReadFileIfExists(codexAuth(env))
	if err != nil {
		s.Warning = err.Error()
	}
	if ok {
		s.LoggedIn = true
		s.Account = codexLabel(b)
		if s.Account == "" {
			s.Account = "(unknown account)"
		}
	}
	if st := codexStore(env); s.Installed && st != "" && st != "file" {
		s.Warning = `set cli_auth_credentials_store = "file" in ` + filepath.Join(codexHome(env), "config.toml")
	}
	return s
}

func (c Codex) Import(ctx context.Context, env *adapter.Env, p *config.Profile) (*config.Account, error) {
	b, ok, err := fsutil.ReadFileIfExists(codexAuth(env))
	if err != nil {
		return nil, err
	}
	if !ok {
		if st := codexStore(env); st != "" && st != "file" {
			return nil, errors.New(`codex stores credentials in the OS keyring; set cli_auth_credentials_store = "file" in config.toml and log in again`)
		}
		return nil, errors.New("not logged in (run `codex login` first)")
	}
	if err := env.Vault.Set(vault.ProfileKey(p.Name, c.ID()), b); err != nil {
		return nil, err
	}
	label := codexLabel(b)
	if label == "" {
		label = "codex account"
	}
	return &config.Account{Label: label, Imported: time.Now().UTC().Truncate(time.Second)}, nil
}

func (c Codex) load(env *adapter.Env, p *config.Profile) ([]byte, error) {
	b, err := env.Vault.Get(vault.ProfileKey(p.Name, c.ID()))
	if errors.Is(err, vault.ErrNotFound) {
		return nil, adapter.ErrNotImported{Adapter: c.ID(), Profile: p.Name}
	}
	return b, err
}

func (c Codex) Activate(ctx context.Context, env *adapter.Env, p *config.Profile, dryRun bool) ([]adapter.Change, error) {
	b, err := c.load(env, p)
	if err != nil {
		return nil, err
	}
	ch := []adapter.Change{{Adapter: c.ID(), Kind: "file", Target: codexAuth(env), Action: "replace"}}
	if dryRun {
		return ch, nil
	}
	return ch, fsutil.WriteFileAtomic(codexAuth(env), b, 0o600)
}

func (c Codex) Lock(ctx context.Context, env *adapter.Env, key, account string, dryRun bool) (bool, []adapter.Change, error) {
	b, ok, err := fsutil.ReadFileIfExists(codexAuth(env))
	if err != nil || !ok {
		return false, nil, err
	}
	if account != "" && codexLabel(b) != account {
		return false, nil, nil
	}
	ch := []adapter.Change{{Adapter: c.ID(), Kind: "file", Target: codexAuth(env), Action: "delete"}}
	if dryRun {
		return true, ch, nil
	}
	if err := env.Vault.Set(key, b); err != nil {
		return false, nil, err
	}
	return true, ch, os.Remove(codexAuth(env))
}

func (c Codex) Unlock(ctx context.Context, env *adapter.Env, key string, dryRun bool) (bool, []adapter.Change, error) {
	b, err := env.Vault.Get(key)
	if errors.Is(err, vault.ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	ch := []adapter.Change{{Adapter: c.ID(), Kind: "file", Target: codexAuth(env), Action: "restore"}}
	if dryRun {
		return true, ch, nil
	}
	if err := fsutil.WriteFileAtomic(codexAuth(env), b, 0o600); err != nil {
		return false, nil, err
	}
	return true, ch, env.Vault.Delete(key)
}

func (c Codex) SessionEnv(ctx context.Context, env *adapter.Env, p *config.Profile) ([]string, func() error, error) {
	b, err := c.load(env, p)
	if err != nil {
		return nil, nil, err
	}
	dir := env.Paths.SessionDir(p.Name, c.ID())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	auth := filepath.Join(dir, "auth.json")
	if err := fsutil.WriteFileAtomic(auth, b, 0o600); err != nil {
		return nil, nil, err
	}
	cfg := filepath.Join(dir, "config.toml")
	if !fsutil.Exists(cfg) {
		g, ok, _ := fsutil.ReadFileIfExists(filepath.Join(codexHome(env), "config.toml"))
		if !ok {
			g = []byte("cli_auth_credentials_store = \"file\"\n")
		}
		_ = fsutil.WriteFileAtomic(cfg, g, 0o600)
	}
	finish := func() error {
		nb, ok, err := fsutil.ReadFileIfExists(auth)
		if err != nil || !ok || string(nb) == string(b) {
			return err
		}
		return env.Vault.Set(vault.ProfileKey(p.Name, c.ID()), nb)
	}
	return []string{"CODEX_HOME=" + dir}, finish, nil
}

func (c Codex) Verify(ctx context.Context, env *adapter.Env) (string, error) {
	if _, err := env.Run.LookPath("codex"); err != nil {
		return "", errors.New("codex is not installed")
	}
	out, err := env.Run.Run(ctx, nil, nil, "codex", "login", "status")
	return strings.TrimSpace(string(out)), err
}
