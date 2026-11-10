package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/testutil"
	"github.com/gdamigos/cream/internal/vault"
)

type harness struct {
	t    *testing.T
	home string
	p    paths.Paths
	v    *vault.Memory
	r    *testutil.FakeRunner
}

func newHarness(t *testing.T) *harness {
	home := t.TempDir()
	t.Setenv("CREAM_NO_UPDATE_CHECK", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GLAB_CONFIG_DIR", filepath.Join(home, "glab"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	return &harness{t: t, home: home, p: paths.Paths{Home: home, CreamHome: filepath.Join(home, "cream")}, v: vault.NewMemory(), r: testutil.NewFakeRunner()}
}

func (h *harness) run(args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	a := &App{
		Build: BuildInfo{Version: "v0.0.0-test", Commit: "abc", Date: "today"},
		Out:   &out, Err: &errb,
		Open: func() (*core.Manager, error) {
			m, err := core.Open(core.Options{Paths: &h.p, Vault: h.v, Runner: h.r, KDFWorkFactor: 10})
			if m != nil {
				m.Env.GOOS = "linux" // file based logins on every host, macos would use the keychain
			}
			return m, err
		},
	}
	code := Execute(context.Background(), a, args)
	return out.String(), errb.String(), code
}

func (h *harness) write(rel, s string) {
	p := filepath.Join(h.home, rel)
	os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) read(rel string) string {
	b, _ := os.ReadFile(filepath.Join(h.home, rel))
	return string(b)
}

func TestEndToEndWorkflow(t *testing.T) {
	h := newHarness(t)
	must := func(args ...string) string {
		out, errs, code := h.run(args...)
		if code != 0 {
			t.Fatalf("cream %v: exit %d\n%s\n%s", args, code, out, errs)
		}
		return out
	}
	must("profile", "add", "work", "--git-name", "Me Work", "--git-email", "me@work.com")
	must("profile", "add", "personal", "--git-name", "Me", "--git-email", "me@home.org")
	if _, _, code := h.run("profile", "add", "Bad Name"); code == 0 {
		t.Fatal("invalid name accepted")
	}

	// log in as work
	h.write(".claude/.credentials.json", `{"tok":"WORK-TOKEN"}`)
	h.write(".claude.json", `{"oauthAccount":{"emailAddress":"me@work.com"}}`)
	h.write(".codex/auth.json", `{"OPENAI_API_KEY":"sk-WORKKEY"}`)
	out := must("import", "work")
	if !strings.Contains(out, "me@work.com") {
		t.Fatalf("import output:\n%s", out)
	}
	// log in as personal
	h.write(".claude/.credentials.json", `{"tok":"HOME-TOKEN"}`)
	h.write(".claude.json", `{"oauthAccount":{"emailAddress":"me@home.org"}}`)
	must("import", "personal", "--only", "claude")

	// dry run changes nothing
	out = must("use", "work", "--dry-run")
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, ".credentials.json") {
		t.Fatalf("dry-run output:\n%s", out)
	}
	if !strings.Contains(h.read(".claude/.credentials.json"), "HOME-TOKEN") {
		t.Fatal("dry run changed files")
	}

	out = must("use", "work")
	if !strings.Contains(h.read(".claude/.credentials.json"), "WORK-TOKEN") {
		t.Fatal("use did not switch")
	}
	for _, secret := range []string{"WORK-TOKEN", "HOME-TOKEN", "sk-WORKKEY"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret %s printed", secret)
		}
	}

	js := must("status", "--json")
	var st struct {
		Active   string `json:"active"`
		Adapters []struct {
			Adapter, Account string
			Match            bool
		} `json:"adapters"`
	}
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		t.Fatal(err, js)
	}
	if st.Active != "work" || st.Adapters[0].Account != "me@work.com" || !st.Adapters[0].Match {
		t.Fatalf("status = %+v", st)
	}

	// with no rules yet the switch made work the global git identity
	before := h.read(".gitconfig")
	if !strings.Contains(before, "me@work.com") {
		t.Fatalf("use did not switch git identity:\n%s", before)
	}

	// identity rules
	must("rule", "add", "work", "--remote", "gitlab.company.com", "--dry-run")
	if h.read(".gitconfig") != before {
		t.Fatal("dry-run rule wrote gitconfig")
	}
	must("rule", "add", "work", "--remote", "gitlab.company.com")
	if !strings.Contains(h.read(".gitconfig"), "gitlab.company.com") {
		t.Fatal("rule not written")
	}
	if !strings.Contains(must("rule", "list"), "gitlab.company.com") {
		t.Fatal("rule list")
	}
	must("rule", "rm", "work", "1")

	// restore
	must("restore")
	if !strings.Contains(h.read(".claude/.credentials.json"), "HOME-TOKEN") {
		t.Fatal("restore did not roll back")
	}
	if !strings.Contains(must("backups"), "switch-to-work") {
		t.Fatal("backups list")
	}

	if _, _, code := h.run("profile", "rm", "personal"); code == 0 {
		t.Fatal("remove without --yes")
	}
	must("profile", "rm", "personal", "--yes")
	if _, err := h.v.Get(vault.ProfileKey("personal", "claude")); err == nil {
		t.Fatal("vault entries survived profile removal")
	}

	// nothing secret ever reaches config.toml
	cfg := h.read("cream/config.toml")
	for _, secret := range []string{"WORK-TOKEN", "HOME-TOKEN", "sk-WORKKEY"} {
		if strings.Contains(cfg, secret) {
			t.Fatalf("secret in config.toml")
		}
	}
}

func TestVersionAndUnknownProfile(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run("version")
	if code != 0 || !strings.Contains(out, "v0.0.0-test") {
		t.Fatalf("version: %q", out)
	}
	_, errs, code := h.run("use", "ghost")
	if code == 0 || !strings.Contains(errs, "unknown profile") {
		t.Fatalf("errs=%q", errs)
	}
}

func TestRunDryRun(t *testing.T) {
	h := newHarness(t)
	h.run("profile", "add", "work")
	out, _, code := h.run("run", "work", "--dry-run", "claude", "--resume")
	// flags after the command belong to the child, but --dry-run before it is ours
	if code != 0 {
		out2, errs, _ := h.run("--dry-run", "run", "work", "claude", "--resume")
		if !strings.Contains(out2, "exec") {
			t.Fatalf("run dry: %s %s %s", out, out2, errs)
		}
		return
	}
	if !strings.Contains(out, "exec") {
		t.Fatalf("run dry: %s", out)
	}
}
