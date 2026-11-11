package core

import (
	"testing"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/fsutil"
)

// a second cream process on the same home and vault
func reopen(t *testing.T, m *Manager) *Manager {
	t.Helper()
	m2, err := Open(Options{Paths: &m.Paths, Vault: m.raw(), Runner: m.Env.Run, Adapters: m.all})
	if err != nil {
		t.Fatal(err)
	}
	m2.Env.Getenv, m2.Env.GOOS = m.Env.Getenv, m.Env.GOOS
	return m2
}

func TestTwoProcessesKeepEachOthersChanges(t *testing.T) {
	m1, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	m1.AddProfile("work", "", config.GitIdentity{Name: "W", Email: "w@corp"})
	m2 := reopen(t, m1)

	loginClaude(t, m1, "w@corp", "WORK1")
	if _, err := m1.Import(ctx, "work", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	// m2 still holds the config from before the import
	if _, err := m2.AddProfile("home", "", config.GitIdentity{}); err != nil {
		t.Fatal(err)
	}
	if err := m2.EditProfile("work", func(p *config.Profile) { p.Description = "day job" }); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(m1.Paths.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	w := cfg.Profiles["work"]
	if w == nil || w.Accounts["claude"] == nil || w.Description != "day job" || cfg.Profiles["home"] == nil {
		t.Fatalf("a change was lost: %+v", cfg.Profiles)
	}
}

func TestExclusiveIsReentrantAndReleases(t *testing.T) {
	m, _ := newManager(t, nil)
	outer, err := m.exclusive(false)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := m.exclusive(false)
	if err != nil {
		t.Fatal(err)
	}
	inner()
	if _, err := fsutil.Lock(m.Paths.LockFile(), 100*time.Millisecond); err == nil {
		t.Fatal("lock released while the outer call still holds it")
	}
	outer()
	unlock, err := fsutil.Lock(m.Paths.LockFile(), time.Second)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	unlock()
}

func TestDryRunTakesNoLockFile(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.GitIdentity{}})
	m.Cfg.AddProfile("work", "")
	if _, err := m.Use(ctx, "work", UseOptions{DryRun: true}, nil); err != nil {
		t.Fatal(err)
	}
	if fsutil.Exists(m.Paths.LockFile()) {
		t.Fatal("dry run wrote the lock file")
	}
}
