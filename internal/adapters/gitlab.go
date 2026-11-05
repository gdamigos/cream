package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

// edits config.yml as a node tree so the user's comments survive
type GitLab struct{}

func (GitLab) ID() string { return "gitlab" }

func (GitLab) Name() string { return "GitLab CLI" }

func glabConfigDir(env *adapter.Env) string {
	if d := env.Env("GLAB_CONFIG_DIR"); d != "" {
		return d
	}
	cands := []string{}
	if d := env.Env("XDG_CONFIG_HOME"); d != "" {
		cands = append(cands, filepath.Join(d, "glab-cli"))
	}
	cands = append(cands, env.Paths.InHome(".config", "glab-cli"))
	if env.OS() == "windows" && env.Env("AppData") != "" {
		cands = append(cands, filepath.Join(env.Env("AppData"), "glab-cli"))
	}
	if env.OS() == "darwin" {
		cands = append(cands, env.Paths.InHome("Library", "Application Support", "glab-cli"))
	}
	for _, c := range cands {
		if fsutil.Exists(filepath.Join(c, "config.yml")) {
			return c
		}
	}
	return cands[0]
}

func glabConfigFile(env *adapter.Env) string { return filepath.Join(glabConfigDir(env), "config.yml") }

func (GitLab) Files(env *adapter.Env) []string { return []string{glabConfigFile(env)} }

type glabDoc struct{ root *yaml.Node }

func readGlab(env *adapter.Env) (*glabDoc, bool, error) {
	b, ok, err := fsutil.ReadFileIfExists(glabConfigFile(env))
	if err != nil || !ok {
		return &glabDoc{root: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}}, false, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", glabConfigFile(env), err)
	}
	if n.Kind == 0 {
		n = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	return &glabDoc{root: &n}, true, nil
}

func (d *glabDoc) top() *yaml.Node { return d.root.Content[0] }

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapSet(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
}

func (d *glabDoc) hosts() *yaml.Node {
	h := mapGet(d.top(), "hosts")
	if h == nil {
		h = &yaml.Node{Kind: yaml.MappingNode}
		mapSet(d.top(), "hosts", h)
	}
	return h
}

func (d *glabDoc) hostNames() []string {
	h := mapGet(d.top(), "hosts")
	var out []string
	if h != nil {
		for i := 0; i+1 < len(h.Content); i += 2 {
			out = append(out, h.Content[i].Value)
		}
	}
	return out
}

func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

func (g GitLab) pickHost(env *adapter.Env, d *glabDoc, p *config.Profile) string {
	if p != nil && p.Accounts[g.ID()] != nil && p.Accounts[g.ID()].Host != "" {
		return p.Accounts[g.ID()].Host
	}
	if h := env.Env("GITLAB_HOST"); h != "" {
		return strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
	}
	if h := scalar(mapGet(d.top(), "host")); h != "" && mapGet(d.hosts(), h) != nil {
		return h
	}
	for _, h := range d.hostNames() {
		if scalar(mapGet(mapGet(d.hosts(), h), "token")) != "" {
			return h
		}
	}
	return "gitlab.com"
}

func glabLabel(host string, hn *yaml.Node) string {
	if u := scalar(mapGet(hn, "user")); u != "" {
		return u
	}
	return host + " token"
}

func (g GitLab) Status(ctx context.Context, env *adapter.Env) adapter.Status {
	var s adapter.Status
	_, err := env.Run.LookPath("glab")
	s.Installed = err == nil || fsutil.Exists(glabConfigFile(env))
	d, _, err := readGlab(env)
	if err != nil {
		s.Warning = err.Error()
		return s
	}
	host := g.pickHost(env, d, nil)
	hn := mapGet(d.hosts(), host)
	s.Detail = host
	if scalar(mapGet(hn, "token")) != "" {
		s.LoggedIn = true
		s.Account = glabLabel(host, hn)
	} else if hn != nil {
		s.Warning = "no token in config.yml for " + host + " (keyring-stored glab tokens are not supported yet)"
	}
	return s
}

func (g GitLab) Import(ctx context.Context, env *adapter.Env, p *config.Profile) (*config.Account, error) {
	d, ok, err := readGlab(env)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("not logged in (run `glab auth login` first)")
	}
	host := g.pickHost(env, d, p)
	hn := mapGet(d.hosts(), host)
	if hn == nil || scalar(mapGet(hn, "token")) == "" {
		return nil, fmt.Errorf("no glab token for %s (run `glab auth login --hostname %s`)", host, host)
	}
	b, err := yaml.Marshal(hn)
	if err != nil {
		return nil, err
	}
	if err := env.Vault.Set(vault.ProfileKey(p.Name, g.ID()), b); err != nil {
		return nil, err
	}
	return &config.Account{Label: glabLabel(host, hn), Host: host, Imported: time.Now().UTC().Truncate(time.Second)}, nil
}

func (g GitLab) load(env *adapter.Env, p *config.Profile) (string, *yaml.Node, error) {
	a := p.Accounts[g.ID()]
	b, err := env.Vault.Get(vault.ProfileKey(p.Name, g.ID()))
	if errors.Is(err, vault.ErrNotFound) || a == nil {
		return "", nil, adapter.ErrNotImported{Adapter: g.ID(), Profile: p.Name}
	}
	if err != nil {
		return "", nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil || len(n.Content) == 0 {
		return "", nil, errors.New("gitlab: corrupt vault entry")
	}
	host := a.Host
	if host == "" {
		host = "gitlab.com"
	}
	return host, n.Content[0], nil
}

func (g GitLab) Activate(ctx context.Context, env *adapter.Env, p *config.Profile, dryRun bool) ([]adapter.Change, error) {
	host, hn, err := g.load(env, p)
	if err != nil {
		return nil, err
	}
	ch := []adapter.Change{{Adapter: g.ID(), Kind: "file", Target: glabConfigFile(env) + " (hosts." + host + ")", Action: "patch"}}
	if dryRun {
		return ch, nil
	}
	d, _, err := readGlab(env)
	if err != nil {
		return nil, err
	}
	mapSet(d.hosts(), host, hn)
	out, err := yaml.Marshal(d.root)
	if err != nil {
		return nil, err
	}
	return ch, fsutil.WriteFileAtomic(glabConfigFile(env), out, 0o600)
}

type glabLocked struct {
	Host string `json:"host"`
	Node string `json:"node"` // the whole hosts.<host> entry as yaml
}

func (g GitLab) writeGlab(env *adapter.Env, d *glabDoc) error {
	out, err := yaml.Marshal(d.root)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(glabConfigFile(env), out, 0o600)
}

func (g GitLab) Lock(ctx context.Context, env *adapter.Env, key, account string, dryRun bool) (bool, []adapter.Change, error) {
	d, ok, err := readGlab(env)
	if err != nil || !ok {
		return false, nil, err
	}
	host := g.pickHost(env, d, nil)
	hn := mapGet(d.hosts(), host)
	if scalar(mapGet(hn, "token")) == "" || (account != "" && glabLabel(host, hn) != account) {
		return false, nil, nil
	}
	ch := []adapter.Change{{Adapter: g.ID(), Kind: "file", Target: glabConfigFile(env) + " (hosts." + host + ".token)", Action: "delete"}}
	if dryRun {
		return true, ch, nil
	}
	node, err := yaml.Marshal(hn)
	if err != nil {
		return false, nil, err
	}
	b, err := json.Marshal(glabLocked{Host: host, Node: string(node)})
	if err != nil {
		return false, nil, err
	}
	if err := env.Vault.Set(key, b); err != nil {
		return false, nil, err
	}
	for i := 0; i+1 < len(hn.Content); i += 2 {
		if hn.Content[i].Value == "token" {
			hn.Content = append(hn.Content[:i], hn.Content[i+2:]...)
			break
		}
	}
	return true, ch, g.writeGlab(env, d)
}

func (g GitLab) Unlock(ctx context.Context, env *adapter.Env, key string, dryRun bool) (bool, []adapter.Change, error) {
	b, err := env.Vault.Get(key)
	if errors.Is(err, vault.ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	var l glabLocked
	if err := json.Unmarshal(b, &l); err != nil {
		return false, nil, fmt.Errorf("gitlab: corrupt lock entry: %w", err)
	}
	ch := []adapter.Change{{Adapter: g.ID(), Kind: "file", Target: glabConfigFile(env) + " (hosts." + l.Host + ")", Action: "restore"}}
	if dryRun {
		return true, ch, nil
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(l.Node), &n); err != nil || len(n.Content) == 0 {
		return false, nil, errors.New("gitlab: corrupt lock entry")
	}
	d, _, err := readGlab(env)
	if err != nil {
		return false, nil, err
	}
	mapSet(d.hosts(), l.Host, n.Content[0])
	if err := g.writeGlab(env, d); err != nil {
		return false, nil, err
	}
	return true, ch, env.Vault.Delete(key)
}

func (g GitLab) SessionEnv(ctx context.Context, env *adapter.Env, p *config.Profile) ([]string, func() error, error) {
	host, hn, err := g.load(env, p)
	if err != nil {
		return nil, nil, err
	}
	tok := scalar(mapGet(hn, "token"))
	if tok == "" {
		return nil, nil, errors.New("gitlab: stored account has no token")
	}
	return []string{"GITLAB_TOKEN=" + tok, "GITLAB_HOST=" + host}, nil, nil
}

func (g GitLab) Verify(ctx context.Context, env *adapter.Env) (string, error) {
	out, err := env.Run.Run(ctx, nil, nil, "glab", "auth", "status")
	return strings.TrimSpace(string(out)), err
}
