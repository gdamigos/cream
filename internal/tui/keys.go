package tui

import (
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/update"
)

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalForm:
		return m.updateForm(k)
	case modalHelp:
		m.modal = modalNone
		return m, nil
	case modalUpdate:
		switch k.String() {
		case "c":
			text := m.inst.Command
			if text == "" {
				text = update.ReleaseURL(m.upd.Latest)
			}
			if err := copyToClipboard(text); err != nil {
				m.toast(2, "could not copy: %s", err.Error())
			} else {
				m.toast(1, "copied · quit with q, then paste it")
			}
			m.modal = modalNone
		case "o":
			if err := openURL(runtime.GOOS, update.ReleaseURL(m.upd.Latest)); err != nil {
				m.toast(2, "could not open the browser: %s", err.Error())
			} else {
				m.toast(1, "release page opened in your browser")
			}
			m.modal = modalNone
		case "esc", "q", "enter", "U":
			m.modal = modalNone
		}
		return m, nil
	case modalResult:
		switch k.String() {
		case "enter", "esc", "q", " ":
			m.modal, m.result = modalNone, nil
		}
		return m, nil
	case modalConfirm:
		if m.pend != nil && m.pend.kind == actQuit && k.String() == "q" {
			return m.syncActive(), tea.Quit
		}
		switch k.String() {
		case "y", "Y", "enter":
			p := *m.pend
			m.modal, m.pend = modalNone, nil
			next, cmd := m.execute(p)
			// quick actions are done here, long ones forget the password in opDoneMsg
			if md, ok := next.(Model); ok && !md.busy {
				m.mgr.Reseal()
			}
			return next, cmd
		case "n", "N", "esc", "q":
			m.modal, m.pend = modalNone, nil
			m.mgr.Reseal()
		}
		return m, nil
	case modalRun:
		switch k.String() {
		case "up", "k":
			m.runCursor = (m.runCursor - 1 + len(m.runOpts)) % len(m.runOpts)
		case "down", "j":
			m.runCursor = (m.runCursor + 1) % len(m.runOpts)
		case "esc", "q":
			m.modal = modalNone
			m.mgr.Reseal()
		case "enter":
			m.modal = modalNone
			return m.launch(m.runOpts[m.runCursor])
		}
		return m, nil
	}

	if m.keyDisabled(k.String()) {
		return m, nil
	}
	if m.busy {
		if k.String() == "q" {
			return m, tea.Quit
		}
		return m, nil
	}
	// nothing is waiting for a password here, so forget any entered before
	m.mgr.Reseal()

	sel := m.selected()
	switch k.String() {
	case "q":
		m.pend = &pending{kind: actQuit, title: "Quit cream?",
			lines: []string{"The active profile's refreshed logins are saved first.", "Press q again to quit."}}
		m.modal = modalConfirm
	case "esc":
		return m.syncActive(), tea.Quit
	case "?":
		m.modal = modalHelp
	case "up", "k":
		return m.move(-1), nil
	case "down", "j":
		return m.move(1), nil
	case "left", "h":
		m.focusRight = false
	case "right", "l":
		m.focusRight = true
	case "tab":
		m.tab = (m.tab + 1) % tabCount
	case "shift+tab":
		m.tab = (m.tab + tabCount - 1) % tabCount
	case "1", "2", "3":
		m.tab = tab(k.String()[0] - '1')
	case "D":
		m.dryRun = !m.dryRun
		if m.dryRun {
			m.toast(0, "dry-run on · actions only preview their changes")
		} else {
			m.toast(0, "dry-run off")
		}
	case "R":
		m, cmd := m.refresh()
		if cmd != nil {
			m.statusLoaded = false
		}
		return m, tea.Batch(cmd, m.loadBackups())
	case "n":
		return m.openProfileForm(formNewProfile, nil)
	case "e":
		if sel != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) { return m.openProfileForm(formEditProfile, sel) })
		}
	case "a":
		if sel != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) { return m.openRuleForm(sel) })
		}
	case "enter", " ":
		if m.focusRight && m.tab == tabBackups && len(m.backups) > 0 {
			b := m.backups[m.rightCursor[tabBackups]]
			return m.confirmRestore(b.ID)
		}
		if sel != nil {
			only := m.focusedTool()
			if only == nil && sel.Name == m.folderProfile() {
				only = m.opts.Folder.Only
			}
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) { return m.previewSwitch(sel.Name, only) })
		}
	case "i":
		if sel != nil {
			only := m.focusedTool()
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) { return m.confirmImport(sel, only) })
		}
	case "r":
		if sel != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) {
				m.runOpts = m.runChoices(sel)
				m.runCursor = 0
				m.modal = modalRun
				return m, nil
			})
		}
	case "f":
		if sel != nil {
			return m.confirmFolder(sel.Name)
		}
	case "U":
		return m.openUpdate()
	case "S":
		return m.switchLimited()
	case "L":
		if sel != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) { return m.confirmLock(sel.Name) })
		}
	case "ctrl+l":
		return m.confirmLock("")
	case "u":
		if len(m.backups) > 0 {
			return m.confirmRestore("")
		}
		m.toast(2, "no backups yet")
	case "x", "delete":
		if m.focusRight && m.tab == tabRules {
			rs := m.allRules()
			if len(rs) > 0 {
				r := rs[m.rightCursor[tabRules]]
				m.pend = &pending{kind: actDeleteRule, profile: r.profile, ruleIdx: r.idx,
					title: "Remove identity rule?", lines: []string{r.profile + "  " + r.rule.String()}}
				m.modal = modalConfirm
			}
			return m, nil
		}
		if only := m.focusedTool(); sel != nil && only != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) {
				acct := sel.Accounts[only[0]]
				if acct == nil {
					m.toast(2, "%s has no saved %s account", sel.Name, m.toolName(only[0]))
					return m, nil
				}
				m.pend = &pending{kind: actForget, profile: sel.Name, only: only,
					title: "Forget " + sel.Name + "'s " + m.toolName(only[0]) + " account?",
					lines: []string{"Deletes the saved " + acct.Label + " login from " + sel.Name + ".", m.toolName(only[0]) + " itself stays logged in as it is now."}}
				m.modal = modalConfirm
				return m, nil
			})
		}
		if sel != nil {
			return m.withPassword(sel.Name, func(m Model) (tea.Model, tea.Cmd) {
				m.pend = &pending{kind: actDelete, profile: sel.Name, title: "Delete profile " + sel.Name + "?",
					lines: []string{"Removes the profile and every credential saved in it.", "Tools stay logged in as they are now."}}
				m.modal = modalConfirm
				return m, nil
			})
		}
	}
	return m, nil
}

// the tool under the cursor when the status list has focus, nil means every tool
func (m Model) focusedTool() []string {
	if !m.focusRight || m.tab != tabStatus || len(m.status) == 0 {
		return nil
	}
	return []string{m.status[m.rightCursor[tabStatus]].ID}
}

func (m Model) toolName(id string) string {
	for _, a := range m.mgr.Adapters {
		if a.ID() == id {
			return a.Name()
		}
	}
	return id
}

// "everything" or the single tool's name
func (m Model) scope(only []string) string {
	if len(only) == 1 {
		return m.toolName(only[0])
	}
	return "everything"
}
