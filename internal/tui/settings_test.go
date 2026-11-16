package tui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsWindow(t *testing.T) {
	md, mgr := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 140, Height: 50})
	md = send(t, md, key("s"))
	if md.modal != modalForm || md.fdata.kind != formSettings {
		t.Fatal("s should open the settings form")
	}
	view := plain(md)
	for _, want := range []string{"Settings", "Adapters", "Codex", "update check", "Stale login warning"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settings window misses %q:\n%s", want, view)
		}
	}

	// turn claude and the update check off, then enter through the inputs to save
	md = send(t, md, key(" "), key("enter"), key(" "), key("enter"), key("enter"), key("enter"))
	if md.modal != modalNone {
		t.Fatalf("form still open:\n%s", plain(md))
	}
	if !slices.Equal(mgr.Cfg.Adapters, []string{"codex"}) || mgr.Cfg.UpdateCheckEnabled() {
		t.Fatalf("settings not applied: adapters %v update %v", mgr.Cfg.Adapters, mgr.Cfg.UpdateCheckEnabled())
	}
	b, _ := os.ReadFile(mgr.Paths.ConfigFile())
	if !strings.Contains(string(b), "update_check = false") {
		t.Fatalf("settings not saved:\n%s", b)
	}
	if len(mgr.Adapters) != 1 {
		t.Fatalf("claude still managed: %d tools", len(mgr.Adapters))
	}
}

func TestSettingsDryRun(t *testing.T) {
	md, mgr := newModel(t)
	md.dryRun = true
	md = send(t, md, tea.WindowSizeMsg{Width: 140, Height: 50})
	md = send(t, md, key("s"), key(" "), key("enter"), key("enter"), key("enter"), key("enter"))
	if md.modal != modalResult || !md.result.dry {
		t.Fatal("a dry run should show what it would change")
	}
	if len(mgr.Cfg.Adapters) != 2 {
		t.Fatal("a dry run changed the settings")
	}
}

func TestSettingsScrollsInSmallWindow(t *testing.T) {
	md, _ := newModel(t)
	md = send(t, md, tea.WindowSizeMsg{Width: 100, Height: 22}, key("s"))
	if h := lipgloss.Height(md.modalBox()); h > 22 {
		t.Fatalf("settings window is %d lines in a 22 line terminal", h)
	}
	if strings.Contains(plain(md), "Disabled keys") {
		t.Fatal("the last field should be scrolled out of view")
	}
	md = send(t, md, key("enter"), key("enter"), key("enter"))
	if !strings.Contains(plain(md), "Disabled keys") {
		t.Fatalf("the focused field didn't scroll into view:\n%s", plain(md))
	}

	// growing the terminal shows the whole form again
	md = send(t, md, tea.WindowSizeMsg{Width: 100, Height: 50})
	view := plain(md)
	if !strings.Contains(view, "Adapters") || !strings.Contains(view, "Disabled keys") {
		t.Fatalf("the form didn't grow back:\n%s", view)
	}
}
