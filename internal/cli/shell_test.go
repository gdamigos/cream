package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/config"
)

func TestHookScripts(t *testing.T) {
	h := newHarness(t)
	for shell, want := range map[string]string{
		"bash": `gh() { command cream hook exec gh "$@"; }`,
		"zsh":  `claude() { command cream hook exec claude "$@"; }`,
		"fish": `function glab --wraps glab; command cream hook exec glab $argv; end`,
		"pwsh": `function global:codex {`,
	} {
		out, errs, code := h.run("hook", shell)
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("%s: %d %s\n%s", shell, code, errs, out)
		}
		for _, tool := range hookTools {
			if !strings.Contains(out, "hook exec "+tool) {
				t.Fatalf("%s: no wrapper for %s", shell, tool)
			}
		}
	}
	if _, errs, code := h.run("hook", "tcsh"); code == 0 || !strings.Contains(errs, "use bash") {
		t.Fatalf("unknown shell: %d %s", code, errs)
	}
}

func TestHookExecFollowsFolder(t *testing.T) {
	h := newHarness(t)
	t.Setenv("CREAM_PROFILE", "")
	h.run("profile", "add", "work")
	h.write("proj/sub/keep", "")
	t.Chdir(filepath.Join(h.home, "proj"))
	h.run("init", "work")
	t.Chdir(filepath.Join(h.home, "proj", "sub"))

	out, errs, code := h.run("--dry-run", "hook", "exec", "claude", "--resume")
	if code != 0 || !strings.Contains(out, "exec    claude --resume") {
		t.Fatalf("in folder: %d %s %s", code, out, errs)
	}

	// outside a .cream folder, or inside a cream run session, the tool runs as usual
	t.Chdir(h.home)
	if out, errs, code := h.run("--dry-run", "hook", "exec", "git", "--version"); code != 0 || strings.Contains(out, "exec") {
		t.Fatalf("outside folder: %d %s %s", code, out, errs)
	}
	t.Chdir(filepath.Join(h.home, "proj"))
	t.Setenv("CREAM_PROFILE", "work")
	if out, errs, code := h.run("--dry-run", "hook", "exec", "git", "--version"); code != 0 || strings.Contains(out, "exec") {
		t.Fatalf("in session: %d %s %s", code, out, errs)
	}

	t.Setenv("CREAM_PROFILE", "")
	h.run("use", "work")
	if out, errs, code := h.run("--dry-run", "hook", "exec", "git", "--version"); code != 0 || strings.Contains(out, "exec") {
		t.Fatalf("folder profile active: %d %s %s", code, out, errs)
	}

	h.write("proj/.cream", "profile = ")
	if _, errs, code := h.run("hook", "exec", "git", "--version"); code == 0 || !strings.Contains(errs, ".cream") {
		t.Fatalf("broken .cream: %d %s", code, errs)
	}
}

func TestPromptText(t *testing.T) {
	cfg := config.New()
	work := &config.Folder{Profile: "work"}
	for _, c := range []struct {
		active, session string
		f               *config.Folder
		want            string
	}{
		{"", "", nil, ""},
		{"", "", work, ""},
		{"work", "", nil, "work"},
		{"work", "", work, "work"},
		{"personal", "", work, "personal*"},
		{"personal", "work", work, "work"},
		{"work", "personal", nil, "personal"},
	} {
		cfg.Active = c.active
		if got := promptText(cfg, c.f, c.session); got != c.want {
			t.Errorf("active=%q session=%q folder=%v: %q, want %q", c.active, c.session, c.f != nil, got, c.want)
		}
	}
}

func TestPromptCmd(t *testing.T) {
	h := newHarness(t)
	t.Setenv("CREAM_HOME", h.p.CreamHome)
	t.Setenv("CREAM_PROFILE", "")
	h.run("profile", "add", "work")
	h.run("profile", "add", "personal")
	h.run("use", "personal")
	t.Chdir(h.home)
	h.run("init", "work")
	if out, errs, _ := h.run("prompt"); out != "personal*\n" {
		t.Fatalf("prompt = %q %s", out, errs)
	}
}
