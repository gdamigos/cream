package tui

import "strings"

func (m Model) viewConfirm() string {
	if m.pend == nil {
		return ""
	}
	lines := []string{sAccent.Render(m.pend.title), ""}
	for _, l := range m.pend.lines {
		switch {
		case strings.HasPrefix(l, "✓"), strings.HasPrefix(l, "●"):
			lines = append(lines, sOK.Render(l[:len("✓")])+sBase.Render(l[len("✓"):]))
		case strings.HasPrefix(l, "✗"):
			lines = append(lines, sErr.Render(l))
		case strings.HasPrefix(l, "○"):
			lines = append(lines, sFaint.Render(l))
		default:
			lines = append(lines, sDim.Render(l))
		}
	}
	lines = append(lines, "", sKey.Render("y")+sKeyDesc.Render(" confirm   ")+sKey.Render("n")+sKeyDesc.Render(" cancel"))
	return sModal.Render(strings.Join(lines, "\n"))
}

func (m Model) viewRun() string {
	p := m.selected()
	name := ""
	if p != nil {
		name = p.Name
	}
	lines := []string{sAccent.Render("Run as " + name), sDim.Render("this terminal only · global state untouched"), ""}
	for i, c := range m.runOpts {
		if i == m.runCursor {
			lines = append(lines, sHi.Render("▸ ")+sBold.Render(c.label))
		} else {
			lines = append(lines, "  "+sBase.Render(c.label))
		}
	}
	lines = append(lines, "", sKey.Render("enter")+sKeyDesc.Render(" launch   ")+sKey.Render("esc")+sKeyDesc.Render(" cancel"))
	return sModal.Render(strings.Join(lines, "\n"))
}

func (m Model) viewHelp() string {
	rows := [][2]string{
		{"↑/↓  j/k", "select profile"},
		{"enter", "switch every tool to the profile (global)"},
		{"→", "on Status: pick one tool, then enter or i act on it only"},
		{"r", "run one tool as the profile, this terminal only"},
		{"i", "import the currently logged-in accounts"},
		{"n / e / x", "new / edit / delete profile"},
		{"x on a tool", "forget the account the profile saved for it"},
		{"a", "add a git identity rule (folder or remote host)"},
		{"L / ctrl+l", "lock or unlock the profile / everything"},
		{"U", "how to update, when a new version is out"},
		{"u", "undo: restore the latest backup"},
		{"tab  1-3", "status · rules · backups"},
		{"→ / ←", "move between profiles and the list on the right"},
		{"D", "toggle dry-run (preview changes only)"},
		{"R", "refresh now (also automatic every few seconds)"},
		{"q", "quit"},
	}
	lines := []string{sAccent.Render("Keys"), ""}
	for _, r := range rows {
		lines = append(lines, sKey.Render(pad(r[0], 12))+sBase.Render(r[1]))
	}
	lines = append(lines, "", sFaint.Render("Every action is also a CLI command: cream --help"))
	return sModal.Render(strings.Join(lines, "\n"))
}

func (m Model) viewResult() string {
	r := m.result
	if r == nil {
		return ""
	}
	var lines []string
	switch {
	case r.err != nil:
		lines = append(lines, sErr.Render("✗ "+firstLine(r.err.Error())))
	case r.dry:
		lines = append(lines, sWarn.Render("DRY RUN · ")+sAccent.Render(r.title), sDim.Render("nothing was changed; this is what would happen:"))
	default:
		lines = append(lines, sOK.Render("✓ ")+sAccent.Render(r.title))
	}
	if len(r.events) > 0 {
		lines = append(lines, "")
		for _, e := range r.events {
			lines = append(lines, stepIcon(e.State, "·")+" "+sBold.Render(pad(e.Name, 14))+sDim.Render(truncate(e.Msg, 60)))
		}
	}
	if len(r.changes) > 0 {
		lines = append(lines, "")
		for _, c := range r.changes {
			lines = append(lines, sHi.Render("→ ")+sBold.Render(pad(c.Action, 8))+sFaint.Render(pad(c.Kind, 10))+sBase.Render(truncate(c.Target, 70)))
		}
	}
	lines = append(lines, "", sFaint.Render("enter to close"))
	return sModal.Render(strings.Join(lines, "\n"))
}
