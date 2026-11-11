package core

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/backup"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

const pw = "correct horse"

// alice is protected and inactive, bob is active
func protectSetup(t *testing.T, ads []adapter.Adapter) *Manager {
	t.Helper()
	m, _ := newManager(t, ads)
	m.kdfWork = 10
	m.AddProfile("alice", "alice's laptop", config.GitIdentity{Name: "Alice", Email: "alice@example.com"})
	m.AddProfile("bob", "", config.GitIdentity{Name: "Bob", Email: "bob@example.com"})
	if _, err := m.AddRule("alice", config.Rule{Kind: "remote", Pattern: "gitlab.example.com"}, false); err != nil {
		t.Fatal(err)
	}
	loginClaude(t, m, "alice@example.com", "ALICE1")
	if _, err := m.Import(ctx, "alice", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	loginClaude(t, m, "bob@example.com", "BOB1")
	if _, err := m.Import(ctx, "bob", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Use(ctx, "bob", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetPassword(ctx, "alice", "", pw, false); err != nil {
		t.Fatal(err)
	}
	m.Reseal()
	return m
}

func rawGet(t *testing.T, m *Manager, key string) string {
	t.Helper()
	b, err := m.raw().Get(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPasswordHidesEverything(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})

	cfg := get(t, m.Paths.ConfigFile())
	for _, leak := range []string{"alice@example.com", "Alice", "gitlab.example.com", "laptop"} {
		if strings.Contains(cfg, leak) {
			t.Fatalf("config.toml shows %q:\n%s", leak, cfg)
		}
	}
	if s := rawGet(t, m, vault.ProfileKey("alice", "claude")); strings.Contains(s, "ALICE1") || !strings.HasPrefix(s, sealMagic) {
		t.Fatalf("vault entry not encrypted: %q", s)
	}
	if strings.Contains(m.gitconfig(t), "gitlab.example.com") || fileExists(filepath.Join(m.Paths.GitDir(), "alice.gitconfig")) {
		t.Fatal("alice's git rule or identity file stayed on disk")
	}
	if p := m.Cfg.Profiles["alice"]; !p.Sealed() || len(p.Accounts) != 0 || p.Git.Email != "" {
		t.Fatalf("alice is readable in memory after Reseal: %+v", p)
	}
	// a fresh process sees the same
	other, err := Open(Options{Paths: &m.Paths, Vault: m.raw(), Runner: m.Env.Run, Adapters: m.all})
	if err != nil {
		t.Fatal(err)
	}
	if p := other.Cfg.Profiles["alice"]; !p.Sealed() || p.Description != "" {
		t.Fatal("reloaded config shows alice")
	}
}

func TestPasswordGuardsSwitch(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})

	var pr PasswordRequired
	if _, err := m.Use(ctx, "alice", UseOptions{}, nil); !errors.As(err, &pr) || pr.Profile != "alice" {
		t.Fatalf("switch without password: %v", err)
	}
	for _, f := range []func() error{
		func() error { _, err := m.Import(ctx, "alice", nil, false, nil); return err },
		func() error { _, err := m.Forget("alice", []string{"claude"}, false); return err },
		func() error { return m.RemoveProfile("alice") },
		func() error { _, _, _, err := m.SessionEnv(ctx, "alice", "claude"); return err },
		func() error { _, err := m.Lock(ctx, "alice", false, nil); return err },
	} {
		if err := f(); !errors.As(err, &pr) {
			t.Fatalf("expected PasswordRequired, got %v", err)
		}
	}
	if err := m.Unseal("alice", "wrong password"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if err := m.Unseal("alice", pw); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Use(ctx, "alice", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, claudeCred(m)), "ALICE1") || !strings.Contains(m.gitconfig(t), "gitlab.example.com") {
		t.Fatal("switch to alice did not bring back her login and rules")
	}
	// active, so readable for syncOut and git, but nothing leaves memory after she is left
	loginClaude(t, m, "alice@example.com", "ALICE2")
	m.Reseal()
	if _, err := m.Use(ctx, "bob", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(get(t, m.Paths.ConfigFile()), "alice@example.com") || strings.Contains(m.gitconfig(t), "gitlab.example.com") {
		t.Fatal("leaving alice left her data readable")
	}
	m.Unseal("alice", pw)
	if b, _ := m.Env.Vault.Get(vault.ProfileKey("alice", "claude")); !strings.Contains(string(b), "ALICE2") {
		t.Fatal("alice's refreshed token was not saved when she was left")
	}
}

func TestBackupOfProtectedNeedsPassword(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}})
	m.Unseal("alice", pw)
	m.Use(ctx, "alice", UseOptions{}, nil)
	m.Reseal()
	m.Use(ctx, "bob", UseOptions{}, nil) // this backup holds alice's live login

	list, _ := m.Backups.List()
	if len(list) == 0 || list[0].Owner != "alice" {
		t.Fatalf("latest backup should be sealed for alice: %+v", list)
	}
	var pr PasswordRequired
	if _, err := m.Restore(ctx, "", false); !errors.As(err, &pr) {
		t.Fatalf("restore without password: %v", err)
	}
	m.Unseal("alice", pw)
	if _, err := m.Restore(ctx, "", false); err != nil || m.Cfg.Active != "alice" {
		t.Fatalf("restore: %v active=%s", err, m.Cfg.Active)
	}
}

func TestOldBackupsAreSealedWithPassword(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.kdfWork = 10
	m.AddProfile("alice", "", config.GitIdentity{})
	m.AddProfile("bob", "", config.GitIdentity{})
	loginClaude(t, m, "alice@example.com", "ALICE1")
	m.Import(ctx, "alice", nil, false, nil)
	loginClaude(t, m, "bob@example.com", "BOB1")
	m.Import(ctx, "bob", nil, false, nil)
	m.Use(ctx, "bob", UseOptions{}, nil) // from alice, unsealed at the time
	if _, err := m.SetPassword(ctx, "alice", "", pw, false); err != nil {
		t.Fatal(err)
	}
	list, _ := m.Backups.List()
	if !slices.ContainsFunc(list, func(i backup.Info) bool { return i.Owner == "alice" }) {
		t.Fatalf("alice's old backup was not sealed: %+v", list)
	}
}

func TestRemovePasswordAndChange(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	if _, err := m.SetPassword(ctx, "alice", "nope", "another password", false); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("change with a wrong old password: %v", err)
	}
	if _, err := m.SetPassword(ctx, "alice", pw, "short", false); err == nil {
		t.Fatal("short password accepted")
	}
	if _, err := m.SetPassword(ctx, "alice", pw, "another password", false); err != nil {
		t.Fatal(err)
	}
	m.Reseal()
	if err := m.Unseal("alice", pw); !errors.Is(err, ErrWrongPassword) {
		t.Fatal("old password still works")
	}
	if _, err := m.RemovePassword(ctx, "alice", "another password", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, m.Paths.ConfigFile()), "alice@example.com") || !strings.Contains(m.gitconfig(t), "gitlab.example.com") {
		t.Fatal("alice's data did not come back")
	}
	if s := rawGet(t, m, vault.ProfileKey("alice", "claude")); !strings.Contains(s, "ALICE1") {
		t.Fatalf("vault entry still encrypted: %q", s)
	}
	if _, err := m.raw().Get(vault.ProtectKey("alice")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatal("alice's key stayed in the vault")
	}
	if _, err := m.Use(ctx, "alice", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDryRunSetPasswordChangesNothing(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.AddProfile("alice", "", config.GitIdentity{})
	before := get(t, m.Paths.ConfigFile())
	ch, err := m.SetPassword(ctx, "alice", "", pw, true)
	if err != nil || len(ch) == 0 {
		t.Fatalf("dry run: %v %v", ch, err)
	}
	if get(t, m.Paths.ConfigFile()) != before || m.Protected("alice") {
		t.Fatal("dry run changed something")
	}
	if _, err := m.raw().Get(vault.ProtectKey("alice")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatal("dry run wrote a key")
	}
}

func TestSyncActiveSkipsUnchangedSealedToken(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}})
	m.Unseal("alice", pw)
	m.Use(ctx, "alice", UseOptions{}, nil)
	m.Reseal()
	if changed, _ := m.SyncActive(ctx); len(changed) != 0 {
		t.Fatalf("unchanged token counted as refreshed: %v", changed)
	}
	loginClaude(t, m, "alice@example.com", "ALICE3")
	if changed, _ := m.SyncActive(ctx); len(changed) != 1 {
		t.Fatal("refreshed token not saved")
	}
}

func TestGitHubLoginLeavesWithProtectedProfile(t *testing.T) {
	m, r := newManager(t, []adapter.Adapter{adapters.GitHub{}})
	m.kdfWork = 10
	hosts := filepath.Join(m.Paths.Home, ".config", "gh", "hosts.yml")
	put(t, hosts, "github.com:\n  user: alice-gh\n  users:\n    alice-gh: {}\n    bob-gh: {}\n")
	m.AddProfile("alice", "", config.GitIdentity{})
	m.AddProfile("bob", "", config.GitIdentity{})
	m.Import(ctx, "alice", nil, false, nil)
	m.Cfg.Profiles["bob"].Accounts["github"] = &config.Account{Label: "bob-gh", Host: "github.com"}
	if _, err := m.SetPassword(ctx, "alice", "", pw, false); err != nil {
		t.Fatal(err)
	}
	r.Outputs["gh auth switch --hostname github.com --user bob-gh"] = ""
	r.Outputs["gh auth token --hostname github.com --user alice-gh"] = "TOKEN-A\n"
	r.Outputs["gh auth logout --hostname github.com --user alice-gh"] = ""
	if _, err := m.Use(ctx, "bob", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.CallStrings(), "gh auth logout --hostname github.com --user alice-gh") {
		t.Fatalf("alice's gh login was not logged out: %v", r.CallStrings())
	}
	if s := rawGet(t, m, vault.ProfileKey("alice", "github")); strings.Contains(s, "TOKEN-A") {
		t.Fatal("stashed gh token is not encrypted")
	}
	// cream run still gets her token from the stash, gh itself no longer has it
	delete(r.Outputs, "gh auth token --hostname github.com --user alice-gh")
	m.Unseal("alice", pw)
	vars, _, _, err := m.SessionEnv(ctx, "alice", "gh")
	if err != nil || !slices.Contains(vars, "GH_TOKEN=TOKEN-A") {
		t.Fatalf("session env from stash: %v %v", vars, err)
	}
}

func TestSessionDropsTokenCopiesOfProtected(t *testing.T) {
	m := protectSetup(t, []adapter.Adapter{adapters.Claude{}})
	m.Unseal("alice", pw)
	_, finish, _, err := m.SessionEnv(ctx, "alice", "claude")
	if err != nil {
		t.Fatal(err)
	}
	cred := filepath.Join(m.Paths.SessionDir("alice", "claude"), ".credentials.json")
	if !fileExists(cred) {
		t.Fatal("session has no credentials")
	}
	finish()
	if fileExists(cred) {
		t.Fatal("session token copy stayed on disk")
	}
	state := filepath.Join(m.Paths.SessionDir("alice", "claude"), ".claude.json")
	if s := get(t, state); s == "" || strings.Contains(s, "alice@example.com") {
		t.Fatalf("session state should stay without the account: %q", s)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
