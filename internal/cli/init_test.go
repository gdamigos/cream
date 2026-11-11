package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInitThenUseAndRunFromFolder(t *testing.T) {
	h := newHarness(t)
	h.run("profile", "add", "work")
	h.run("profile", "add", "personal")
	dir := filepath.Join(h.home, "proj")
	h.write("proj/sub/keep", "")
	t.Chdir(filepath.Join(dir, "sub"))

	if _, errs, code := h.run("use"); code == 0 || !strings.Contains(errs, "cream init") {
		t.Fatalf("use with no .cream: %d %s", code, errs)
	}

	t.Chdir(dir)
	if _, errs, code := h.run("init", "ghost"); code == 0 || !strings.Contains(errs, "unknown profile") {
		t.Fatalf("init ghost: %d %s", code, errs)
	}
	if out, errs, code := h.run("init", "work", "--only", "codex", "--dry-run"); code != 0 || !strings.Contains(out, "write") || h.read("proj/.cream") != "" {
		t.Fatalf("init dry run: %d %s %s", code, out, errs)
	}
	if _, errs, code := h.run("init", "work", "--only", "codex"); code != 0 {
		t.Fatalf("init: %s", errs)
	}
	if !strings.Contains(h.read("proj/.cream"), `profile = "work"`) {
		t.Fatalf(".cream = %q", h.read("proj/.cream"))
	}
	if _, errs, code := h.run("init", "personal"); code == 0 || !strings.Contains(errs, "--force") {
		t.Fatalf("overwrite without --force: %s", errs)
	}

	t.Chdir(filepath.Join(dir, "sub"))
	out, errs, code := h.run("use", "--dry-run")
	if code != 0 || !strings.Contains(out, "Switching to") || !strings.Contains(out, "work") || !strings.Contains(out, ".cream") {
		t.Fatalf("use from folder: %d %s %s", code, out, errs)
	}
	// a named profile still wins over the folder's
	if out, _, _ := h.run("use", "personal", "--dry-run"); !strings.Contains(out, "personal") {
		t.Fatalf("named profile: %s", out)
	}

	out, errs, code = h.run("--dry-run", "run", "claude", "--resume")
	if code != 0 || !strings.Contains(out, "exec") || !strings.Contains(out, "claude --resume") || !strings.Contains(errs, "work from") {
		t.Fatalf("run from folder: %d %s %s", code, out, errs)
	}
	// a lone profile name or anything after a leading -- is the command
	h.run("profile", "add", "gh")
	for _, args := range [][]string{{"run", "gh"}, {"run", "--", "gh", "pr", "list"}} {
		out, errs, code = h.run(append([]string{"--dry-run"}, args...)...)
		if code != 0 || !strings.Contains(out, "exec    gh") || !strings.Contains(errs, "work from") {
			t.Fatalf("%v: %d %s %s", args, code, out, errs)
		}
	}
	if out, _, _ := h.run("--dry-run", "run", "gh", "claude"); !strings.Contains(out, "exec    claude") {
		t.Fatalf("named profile first: %s", out)
	}

	t.Chdir(h.home)
	if _, errs, code := h.run("run", "work"); code == 0 || !strings.Contains(errs, "name the command") {
		t.Fatalf("run without command: %s", errs)
	}
	t.Chdir(filepath.Join(dir, "sub"))

	if out, _, _ := h.run("doctor"); !strings.Contains(out, "This folder") || !strings.Contains(out, "uses work") {
		t.Fatalf("doctor:\n%s", out)
	}
}
