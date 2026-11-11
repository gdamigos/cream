package cli

import (
	"strings"
	"testing"
)

func TestProfilePassword(t *testing.T) {
	h := newHarness(t)
	must := func(args ...string) string {
		t.Helper()
		out, errs, code := h.run(args...)
		if code != 0 {
			t.Fatalf("cream %v: exit %d\n%s\n%s", args, code, out, errs)
		}
		return out
	}
	must("profile", "add", "alice", "--git-name", "Alice", "--git-email", "alice@example.com")
	must("profile", "add", "bob")
	h.write(".claude/.credentials.json", `{"tok":"ALICE-TOKEN"}`)
	h.write(".claude.json", `{"oauthAccount":{"emailAddress":"alice@example.com"}}`)
	must("import", "alice", "--only", "claude")
	h.write(".claude/.credentials.json", `{"tok":"BOB-TOKEN"}`)
	h.write(".claude.json", `{"oauthAccount":{"emailAddress":"bob@example.com"}}`)
	must("import", "bob", "--only", "claude")

	t.Setenv("CREAM_PROFILE_PASSWORD", "alice password")
	must("profile", "password", "alice")
	t.Setenv("CREAM_PROFILE_PASSWORD", "")
	must("use", "bob") // no password needed to leave alice

	if strings.Contains(h.read("cream/config.toml"), "alice@example.com") {
		t.Fatal("config.toml shows alice's account")
	}
	if out := must("profile", "list"); !strings.Contains(out, "password protected") || strings.Contains(out, "alice@example.com") {
		t.Fatalf("profile list:\n%s", out)
	}
	if _, errs, code := h.run("use", "alice"); code == 0 || !strings.Contains(errs, "CREAM_PROFILE_PASSWORD") {
		t.Fatalf("use without a password: exit %d %s", code, errs)
	}
	t.Setenv("CREAM_PROFILE_PASSWORD", "not it at all")
	if _, errs, code := h.run("use", "alice"); code == 0 || !strings.Contains(errs, "wrong password") {
		t.Fatalf("use with a wrong password: exit %d %s", code, errs)
	}
	t.Setenv("CREAM_PROFILE_PASSWORD", "alice password")
	must("use", "alice")
	if !strings.Contains(h.read(".claude/.credentials.json"), "ALICE-TOKEN") {
		t.Fatal("use alice did not switch")
	}
	must("use", "bob")
	t.Setenv("CREAM_PROFILE_PASSWORD", "")
	must("profile", "remove", "alice", "--yes", "--forgot-password")
	if out := must("profile", "list"); strings.Contains(out, "alice") {
		t.Fatal("alice still listed")
	}
}
