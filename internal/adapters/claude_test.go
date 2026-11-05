package adapters

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/testutil"
)

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

// the files as Claude Code writes them: the saved login comes back byte for byte, everything else is the user's
func TestClaudeRealFiles(t *testing.T) {
	e := testutil.NewEnv(t, "linux")
	cred := filepath.Join(e.Paths.Home, ".claude", ".credentials.json")
	state := filepath.Join(e.Paths.Home, ".claude.json")
	write(t, cred, fixture(t, "claude/credentials.json"))
	write(t, state, fixture(t, "claude/claude.json"))
	c := Claude{}
	if st := c.Status(ctx, e.Env); !st.LoggedIn || st.Account != "dev@work.example.com" {
		t.Fatalf("status = %+v", st)
	}
	work := e.Profile(t, "work")
	acct, err := c.Import(ctx, e.Env, work)
	if err != nil || acct.Label != "dev@work.example.com" {
		t.Fatalf("import: %+v %v", acct, err)
	}
	work.Accounts["claude"] = acct

	// another account logs in, and Claude Code keeps updating its own settings
	write(t, cred, `{"claudeAiOauth":{"accessToken":"OTHER"}}`)
	var m map[string]any
	json.Unmarshal([]byte(read(t, state)), &m)
	m["oauthAccount"] = map[string]any{"emailAddress": "dev@home.example.com"}
	m["numStartups"] = 43
	b, _ := json.Marshal(m)
	write(t, state, string(b))

	// a dry run reports both files and touches neither
	if ch, err := c.Activate(ctx, e.Env, work, true); err != nil || len(ch) != 2 || !strings.Contains(read(t, cred), "OTHER") {
		t.Fatalf("dry run: %v %v", ch, err)
	}
	if _, err := c.Activate(ctx, e.Env, work, false); err != nil {
		t.Fatal(err)
	}
	if read(t, cred) != fixture(t, "claude/credentials.json") {
		t.Fatal("credentials not restored byte for byte")
	}
	var got, want map[string]any
	json.Unmarshal([]byte(read(t, state)), &got)
	json.Unmarshal([]byte(fixture(t, "claude/claude.json")), &want)
	if !reflect.DeepEqual(got["oauthAccount"], want["oauthAccount"]) {
		t.Fatalf("oauthAccount = %v", got["oauthAccount"])
	}
	if got["numStartups"].(float64) != 43 || !reflect.DeepEqual(got["projects"], want["projects"]) || got["userID"] != want["userID"] {
		t.Fatal("the user's own .claude.json settings were not kept")
	}
}
