package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/update"
)

// where text is drawn below the header, between columns minX and maxX, so clicks follow the real layout
func find(t *testing.T, md Model, text string, minX, maxX int) (int, int) {
	t.Helper()
	headerH := lipgloss.Height(md.viewHeader()) // the header repeats names like the active profile
	for y, line := range strings.Split(plain(md), "\n") {
		if y < headerH {
			continue
		}
		for from := 0; ; {
			i := strings.Index(line[from:], text)
			if i < 0 {
				break
			}
			if x := ansi.StringWidth(line[:from+i]); x >= minX && x < maxX {
				return x, y
			}
			from += i + len(text)
		}
	}
	t.Fatalf("%q not on screen:\n%s", text, plain(md))
	return 0, 0
}

func click(t *testing.T, md Model, x, y int) Model {
	t.Helper()
	return send(t, md, tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
}

func clickOn(t *testing.T, md Model, text string, minX int) Model {
	t.Helper()
	x, y := find(t, md, text, minX, md.w)
	return click(t, md, x+1, y) // inside the text, not on its first cell
}

// a key hint, the first click boxes it and the second runs it
func pressOn(t *testing.T, md Model, text string, minX int) Model {
	t.Helper()
	return clickOn(t, clickOn(t, md, text, minX), text, minX)
}

// a profile row in the left panel, names also show up in emails and headers elsewhere
func clickProfile(t *testing.T, md Model, name string) Model {
	t.Helper()
	x, y := find(t, md, name, 0, leftWidth(md.w))
	return click(t, md, x+1, y)
}

func TestClickProfilesAndTabs(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		md, mgr := newModel(t)
		mgr.Import(ctxBG(), "work", nil, false, nil)
		md = send(t, md, tea.WindowSizeMsg{Width: size[0], Height: size[1]}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
		right := leftWidth(size[0])

		md = clickProfile(t, md, "work")
		if md.cursor != 1 || md.focusRight || md.modal != modalNone {
			t.Fatalf("%v: first click should only select work, cursor %d modal %v", size, md.cursor, md.modal)
		}
		md = clickProfile(t, md, "personal")
		if md.cursor != 0 {
			t.Fatalf("%v: cursor = %d", size, md.cursor)
		}
		// the selected profile again is enter, which previews the switch
		md = clickProfile(t, md, "personal")
		if md.modal != modalConfirm || md.pend.kind != actSwitch || md.pend.profile != "personal" {
			t.Fatalf("%v: second click should preview the switch, got %+v", size, md.pend)
		}
		md = send(t, md, key("esc"))

		for name, want := range map[string]tab{"2 Rules": tabRules, "3 Backups": tabBackups, "1 Status": tabStatus} {
			if md = clickOn(t, md, name, right); md.tab != want {
				t.Fatalf("%v: clicking %q gave tab %v", size, name, md.tab)
			}
		}
	}
}

func TestClickToolRowAndConfirm(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	right := leftWidth(120)
	md = clickProfile(t, md, "work")

	md = clickOn(t, md, "Codex", right)
	if !md.focusRight || md.rightCursor[tabStatus] != 1 || md.modal != modalNone {
		t.Fatalf("first click should focus the Codex row, focus %v cursor %d", md.focusRight, md.rightCursor[tabStatus])
	}
	md = clickOn(t, md, "Codex", right)
	if md.modal != modalConfirm || md.pend.title != "Switch Codex to work?" {
		t.Fatalf("second click should preview switching only Codex, got %+v", md.pend)
	}

	// n cancels, a click outside the window cancels too
	if md = pressOn(t, md, "n cancel", 0); md.modal != modalNone {
		t.Fatal("clicking n cancel left the window open")
	}
	md = clickOn(t, md, "Codex", right)
	if md = click(t, md, 0, 0); md.modal != modalNone {
		t.Fatal("clicking outside the window left it open")
	}

	// y confirms and the switch runs
	md = clickOn(t, md, "Codex", right)
	md = pressOn(t, md, "y confirm", 0)
	if md.modal != modalNone || md.busy {
		t.Fatalf("y confirm did not run the switch, modal %v", md.modal)
	}
	if len(md.steps) != 1 || md.steps[0].id != "codex" {
		t.Fatalf("expected a Codex only switch, steps %+v", md.steps)
	}
}

func TestClickBackupRowAndWindows(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	if _, err := mgr.Use(ctxBG(), "work", core.UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	list, err := mgr.Backups.List()
	if err != nil || len(list) == 0 {
		t.Fatalf("no backup to click: %v", err)
	}
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, backupsMsg(list))
	right := leftWidth(120)

	md = clickOn(t, md, "3 Backups", right)
	md = clickOn(t, md, "switch-to-work", right)
	if !md.focusRight || md.rightCursor[tabBackups] != 0 {
		t.Fatal("backup row not selected")
	}
	if md = clickOn(t, md, "switch-to-work", right); md.modal != modalConfirm || md.pend.kind != actRestore {
		t.Fatalf("second click should ask to restore, got %+v", md.pend)
	}
	md = send(t, md, key("n"))

	// help closes on any click, the run window on a click outside it
	if md = click(t, send(t, md, key("?")), 60, 20); md.modal != modalNone {
		t.Fatal("help stayed open")
	}
	md = send(t, md, key("left"), key("r"))
	if md.modal != modalRun {
		t.Fatal("run window not open")
	}
	if md = click(t, md, 0, 0); md.modal != modalNone {
		t.Fatal("run window stayed open after a click outside it")
	}

	// while an operation runs nothing reacts
	md.busy = true
	before := md.cursor
	md = clickProfile(t, md, "personal")
	if md.cursor != before || md.modal != modalNone {
		t.Fatal("click acted while busy")
	}
}

// a key hint in the header or the footer, which find skips or would reach last, clicked twice
func pressEdge(t *testing.T, md Model, text string, footer bool) Model {
	t.Helper()
	lines := strings.Split(plain(md), "\n")
	from, to := 0, lipgloss.Height(md.viewHeader())
	if footer {
		from, to = len(lines)-lipgloss.Height(md.viewFooter()), len(lines)
	}
	for y := from; y < to; y++ {
		if i := strings.Index(lines[y], text); i >= 0 {
			x := ansi.StringWidth(lines[y][:i]) + 1
			return click(t, click(t, md, x, y), x, y)
		}
	}
	t.Fatalf("%q not in the header or footer:\n%s", text, plain(md))
	return md
}

func TestClickTwiceOnHints(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	md = clickProfile(t, md, "work")
	md = send(t, md, key("enter"))
	if md.modal != modalConfirm {
		t.Fatal("switch preview not open")
	}

	// the first click only boxes the hint
	md = clickOn(t, md, "n cancel", 0)
	if md.modal != modalConfirm || md.armed.text != "n cancel" {
		t.Fatalf("first click should only arm n cancel, modal %v armed %+v", md.modal, md.armed)
	}
	if !strings.Contains(md.View(), sArmed.Render("n cancel")) {
		t.Fatal("armed hint is not drawn boxed")
	}
	// another hint moves the box, a key press clears it
	if md = clickOn(t, md, "y confirm", 0); md.armed.text != "y confirm" || md.modal != modalConfirm {
		t.Fatalf("clicking y confirm should move the box, armed %+v", md.armed)
	}
	if md = send(t, md, key("down")); md.armed.text != "" {
		t.Fatal("a key press left the hint armed")
	}
	// so the next click arms again instead of confirming
	if md = clickOn(t, md, "y confirm", 0); md.modal != modalConfirm {
		t.Fatal("a click after a key press confirmed right away")
	}
	if md = clickOn(t, md, "y confirm", 0); md.modal != modalNone || md.armed.text != "" {
		t.Fatalf("second click should confirm, modal %v", md.modal)
	}

	// with ui.click_twice off one click runs the key
	mgr.Cfg.UI.ClickTwice = false
	if md = send(t, md, key("L")); md.modal != modalConfirm {
		t.Fatal("lock confirm not open")
	}
	if md = clickOn(t, md, "n cancel", 0); md.modal != modalNone {
		t.Fatal("with click_twice off one click should cancel")
	}
}

func TestClickBackToProfiles(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Import(ctxBG(), "work", nil, false, nil)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	right := leftWidth(120)

	// the "→ pick one tool" hint focuses the tool list
	md = pressOn(t, md, "→ pick", right)
	if !md.focusRight {
		t.Fatal("hint did not focus the tool list")
	}
	// the panel title is empty space, a click there goes back to the profiles
	md = clickProfile(t, md, "Profiles")
	if md.focusRight {
		t.Fatal("clicking the profiles panel did not move the focus back")
	}
	// empty space in the tab panel focuses it again, the footer's ← profiles goes back
	md = click(t, md, right+10, md.h-4)
	if !md.focusRight {
		t.Fatal("clicking empty space in the tab panel did not focus it")
	}
	if md = pressEdge(t, md, "← profiles", true); md.focusRight {
		t.Fatal("footer ← profiles did not go back")
	}
}

func TestClickFooterHeaderAndWindows(t *testing.T) {
	_, mgr := newModel(t)
	md := New(mgr, Options{Version: "v0.1.3", SkipIntro: true, UpdateCheck: true})
	md = send(t, md, tea.WindowSizeMsg{Width: 140, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})

	if md = pressEdge(t, md, "L lock", true); md.modal != modalConfirm || md.pend.kind != actLock {
		t.Fatalf("footer L lock should ask to lock, got %+v", md.pend)
	}
	md = send(t, md, key("n"))
	if md = pressEdge(t, md, "? help", true); md.modal != modalHelp {
		t.Fatal("footer ? help did not open help")
	}
	md = click(t, md, 1, 1)

	// D in the footer turns dry run on, the DRY RUN badge turns it off again
	if md = pressEdge(t, md, "D dry-run", true); !md.dryRun {
		t.Fatal("footer D did not turn dry run on")
	}
	if md = pressEdge(t, md, "DRY RUN", false); md.dryRun {
		t.Fatal("the DRY RUN badge did not turn dry run off")
	}

	// the update badge opens the update window, its own keys are clickable
	md = send(t, md, updateMsg{&update.Result{Latest: "v0.2.0", Newer: true}})
	if md = pressEdge(t, md, "v0.2.0 · U", false); md.modal != modalUpdate {
		t.Fatal("update badge did not open the update window")
	}
	var opened string
	openURL = func(_, url string) error { opened = url; return nil }
	defer func() { openURL = openInBrowser }()
	if md = pressOn(t, md, "o open", 0); opened == "" || md.modal != modalNone {
		t.Fatalf("o open did nothing, opened %q", opened)
	}
	md = pressEdge(t, md, "v0.2.0 · U", false)
	if md = pressOn(t, md, "esc close", 0); md.modal != modalNone {
		t.Fatal("esc close left the update window open")
	}

	// the rules tab's own "a add" hint opens the rule form for the selected profile
	md = clickProfile(t, md, "work")
	md = clickOn(t, md, "2 Rules", leftWidth(140))
	if md = pressOn(t, md, "a add", leftWidth(140)); md.modal != modalForm {
		t.Fatal("a add did not open the rule form")
	}
	// a click outside the form's border closes it, like esc
	if md = click(t, md, 0, 0); md.modal != modalNone || md.form != nil {
		t.Fatal("a click outside the border left the form open")
	}
}

func TestClickFormFields(t *testing.T) {
	md, _ := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})
	md = clickProfile(t, md, "work")
	md = send(t, md, key("e"))
	if md.modal != modalForm || md.form.GetFocusedField() != md.formFields[0] {
		t.Fatal("edit form not open on its first field")
	}
	focusedAfter := func(title string) int {
		t.Helper()
		md = clickOn(t, md, title, 0)
		for i, f := range md.formFields {
			if f == md.form.GetFocusedField() {
				return i
			}
		}
		return -1
	}
	if got := focusedAfter("Signing key"); got != 3 {
		t.Fatalf("clicking Signing key focused field %d", got)
	}
	if got := focusedAfter("Git user.name"); got != 1 {
		t.Fatalf("clicking Git user.name focused field %d", got)
	}
	// typing goes into the clicked field
	md = send(t, md, key("x"))
	if md.fdata.gName != "Wx" {
		t.Fatalf("typed into the wrong field, git name = %q", md.fdata.gName)
	}
	if md.modal != modalForm {
		t.Fatal("moving between fields submitted or closed the form")
	}
}

func TestClickLiveAccountImports(t *testing.T) {
	md, mgr := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	right := leftWidth(120)
	if md.selected().Name != "personal" {
		t.Fatal("personal should be selected from the start, it sorts first")
	}

	md = clickOn(t, md, "me@work.com", right)
	if md.modal != modalConfirm || md.pend.kind != actImport || len(md.pend.only) != 1 || md.pend.only[0] != "claude" {
		t.Fatalf("clicking the live account should ask to import only Claude Code, got %+v", md.pend)
	}
	md = pressOn(t, md, "y confirm", 0)
	md = send(t, md, key("enter")) // close the result window
	if a := mgr.Cfg.Profiles["personal"].Accounts["claude"]; a == nil || a.Label != "me@work.com" {
		t.Fatalf("personal claude = %+v", a)
	}

	// a tool that isn't logged in only gets selected, there's nothing to import
	md = clickOn(t, md, "not installed", right)
	if md.modal != modalNone {
		t.Fatal("clicking a tool that isn't logged in opened a window")
	}
}
