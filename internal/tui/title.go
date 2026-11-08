package tui

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// the terminal tab title, animated with the spinner while an operation runs
func (m Model) windowTitle() string {
	if m.busy {
		f := m.spin.Spinner.Frames
		return f[m.frame*12/30%len(f)] + " cream · " + m.busyLabel
	}
	// the newest toast shows for as long as it is on screen
	if n := len(m.toasts); n > 0 {
		t := m.toasts[n-1]
		return [...]string{"●", "✓", "✗"}[t.kind] + " cream · " + t.text
	}
	if s := m.modalTitle(); s != "" {
		return "cream · " + s
	}
	parts := []string{"cream"}
	switch {
	case m.mgr.Cfg.Lock.All:
		parts = append(parts, "locked")
	case m.mgr.Cfg.Active != "":
		parts = append(parts, m.mgr.Cfg.Active)
	}
	if m.tab != tabStatus {
		parts = append(parts, strings.ToLower(tabNames[m.tab]))
	}
	t := strings.Join(parts, " · ")
	if m.dryRun {
		t += " (dry run)"
	}
	return t
}

func (m Model) modalTitle() string {
	switch m.modal {
	case modalConfirm:
		if m.pend != nil && m.pend.title != "" {
			return strings.ToLower(m.pend.title[:1]) + m.pend.title[1:]
		}
	case modalForm:
		if m.fdata != nil {
			switch m.fdata.kind {
			case formNewProfile:
				return "new profile"
			case formEditProfile:
				return "editing " + m.fdata.profile
			case formRule:
				return "new rule for " + m.fdata.profile
			}
		}
	case modalRun:
		return "run a tool"
	case modalHelp:
		return "help"
	case modalUpdate:
		return "update"
	case modalResult:
		if m.result != nil && m.result.dry {
			return "dry run of " + m.result.title
		}
		return "result"
	}
	return ""
}

// only sends the escape sequence when the title changed
func (m Model) syncTitle() (Model, tea.Cmd) {
	t := ""
	if m.mgr.Cfg.UI.DynamicTitle {
		t = m.windowTitle()
	}
	if t == m.title {
		return m, nil
	}
	m.title = t
	return m, tea.SetWindowTitle(t)
}

// an empty title hands the tab back to the shell
func resetTitle(m tea.Model) {
	if md, ok := m.(Model); ok && md.title != "" {
		_, _ = os.Stdout.WriteString(ansi.SetWindowTitle(""))
	}
}
