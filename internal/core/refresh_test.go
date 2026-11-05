package core

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

// the two tab case: cream holds work, then claude is logged out and back in as personal elsewhere
func TestSyncActiveThenLoginElsewhere(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.AddProfile("work", "", config.GitIdentity{})
	m.AddProfile("personal", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "WORK1")
	if _, err := m.Import(ctx, "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	stamp := m.Cfg.Profiles["work"].Accounts["claude"].Imported

	// nothing changed, so nothing is rewritten
	if ch, err := m.SyncActive(ctx); err != nil || len(ch) != 0 {
		t.Fatalf("sync without changes = %v, %v", ch, err)
	}

	// claude refreshed its token while cream was open
	loginClaude(t, m, "w@corp", "WORK2")
	ch, err := m.SyncActive(ctx)
	if err != nil || len(ch) != 1 {
		t.Fatalf("sync after refresh = %v, %v", ch, err)
	}
	b, _ := m.Env.Vault.Get(vault.ProfileKey("work", "claude"))
	if !strings.Contains(string(b), "WORK2") {
		t.Fatal("refreshed token not saved")
	}
	if !m.Cfg.Profiles["work"].Accounts["claude"].Imported.After(stamp.Add(-time.Second)) {
		t.Fatal("import time not updated")
	}

	// a different account is live now, work must keep its token
	loginClaude(t, m, "me@home", "HOME1")
	if ch, _ := m.SyncActive(ctx); len(ch) != 0 {
		t.Fatalf("synced a foreign account into work: %v", ch)
	}
	if _, err := m.Import(ctx, "personal", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	b, _ = m.Env.Vault.Get(vault.ProfileKey("work", "claude"))
	if !strings.Contains(string(b), "WORK2") {
		t.Fatal("work token was overwritten")
	}
	if got := m.Owners("claude", "me@home"); len(got) != 1 || got[0] != "personal" {
		t.Fatalf("owners = %v", got)
	}

	// switching back restores work
	if _, err := m.Use(ctx, "work", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, adapters.Claude{}.Files(m.Env)[0]), "WORK2") {
		t.Fatal("switch did not restore the work token")
	}
}

func TestForget(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.Codex{}})
	m.AddProfile("work", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "WORK1")
	if _, err := m.Import(ctx, "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Forget("work", []string{"codex"}, false); err == nil {
		t.Fatal("forgetting an account that was never saved should fail")
	}
	ch, err := m.Forget("work", []string{"claude"}, true)
	if err != nil || len(ch) == 0 {
		t.Fatalf("dry run = %v, %v", ch, err)
	}
	if m.Cfg.Profiles["work"].Accounts["claude"] == nil {
		t.Fatal("dry run changed the profile")
	}
	if _, err := m.Forget("work", []string{"claude"}, false); err != nil {
		t.Fatal(err)
	}
	if m.Cfg.Profiles["work"].Accounts["claude"] != nil {
		t.Fatal("account still in profile")
	}
	if _, err := m.Env.Vault.Get(vault.ProfileKey("work", "claude")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("secret still in vault: %v", err)
	}
	if !strings.Contains(get(t, adapters.Claude{}.Files(m.Env)[0]), "WORK1") {
		t.Fatal("the live login must stay")
	}
	if _, ok := m.Cfg.Profiles["work"]; !ok {
		t.Fatal("profile itself was removed")
	}
}

func TestReloadIfChanged(t *testing.T) {
	m, _ := newManager(t, nil)
	m.AddProfile("work", "", config.GitIdentity{})
	if changed, err := m.ReloadIfChanged(); err != nil || changed {
		t.Fatalf("own save counted as a change: %v %v", changed, err)
	}

	// another cream process adds a profile
	other, err := Open(Options{Paths: &m.Paths, Vault: m.Env.Vault, Adapters: m.Adapters})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.AddProfile("personal", "", config.GitIdentity{}); err != nil {
		t.Fatal(err)
	}
	// make sure the stamp differs even on filesystems with coarse mtimes
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(m.Paths.ConfigFile(), later, later)

	changed, err := m.ReloadIfChanged()
	if err != nil || !changed {
		t.Fatalf("reload = %v, %v", changed, err)
	}
	if _, ok := m.Cfg.Profiles["personal"]; !ok || m.Env.Config != m.Cfg {
		t.Fatal("reloaded config not in use")
	}
}
