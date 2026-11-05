package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

func TestImportSwitchRestore(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.AddProfile("work", "", config.GitIdentity{})
	m.AddProfile("home", "", config.GitIdentity{})

	loginClaude(t, m, "w@corp", "W1")
	if _, err := m.Import(ctx, "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if m.Cfg.Active != "work" {
		t.Fatal("first import should mark the profile active")
	}
	loginClaude(t, m, "h@home", "H1")
	m.Import(ctx, "home", nil, false, nil)
	m.Cfg.Active = "home"

	cred := filepath.Join(m.Paths.Home, ".claude", ".credentials.json")
	put(t, cred, `{"tok":"H2"}`)

	var events []Event
	res, err := m.Use(ctx, "work", UseOptions{}, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath == "" || !strings.Contains(get(t, cred), "W1") {
		t.Fatal("switch to work failed")
	}
	if len(events) != 2 || events[1].State != Done {
		t.Fatalf("events = %+v", events)
	}
	// sync out saved the refreshed home token
	b, _ := m.Env.Vault.Get(vault.ProfileKey("home", "claude"))
	if !strings.Contains(string(b), "H2") {
		t.Fatal("refreshed token of outgoing profile was not saved")
	}
	if _, err := m.Use(ctx, "home", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, cred), "H2") {
		t.Fatal("switch back did not use the refreshed token")
	}
	if _, err := m.Restore(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, cred), "W1") || m.Cfg.Active != "work" {
		t.Fatalf("restore: cred=%s active=%s", get(t, cred), m.Cfg.Active)
	}
	// config on disk has no secrets
	if strings.Contains(get(t, m.Paths.ConfigFile()), "W1") || strings.Contains(get(t, m.Paths.ConfigFile()), "H2") {
		t.Fatal("secret written to config.toml")
	}
}

func TestFailedSwitchRollsBack(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, failing{}})
	m.AddProfile("work", "", config.GitIdentity{})
	m.AddProfile("home", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "W1")
	m.Import(ctx, "work", []string{"claude"}, false, nil)
	loginClaude(t, m, "h@home", "H1")
	m.Import(ctx, "home", []string{"claude"}, false, nil)
	m.Cfg.Active = "home"

	res, err := m.Use(ctx, "work", UseOptions{}, nil)
	if err == nil || !res.RolledBack {
		t.Fatalf("expected rollback, got %v", err)
	}
	if !strings.Contains(get(t, filepath.Join(m.Paths.Home, ".claude", ".credentials.json")), "H1") {
		t.Fatal("claude credentials were not rolled back")
	}
	if m.Cfg.Active != "home" {
		t.Fatal("active profile changed despite failure")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.Codex{}})
	m.AddProfile("work", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "W1")
	m.Import(ctx, "work", nil, false, nil)
	loginClaude(t, m, "h@home", "H1")
	before := get(t, filepath.Join(m.Paths.Home, ".claude", ".credentials.json"))
	res, err := m.Use(ctx, "work", UseOptions{DryRun: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if get(t, filepath.Join(m.Paths.Home, ".claude", ".credentials.json")) != before {
		t.Fatal("dry run modified files")
	}
	if list, _ := m.Backups.List(); len(list) != 0 {
		t.Fatal("dry run wrote a backup")
	}
	if len(res.Events[0].Changes) == 0 {
		t.Fatal("dry run reported no changes")
	}
}

func TestRulesAndSessionEnv(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	gc := filepath.Join(m.Paths.Home, ".gitconfig")
	m.Env.Getenv = func(k string) string {
		if k == "GIT_CONFIG_GLOBAL" {
			return gc
		}
		return ""
	}
	if _, err := m.AddProfile("work", "", config.GitIdentity{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddRule("work", config.Rule{Kind: "remote", Pattern: "gitlab.co"}, false); err == nil {
		t.Fatal("rule without identity should fail")
	}
	m.Cfg.Profiles["work"].Git = config.GitIdentity{Name: "W", Email: "w@co"}
	ch, err := m.AddRule("work", config.Rule{Kind: "remote", Pattern: "https://gitlab.co/"}, true)
	if err != nil || len(ch) == 0 {
		t.Fatalf("%v %v", ch, err)
	}
	if _, err := os.Stat(gc); !os.IsNotExist(err) {
		t.Fatal("dry-run rule wrote gitconfig")
	}
	if _, err := m.AddRule("work", config.Rule{Kind: "remote", Pattern: "gitlab.co"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, gc), "https://gitlab.co/**") {
		t.Fatal("block not written")
	}
	if _, err := m.RemoveRule("work", 0, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, gc), "gitlab.co") {
		t.Fatal("rule not removed from block")
	}

	loginClaude(t, m, "w@corp", "W1")
	m.Import(ctx, "work", []string{"claude"}, false, nil)
	vars, finish, warns, err := m.SessionEnv(ctx, "work", "/usr/bin/claude")
	if err != nil || len(warns) != 0 {
		t.Fatal(err, warns)
	}
	finish()
	joined := strings.Join(vars, "\n")
	if !strings.Contains(joined, "CLAUDE_CONFIG_DIR=") || !strings.Contains(joined, "CREAM_PROFILE=work") {
		t.Fatalf("vars = %v", vars)
	}
}

func TestLogHasNoSecrets(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.AddProfile("work", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "SUPERSECRET")
	m.Import(ctx, "work", nil, false, nil)
	m.Use(ctx, "work", UseOptions{}, nil)
	if strings.Contains(get(t, m.Paths.LogFile()), "SUPERSECRET") {
		t.Fatal("secret in log")
	}
	if get(t, m.Paths.LogFile()) == "" {
		t.Fatal("log empty")
	}
}
