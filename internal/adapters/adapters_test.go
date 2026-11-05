package adapters

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/testutil"
)

var ctx = context.Background()

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func claudeState(email string) string {
	return `{"numStartups": 7, "theme": "dark", "hasCompletedOnboarding": true, "oauthAccount": {"emailAddress": "` + email + `", "organizationName": "Org"}}`
}

func TestClaudeImportActivateRoundTrip(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cred := filepath.Join(e.Paths.Home, ".claude", ".credentials.json")
	state := filepath.Join(e.Paths.Home, ".claude.json")
	c := Claude{}

	write(t, cred, `{"claudeAiOauth":{"accessToken":"WORK-SECRET"}}`)
	write(t, state, claudeState("me@work.com"))
	work := e.Profile(t, "work")
	acct, err := c.Import(ctx, e.Env, work)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Label != "me@work.com" {
		t.Fatalf("label = %q", acct.Label)
	}
	work.Accounts["claude"] = acct

	// log in as personal and import
	write(t, cred, `{"claudeAiOauth":{"accessToken":"HOME-SECRET"}}`)
	write(t, state, claudeState("me@home.org"))
	home := e.Profile(t, "personal")
	if _, err := c.Import(ctx, e.Env, home); err != nil {
		t.Fatal(err)
	}

	if st := c.Status(ctx, e.Env); st.Account != "me@home.org" || !st.LoggedIn {
		t.Fatalf("status before switch = %+v", st)
	}
	// dry-run changes nothing
	ch, err := c.Activate(ctx, e.Env, work, true)
	if err != nil || len(ch) != 2 {
		t.Fatalf("dry run: %v %v", ch, err)
	}
	if !strings.Contains(read(t, cred), "HOME-SECRET") {
		t.Fatal("dry run modified credentials")
	}
	if _, err := c.Activate(ctx, e.Env, work, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, cred), "WORK-SECRET") {
		t.Fatal("credentials not swapped")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, state)), &m); err != nil {
		t.Fatal(err)
	}
	if m["numStartups"].(float64) != 7 || m["theme"] != "dark" {
		t.Fatal("other ~/.claude.json keys were not preserved")
	}
	if c.Status(ctx, e.Env).Account != "me@work.com" {
		t.Fatal("oauthAccount not swapped")
	}
}

func TestClaudeNotLoggedIn(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	if _, err := (Claude{}).Import(ctx, e.Env, e.Profile(t, "x")); err == nil {
		t.Fatal("expected error")
	}
	_, err := (Claude{}).Activate(ctx, e.Env, e.Config.Profiles["x"], false)
	var ni adapter.ErrNotImported
	if !errors.As(err, &ni) {
		t.Fatalf("want ErrNotImported, got %v", err)
	}
}

func TestClaudeMacKeychainUsesStdinNotArgs(t *testing.T) {
	e := testutil.NewEnv(t, "darwin")
	e.Vars["USER"] = "joji"
	secret := `{"claudeAiOauth":{"accessToken":"MAC-SECRET"}}`
	e.Runner.Outputs["/usr/bin/security find-generic-password -s Claude Code-credentials -w"] = secret + "\n"
	e.Runner.Outputs["/usr/bin/security -i"] = ""
	write(t, filepath.Join(e.Paths.Home, ".claude.json"), claudeState("mac@work.com"))
	p := e.Profile(t, "work")
	if _, err := (Claude{}).Import(ctx, e.Env, p); err != nil {
		t.Fatal(err)
	}
	if _, err := (Claude{}).Activate(ctx, e.Env, p, false); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range e.Runner.Calls {
		for _, a := range c.Args {
			if strings.Contains(a, "MAC-SECRET") {
				t.Fatalf("secret leaked into argv: %v", c.Args)
			}
		}
		if c.Name == "/usr/bin/security" && len(c.Args) == 1 && c.Args[0] == "-i" {
			found = true
			if !strings.Contains(c.Stdin, hex.EncodeToString([]byte(secret))) {
				t.Fatal("keychain write did not carry hex secret on stdin")
			}
		}
	}
	if !found {
		t.Fatal("no keychain write happened")
	}
	if fileExists(filepath.Join(e.Paths.Home, ".claude", ".credentials.json")) {
		t.Fatal("macOS must not write .credentials.json")
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestClaudeSessionEnvIsolatedAndSyncsBack(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".claude", ".credentials.json"), `{"t":"A"}`)
	write(t, filepath.Join(e.Paths.Home, ".claude.json"), claudeState("a@x.com"))
	p := e.Profile(t, "work")
	if _, err := (Claude{}).Import(ctx, e.Env, p); err != nil {
		t.Fatal(err)
	}
	vars, finish, err := (Claude{}).SessionEnv(ctx, e.Env, p)
	if err != nil {
		t.Fatal(err)
	}
	dir := e.Paths.SessionDir("work", "claude")
	if len(vars) != 1 || vars[0] != "CLAUDE_CONFIG_DIR="+dir {
		t.Fatalf("vars = %v", vars)
	}
	if !strings.Contains(read(t, filepath.Join(dir, ".claude.json")), "a@x.com") {
		t.Fatal("session state missing oauthAccount")
	}
	// claude refreshed its token during the session
	write(t, filepath.Join(dir, ".credentials.json"), `{"t":"REFRESHED"}`)
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	b, _ := e.Mem.Get("profile:work:claude")
	if !strings.Contains(string(b), "REFRESHED") {
		t.Fatal("refreshed token not synced back to vault")
	}
}

func fakeIDToken(email string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	return "eyJhbGciOiJub25lIn0." + payload + ".sig"
}

func TestCodexRoundTrip(t *testing.T) {
	e := testutil.NewEnv(t, "windows")
	auth := filepath.Join(e.Paths.Home, ".codex", "auth.json")
	write(t, auth, `{"OPENAI_API_KEY":null,"tokens":{"id_token":"`+fakeIDToken("w@corp.com")+`","access_token":"SECRET1"}}`)
	c := Codex{}
	w := e.Profile(t, "work")
	a, err := c.Import(ctx, e.Env, w)
	if err != nil || a.Label != "w@corp.com" {
		t.Fatalf("%v %v", a, err)
	}
	write(t, auth, `{"OPENAI_API_KEY":"sk-SECRET2"}`)
	h := e.Profile(t, "home")
	a, err = c.Import(ctx, e.Env, h)
	if err != nil || a.Label != "API key" {
		t.Fatalf("api key label: %v %v", a, err)
	}
	if _, err := c.Activate(ctx, e.Env, w, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, auth), "SECRET1") {
		t.Fatal("auth.json not swapped")
	}
	vars, _, err := c.SessionEnv(ctx, e.Env, h)
	if err != nil || !strings.HasPrefix(vars[0], "CODEX_HOME=") {
		t.Fatalf("%v %v", vars, err)
	}
	if !strings.Contains(read(t, filepath.Join(strings.TrimPrefix(vars[0], "CODEX_HOME="), "auth.json")), "SECRET2") {
		t.Fatal("session auth.json wrong")
	}
}

func TestCodexKeyringStoreWarning(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".codex", "config.toml"), "model = \"x\"\ncli_auth_credentials_store = \"keyring\"\n")
	st := (Codex{}).Status(ctx, e.Env)
	if !strings.Contains(st.Warning, "cli_auth_credentials_store") {
		t.Fatalf("warning = %q", st.Warning)
	}
	if _, err := (Codex{}).Import(ctx, e.Env, e.Profile(t, "w")); err == nil || !strings.Contains(err.Error(), "keyring") {
		t.Fatalf("err = %v", err)
	}
}

const ghHosts = `github.com:
    users:
        alice-work:
            git_protocol: https
        alice:
            git_protocol: ssh
    git_protocol: ssh
    user: alice
`

func TestGitHubSwitchUsesGhAuthSwitch(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	write(t, filepath.Join(e.Paths.Home, ".config", "gh", "hosts.yml"), ghHosts)
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

const glabCfg = `# glab config with a comment that must survive
git_protocol: ssh
host: gitlab.company.com
hosts:
    gitlab.com:
        token: glpat-HOME
        api_protocol: https
        user: me-home
    gitlab.company.com:
        token: glpat-WORK
        api_protocol: https
        user: me-work
`

func TestGitLabRoundTrip(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cfg := filepath.Join(e.Paths.Home, ".config", "glab-cli", "config.yml")
	write(t, cfg, glabCfg)
	g := GitLab{}
	w := e.Profile(t, "work")
	a, err := g.Import(ctx, e.Env, w)
	if err != nil || a.Label != "me-work" || a.Host != "gitlab.company.com" {
		t.Fatalf("%+v %v", a, err)
	}
	w.Accounts["gitlab"] = a

	// user rotates the token in glab then switches back to work
	write(t, cfg, strings.Replace(glabCfg, "glpat-WORK", "glpat-OTHER", 1))
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
	user := "[core]\n\tautocrlf = false\n[alias]\n\tst = status\n"
	once := ReplaceBlock(user, block)
	twice := ReplaceBlock(once, block)
	if once != twice {
		t.Fatal("ReplaceBlock is not idempotent")
	}
	if !strings.HasPrefix(once, user) {
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
	h.Rules = []config.Rule{{Kind: "dir", Pattern: filepath.Join(e.Paths.Home, "personal")}}
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
