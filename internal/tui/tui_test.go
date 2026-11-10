package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/vault"
)

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

func TestStartsOnFolderProfile(t *testing.T) {
	_, mgr := newModel(t)
	mgr.Cfg.Active = "work"
	md := New(mgr, Options{Version: "test", SkipIntro: true, Folder: &config.Folder{Profile: "personal"}})
	if md.selected().Name != "personal" {
		t.Fatalf("selected %s", md.selected().Name)
	}
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 30})
	if v := plain(md); !strings.Contains(v, "folder ● personal") {
		t.Fatalf("no folder in header:\n%s", v)
	}
}

func TestFolderKeyWritesCream(t *testing.T) {
	_, mgr := newModel(t)
	dir := t.TempDir()
	md := New(mgr, Options{Version: "test", SkipIntro: true, Dir: dir})
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 30})
	if md.selected().Name != "personal" {
		t.Fatalf("selected %s", md.selected().Name)
	}
	md = send(t, md, key("f"))
	if md.modal != modalConfirm {
		t.Fatalf("no confirm, modal %v", md.modal)
	}
	md = send(t, md, key("y"))
	f, err := config.FindFolder(dir)
	if err != nil || f == nil || f.Profile != "personal" {
		t.Fatalf("f=%v err=%v", f, err)
	}
	if md.folderProfile() != "personal" {
		t.Fatal("folder not shown after writing")
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
func TestIntroWaitsForInput(t *testing.T) {
	_, mgr := newModel(t)
	md := New(mgr, Options{Version: "test"})
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})
	at := func(sec float64) Model {
		md.start = time.Now().Add(-time.Duration(sec * float64(time.Second)))
		return send(t, md, frameMsg(time.Now()))
	}

	md = at(introHint + 0.2)
	v := plain(md)
	if !md.intro || !strings.Contains(v, "one profile switch for every CLI login you own") || !strings.Contains(v, "press any key") {
		t.Fatalf("bottom text missing:\n%s", v)
	}
	if strings.Contains(v, "still there") {
		t.Fatal("idle message shown too early")
	}

	md = at(introIdle + 3)
	if !md.intro || !strings.Contains(plain(md), "still there?...") {
		t.Fatalf("intro should still wait and ask:\n%s", plain(md))
	}

	if md = send(t, md, tea.MouseMsg{Action: tea.MouseActionMotion}); !md.intro {
		t.Fatal("moving the mouse skipped the intro")
	}
	if md = send(t, md, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}); md.intro {
		t.Fatal("a click should skip the intro")
	}
}

func TestStatusAndImportFlow(t *testing.T) {
	md, mgr := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
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
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})

	// select work, focus the status list, claude is the first row
	md = send(t, md, key("down"), key("l"))
	if !strings.Contains(plain(md), "switch this tool") {
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

// cream open in one terminal, claude logged out and in as another account in a second one
func TestLoginInOtherTerminal(t *testing.T) {
	md, mgr := newModel(t)
	home := mgr.Paths.Home
	if _, err := mgr.Import(ctxBG(), "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	md.statusLoads = 0

	// leaving the terminal saves work's refreshed token before it can be logged out
	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"t":"WORK2"}`), 0o600)
	md = send(t, md, tea.BlurMsg{})
	if b, _ := mgr.Env.Vault.Get(vault.ProfileKey("work", "claude")); !strings.Contains(string(b), "WORK2") {
		t.Fatal("blur did not sync the active profile")
	}

	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"t":"HOME"}`), 0o600)
	os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"me@home.org"}}`), 0o600)
	md = send(t, md, tea.FocusMsg{})
	if !strings.Contains(plain(md), "me@home.org") {
		t.Fatalf("new login not shown:\n%s", plain(md))
	}
	if len(md.toasts) == 0 || !strings.Contains(md.toasts[len(md.toasts)-1].text, "not saved yet") {
		t.Fatalf("expected a not saved toast, got %+v", md.toasts)
	}

	// personal is first, focus claude and import just that tool
	md = send(t, md, key("l"), key("i"), key("y"))
	if a := mgr.Cfg.Profiles["personal"].Accounts["claude"]; a == nil || a.Label != "me@home.org" {
		t.Fatalf("personal claude = %+v", a)
	}
	if b, _ := mgr.Env.Vault.Get(vault.ProfileKey("work", "claude")); !strings.Contains(string(b), "WORK2") {
		t.Fatal("work token was touched")
	}

	// x on the tool forgets it again, the profile stays
	md = send(t, md, key("enter"), key("x"))
	if md.modal != modalConfirm || md.pend.kind != actForget {
		t.Fatalf("expected forget confirm, got %+v", md.pend)
	}
	send(t, md, key("y"))
	if mgr.Cfg.Profiles["personal"] == nil || mgr.Cfg.Profiles["personal"].Accounts["claude"] != nil {
		t.Fatal("forget did not drop only the claude account")
	}
}

func TestLockAndUnlockKeys(t *testing.T) {
	md, mgr := newModel(t)
	if _, err := mgr.Import(ctxBG(), "work", nil, false, nil); err != nil {
		t.Fatal(err)
	}
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})

	// L on work locks just that profile
	md = send(t, md, key("down"), key("L"))
	if md.modal != modalConfirm || md.pend.kind != actLock || md.pend.profile != "work" {
		t.Fatalf("expected a lock confirm for work, got %+v", md.pend)
	}
	md = send(t, md, key("y"), key("enter"))
	if !mgr.Cfg.Locked("work") || mgr.Cfg.Lock.All {
		t.Fatalf("lock state = %+v", mgr.Cfg.Lock)
	}
	if !strings.Contains(plain(md), "locked") {
		t.Fatal("profile list doesn't show the lock")
	}
	// the switch preview is refused, so no confirm opens and the error shows as a toast
	md = send(t, md, key("enter"))
	if md.modal != modalNone || len(md.toasts) == 0 || !strings.Contains(md.toasts[len(md.toasts)-1].text, "locked") {
		t.Fatalf("switching to a locked profile should be refused, modal %v toasts %+v", md.modal, md.toasts)
	}

	// L again unlocks it
	md = send(t, md, key("L"))
	if md.pend == nil || md.pend.kind != actUnlock {
		t.Fatalf("expected an unlock confirm, got %+v", md.pend)
	}
	md = send(t, md, key("y"), key("enter"))
	if mgr.Cfg.AnyLocked() {
		t.Fatal("still locked")
	}

	// ctrl+l locks everything and shows the badge
	md = send(t, md, tea.KeyMsg{Type: tea.KeyCtrlL}, key("y"), key("enter"))
	if !mgr.Cfg.Lock.All || !strings.Contains(plain(md), "LOCKED") {
		t.Fatal("everything lock or its badge missing")
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
	// the whole logo is visible before the eased reveal reaches 1, so the end never crawls
	full := ansi.Strip(paintGradient(bigLogoLines(), 0, 1))
	if got := ansi.Strip(paintGradient(bigLogoLines(), 0, 0.9)); got != full {
		t.Fatal("reveal stalls on the last block")
	}
	if len(bigLogoLines()) != 12 {
		t.Fatal("embedded logo missing")
	}
}
