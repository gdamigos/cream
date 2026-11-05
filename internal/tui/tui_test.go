package tui

import (
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

func TestRendersAtManySizes(t *testing.T) {
	md, _ := newModel(t)
	for _, sz := range [][2]int{{40, 10}, {64, 20}, {100, 30}, {200, 60}} {
		m := send(t, md, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		v := m.View()
		if v == "" {
			t.Fatalf("empty view at %v", sz)
		}
		lines := strings.Split(v, "\n")
		if len(lines) > sz[1] {
			t.Errorf("%v: view has %d lines, taller than terminal", sz, len(lines))
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > sz[0] {
				t.Errorf("%v: line wider than terminal (%d): %q", sz, w, ansi.Strip(l))
				break
			}
		}
	}
}

func TestIntroThenMain(t *testing.T) {
	_, mgr := newModel(t)
	md := New(mgr, Options{Version: "test"})
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})
	if !md.intro {
		t.Fatal("intro should be showing")
	}
	md = send(t, md, key("x"))
	if md.intro {
		t.Fatal("key should skip intro")
	}
	if !strings.Contains(plain(md), "Profiles") {
		t.Fatal("main view missing")
	}
}

func TestStatusAndImportFlow(t *testing.T) {
	md, mgr := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg(mgr.StatusFor(ctxBG(), nil)))
	v := plain(md)
	if !strings.Contains(v, "me@work.com") || !strings.Contains(v, "personal") {
		t.Fatalf("status view:\n%s", v)
	}
	if strings.Contains(v, "SECRET") {
		t.Fatal("secret rendered in TUI")
	}
	// cursor starts on personal since it sorts first
	md = send(t, md, key("i"))
	if md.modal != modalConfirm {
		t.Fatal("import should ask for confirmation")
	}
	md = send(t, md, key("y"))
	if md.busy {
		t.Fatal("import did not finish")
	}
	if mgr.Cfg.Profiles["personal"].Accounts["claude"] == nil {
		t.Fatal("import did not save the account")
	}
}

func TestDryRunToggleAndSwitch(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})
	md = send(t, md, key("D"))
	if !md.dryRun || !strings.Contains(plain(md), "DRY RUN") {
		t.Fatal("dry-run badge missing")
	}
	// in dry run enter shows a result modal
	md = send(t, md, key("down"), key("enter"))
	if md.modal != modalResult || md.result == nil || !md.result.dry {
		t.Fatalf("expected dry-run result modal, modal=%v", md.modal)
	}
	if !strings.Contains(plain(md), "would happen") {
		t.Fatal("dry-run modal text missing")
	}
	md = send(t, md, key("enter"))
	if md.modal != modalNone {
		t.Fatal("modal did not close")
	}
}

func TestSwitchOneTool(t *testing.T) {
	md, mgr := newModel(t)
	home := mgr.Paths.Home
	cred := filepath.Join(home, ".claude", ".credentials.json")
	if _, err := mgr.Import(ctxBG(), "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cred, []byte(`{"t":"HOME"}`), 0o600)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"me@home.org"}}`), 0o600)
	if _, err := mgr.Import(ctxBG(), "personal", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg(mgr.StatusFor(ctxBG(), nil)))

	// select work, focus the status list, claude is the first row
	md = send(t, md, key("down"), key("l"))
	if !strings.Contains(plain(md), "switch only Claude Code to work") {
		t.Fatalf("tool hint missing:\n%s", plain(md))
	}
	md = send(t, md, key("enter"))
	if md.modal != modalConfirm || md.pend.title != "Switch Claude Code to work?" || len(md.pend.only) != 1 {
		t.Fatalf("expected a one tool confirm, got modal=%v pend=%+v", md.modal, md.pend)
	}
	md = send(t, md, key("y"))
	if len(md.steps) != 1 || md.steps[0].id != "claude" {
		t.Fatalf("steps = %+v", md.steps)
	}
	b, _ := os.ReadFile(cred)
	if !strings.Contains(string(b), "SECRET") {
		t.Fatalf("claude was not switched back to work: %s", b)
	}

	// i on a focused tool only imports that tool
	md = send(t, md, key("i"))
	if md.pend == nil || len(md.pend.only) != 1 || !strings.Contains(md.pend.title, "Claude Code") {
		t.Fatalf("import should be for one tool, got %+v", md.pend)
	}
}

func TestNewProfileFormAndHelp(t *testing.T) {
	md, _ := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, key("n"))
	if md.modal != modalForm || !strings.Contains(plain(md), "Profile name") {
		t.Fatal("form not shown")
	}
	md = send(t, md, key("esc"))
	if md.modal != modalNone {
		t.Fatal("esc should close the form")
	}
	md = send(t, md, key("?"))
	if md.modal != modalHelp || !strings.Contains(plain(md), "Keys") {
		t.Fatal("help not shown")
	}
	md = send(t, md, key("x"))
	md = send(t, md, key("tab"), key("tab"))
	if md.tab != tabBackups {
		t.Fatal("tab cycling")
	}
}

func TestGradientHelpers(t *testing.T) {
	out := paintGradient(smallLogo, 0.3, 0.5)
	if ansi.StringWidth(strings.Split(out, "\n")[0]) != ansi.StringWidth(smallLogo[0]) {
		t.Fatal("gradient changed the logo width")
	}
	if len(bigLogoLines()) != 12 {
		t.Fatal("embedded logo missing")
	}
}
