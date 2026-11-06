package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/paths"
	"github.com/gdamigos/cream/internal/testutil"
	"github.com/gdamigos/cream/internal/vault"
)

func fmtType(v any) string { return fmt.Sprintf("%T", v) }

func ctxBG() context.Context { return context.Background() }

func newModel(t *testing.T) (Model, *core.Manager) {
	t.Helper()
	home := t.TempDir()
	p := paths.Paths{Home: home, CreamHome: filepath.Join(home, "cream")}
	m, err := core.Open(core.Options{Paths: &p, Vault: vault.NewMemory(), Runner: testutil.NewFakeRunner(),
		Adapters: []adapter.Adapter{adapters.Claude{}, adapters.Codex{}}})
	if err != nil {
		t.Fatal(err)
	}
	m.Env.Getenv = func(string) string { return "" }
	m.Env.GOOS = "linux" // file based logins on every host, macos would use the keychain
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"t":"SECRET"}`), 0o600)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"me@work.com"}}`), 0o600)
	m.AddProfile("work", "day job", config.GitIdentity{Name: "W", Email: "w@co"})
	m.AddProfile("personal", "", config.GitIdentity{})
	md := New(m, Options{Version: "test", SkipIntro: true})
	return md, m
}

func send(t *testing.T, md Model, msgs ...tea.Msg) Model {
	t.Helper()
	var cur tea.Model = md
	for _, msg := range msgs {
		var cmd tea.Cmd
		cur, cmd = cur.Update(msg)
		cur = drain(t, cur, cmd, 0)
	}
	return cur.(Model)
}

// runs commands synchronously and skips timers so async ops finish
func drain(t *testing.T, cur tea.Model, cmd tea.Cmd, depth int) tea.Model {
	if cmd == nil || depth > 50 {
		return cur
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(150 * time.Millisecond):
		return cur // timer tick
	}
	switch m := msg.(type) {
	case nil:
		return cur
	case tea.BatchMsg:
		for _, c := range m {
			cur = drain(t, cur, c, depth+1)
		}
		return cur
	case frameMsg:
		return cur
	}
	if strings.Contains(strings.ToLower(typeName(msg)), "exec") {
		return cur
	}
	next, c := cur.Update(msg)
	return drain(t, next, c, depth+1)
}

func typeName(v any) string { return strings.TrimPrefix(strings.TrimPrefix(fmtType(v), "*"), "tui.") }

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func plain(md Model) string { return ansi.Strip(md.View()) }
