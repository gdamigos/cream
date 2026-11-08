// cli and tui must go through this package, never call adapters directly
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/backup"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/vault"
)

type Manager struct {
	Paths    paths.Paths
	Cfg      *config.Config
	Env      *adapter.Env
	Adapters []adapter.Adapter
	Backups  *backup.Store

	all      []adapter.Adapter // every tool, Adapters keeps the ones config.toml enables
	cfgStamp fileStamp
}

type fileStamp struct {
	mod  time.Time
	size int64
}

func stampOf(path string) fileStamp {
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{fi.ModTime(), fi.Size()}
}

type Options struct {
	Paths      *paths.Paths
	Vault      vault.Vault // nil opens the one from config
	Runner     adapter.Runner
	Passphrase func() (string, error)
	Adapters   []adapter.Adapter
}

func Open(o Options) (*Manager, error) {
	var p paths.Paths
	if o.Paths != nil {
		p = *o.Paths
	} else {
		var err error
		if p, err = paths.Resolve(); err != nil {
			return nil, err
		}
	}
	cfg, err := config.Load(p.ConfigFile())
	if err != nil {
		return nil, err
	}
	v := o.Vault
	if v == nil {
		v, err = vault.Open(vault.OpenOptions{Backend: cfg.Vault.Backend, FilePath: p.VaultFile(), Passphrase: o.Passphrase})
		if err != nil {
			return nil, err
		}
	}
	r := o.Runner
	if r == nil {
		r = adapter.ExecRunner{}
	}
	ads := o.Adapters
	if ads == nil {
		ads = adapters.All()
	}
	enabled, err := enabledAdapters(ads, cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{
		Paths:    p,
		Cfg:      cfg,
		Env:      &adapter.Env{Paths: p, Vault: v, Run: r, Config: cfg},
		Adapters: enabled,
		all:      ads,
		Backups:  &backup.Store{Dir: p.BackupDir(), Vault: v},
		cfgStamp: stampOf(p.ConfigFile()),
	}, nil
}

func (m *Manager) Save() error {
	if err := m.Cfg.Save(m.Paths.ConfigFile()); err != nil {
		return err
	}
	m.cfgStamp = stampOf(m.Paths.ConfigFile())
	return nil
}

// picks up config.toml changes made by another cream process, callers must not run ops concurrently
func (m *Manager) ReloadIfChanged() (bool, error) {
	st := stampOf(m.Paths.ConfigFile())
	if st == m.cfgStamp {
		return false, nil
	}
	cfg, err := config.Load(m.Paths.ConfigFile())
	if err != nil {
		return false, err
	}
	enabled, err := enabledAdapters(m.all, cfg)
	if err != nil {
		return false, err
	}
	m.Cfg, m.Env.Config, m.cfgStamp, m.Adapters = cfg, cfg, st, enabled
	return true, nil
}

func (m *Manager) Select(only []string) ([]adapter.Adapter, error) {
	if len(only) == 0 {
		return m.Adapters, nil
	}
	var out []adapter.Adapter
	for _, id := range only {
		id = strings.ToLower(strings.TrimSpace(id))
		found := false
		for _, a := range m.Adapters {
			if a.ID() == id {
				out = append(out, a)
				found = true
			}
		}
		if !found {
			var ids []string
			for _, a := range m.Adapters {
				ids = append(ids, a.ID())
			}
			return nil, fmt.Errorf("unknown adapter %q (available: %s)", id, strings.Join(ids, ", "))
		}
	}
	return out, nil
}

type State int

const (
	Pending State = iota
	Running
	Done
	Skipped
	Failed
)

type Event struct {
	Adapter string
	Name    string
	State   State
	Msg     string
	Changes []adapter.Change
}

type Progress func(Event)

func emit(p Progress, e Event) {
	if p != nil {
		p(e)
	}
}

func (m *Manager) Import(ctx context.Context, profile string, only []string, dryRun bool, progress Progress) ([]Event, error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, err
	}
	if err := m.checkUnlocked(profile); err != nil {
		return nil, err
	}
	ads, err := m.Select(only)
	if err != nil {
		return nil, err
	}
	var results []Event
	imported := 0
	for _, a := range ads {
		emit(progress, Event{Adapter: a.ID(), Name: a.Name(), State: Running})
		var ev Event
		if dryRun {
			st := a.Status(ctx, m.Env)
			if st.LoggedIn {
				ev = Event{Adapter: a.ID(), Name: a.Name(), State: Done, Msg: "would import " + st.Account,
					Changes: []adapter.Change{{Adapter: a.ID(), Kind: "keyring", Target: vault.ProfileKey(p.Name, a.ID()), Action: "write"}}}
			} else {
				ev = Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "not logged in"}
			}
		} else {
			acct, err := a.Import(ctx, m.Env, p)
			if err != nil {
				ev = Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: err.Error()}
			} else {
				p.Accounts[a.ID()] = acct
				imported++
				ev = Event{Adapter: a.ID(), Name: a.Name(), State: Done, Msg: acct.Label}
			}
		}
		results = append(results, ev)
		emit(progress, ev)
	}
	if dryRun {
		return results, nil
	}
	if imported > 0 {
		if m.Cfg.Active == "" {
			// the live accounts belong to this profile now
			m.Cfg.Active = p.Name
		}
		if err := m.Save(); err != nil {
			return results, err
		}
		m.Log("import profile=%s adapters=%d", p.Name, imported)
	}
	return results, nil
}

type UseOptions struct {
	DryRun bool
	Only   []string
}

type UseResult struct {
	From, To   string
	BackupPath string
	Events     []Event
	RolledBack bool
}

// order is sync out, backup, activate, and rollback on any failure
func (m *Manager) Use(ctx context.Context, profile string, o UseOptions, progress Progress) (*UseResult, error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, err
	}
	if err := m.checkUnlocked(profile); err != nil {
		return nil, err
	}
	ads, err := m.Select(o.Only)
	if err != nil {
		return nil, err
	}
	res := &UseResult{From: m.Cfg.Active, To: profile}

	// save refreshed tokens of the outgoing profile so switching back is not stale
	if !o.DryRun && res.From != "" && res.From != profile {
		if from, err := m.Cfg.Profile(res.From); err == nil {
			m.syncOut(ctx, from, ads)
		}
	}

	snap, err := m.capture(ads, "switch-to-"+profile, res.From, profile)
	if err != nil {
		return nil, err
	}
	if !o.DryRun {
		if res.BackupPath, err = m.Backups.Save(snap); err != nil {
			return nil, fmt.Errorf("backup failed, nothing was changed: %w", err)
		}
	}

	for _, a := range ads {
		emit(progress, Event{Adapter: a.ID(), Name: a.Name(), State: Running})
		changes, err := a.Activate(ctx, m.Env, p, o.DryRun)
		var ni adapter.ErrNotImported
		var skip adapter.SkipError
		switch {
		case errors.As(err, &ni):
			ev := Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: "no saved account"}
			res.Events = append(res.Events, ev)
			emit(progress, ev)
		case errors.As(err, &skip):
			ev := Event{Adapter: a.ID(), Name: a.Name(), State: Skipped, Msg: skip.Reason}
			res.Events = append(res.Events, ev)
			emit(progress, ev)
		case err != nil:
			ev := Event{Adapter: a.ID(), Name: a.Name(), State: Failed, Msg: err.Error()}
			res.Events = append(res.Events, ev)
			emit(progress, ev)
			if o.DryRun {
				continue
			}
			rerr := m.rollback(snap, ads)
			res.RolledBack = true
			m.Log("switch %s -> %s FAILED at %s; rolled back", res.From, profile, a.ID())
			if rerr != nil {
				return res, fmt.Errorf("%s: %w (rollback also failed: %w; restore manually with `cream restore`)", a.ID(), err, rerr)
			}
			return res, fmt.Errorf("%s: %w (all changes rolled back, %q is still active)", a.ID(), err, res.From)
		default:
			msg := "switched"
			if acct := p.Accounts[a.ID()]; acct != nil {
				msg = acct.Label
			}
			if len(changes) == 0 && !o.DryRun {
				msg += " (already active)"
			}
			ev := Event{Adapter: a.ID(), Name: a.Name(), State: Done, Msg: msg, Changes: changes}
			res.Events = append(res.Events, ev)
			emit(progress, ev)
		}
	}
	if o.DryRun {
		return res, nil
	}
	m.Cfg.Active = profile
	if err := m.Save(); err != nil {
		return res, err
	}
	m.Log("switch %s -> %s ok", res.From, profile)
	return res, nil
}

// re-imports tools still logged in as the profile's account, returns the ids whose secret changed
func (m *Manager) syncOut(ctx context.Context, from *config.Profile, ads []adapter.Adapter) []string {
	var changed []string
	for _, a := range ads {
		if a.ID() == "git" || a.ID() == "github" {
			continue // cream holds no secret for these
		}
		acct := from.Accounts[a.ID()]
		if acct == nil {
			continue
		}
		st := a.Status(ctx, m.Env)
		if !st.LoggedIn || st.Account != acct.Label {
			continue
		}
		// identical secrets are not rewritten, so the import time only moves on a real refresh
		dv := &dedupVault{Vault: m.Env.Vault}
		env := *m.Env
		env.Vault = dv
		if na, err := a.Import(ctx, &env, from); err == nil && dv.wrote {
			na.Host = acct.Host
			from.Accounts[a.ID()] = na
			changed = append(changed, a.ID())
		}
	}
	return changed
}

type dedupVault struct {
	vault.Vault
	wrote bool
}

func (d *dedupVault) Set(key string, value []byte) error {
	if old, err := d.Get(key); err == nil && string(old) == string(value) {
		return nil
	}
	d.wrote = true
	return d.Vault.Set(key, value)
}

// saves refreshed tokens of the active profile, e.g. before the user logs out of a tool elsewhere
func (m *Manager) SyncActive(ctx context.Context) ([]string, error) {
	p := m.Cfg.Profiles[m.Cfg.Active]
	if p == nil {
		return nil, nil
	}
	changed := m.syncOut(ctx, p, m.Adapters)
	if len(changed) == 0 {
		return nil, nil
	}
	if err := m.Save(); err != nil {
		return changed, err
	}
	m.Log("sync profile=%s adapters=%s", p.Name, strings.Join(changed, ","))
	return changed, nil
}

// profiles whose saved account for the adapter has this label
func (m *Manager) Owners(adapterID, label string) []string {
	var out []string
	for _, n := range m.Cfg.Names() {
		if a := m.Cfg.Profiles[n].Accounts[adapterID]; a != nil && a.Label == label {
			out = append(out, n)
		}
	}
	return out
}

// drops saved accounts from a profile without touching the live logins, empty only means all of them
func (m *Manager) Forget(profile string, only []string, dryRun bool) ([]adapter.Change, error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, err
	}
	ads, err := m.Select(only)
	if err != nil {
		return nil, err
	}
	var changes []adapter.Change
	for _, a := range ads {
		if p.Accounts[a.ID()] == nil {
			continue
		}
		changes = append(changes,
			adapter.Change{Adapter: a.ID(), Kind: "keyring", Target: vault.ProfileKey(p.Name, a.ID()), Action: "delete"},
			adapter.Change{Adapter: a.ID(), Kind: "file", Target: m.Paths.ConfigFile() + " (profiles." + p.Name + ".accounts." + a.ID() + ")", Action: "remove"})
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("profile %q has no saved account for %s", p.Name, strings.Join(idsOf(ads), ", "))
	}
	if dryRun {
		return changes, nil
	}
	var forgot []string
	for _, a := range ads {
		if p.Accounts[a.ID()] == nil {
			continue
		}
		if err := m.Env.Vault.Delete(vault.ProfileKey(p.Name, a.ID())); err != nil && !errors.Is(err, vault.ErrNotFound) {
			return nil, fmt.Errorf("%s: %w", a.ID(), err)
		}
		delete(p.Accounts, a.ID())
		_ = os.RemoveAll(m.Paths.SessionDir(p.Name, a.ID())) // run sessions hold a copy of the tokens
		forgot = append(forgot, a.ID())
	}
	if err := m.Save(); err != nil {
		return nil, err
	}
	m.Log("forget profile=%s adapters=%s", p.Name, strings.Join(forgot, ","))
	return changes, nil
}

func idsOf(ads []adapter.Adapter) []string {
	out := make([]string, 0, len(ads))
	for _, a := range ads {
		out = append(out, a.ID())
	}
	return out
}

// every file the adapters may touch plus non-file state like the macos keychain, not saved yet
func (m *Manager) capture(ads []adapter.Adapter, reason, from, to string) (*backup.Snapshot, error) {
	var files []string
	for _, a := range ads {
		files = append(files, a.Files(m.Env)...)
	}
	snap, err := backup.Capture(files)
	if err != nil {
		return nil, err
	}
	snap.Reason, snap.From, snap.To = reason, from, to
	snap.Extra = map[string]map[string][]byte{}
	for _, a := range ads {
		if s, ok := a.(adapter.Snapshotter); ok {
			x, err := s.Snapshot(m.Env)
			if err != nil {
				return nil, fmt.Errorf("%s: backup: %w", a.ID(), err)
			}
			if x != nil {
				snap.Extra[a.ID()] = x
			}
		}
	}
	return snap, nil
}

func (m *Manager) rollback(snap *backup.Snapshot, ads []adapter.Adapter) error {
	errs := []error{backup.RestoreFiles(snap)}
	for _, a := range ads {
		if s, ok := a.(adapter.Snapshotter); ok {
			if x := snap.Extra[a.ID()]; x != nil {
				errs = append(errs, s.RestoreSnapshot(m.Env, x))
			}
		}
	}
	return errors.Join(errs...)
}

// empty id means latest, the current state is backed up first so a restore can be undone
func (m *Manager) Restore(ctx context.Context, id string, dryRun bool) (*backup.Snapshot, error) {
	snap, err := m.Backups.Load(id)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return snap, nil
	}
	var files []string
	for _, f := range snap.Files {
		files = append(files, f.Path)
	}
	cur, err := backup.Capture(files)
	if err != nil {
		return nil, err
	}
	cur.Reason, cur.From, cur.To = "pre-restore", m.Cfg.Active, snap.From
	cur.Extra = map[string]map[string][]byte{}
	for _, a := range m.Adapters {
		if s, ok := a.(adapter.Snapshotter); ok && snap.Extra[a.ID()] != nil {
			if x, err := s.Snapshot(m.Env); err == nil && x != nil {
				cur.Extra[a.ID()] = x
			}
		}
	}
	if _, err := m.Backups.Save(cur); err != nil {
		return nil, err
	}
	if err := m.rollback(snap, m.Adapters); err != nil {
		return snap, err
	}
	if _, ok := m.Cfg.Profiles[snap.From]; ok || snap.From == "" {
		m.Cfg.Active = snap.From
	}
	if err := m.Save(); err != nil {
		return snap, err
	}
	m.Log("restore backup=%s active=%s", snap.ID, m.Cfg.Active)
	return snap, nil
}

type AdapterStatus struct {
	ID, Name string
	Live     adapter.Status
	Expected string
	Match    bool
}

func (m *Manager) Status(ctx context.Context) []AdapterStatus {
	var active *config.Profile
	if m.Cfg.Active != "" {
		active = m.Cfg.Profiles[m.Cfg.Active]
	}
	return m.StatusFor(ctx, active)
}

func (m *Manager) StatusFor(ctx context.Context, p *config.Profile) []AdapterStatus {
	// each tool is checked in parallel, Status is read only so this is safe
	out := make([]AdapterStatus, len(m.Adapters))
	var wg sync.WaitGroup
	for i, a := range m.Adapters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = AdapterStatus{ID: a.ID(), Name: a.Name(), Live: a.Status(ctx, m.Env)}
		}()
	}
	wg.Wait()
	if p != nil {
		for i := range out {
			if acct := p.Accounts[out[i].ID]; acct != nil {
				out[i].Expected = acct.Label
				out[i].Match = out[i].Live.LoggedIn && out[i].Live.Account == acct.Label
			}
		}
	}
	return out
}

func (m *Manager) AddProfile(name, desc string, git config.GitIdentity) (*config.Profile, error) {
	p, err := m.Cfg.AddProfile(name, desc)
	if err != nil {
		return nil, err
	}
	p.Git = git
	if err := m.Save(); err != nil {
		return nil, err
	}
	m.Log("profile add %s", name)
	return p, nil
}

func (m *Manager) RemoveProfile(name string) error {
	p, err := m.Cfg.Profile(name)
	if err != nil {
		return err
	}
	hadRules := len(p.Rules) > 0
	if err := m.Cfg.RemoveProfile(name); err != nil {
		return err
	}
	var ids []string
	for _, a := range m.Adapters {
		ids = append(ids, a.ID())
	}
	vault.DeleteProfile(m.Env.Vault, name, ids)
	for _, id := range ids {
		_ = m.Env.Vault.Delete(vault.LockKey(name, id))
	}
	_ = os.Remove(filepath.Join(m.Paths.GitDir(), name+".gitconfig"))
	_ = os.RemoveAll(filepath.Join(m.Paths.CreamHome, "sessions", name))
	if err := m.Save(); err != nil {
		return err
	}
	if hadRules {
		_, _ = m.SyncGit(false) // drop the removed profile's includeIf entries
	}
	m.Log("profile remove %s", name)
	return nil
}

func (m *Manager) SyncGit(dryRun bool) ([]adapter.Change, error) {
	var fallback *config.Profile
	if p := m.Cfg.Profiles[m.Cfg.Active]; p != nil && !p.Git.IsZero() {
		fallback = adapters.Fallback(m.Cfg, p)
	}
	return adapters.Sync(m.Env, m.Cfg, fallback, dryRun)
}

func (m *Manager) AddRule(profile string, r config.Rule, dryRun bool) ([]adapter.Change, error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, err
	}
	if r, err = config.ValidateRule(r); err != nil {
		return nil, err
	}
	if p.Git.IsZero() {
		return nil, fmt.Errorf("profile %q has no git identity yet (use `cream profile set %s --git-name … --git-email …`)", p.Name, p.Name)
	}
	if slices.Contains(p.Rules, r) {
		return nil, fmt.Errorf("rule %s already exists on %q", r, p.Name)
	}
	p.Rules = append(p.Rules, r)
	changes, err := m.SyncGit(dryRun)
	if err != nil || dryRun {
		p.Rules = p.Rules[:len(p.Rules)-1]
		return changes, err
	}
	if err := m.Save(); err != nil {
		return nil, err
	}
	m.Log("rule add %s %s", p.Name, r)
	return changes, nil
}

func (m *Manager) RemoveRule(profile string, idx int, dryRun bool) ([]adapter.Change, error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(p.Rules) {
		return nil, fmt.Errorf("profile %q has no rule #%d", p.Name, idx+1)
	}
	old := p.Rules
	p.Rules = slices.Delete(slices.Clone(p.Rules), idx, idx+1)
	changes, err := m.SyncGit(dryRun)
	if err != nil || dryRun {
		p.Rules = old
		return changes, err
	}
	if err := m.Save(); err != nil {
		return nil, err
	}
	m.Log("rule remove %s #%d", p.Name, idx+1)
	return changes, nil
}

var toolAdapter = map[string]string{"claude": "claude", "codex": "codex", "gh": "github", "glab": "gitlab"}

// only errors from the launched tool's own adapter are fatal, others become warnings
func (m *Manager) SessionEnv(ctx context.Context, profile, tool string) (vars []string, finish func(), warnings []string, err error) {
	p, err := m.Cfg.Profile(profile)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := m.checkUnlocked(profile); err != nil {
		return nil, nil, nil, err
	}
	base := strings.TrimSuffix(strings.ToLower(baseName(tool)), ".exe")
	primary := toolAdapter[base]
	var finishers []func() error
	for _, a := range m.Adapters {
		if p.Accounts[a.ID()] == nil && a.ID() != primary {
			continue
		}
		v, fin, err := a.SessionEnv(ctx, m.Env, p)
		if err != nil {
			if a.ID() == primary {
				return nil, nil, nil, fmt.Errorf("%s: %w", a.Name(), err)
			}
			warnings = append(warnings, a.Name()+": "+err.Error())
			continue
		}
		vars = append(vars, v...)
		if fin != nil {
			finishers = append(finishers, fin)
		}
	}
	vars = append(vars, "CREAM_PROFILE="+p.Name)
	finish = func() {
		for _, f := range finishers {
			_ = f()
		}
	}
	m.Log("run profile=%s tool=%s", p.Name, base)
	return vars, finish, warnings, nil
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (m *Manager) Log(format string, args ...any) {
	if !m.Cfg.LogEnabled() {
		return
	}
	if err := os.MkdirAll(m.Paths.CreamHome, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(m.Paths.LogFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}
