package adapters

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/testutil"
)

func TestAdaptersLock(t *testing.T) {
	for _, a := range All() {
		_, isLocker := a.(adapter.Locker)
		if want := a.ID() != "git"; isLocker != want {
			t.Errorf("%s: Locker = %v, want %v", a.ID(), isLocker, want)
		}
	}
}

func TestClaudeLockUnlock(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cred := filepath.Join(e.Paths.Home, ".claude", ".credentials.json")
	state := filepath.Join(e.Paths.Home, ".claude.json")
	write(t, cred, `{"t":"WORK-SECRET"}`)
	write(t, state, claudeState("me@work.com"))
	c := Claude{}

	if locked, _, _ := c.Lock(ctx, e.Env, "k", "someone@else.com", false); locked {
		t.Fatal("locked another account's login")
	}
	locked, ch, err := c.Lock(ctx, e.Env, "k", "me@work.com", true)
	if err != nil || !locked || len(ch) == 0 || !fsutil.Exists(cred) {
		t.Fatalf("dry run: %v %v %v, file must stay", locked, ch, err)
	}
	if locked, _, err := c.Lock(ctx, e.Env, "k", "", false); err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	if fsutil.Exists(cred) || c.Status(ctx, e.Env).LoggedIn {
		t.Fatal("still logged in after lock")
	}
	if !strings.Contains(read(t, state), `"theme"`) {
		t.Fatal("lock dropped unrelated .claude.json settings")
	}
	if restored, _, err := c.Unlock(ctx, e.Env, "k", false); err != nil || !restored {
		t.Fatalf("unlock: %v %v", restored, err)
	}
	if !strings.Contains(read(t, cred), "WORK-SECRET") || c.Status(ctx, e.Env).Account != "me@work.com" {
		t.Fatal("login not restored")
	}
	if restored, _, _ := c.Unlock(ctx, e.Env, "k", false); restored {
		t.Fatal("the saved login must be gone after unlock")
	}
}

func TestCodexLockUnlock(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	auth := filepath.Join(e.Paths.Home, ".codex", "auth.json")
	write(t, auth, `{"OPENAI_API_KEY":"sk-SECRET"}`)
	c := Codex{}
	if locked, _, err := c.Lock(ctx, e.Env, "k", "", false); err != nil || !locked || fsutil.Exists(auth) {
		t.Fatalf("lock: %v %v", locked, err)
	}
	if restored, _, err := c.Unlock(ctx, e.Env, "k", false); err != nil || !restored || !strings.Contains(read(t, auth), "sk-SECRET") {
		t.Fatalf("unlock: %v %v", restored, err)
	}
}

func TestGitLabLockUnlockKeepsOtherHosts(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cfg := filepath.Join(e.Paths.Home, ".config", "glab-cli", "config.yml")
	write(t, cfg, glabCfg)
	g := GitLab{}
	if locked, _, err := g.Lock(ctx, e.Env, "k", "me-work", false); err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	out := read(t, cfg)
	if strings.Contains(out, "glpat-WORK") || !strings.Contains(out, "glpat-HOME") || !strings.Contains(out, "comment that must survive") {
		t.Fatalf("after lock:\n%s", out)
	}
	if g.Status(ctx, e.Env).LoggedIn {
		t.Fatal("glab still logged in")
	}
	if restored, _, err := g.Unlock(ctx, e.Env, "k", false); err != nil || !restored {
		t.Fatalf("unlock: %v %v", restored, err)
	}
	if !strings.Contains(read(t, cfg), "glpat-WORK") {
		t.Fatal("token not restored")
	}
}

func TestGitHubLockUnlockTokensOnlyViaStdin(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), ghHosts)
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
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), ghHosts)
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
