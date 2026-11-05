package adapters

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/testutil"
)

func TestRenderAndReplaceBlock(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	w := e.Profile(t, "work")
	w.Git = config.GitIdentity{Name: "Me Work", Email: "me@work.com"}
	w.Rules = []config.Rule{{Kind: "remote", Pattern: "gitlab.company.com"}, {Kind: "dir", Pattern: "~/work"}}
	block := RenderBlock(e.Env, e.Config, nil)
	for _, want := range []string{"useConfigOnly = true", `[includeIf "gitdir:~/work/"]`, `hasconfig:remote.*.url:https://gitlab.company.com/**`, `hasconfig:remote.*.url:*@gitlab.company.com:*/**`} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "email =") {
		t.Error("strict block must not set a global email")
	}
	// a ~/.gitconfig as people really keep it, with credential helpers, url rewrites and their own includeIf
	user := fixture(t, "git/gitconfig")
	once := ReplaceBlock(user, block)
	twice := ReplaceBlock(once, block)
	if once != twice {
		t.Fatal("ReplaceBlock is not idempotent")
	}
	if !strings.HasPrefix(once, strings.TrimRight(user, "\n")) {
		t.Fatal("user config not preserved")
	}
	if HasGlobalEmailOutsideBlock(once) {
		t.Fatal("false positive email detection")
	}
	if !HasGlobalEmailOutsideBlock("[user]\n\tname = x\n\temail = x@y\n" + block) {
		t.Fatal("missed global email outside block")
	}
	if RuleConditions(config.Rule{Kind: "dir", Pattern: `C:\work`}, "windows")[0] != "gitdir/i:C:/work/" {
		t.Fatal("windows dir rule")
	}
}

func TestGitWithoutRulesSwitchesGlobalIdentity(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	w := e.Profile(t, "work")
	w.Git = config.GitIdentity{Name: "Work", Email: "me@work.com"}
	h := e.Profile(t, "home")
	h.Git = config.GitIdentity{Name: "Home", Email: "me@home.org"}
	gc := filepath.Join(e.Paths.Home, ".gitconfig")
	for _, p := range []*config.Profile{w, h} {
		if _, err := (GitIdentity{}).Activate(ctx, e.Env, p, false); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(read(t, gc), "email = \""+p.Git.Email+"\"") {
			t.Fatalf("after switching to %s:\n%s", p.Name, read(t, gc))
		}
	}
	// once a rule exists strict mode drops the global identity
	w.Rules = []config.Rule{{Kind: "dir", Pattern: "~/work"}}
	if _, err := (GitIdentity{}).Activate(ctx, e.Env, h, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, gc), "email = \"me@home.org\"") {
		t.Fatalf("strict mode with rules kept a global email:\n%s", read(t, gc))
	}
}

// a work remote always commits as work and a repo matching no rule cannot commit
func TestGitRulesWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	e := testutil.NewEnv(t, "")
	gc := filepath.Join(e.Paths.Home, ".gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", e.Paths.Home)
	e.Vars["GIT_CONFIG_GLOBAL"] = gc
	write(t, gc, "[init]\n\tdefaultBranch = main\n")

	w := e.Profile(t, "work")
	w.Git = config.GitIdentity{Name: "Me Work", Email: "me@work.com"}
	w.Rules = []config.Rule{{Kind: "remote", Pattern: "gitlab.company.com"}}
	h := e.Profile(t, "home")
	h.Git = config.GitIdentity{Name: "Me", Email: "me@home.org"}
	// git matches gitdir against the real path, ci temp dirs can be 8.3 short names or symlinks
	realHome, err := filepath.EvalSymlinks(e.Paths.Home)
	if err != nil {
		t.Fatal(err)
	}
	h.Rules = []config.Rule{{Kind: "dir", Pattern: filepath.Join(realHome, "personal")}}
	if _, err := Sync(e.Env, e.Config, nil, false); err != nil {
		t.Fatal(err)
	}

	git := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	mkrepo := func(dir string, remotes ...string) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if out, err := git(dir, "init", "-q"); err != nil {
			t.Fatal(out)
		}
		for i, r := range remotes {
			if out, err := git(dir, "remote", "add", "r"+string(rune('a'+i)), r); err != nil {
				t.Fatal(out)
			}
		}
	}
	root := filepath.Join(e.Paths.Home, "src")
	cases := map[string]string{
		"git@gitlab.company.com:team/app.git":            "me@work.com",
		"git@gitlab.company.com:group/sub/app.git":       "me@work.com",
		"git@gitlab.company.com:app.git":                 "me@work.com",
		"https://gitlab.company.com/team/app.git":        "me@work.com",
		"ssh://git@gitlab.company.com:2222/team/app.git": "me@work.com",
	}
	i := 0
	for url, want := range cases {
		dir := filepath.Join(root, "r"+string(rune('0'+i)))
		i++
		mkrepo(dir, url)
		got, err := git(dir, "config", "user.email")
		if err != nil || got != want {
			t.Errorf("%s: user.email = %q (%v), want %q", url, got, err, want)
		}
	}
	personal := filepath.Join(e.Paths.Home, "personal", "blog")
	mkrepo(personal, "https://github.com/me/blog.git")
	if got, _ := git(personal, "config", "user.email"); got != "me@home.org" {
		t.Errorf("dir rule: got %q", got)
	}
	stray := filepath.Join(root, "stray")
	mkrepo(stray, "https://example.com/x.git")
	write(t, filepath.Join(stray, "f"), "x")
	git(stray, "add", "f")
	if out, err := git(stray, "commit", "-m", "x"); err == nil {
		t.Errorf("commit without matching rule should fail, got: %s", out)
	}
}

func TestRemoteMatches(t *testing.T) {
	host := config.Rule{Kind: "remote", Pattern: "gitlab.corp.com"}
	group := config.Rule{Kind: "remote", Pattern: "github.com/acme"}
	cases := []struct {
		rule config.Rule
		url  string
		want bool
	}{
		{host, "https://gitlab.corp.com/team/app.git", true},
		{host, "https://me@gitlab.corp.com/team/app", true},
		{host, "ssh://git@gitlab.corp.com:2222/team/app.git", true},
		{host, "git@gitlab.corp.com:team/app.git", true},
		{host, "git@GitLab.Corp.com:team/app.git", true},
		{host, "https://gitlab.corp.com.evil.io/team/app.git", false},
		{host, "https://github.com/team/app.git", false},
		{group, "git@github.com:acme/app.git", true},
		{group, "https://github.com/acme-other/app.git", false},
		{config.Rule{Kind: "dir", Pattern: "gitlab.corp.com"}, "https://gitlab.corp.com/x", false},
	}
	for _, c := range cases {
		if got := RemoteMatches(c.rule, c.url); got != c.want {
			t.Errorf("%s vs %s: got %v", c.rule, c.url, got)
		}
	}
}

func TestDirMatches(t *testing.T) {
	r := config.Rule{Kind: "dir", Pattern: "~/work"}
	if !DirMatches(r, "/home/me/work/app", "/home/me", "linux") || !DirMatches(r, "/home/me/work", "/home/me", "linux") {
		t.Fatal("repo under the folder should match")
	}
	if DirMatches(r, "/home/me/workshop/app", "/home/me", "linux") {
		t.Fatal("a sibling folder with the same prefix must not match")
	}
	if !DirMatches(config.Rule{Kind: "dir", Pattern: `C:\Code\Work`}, `c:/code/work/app`, `C:\Users\me`, "windows") {
		t.Fatal("windows paths are case insensitive")
	}
	if DirMatches(config.Rule{Kind: "dir", Pattern: "/Code/Work"}, "/code/work/app", "/home/me", "linux") {
		t.Fatal("linux paths are case sensitive")
	}
}

// the user's gitconfig is arbitrary text, replacing our block must keep it and be repeatable
func FuzzReplaceBlock(f *testing.F) {
	block := BlockBegin + "\n[user]\n\tuseConfigOnly = true\n" + BlockEnd + "\n"
	f.Add("[core]\n\tautocrlf = false\n")
	f.Add("")
	f.Add(block)
	f.Add("[user]\n\temail = a@b\n" + block + "[alias]\n\tst = status\n")
	f.Add(BlockBegin + "\nunterminated")
	f.Fuzz(func(t *testing.T, user string) {
		once := ReplaceBlock(user, block)
		if strings.Count(once, BlockBegin) != 1 || !strings.HasSuffix(once, block) {
			t.Fatalf("block not placed once at the end:\n%q", once)
		}
		if twice := ReplaceBlock(once, block); twice != once {
			t.Fatalf("not idempotent:\n%q\n%q", once, twice)
		}
		if !strings.Contains(user, BlockBegin) && !strings.HasPrefix(once, strings.TrimRight(user, "\n")) {
			t.Fatalf("user config lost:\n%q", once)
		}
		_ = HasGlobalEmailOutsideBlock(once)
	})
}

// an email of the user's own outside the block defeats strict mode, and status says so
func TestGitStatusWarnsAboutGlobalEmail(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	w := e.Profile(t, "work")
	w.Git = config.GitIdentity{Name: "Dev", Email: "dev@work.example.com"}
	w.Rules = []config.Rule{{Kind: "remote", Pattern: "gitlab.corp.example.com"}}
	block := RenderBlock(e.Env, e.Config, nil)
	gc := filepath.Join(e.Paths.Home, ".gitconfig")
	write(t, gc, ReplaceBlock(fixture(t, "git/gitconfig_with_email"), block))
	if st := (GitIdentity{}).Status(ctx, e.Env); !strings.Contains(st.Warning, "defeats strict mode") {
		t.Fatalf("status = %+v", st)
	}
}
