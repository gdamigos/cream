package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDisabledKeys(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Cfg.UI.DisabledKeys = []string{"x", "D"}
	md = send(t, md, tea.WindowSizeMsg{Width: 140, Height: 40})

	md = send(t, md, key("x"))
	if md.modal != modalNone {
		t.Fatal("disabled x still opened the delete confirm")
	}
	md = send(t, md, tea.KeyMsg{Type: tea.KeyDelete})
	if md.modal != modalNone {
		t.Fatal("delete, the alias of x, still worked")
	}
	md = send(t, md, key("D"))
	if md.dryRun {
		t.Fatal("disabled D still toggled dry run")
	}
	footer := plain(md)
	if strings.Contains(footer, "x delete") || strings.Contains(footer, "D dry-run") {
		t.Fatal("footer still shows disabled keys")
	}

	// windows keep their keys even when the same key is disabled on the main view
	mgr.Cfg.UI.DisabledKeys = []string{"n", "enter"}
	md = send(t, md, key("e"))
	if md.modal != modalForm {
		t.Fatal("e should still open the form")
	}
	md = send(t, md, key("esc"))
	md.pend, md.modal = &pending{kind: actDeleteRule, title: "t"}, modalConfirm
	md = send(t, md, key("n"))
	if md.modal != modalNone {
		t.Fatal("n should still cancel a confirm")
	}
}

func TestDynamicTitleOff(t *testing.T) {
	md, mgr := newModel(t)
	mgr.Cfg.UI.DynamicTitle = false
	if _, cmd := md.syncTitle(); cmd != nil {
		t.Fatal("title sent while dynamic_title is off")
	}
	mgr.Cfg.UI.DynamicTitle = true
	md, _ = md.syncTitle()
	mgr.Cfg.UI.DynamicTitle = false
	if md, cmd := md.syncTitle(); cmd == nil || md.title != "" {
		t.Fatal("turning dynamic_title off should hand the tab back")
	}
}
