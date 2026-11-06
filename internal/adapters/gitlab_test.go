package adapters

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/testutil"
)

func TestGitLabRoundTrip(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cfg := filepath.Join(e.Paths.Home, ".config", "glab-cli", "config.yml")
	write(t, cfg, fixture(t, "glab/config.yml"))
	g := GitLab{}
	w := e.Profile(t, "work")
	a, err := g.Import(ctx, e.Env, w)
	if err != nil || a.Label != "me-work" || a.Host != "gitlab.company.com" {
		t.Fatalf("%+v %v", a, err)
	}
	w.Accounts["gitlab"] = a

	// user rotates the token in glab then switches back to work
	write(t, cfg, strings.Replace(fixture(t, "glab/config.yml"), "glpat-WORK", "glpat-OTHER", 1))
	if _, err := g.Activate(ctx, e.Env, w, false); err != nil {
		t.Fatal(err)
	}
	out := read(t, cfg)
	if !strings.Contains(out, "glpat-WORK") || !strings.Contains(out, "glpat-HOME") {
		t.Fatalf("config after activate:\n%s", out)
	}
	if !strings.Contains(out, "comment that must survive") {
		t.Fatal("comments not preserved")
	}
	vars, _, err := g.SessionEnv(ctx, e.Env, w)
	if err != nil || vars[0] != "GITLAB_TOKEN=glpat-WORK" || vars[1] != "GITLAB_HOST=gitlab.company.com" {
		t.Fatalf("%v %v", vars, err)
	}
}

func TestGitLabLockUnlockKeepsOtherHosts(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cfg := filepath.Join(e.Paths.Home, ".config", "glab-cli", "config.yml")
	write(t, cfg, fixture(t, "glab/config.yml"))
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
