package adapters

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/fsutil"
	"github.com/gdamigos/cream/internal/testutil"
)

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
	write(t, filepath.Join(e.Paths.Home, ".codex", "config.toml"), fixture(t, "codex/config_keyring_store.toml"))
	st := (Codex{}).Status(ctx, e.Env)
	if !strings.Contains(st.Warning, "cli_auth_credentials_store") {
		t.Fatalf("warning = %q", st.Warning)
	}
	if _, err := (Codex{}).Import(ctx, e.Env, e.Profile(t, "w")); err == nil || !strings.Contains(err.Error(), "keyring") {
		t.Fatalf("err = %v", err)
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

// auth.json as Codex writes it for a ChatGPT login and for an API key
func TestCodexRealFiles(t *testing.T) {
	for file, label := range map[string]string{"codex/auth_chatgpt.json": "dev@work.example.com", "codex/auth_apikey.json": "API key"} {
		e := testutil.NewEnv(t, "linux")
		auth := filepath.Join(e.Paths.Home, ".codex", "auth.json")
		write(t, filepath.Join(e.Paths.Home, ".codex", "config.toml"), fixture(t, "codex/config_file_store.toml"))
		write(t, auth, fixture(t, file))
		c := Codex{}
		if st := c.Status(ctx, e.Env); !st.LoggedIn || st.Account != label || st.Warning != "" {
			t.Fatalf("%s: status = %+v", file, st)
		}
		p := e.Profile(t, "work")
		if _, err := c.Import(ctx, e.Env, p); err != nil {
			t.Fatal(err)
		}
		write(t, auth, `{"OPENAI_API_KEY":"sk-OTHER"}`)
		if _, err := c.Activate(ctx, e.Env, p, false); err != nil {
			t.Fatal(err)
		}
		if read(t, auth) != fixture(t, file) {
			t.Fatalf("%s: auth.json not restored byte for byte", file)
		}
	}
}
