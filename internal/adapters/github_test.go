package adapters

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/testutil"
)

func TestGitHubSwitchUsesGhAuthSwitch(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), fixture(t, "gh/hosts.yml"))
	g := GitHub{}
	st := g.Status(ctx, e.Env)
	if st.Account != "alice" || !strings.Contains(st.Detail, "alice-work") {
		t.Fatalf("status = %+v", st)
	}
	p := e.Profile(t, "work")
	p.Accounts["github"] = &config.Account{Label: "alice-work", Host: "github.com"}
	e.Runner.Outputs["gh auth switch --hostname github.com --user alice-work"] = ""
	ch, err := g.Activate(ctx, e.Env, p, false)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %v", ch, err)
	}
	if got := e.Runner.CallStrings(); len(got) != 1 || got[0] != "gh auth switch --hostname github.com --user alice-work" {
		t.Fatalf("calls = %v", got)
	}
	home := e.Profile(t, "home")
	home.Accounts["github"] = &config.Account{Label: "alice", Host: "github.com"}
	if ch, err := g.Activate(ctx, e.Env, home, false); err != nil || len(ch) != 0 {
		t.Fatalf("expected no-op, got %v %v", ch, err)
	}
	// unknown user is a clear error
	bad := e.Profile(t, "bad")
	bad.Accounts["github"] = &config.Account{Label: "mallory", Host: "github.com"}
	if _, err := g.Activate(ctx, e.Env, bad, false); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("err = %v", err)
	}
}

func TestGitHubSessionTokenViaStdout(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	p := e.Profile(t, "work")
	p.Accounts["github"] = &config.Account{Label: "alice-work", Host: "github.com"}
	e.Runner.Outputs["gh auth token --hostname github.com --user alice-work"] = "gho_SECRET\n"
	vars, _, err := (GitHub{}).SessionEnv(ctx, e.Env, p)
	if err != nil || vars[0] != "GH_TOKEN=gho_SECRET" {
		t.Fatalf("%v %v", vars, err)
	}
}

func TestGitHubLockUnlockTokensOnlyViaStdin(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), fixture(t, "gh/hosts.yml"))
	r := e.Runner
	r.Outputs["gh auth token --hostname github.com --user alice"] = "gho_HOME\n"
	r.Outputs["gh auth token --hostname github.com --user alice-work"] = "gho_WORK\n"
	r.Outputs["gh auth logout --hostname github.com --user alice"] = ""
	r.Outputs["gh auth logout --hostname github.com --user alice-work"] = ""
	r.Outputs["gh auth login --hostname github.com --with-token"] = ""
	r.Outputs["gh auth switch --hostname github.com --user alice"] = ""
	g := GitHub{}

	// everything: both accounts go, or gh would fall back to the other one
	if locked, _, err := g.Lock(ctx, e.Env, "k", "", false); err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	for _, u := range []string{"alice", "alice-work"} {
		if !slices.Contains(r.CallStrings(), "gh auth logout --hostname github.com --user "+u) {
			t.Fatalf("%s not logged out: %v", u, r.CallStrings())
		}
	}
	// gh rewrote hosts.yml when logging out
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), "{}\n")

	r.Calls = nil
	if restored, _, err := g.Unlock(ctx, e.Env, "k", false); err != nil || !restored {
		t.Fatalf("unlock: %v %v", restored, err)
	}
	var stdin []string
	for _, c := range r.Calls {
		args := strings.Join(c.Args, " ")
		if strings.Contains(args, "gho_") {
			t.Fatalf("token in argv: %s", args)
		}
		if strings.Contains(args, "--with-token") {
			stdin = append(stdin, strings.TrimSpace(c.Stdin))
		}
	}
	if strings.Join(stdin, ",") != "gho_HOME,gho_WORK" {
		t.Fatalf("tokens via stdin = %v", stdin)
	}
	if last := r.CallStrings()[len(r.Calls)-1]; last != "gh auth switch --hostname github.com --user alice" {
		t.Fatalf("the account active at lock time must be active again, last call %q", last)
	}
}

func TestGitHubLockOneAccount(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), fixture(t, "gh/hosts.yml"))
	e.Runner.Outputs["gh auth token --hostname github.com --user alice-work"] = "gho_WORK\n"
	e.Runner.Outputs["gh auth logout --hostname github.com --user alice-work"] = ""
	if locked, _, err := (GitHub{}).Lock(ctx, e.Env, "k", "alice-work", false); err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	for _, c := range e.Runner.CallStrings() {
		if strings.Contains(c, "--user alice") && !strings.Contains(c, "alice-work") {
			t.Fatalf("touched the other account: %s", c)
		}
	}
	if locked, _, _ := (GitHub{}).Lock(ctx, e.Env, "k2", "mallory", false); locked {
		t.Fatal("locked an account gh doesn't know")
	}
}

// github.com plus a GitHub Enterprise host with two accounts, GH_HOST picks the host
func TestGitHubEnterpriseHosts(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), fixture(t, "gh/hosts_enterprise.yml"))
	g := GitHub{}
	if st := g.Status(ctx, e.Env); st.Account != "alice" || st.Detail != "github.com" {
		t.Fatalf("default host status = %+v", st)
	}
	e.Vars["GH_HOST"] = "ghe.corp.example.com"
	st := g.Status(ctx, e.Env)
	if st.Account != "alice-corp" || !strings.Contains(st.Detail, "also: alice-admin") {
		t.Fatalf("enterprise status = %+v", st)
	}
	p := e.Profile(t, "corp")
	acct, err := g.Import(ctx, e.Env, p)
	if err != nil || acct.Label != "alice-corp" || acct.Host != "ghe.corp.example.com" {
		t.Fatalf("import = %+v %v", acct, err)
	}
	p.Accounts["github"] = &config.Account{Label: "alice-admin", Host: "ghe.corp.example.com"}
	e.Runner.Outputs["gh auth switch --hostname ghe.corp.example.com --user alice-admin"] = ""
	if _, err := g.Activate(ctx, e.Env, p, false); err != nil {
		t.Fatal(err)
	}
	if calls := e.Runner.CallStrings(); len(calls) != 1 || !strings.Contains(calls[0], "--hostname ghe.corp.example.com --user alice-admin") {
		t.Fatalf("calls = %v", calls)
	}
}
