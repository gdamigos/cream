package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/vault"
)

func lockSetup(t *testing.T, ads []adapter.Adapter) *Manager {
	t.Helper()
	m, _ := newManager(t, ads)
	m.AddProfile("work", "", config.GitIdentity{Name: "W", Email: "w@corp"})
	m.AddProfile("personal", "", config.GitIdentity{Name: "P", Email: "p@home"})
	if _, err := m.AddRule("work", config.Rule{Kind: "remote", Pattern: "gitlab.corp"}, false); err != nil {
		t.Fatal(err)
	}
	loginClaude(t, m, "w@corp", "WORK1")
	if _, err := m.Import(ctx, "work", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *Manager) gitconfig(t *testing.T) string {
	return get(t, filepath.Join(m.Paths.Home, ".gitconfig"))
}

func claudeCred(m *Manager) string {
	return filepath.Join(m.Paths.Home, ".claude", ".credentials.json")
}

func TestLockEverything(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.Codex{}, adapters.GitIdentity{}})
	put(t, filepath.Join(m.Paths.Home, ".codex", "auth.json"), `{"OPENAI_API_KEY":"sk-X"}`)
	if !strings.Contains(m.gitconfig(t), "includeIf") {
		t.Fatal("setup: rule missing from the block")
	}

	if _, err := m.Lock(ctx, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if fsutil.Exists(claudeCred(m)) || fsutil.Exists(filepath.Join(m.Paths.Home, ".codex", "auth.json")) {
		t.Fatal("tools still logged in")
	}
	if g := m.gitconfig(t); strings.Contains(g, "includeIf") || strings.Contains(g, "email") || !strings.Contains(g, "useConfigOnly = true") {
		t.Fatalf("git identity still on:\n%s", g)
	}
	if cfg, _ := config.Load(m.Paths.ConfigFile()); !cfg.Lock.All {
		t.Fatal("lock state not saved")
	}
	if _, err := m.Use(ctx, "personal", UseOptions{}, nil); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("use while locked: %v", err)
	}
	if _, _, _, err := m.SessionEnv(ctx, "work", "claude"); err == nil {
		t.Fatal("run while locked")
	}
	if _, err := m.Import(ctx, "work", nil, false, nil); err == nil {
		t.Fatal("import while locked")
	}
	if _, err := m.Lock(ctx, "work", false, nil); err == nil {
		t.Fatal("locking a profile inside an everything lock")
	}

	if _, err := m.Unlock(ctx, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, claudeCred(m)), "WORK1") || !strings.Contains(get(t, filepath.Join(m.Paths.Home, ".codex", "auth.json")), "sk-X") {
		t.Fatal("logins not restored")
	}
	if !strings.Contains(m.gitconfig(t), "includeIf") || m.Cfg.AnyLocked() {
		t.Fatal("git or lock state not restored")
	}
	for _, id := range []string{"claude", "codex"} {
		if _, err := m.Env.Vault.Get(vault.LockKey(vault.LockAll, id)); !errors.Is(err, vault.ErrNotFound) {
			t.Fatalf("%s: lock entry left in the vault", id)
		}
	}
	if _, err := m.Unlock(ctx, "", false, nil); err == nil {
		t.Fatal("unlocking twice should say nothing is locked")
	}
}

func TestRestoreRespectsLocks(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	if _, err := m.Use(ctx, "personal", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Lock(ctx, "", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(ctx, "", false); err == nil || !strings.Contains(err.Error(), "cream unlock") {
		t.Fatalf("restore while locked: %v", err)
	}
	if fsutil.Exists(claudeCred(m)) {
		t.Fatal("restore logged a tool back in behind the lock")
	}
	if _, err := m.Unlock(ctx, "", false, nil); err != nil {
		t.Fatal(err)
	}

	// the backup of the unlock still has work's rule turned off, work isn't locked anymore
	if _, err := m.Lock(ctx, "work", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Unlock(ctx, "work", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.gitconfig(t), "gitlab.corp") {
		t.Fatalf("restore brought back an old git block:\n%s", m.gitconfig(t))
	}
}

func TestLockOneProfileThenUseAnother(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	loginClaude(t, m, "me@home", "HOME1")
	if _, err := m.Import(ctx, "personal", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	loginClaude(t, m, "w@corp", "WORK2") // work refreshed its token while active

	if _, err := m.Lock(ctx, "work", false, nil); err != nil {
		t.Fatal(err)
	}
	if fsutil.Exists(claudeCred(m)) {
		t.Fatal("work's claude login still live")
	}
	if strings.Contains(m.gitconfig(t), "gitlab.corp") {
		t.Fatal("work's rule still active")
	}
	if _, err := m.Use(ctx, "work", UseOptions{}, nil); err == nil {
		t.Fatal("switched to a locked profile")
	}
	if _, err := m.Use(ctx, "personal", UseOptions{}, nil); err != nil {
		t.Fatalf("other profiles must keep working: %v", err)
	}

	evs, err := m.Unlock(ctx, "work", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, claudeCred(m)), "HOME1") {
		t.Fatal("unlocking work overwrote the personal login made since")
	}
	if !strings.Contains(evs[0].Msg, "kept the current login") {
		t.Fatalf("events = %+v", evs)
	}
	if !strings.Contains(m.gitconfig(t), "gitlab.corp") {
		t.Fatal("work's rule not back")
	}
	if _, err := m.Use(ctx, "work", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, claudeCred(m)), "WORK2") {
		t.Fatal("work's refreshed token was lost")
	}
}

func TestLockDryRunChangesNothing(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	before := m.gitconfig(t)
	evs, err := m.Lock(ctx, "", true, nil)
	if err != nil || len(evs) == 0 {
		t.Fatalf("%v %v", evs, err)
	}
	var changes int
	for _, e := range evs {
		changes += len(e.Changes)
	}
	if changes == 0 {
		t.Fatal("dry run reported no changes")
	}
	if !fsutil.Exists(claudeCred(m)) || m.gitconfig(t) != before || m.Cfg.AnyLocked() {
		t.Fatal("dry run changed something")
	}
	if l, _ := m.Backups.List(); len(l) != 0 {
		t.Fatal("dry run wrote a backup")
	}
}

// claude locks fine, then this one fails
type failingLock struct{ adapters.Codex }

func (failingLock) ID() string   { return "boom" }
func (failingLock) Name() string { return "Boom" }
func (failingLock) Lock(context.Context, *adapter.Env, string, string, bool) (bool, []adapter.Change, error) {
	return false, nil, errors.New("simulated failure")
}

func TestLockFailureUndoesEverything(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, failingLock{}, adapters.GitIdentity{}})
	before := get(t, claudeCred(m))
	if _, err := m.Lock(ctx, "", false, nil); err == nil || !strings.Contains(err.Error(), "nothing was locked") {
		t.Fatalf("err = %v", err)
	}
	if get(t, claudeCred(m)) != before || m.Cfg.AnyLocked() {
		t.Fatal("failed lock left claude logged out or the state locked")
	}
	if _, err := m.Env.Vault.Get(vault.LockKey(vault.LockAll, "claude")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatal("lock entry left behind")
	}
}

func TestRemoveLockedProfileCleansUp(t *testing.T) {
	m := lockSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	if _, err := m.Lock(ctx, "work", false, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveProfile("work"); err != nil {
		t.Fatal(err)
	}
	if m.Cfg.AnyLocked() {
		t.Fatal("removed profile still listed as locked")
	}
	if _, err := m.Env.Vault.Get(vault.LockKey("work", "claude")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatal("lock entry of a removed profile left in the vault")
	}
}
