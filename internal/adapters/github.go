package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

// stores no secret, uses gh auth switch and reads gh auth token for sessions
type GitHub struct{}

func (GitHub) ID() string { return "github" }

func (GitHub) Name() string { return "GitHub CLI" }

func ghConfigDir(env *adapter.Env) string {
	if d := env.Env("GH_CONFIG_DIR"); d != "" {
		return d
	}
	if d := env.Env("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gh")
	}
	if env.OS() == "windows" {
		if d := env.Env("AppData"); d != "" {
			return filepath.Join(d, "GitHub CLI")
		}
	}
	return env.Paths.InHome(".config", "gh")
}

func ghHostsFile(env *adapter.Env) string { return filepath.Join(ghConfigDir(env), "hosts.yml") }

func (GitHub) Files(env *adapter.Env) []string { return []string{ghHostsFile(env)} }

type ghHost struct {
	User  string               `yaml:"user"`
	Users map[string]yaml.Node `yaml:"users"`
}

func readGhHosts(env *adapter.Env) (map[string]ghHost, error) {
	b, ok, err := fsutil.ReadFileIfExists(ghHostsFile(env))
	if err != nil || !ok {
		return nil, err
	}
	hosts := map[string]ghHost{}
	if err := yaml.Unmarshal(b, &hosts); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ghHostsFile(env), err)
	}
	return hosts, nil
}

func ghDefaultHost(env *adapter.Env, hosts map[string]ghHost) string {
	if h := env.Env("GH_HOST"); h != "" {
		return h
	}
	if _, ok := hosts["github.com"]; ok || len(hosts) == 0 {
		return "github.com"
	}
	keys := make([]string, 0, len(hosts))
	for k := range hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}

func (g GitHub) Status(ctx context.Context, env *adapter.Env) adapter.Status {
	var s adapter.Status
	_, err := env.Run.LookPath("gh")
	s.Installed = err == nil || fsutil.Exists(ghConfigDir(env))
	hosts, err := readGhHosts(env)
	if err != nil {
		s.Warning = err.Error()
		return s
	}
	host := ghDefaultHost(env, hosts)
	h, ok := hosts[host]
	if ok && h.User != "" {
		s.LoggedIn = true
		s.Account = h.User
		others := []string{}
		for u := range h.Users {
			if u != h.User {
				others = append(others, u)
			}
		}
		sort.Strings(others)
		s.Detail = host
		if len(others) > 0 {
			s.Detail += " · also: " + strings.Join(others, ", ")
		}
	}
	return s
}

func (g GitHub) Import(ctx context.Context, env *adapter.Env, p *config.Profile) (*config.Account, error) {
	hosts, err := readGhHosts(env)
	if err != nil {
		return nil, err
	}
	host := ghDefaultHost(env, hosts)
	h, ok := hosts[host]
	if !ok || h.User == "" {
		return nil, errors.New("not logged in (run `gh auth login` first)")
	}
	return &config.Account{Label: h.User, Host: host, Imported: time.Now().UTC().Truncate(time.Second)}, nil
}

func (g GitHub) account(p *config.Profile) (*config.Account, error) {
	a := p.Accounts[g.ID()]
	if a == nil || a.Label == "" {
		return nil, adapter.ErrNotImported{Adapter: g.ID(), Profile: p.Name}
	}
	if a.Host == "" {
		a.Host = "github.com"
	}
	return a, nil
}

func (g GitHub) Activate(ctx context.Context, env *adapter.Env, p *config.Profile, dryRun bool) ([]adapter.Change, error) {
	a, err := g.account(p)
	if err != nil {
		return nil, err
	}
	hosts, err := readGhHosts(env)
	if err != nil {
		return nil, err
	}
	if h, ok := hosts[a.Host]; ok && h.User == a.Label {
		return nil, nil // already active
	}
	if h, ok := hosts[a.Host]; ok {
		if _, known := h.Users[a.Label]; !known && len(h.Users) > 0 {
			return nil, fmt.Errorf("gh has no login for %s on %s (run `gh auth login --hostname %s` once)", a.Label, a.Host, a.Host)
		}
	}
	ch := []adapter.Change{{Adapter: g.ID(), Kind: "exec", Target: "gh auth switch --hostname " + a.Host + " --user " + a.Label, Action: "run"}}
	if dryRun {
		return ch, nil
	}
	if _, err := env.Run.Run(ctx, nil, nil, "gh", "auth", "switch", "--hostname", a.Host, "--user", a.Label); err != nil {
		return nil, err
	}
	return ch, nil
}

func (g GitHub) SessionEnv(ctx context.Context, env *adapter.Env, p *config.Profile) ([]string, func() error, error) {
	a, err := g.account(p)
	if err != nil {
		return nil, nil, err
	}
	out, err := env.Run.Run(ctx, nil, nil, "gh", "auth", "token", "--hostname", a.Host, "--user", a.Label)
	if err != nil {
		return nil, nil, fmt.Errorf("read gh token for %s: %w", a.Label, err)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return nil, nil, fmt.Errorf("gh returned no token for %s", a.Label)
	}
	vars := []string{"GH_TOKEN=" + tok}
	if a.Host != "github.com" {
		vars = []string{"GH_ENTERPRISE_TOKEN=" + tok, "GH_HOST=" + a.Host}
	}
	return vars, nil, nil
}

type ghLocked struct {
	Host   string            `json:"host"`
	Active string            `json:"active"`
	Tokens map[string]string `json:"tokens"`
}

// logs out account, or every gh account on the host when account is empty, so gh can't fall back to another one
func (g GitHub) Lock(ctx context.Context, env *adapter.Env, key, account string, dryRun bool) (bool, []adapter.Change, error) {
	hosts, err := readGhHosts(env)
	if err != nil {
		return false, nil, err
	}
	host := ghDefaultHost(env, hosts)
	h, ok := hosts[host]
	if !ok || h.User == "" {
		return false, nil, nil
	}
	known := map[string]bool{h.User: true}
	for u := range h.Users {
		known[u] = true
	}
	var users []string
	if account != "" {
		if !known[account] {
			return false, nil, nil
		}
		users = []string{account}
	} else {
		for u := range known {
			users = append(users, u)
		}
		sort.Strings(users)
	}
	var ch []adapter.Change
	for _, u := range users {
		ch = append(ch, adapter.Change{Adapter: g.ID(), Kind: "exec", Target: "gh auth logout --hostname " + host + " --user " + u, Action: "run"})
	}
	if dryRun {
		return true, ch, nil
	}
	l := ghLocked{Host: host, Active: h.User, Tokens: map[string]string{}}
	for _, u := range users {
		out, err := env.Run.Run(ctx, nil, nil, "gh", "auth", "token", "--hostname", host, "--user", u)
		if err != nil {
			return false, nil, fmt.Errorf("read gh token for %s: %w", u, err)
		}
		l.Tokens[u] = strings.TrimSpace(string(out))
	}
	b, err := json.Marshal(l)
	if err != nil {
		return false, nil, err
	}
	if err := env.Vault.Set(key, b); err != nil {
		return false, nil, err
	}
	for _, u := range users {
		if _, err := env.Run.Run(ctx, nil, nil, "gh", "auth", "logout", "--hostname", host, "--user", u); err != nil {
			return true, nil, err
		}
	}
	return true, ch, nil
}

// logs the saved accounts back in, then keeps whoever is active now, or the one active at lock time
func (g GitHub) Unlock(ctx context.Context, env *adapter.Env, key string, dryRun bool) (bool, []adapter.Change, error) {
	b, err := env.Vault.Get(key)
	if errors.Is(err, vault.ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	var l ghLocked
	if err := json.Unmarshal(b, &l); err != nil {
		return false, nil, fmt.Errorf("github: corrupt lock entry: %w", err)
	}
	users := make([]string, 0, len(l.Tokens))
	for u := range l.Tokens {
		users = append(users, u)
	}
	sort.Strings(users)
	var ch []adapter.Change
	for _, u := range users {
		ch = append(ch, adapter.Change{Adapter: g.ID(), Kind: "exec", Target: "gh auth login --hostname " + l.Host + " --with-token (" + u + ")", Action: "run"})
	}
	if dryRun {
		return true, ch, nil
	}
	current := ""
	if hosts, err := readGhHosts(env); err == nil {
		current = hosts[l.Host].User
	}
	for _, u := range users {
		// the token goes through stdin, never argv
		if _, err := env.Run.Run(ctx, []byte(l.Tokens[u]+"\n"), nil, "gh", "auth", "login", "--hostname", l.Host, "--with-token"); err != nil {
			return false, nil, fmt.Errorf("gh login for %s: %w", u, err)
		}
	}
	active := current
	if active == "" {
		active = l.Active
	}
	if active != "" {
		if _, err := env.Run.Run(ctx, nil, nil, "gh", "auth", "switch", "--hostname", l.Host, "--user", active); err != nil {
			return false, nil, err
		}
	}
	return true, ch, env.Vault.Delete(key)
}

func (g GitHub) Verify(ctx context.Context, env *adapter.Env) (string, error) {
	out, err := env.Run.Run(ctx, nil, nil, "gh", "api", "user", "--jq", ".login")
	return strings.TrimSpace(string(out)), err
}
