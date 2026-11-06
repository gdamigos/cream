package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/testutil"
	"github.com/gdamigos/cream/internal/vault"
)

var ctx = context.Background()

// always fails activate to test rollback
type failing struct{ adapters.Codex }

func (failing) ID() string   { return "boom" }
func (failing) Name() string { return "Boom" }
func (failing) Activate(context.Context, *adapter.Env, *config.Profile, bool) ([]adapter.Change, error) {
	return nil, errors.New("simulated failure")
}

func newManager(t *testing.T, ads []adapter.Adapter) (*Manager, *testutil.FakeRunner) {
	t.Helper()
	home := t.TempDir()
	p := paths.Paths{Home: home, CreamHome: filepath.Join(home, ".config", "cream")}
	r := testutil.NewFakeRunner()
	m, err := Open(Options{Paths: &p, Vault: vault.NewMemory(), Runner: r, Adapters: ads})
	if err != nil {
		t.Fatal(err)
	}
	m.Env.Getenv = func(string) string { return "" }
	m.Env.GOOS = "linux"
	return m, r
}

func put(t *testing.T, path, s string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func loginClaude(t *testing.T, m *Manager, email, token string) {
	put(t, filepath.Join(m.Paths.Home, ".claude", ".credentials.json"), `{"tok":"`+token+`"}`)
	put(t, filepath.Join(m.Paths.Home, ".claude.json"), `{"oauthAccount":{"emailAddress":"`+email+`"}}`)
}
