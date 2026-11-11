package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/core"
)

func typeText(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestProtectedProfileAsksForPassword(t *testing.T) {
	md, mgr := newModel(t)
	if _, err := mgr.Import(ctxBG(), "work", []string{"claude"}, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Use(ctxBG(), "personal", core.UseOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetPassword(ctxBG(), "work", "", "work password", false); err != nil {
		t.Fatal(err)
	}
	mgr.Reseal()
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40}, statusMsg{list: mgr.StatusFor(ctxBG(), nil)})
	md = clickProfile(t, md, "work")

	screen := plain(md)
	if !strings.Contains(screen, "password protected") || strings.Contains(screen, "w@co") || strings.Contains(screen, "day job") {
		t.Fatalf("protected profile shows its data:\n%s", screen)
	}

	md = send(t, md, key("enter"))
	if md.modal != modalForm || !strings.Contains(plain(md), "Password for work") {
		t.Fatalf("no password window:\n%s", plain(md))
	}
	if strings.Contains(plain(md), "nope nope") {
		t.Fatal("password is echoed")
	}
	md = send(t, md, typeText("nope nope"), key("enter"))
	if md.modal != modalNone || len(md.toasts) == 0 || !strings.Contains(md.toasts[len(md.toasts)-1].text, "wrong password") || !mgr.NeedsPassword("work") {
		t.Fatalf("wrong password accepted, modal %v toasts %+v", md.modal, md.toasts)
	}

	md = send(t, md, key("enter"), typeText("work password"), key("enter"))
	if md.modal != modalConfirm || md.pend.kind != actSwitch {
		t.Fatalf("right password should preview the switch, modal %v", md.modal)
	}
	md = send(t, md, key("y"))
	if mgr.Cfg.Active != "work" {
		t.Fatalf("switch did not run, active %q", mgr.Cfg.Active)
	}
	if !mgr.NeedsPassword("work") {
		t.Fatal("the password is remembered after the switch")
	}
}

func TestPasswordWindowEscForgets(t *testing.T) {
	md, mgr := newModel(t)
	mgr.SetPassword(ctxBG(), "work", "", "work password", false)
	mgr.Reseal()
	md = send(t, md, tea.WindowSizeMsg{Width: 120, Height: 40})
	md = clickProfile(t, md, "work")
	md = send(t, md, key("e"))
	if md.modal != modalForm || md.fdata.kind != formPassword {
		t.Fatal("edit of a protected profile did not ask for the password")
	}
	md = send(t, md, key("esc"))
	if md.modal != modalNone || md.afterPw != nil {
		t.Fatal("esc left the password window or its action")
	}
	md = send(t, md, key("e"), typeText("work password"), key("enter"))
	if md.modal != modalForm || md.fdata.kind != formEditProfile || md.fdata.gEmail != "w@co" {
		t.Fatalf("the edit form should open with the profile's data, got %+v", md.fdata)
	}
}
