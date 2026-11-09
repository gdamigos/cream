package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// a key hint clicked once, drawn boxed until a second click on it runs its key
type armedHint struct {
	text string
	x, y int
}

// with ui.click_twice the first click on a hint only arms it, prev is what was armed before this click
func (m Model) pressLabel(prev armedHint, l clickLabel, y int) (Model, bool) {
	if !m.mgr.Cfg.UI.ClickTwice {
		return m, true
	}
	hint := armedHint{l.text, l.x, y}
	if prev == hint {
		return m, true
	}
	m.armed = hint
	return m, false
}

// draws the armed hint boxed, only while it is still on screen where it was clicked
func (m Model) drawArmed(view string) string {
	a := m.armed
	if a.text == "" {
		return view
	}
	lines := strings.Split(view, "\n")
	if a.y < 0 || a.y >= len(lines) {
		return view
	}
	end := a.x + ansi.StringWidth(a.text)
	line := lines[a.y]
	if ansi.Strip(ansi.Cut(line, a.x, end)) != a.text {
		return view
	}
	lines[a.y] = ansi.Truncate(line, a.x, "") + ansi.ResetStyle + sArmed.Render(a.text) + ansi.TruncateLeft(line, end, "")
	return strings.Join(lines, "\n")
}
