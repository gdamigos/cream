package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// what a left click landed on in the main view
type hitKind int

const (
	hitNone hitKind = iota
	hitProfile
	hitTab
	hitRow // a row of the list in the current tab
)

type hit struct {
	kind  hitKind
	index int
	live  bool // on the LIVE NOW cell of a status row
}

// mirrors viewMain: header, then the profiles panel and the tab panel side by side, both with a border
// a panel's content starts one line below its top border and two columns in (border plus padding)
func (m Model) hitTest(x, y int) hit {
	headerH := lipgloss.Height(m.viewHeader())
	bodyH := m.h - headerH - lipgloss.Height(m.viewFooter())
	leftW := leftWidth(m.w)
	line := y - headerH - 1
	if line < 0 || line >= bodyH-2 {
		return hit{}
	}

	if x < leftW {
		// a title and a blank line, then two lines per profile
		if i := (line - 2) / 2; line >= 2 && i < len(m.names()) {
			return hit{kind: hitProfile, index: i}
		}
		return hit{}
	}

	if line == 0 {
		cx := leftW + 2
		for i, n := range tabNames {
			w := len(fmt.Sprintf("%d %s", i+1, n)) + 2 // tabs have one column of padding on each side
			if x >= cx && x < cx+w {
				return hit{kind: hitTab, index: i}
			}
			cx += w + 1
		}
		return hit{}
	}

	// the tab row and a blank line, then the tab's own header line
	row := line - 3
	if row < 0 {
		return hit{}
	}
	switch m.tab {
	case tabStatus:
		if !m.statusLoaded {
			return hit{}
		}
		start, end := liveCell(m.w - leftW - 4)
		rel := x - leftW - 2
		ln := 0
		for i, s := range m.status {
			if row == ln {
				return hit{hitRow, i, rel >= start && rel < end}
			}
			ln++
			if s.Live.Warning != "" {
				ln++ // the warning sits on its own line under the tool
			}
		}
	case tabRules:
		if row < len(m.allRules()) {
			return hit{kind: hitRow, index: row}
		}
	case tabBackups:
		start, limit := backupWindow(bodyH-6, m.rightCursor[tabBackups])
		if row < limit && start+row < len(m.backups) {
			return hit{kind: hitRow, index: start + row}
		}
	}
	return hit{}
}

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyCtrlL = tea.KeyMsg{Type: tea.KeyCtrlL}
)

// the key a hint stands for, false for hints like ↑↓ that aren't one key
func keyFor(k string) (tea.KeyMsg, bool) {
	switch k {
	case "enter":
		return keyEnter, true
	case "esc":
		return keyEsc, true
	case "tab":
		return keyTab, true
	case "←":
		return keyLeft, true
	case "→":
		return keyRight, true
	}
	if r := []rune(k); len(r) == 1 {
		return keyRune(r[0]), true
	}
	return tea.KeyMsg{}, false
}

type focusTo int

const (
	focusKeep focusTo = iota
	focusProfiles
	focusPanel // keys like x on a rule act on the list, so it needs focus first
)

// a piece of drawn text that acts like a key when clicked
type clickLabel struct {
	text  string
	key   tea.KeyMsg
	focus focusTo
}

func label(text string, k tea.KeyMsg) clickLabel { return clickLabel{text: text, key: k} }

// finds the label drawn under x on screen line y, only where it lies fully between minX and maxX
// it reads the rendered screen, so it follows the layout wherever the text ends up
func (m Model) labelAt(x, y, minX, maxX int, labels []clickLabel) (clickLabel, bool) {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y < 0 || y >= len(lines) {
		return clickLabel{}, false
	}
	line := lines[y]
	for _, l := range labels {
		for from := 0; ; {
			i := strings.Index(line[from:], l.text)
			if i < 0 {
				break
			}
			start := ansi.StringWidth(line[:from+i])
			end := start + ansi.StringWidth(l.text)
			if start >= minX && end <= maxX && x >= start && x < end {
				return l, true
			}
			from += i + len(l.text)
		}
	}
	return clickLabel{}, false
}

func (m Model) headerLabels() []clickLabel {
	var out []clickLabel
	if m.mgr.Cfg.Lock.All {
		out = append(out, label("LOCKED", keyCtrlL))
	}
	if m.dryRun {
		out = append(out, label("DRY RUN", keyRune('D')))
	}
	if m.upd != nil && m.upd.Newer {
		out = append(out, label("↑ "+m.upd.Latest+" · U", keyRune('U')))
	}
	return out
}

func (m Model) footerLabels() []clickLabel {
	var out []clickLabel
	for _, h := range append(m.footerHints(), m.enabledHints(footerTail)...) {
		if k, ok := keyFor(h.k); ok {
			out = append(out, label(h.k+" "+h.v, k))
		}
	}
	return out
}

// the key hints drawn inside the tab panel, see statusHints, viewRules and viewBackups
func (m Model) panelLabels() []clickLabel {
	focusList := label("→ focus list", keyRight)
	switch m.tab {
	case tabRules:
		return []clickLabel{focusList, {"x remove", keyRune('x'), focusPanel}, label("a add", keyRune('a'))}
	case tabBackups:
		return []clickLabel{focusList, {"enter restore", keyEnter, focusPanel}, label("u restore latest", keyRune('u'))}
	}
	return []clickLabel{
		label("enter switch", keyEnter), label("i save", keyRune('i')), label("i import", keyRune('i')),
		label("x forget", keyRune('x')), label("← back", keyLeft), label("→ pick", keyRight), label("r run", keyRune('r')),
	}
}

// a click does what the matching key does, clicking the selected item again is enter
func (m Model) handleClick(x, y int) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m.clickModal(x, y)
	}
	if m.busy {
		return m, nil
	}

	headerH := lipgloss.Height(m.viewHeader())
	footerH := lipgloss.Height(m.viewFooter())
	leftW := leftWidth(m.w)
	inBody := y >= headerH && y < m.h-footerH
	var labels []clickLabel
	minX, maxX := 0, m.w
	switch {
	case y < headerH:
		labels = m.headerLabels()
	case !inBody:
		labels = m.footerLabels()
	case x < leftW:
		labels, maxX = []clickLabel{label("n create one", keyRune('n'))}, leftW
	default:
		labels, minX = m.panelLabels(), leftW
	}
	if l, ok := m.labelAt(x, y, minX, maxX, labels); ok {
		switch l.focus {
		case focusProfiles:
			m.focusRight = false
		case focusPanel:
			m.focusRight = true
		}
		return m.handleKey(l.key)
	}

	h := m.hitTest(x, y)
	switch h.kind {
	case hitProfile:
		again := !m.focusRight && m.cursor == h.index
		m.focusRight, m.cursor = false, h.index
		if again {
			return m.handleKey(keyEnter)
		}
	case hitTab:
		m.tab = tab(h.index)
	case hitRow:
		again := m.focusRight && m.rightCursor[m.tab] == h.index
		m.focusRight, m.rightCursor[m.tab] = true, h.index
		// the live account itself imports that login into the selected profile, like i on the row
		if m.tab == tabStatus && h.live && m.status[h.index].Live.LoggedIn {
			if sel := m.selected(); sel != nil {
				return m.confirmImport(sel, []string{m.status[h.index].ID})
			}
		}
		// enter on a rule does nothing useful, on a tool or backup it opens a confirm
		if again && m.tab != tabRules {
			return m.handleKey(keyEnter)
		}
	case hitNone:
		// empty space in a panel moves the focus there, the way back to the profile list
		if inBody {
			m.focusRight = x >= leftW
		}
	}
	return m, nil
}

// outside the border closes the form like esc, inside a field's title or input moves the cursor there
func (m Model) clickForm(x, y int) (tea.Model, tea.Cmd) {
	box := m.modalBox()
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	bx, by := max(0, (m.w-bw)/2), max(0, (m.h-bh)/2)
	if x < bx || x >= bx+bw || y < by || y >= by+bh {
		return m.updateForm(keyEsc)
	}
	// a field runs from its title down to the next title, so the last title above the click wins
	target := -1
	lines := strings.Split(ansi.Strip(box), "\n")
	for ln := 0; ln <= y-by && ln < len(lines); ln++ {
		for i, title := range m.formTitles {
			if strings.Contains(lines[ln], title) {
				target = i
			}
		}
	}
	cur := -1
	focused := m.form.GetFocusedField()
	for i, f := range m.formFields {
		if f == focused {
			cur = i
		}
	}
	if target < 0 || cur < 0 {
		return m, nil
	}
	// one step at a time like tab, and never past the last field, which would submit the form
	var cmds []tea.Cmd
	for ; cur < target; cur++ {
		cmds = append(cmds, m.form.NextField())
	}
	for ; cur > target; cur-- {
		cmds = append(cmds, m.form.PrevField())
	}
	return m, tea.Batch(cmds...)
}

// windows are centred boxes with a border and padding of 1 line and 3 columns, see sModal
func (m Model) clickModal(x, y int) (tea.Model, tea.Cmd) {
	if m.modal == modalForm {
		return m.clickForm(x, y)
	}
	box := m.modalBox()
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	bx, by := max(0, (m.w-bw)/2), max(0, (m.h-bh)/2)
	inside := x >= bx && x < bx+bw && y >= by && y < by+bh
	cx, cy := bx+4, by+2

	switch m.modal {
	case modalHelp, modalResult:
		return m.handleKey(keyEsc)
	case modalConfirm:
		if !inside {
			return m.handleKey(keyEsc)
		}
		// the last line reads "y confirm   n cancel"
		if y == by+bh-3 {
			switch {
			case x >= cx && x < cx+len("y confirm"):
				return m.handleKey(keyRune('y'))
			case x >= cx+len("y confirm   ") && x < cx+len("y confirm   n cancel"):
				return m.handleKey(keyRune('n'))
			}
		}
	case modalRun:
		if !inside {
			return m.handleKey(keyEsc)
		}
		// a title, a subtitle and a blank line, then one choice per line
		if i := y - cy - 3; i >= 0 && i < len(m.runOpts) {
			if i == m.runCursor {
				return m.handleKey(keyEnter)
			}
			m.runCursor = i
			return m, nil
		}
		labels := []clickLabel{label("enter launch", keyEnter), label("esc cancel", keyEsc)}
		if l, ok := m.labelAt(x, y, bx, bx+bw, labels); ok {
			return m.handleKey(l.key)
		}
	case modalUpdate:
		if !inside {
			return m.handleKey(keyEsc)
		}
		labels := []clickLabel{label("c copy", keyRune('c')), label("o open", keyRune('o')), label("esc close", keyEsc)}
		if l, ok := m.labelAt(x, y, bx, bx+bw, labels); ok {
			return m.handleKey(l.key)
		}
	}
	return m, nil
}
