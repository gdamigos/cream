package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

func findCheck(checks []Check, area string) []Check {
	var out []Check
	for _, c := range checks {
		if c.Area == area {
			out = append(out, c)
		}
	}
	return out
}

func worst(checks []Check) CheckState {
	w := CheckOK
	for _, c := range checks {
		w = max(w, c.State)
	}
	return w
}

func TestDoctorWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	m, r := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.GitIdentity{}})
	r.Passthrough["git"] = true
	m.Env.GOOS = runtime.GOOS // real paths from git, so real case rules
	home, err := filepath.EvalSymlinks(m.Paths.Home)
	if err != nil {
		t.Fatal(err)
	}
	gc := filepath.Join(m.Paths.Home, ".gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", m.Paths.Home)
	put(t, gc, "[init]\n\tdefaultBranch = main\n")

	m.AddProfile("work", "", config.GitIdentity{Name: "W", Email: "w@corp"})
	m.AddProfile("personal", "", config.GitIdentity{Name: "P", Email: "p@home"})
	m.AddRule("work", config.Rule{Kind: "remote", Pattern: "gitlab.corp"}, false)
	m.AddRule("personal", config.Rule{Kind: "dir", Pattern: filepath.Join(home, "personal")}, false)
	loginClaude(t, m, "w@corp", "T")
	m.Import(ctx, "work", []string{"claude"}, false, nil)
	if _, err := m.Use(ctx, "work", UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}

	checks := m.Doctor(ctx, "")
	if worst(findCheck(checks, "accounts")) != CheckOK || worst(findCheck(checks, "git")) != CheckOK {
		t.Fatalf("healthy setup reported problems: %+v", checks)
	}
	if len(findCheck(checks, "repo")) != 0 {
		t.Fatal("repo checks outside a repo")
	}

	// a saved account whose secret vanished from the vault
	m.Env.Vault.Delete(vault.ProfileKey("work", "claude"))
	if acc := findCheck(m.Doctor(ctx, ""), "accounts"); worst(acc) != CheckFail || !strings.Contains(acc[0].Fix, "cream import work --only claude") {
		t.Fatalf("missing vault entry not caught: %+v", acc)
	}

	git := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return string(out)
	}
	repo := func(dir string, remote string) string {
		os.MkdirAll(dir, 0o700)
		git(dir, "init", "-q")
		if remote != "" {
			git(dir, "remote", "add", "origin", remote)
		}
		return dir
	}

	// a work repo, then the same repo with a wrong local email
	app := repo(filepath.Join(home, "src", "app"), "git@gitlab.corp:team/app.git")
	if rc := findCheck(m.Doctor(ctx, app), "repo"); worst(rc) != CheckOK || !strings.Contains(rc[0].Msg, "w@corp (work)") {
		t.Fatalf("work repo: %+v", rc)
	}
	git(app, "config", "user.email", "oops@home")
	if rc := findCheck(m.Doctor(ctx, app), "repo"); worst(rc) != CheckFail || !strings.Contains(rc[0].Msg, "commits as oops@home") {
		t.Fatalf("wrong email not caught: %+v", rc)
	}

	// a repo no rule matches can't commit in strict mode
	stray := repo(filepath.Join(home, "src", "stray"), "https://example.com/x.git")
	if rc := findCheck(m.Doctor(ctx, stray), "repo"); worst(rc) != CheckWarn || !strings.Contains(rc[0].Msg, "refuses to commit") {
		t.Fatalf("stray repo: %+v", rc)
	}

	// a personal repo with an unpushed commit made with the wrong email
	up := filepath.Join(home, "up.git")
	git(home, "init", "-q", "--bare", up)
	blog := repo(filepath.Join(home, "personal", "blog"), up)
	put(t, filepath.Join(blog, "a"), "a")
	git(blog, "add", "a")
	git(blog, "commit", "-qm", "a")
	git(blog, "push", "-q", "-u", "origin", "main")
	put(t, filepath.Join(blog, "b"), "b")
	git(blog, "add", "b")
	git(blog, "-c", "user.email=w@corp", "commit", "-qm", "b")
	rc := findCheck(m.Doctor(ctx, blog), "repo")
	if len(rc) != 2 || rc[0].State != CheckOK || rc[1].State != CheckFail || !strings.Contains(rc[1].Msg, "1 not pushed") {
		t.Fatalf("unpushed wrong commit not caught: %+v", rc)
	}
}

func TestDoctorFlagsLiveAccountMismatch(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}})
	m.AddProfile("work", "", config.GitIdentity{})
	loginClaude(t, m, "w@corp", "T")
	m.Import(ctx, "work", nil, false, nil)
	loginClaude(t, m, "me@home", "H") // logged in elsewhere as someone else

	tools := findCheck(m.Doctor(ctx, ""), "tools")
	if len(tools) != 1 || tools[0].State != CheckWarn || tools[0].Fix != "cream use work --only claude" {
		t.Fatalf("mismatch not flagged: %+v", tools)
	}
}
