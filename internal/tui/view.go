package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

const (
	minW = 64
	minH = 20
)

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		msg := sAccent.Render("CRE.A.M") + "\n" + sDim.Render(fmt.Sprintf("terminal too small (%dx%d) · need %dx%d", m.w, m.h, minW, minH))
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, msg)
	}
	if m.intro {
		return m.viewIntro()
	}
	base := m.viewMain()
	if box := m.modalBox(); box != "" {
		base = overlay(base, box, m.w, m.h)
	}
	return m.drawArmed(base)
}

// the open window, drawn centred over the main view, mouse.go hit tests the same box
func (m Model) modalBox() string {
	switch m.modal {
	case modalConfirm:
		return m.viewConfirm()
	case modalForm:
		if m.form != nil {
			return sModal.Render(m.form.View())
		}
	case modalRun:
		return m.viewRun()
	case modalHelp:
		return m.viewHelp()
	case modalResult:
		return m.viewResult()
	case modalUpdate:
		return m.viewUpdate()
	}
	return ""
}

func leftWidth(w int) int { return max(26, min(36, w/3)) }

// column widths of the status table for a panel content width w
func statusCols(w int) (colTool, colLive, colProf int) {
	colLive = max(18, (w-16)/2)
	colProf = max(10, w-16-colLive-2)
	return 14, colLive, colProf
}

// where the LIVE NOW cell sits in a status row, counted from the panel's content edge
// a row is a 2 column marker, the tool name padded to colTool-2, two spaces, then the cell
func liveCell(w int) (start, end int) {
	colTool, colLive, _ := statusCols(w)
	start = 2 + (colTool - 2) + 2
	return start, start + colLive
}

// the backups that fit in h lines, scrolled so the cursor stays visible
func backupWindow(h, cursor int) (start, limit int) {
	limit = max(1, h-3)
	if cursor >= limit {
		start = cursor - limit + 1
	}
	return start, limit
}

// intro timeline in seconds
const (
	introReveal = 1.8
	introTag    = 1.6
	introTagCPS = 20
	introSub    = 2.7
	introHint   = 3.4
	introIdle   = 10
)

func (m Model) viewIntro() string {
	t := m.tFrame()
	reveal := math.Min(1, t/introReveal)
	inv := 1 - reveal
	reveal = 1 - inv*inv
	lines := bigLogoLines()
	if m.w < 84 || m.h < 24 {
		lines = smallLogo
	}
	logo := paintGradient(lines, t*0.12, reveal)
	tag := ""
	if t > introTag {
		full := "CREdentiAls Master"
		n := int(math.Min(float64(len(full)), (t-introTag)*introTagCPS))
		tag = sBold.Render(full[:n])
	}
	sub := ""
	if t > introSub {
		sub = shimmerText("one profile switch for every CLI login you own", t*0.5, cDim, cBright)
	}
	hint := ""
	if t > introHint {
		hint = sFaint.Render("press any key")
	}
	full := []rune("still there?...")
	idle := strings.Repeat(" ", len(full))
	if t > introIdle {
		n := min(len(full), int((t-introIdle)*introTagCPS*0.6))
		idle = shimmerText(string(full[:n]), t*0.35, cFaint, cDim) + strings.Repeat(" ", len(full)-n)
	}
	block := lipgloss.JoinVertical(lipgloss.Center, logo, "", tag, sub, "", hint, "", idle)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, block)
}

func (m Model) viewMain() string {
	header := m.viewHeader()
	footer := m.viewFooter()
	bodyH := m.h - lipgloss.Height(header) - lipgloss.Height(footer)
	leftW := leftWidth(m.w)
	rightW := m.w - leftW
	left := m.viewProfiles(leftW, bodyH)
	right := m.viewRight(rightW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return clamp(lipgloss.JoinVertical(lipgloss.Left, header, body, footer), m.w, m.h)
}

// every frame must fit the terminal or bubble tea scrolls and flickers
func clamp(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewHeader() string {
	t := m.tFrame()
	logo := paintGradient(smallLogo, t*0.12, 1)
	active := m.mgr.Cfg.Active
	activeStr := sDim.Render("no active profile")
	if active != "" {
		activeStr = sDim.Render("active ") + m.profileDot(active) + " " + sBold.Render(active)
	}
	info := lipgloss.JoinVertical(lipgloss.Left,
		sHi.Render("CREdentiAls Master")+sFaint.Render("  "+m.opts.Version),
		activeStr+sFaint.Render("  ·  ")+sDim.Render(vaultLabel(m.mgr.Env.Vault.Backend())),
	)
	var badges []string
	if m.mgr.Cfg.Lock.All {
		badges = append(badges, sBadgeDry.Render("LOCKED"))
	}
	if m.dryRun {
		badges = append(badges, sBadgeDry.Render("DRY RUN"))
	}
	if m.upd != nil && m.upd.Newer {
		badges = append(badges, sBadgeUpd.Render("↑ "+m.upd.Latest+" · U"))
	}
	leftPart := lipgloss.JoinHorizontal(lipgloss.Center, logo, "   ", info)
	// in dry run the badge is the message, no splash text next to it
	if !m.dryRun && lipgloss.Width(leftPart)+len(badges)*12+taglineWidth()+6 < m.w {
		badges = append([]string{renderTagline(m.tagSeed, t)}, badges...)
	}
	right := strings.Join(badges, " ")
	if lipgloss.Width(leftPart)+lipgloss.Width(right)+3 > m.w {
		leftPart = lipgloss.JoinVertical(lipgloss.Left, logo) // narrow: logo only
		if lipgloss.Width(leftPart)+lipgloss.Width(right)+3 > m.w {
			right = ""
		}
	}
	gap := max(1, m.w-2-lipgloss.Width(leftPart)-lipgloss.Width(right))
	row := lipgloss.JoinHorizontal(lipgloss.Center, leftPart, strings.Repeat(" ", gap), right)
	return lipgloss.NewStyle().Padding(1, 1, 0, 1).Render(row)
}

func (m Model) profileDot(name string) string {
	p := m.mgr.Cfg.Profiles[name]
	c := cAccent
	if p != nil && p.Color != "" {
		c = lipgloss.Color(p.Color)
	}
	return lipgloss.NewStyle().Foreground(c).Render("●")
}

func (m Model) viewProfiles(w, h int) string {
	style := sPanel
	if !m.focusRight {
		style = sPanelHot
	}
	title := sAccent.Render("Profiles")
	if !m.focusRight {
		title = shimmerText("Profiles", m.tFrame()*0.5, cAccent, cBright)
	}
	lines := []string{title, ""}
	names := m.names()
	if len(names) == 0 {
		lines = append(lines,
			sDim.Render("No profiles yet."), "",
			sKey.Render("n")+sKeyDesc.Render(" create one, e.g. work"),
		)
	}
	for i, n := range names {
		p := m.mgr.Cfg.Profiles[n]
		marker := "  "
		if i == m.cursor {
			marker = sHi.Render("▌ ")
		}
		name := n
		nameSt := sBase
		if i == m.cursor {
			nameSt = sBold
		}
		row := marker + m.profileDot(n) + " " + nameSt.Render(name)
		if n == m.mgr.Cfg.Active {
			row += " " + sOK.Render("active")
		}
		if m.mgr.Cfg.Locked(n) {
			row += " " + sWarn.Render("locked")
		}
		accts := len(p.Accounts)
		sub := fmt.Sprintf("%d/%d tools", accts, len(m.mgr.Adapters))
		if len(p.Rules) > 0 {
			sub += fmt.Sprintf(" · %d rule", len(p.Rules))
			if len(p.Rules) > 1 {
				sub += "s"
			}
		}
		subMarker := "  "
		if i == m.cursor {
			subMarker = sHi.Render("▌ ")
		}
		subRow := subMarker + "  " + sFaint.Render(sub)
		lines = append(lines, row, subRow)
	}
	content := strings.Join(lines, "\n")
	return style.Width(w - 2).Height(h - 2).Render(content)
}

func (m Model) viewRight(w, h int) string {
	style := sPanel
	if m.focusRight {
		style = sPanelHot
	}
	inner := w - 4
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	tabRow := strings.Join(tabs, " ")
	var body string
	switch m.tab {
	case tabStatus:
		body = m.viewStatus(inner)
	case tabRules:
		body = m.viewRules(inner)
	case tabBackups:
		body = m.viewBackups(h - 6)
	}
	content := tabRow + "\n\n" + body
	return style.Width(w - 2).Height(h - 2).Render(content)
}

func (m Model) viewStatus(w int) string {
	sel := m.selected()
	if !m.statusLoaded {
		return m.spin.View() + sDim.Render(" reading tool state…")
	}
	colTool, colLive, colProf := statusCols(w)
	selName := "profile"
	if sel != nil {
		selName = sel.Name
	}
	head := sFaint.Render("  " + pad("TOOL", colTool) + pad("LIVE NOW", colLive) + truncate(strings.ToUpper(selName), colProf))
	lines := []string{head}
	focused := m.focusRight && m.tab == tabStatus
	for i, s := range m.status {
		icon := sFaint.Render("○")
		live := sFaint.Render("not logged in")
		if !s.Live.Installed {
			live = sFaint.Render("not installed")
		}
		if s.Live.LoggedIn {
			// gentle pulse on live accounts
			pulse := 0.5 + 0.5*math.Sin(m.tFrame()*2.4+float64(i)*0.7)
			c := hex("#7BD88F").BlendLuv(hex("#FAF6EC"), pulse*0.5).Clamped()
			icon = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render("●")
			live = sBase.Render(truncate(s.Live.Account, colLive-3))
		}
		prof := sFaint.Render("–  nothing saved")
		if sel != nil {
			if a := sel.Accounts[s.ID]; a != nil {
				switch {
				case s.Live.LoggedIn && s.Live.Account == a.Label:
					prof = sOK.Render("✓ ") + sBase.Render(truncate(a.Label, colProf-2))
				default:
					prof = sHi.Render("→ ") + sDim.Render(truncate(a.Label, colProf-2))
				}
			}
		}
		marker := "  "
		if focused && i == m.rightCursor[tabStatus] {
			marker = sHi.Render("▌ ")
		}
		row := marker + sBold.Render(pad(s.Name, colTool-2)) + "  " + icon + " " + padANSI(live, colLive-2) + prof
		lines = append(lines, row)
		if s.Live.Warning != "" {
			lines = append(lines, "    "+sWarn.Render("! "+truncate(s.Live.Warning, w-6)))
		}
	}
	lines = append(lines, "")
	if sel != nil {
		lines = append(lines, m.statusHints(sel, focused)...)
		lines = append(lines, "", m.profileCard(sel, w))
	}
	return strings.Join(lines, "\n")
}

func (m Model) statusHints(sel *config.Profile, focused bool) []string {
	key := func(k, desc string) string { return sKey.Render(k) + sKeyDesc.Render(" "+desc) }
	dot := sKeyDesc.Render(" · ")
	if focused && len(m.status) > 0 {
		s := m.status[m.rightCursor[tabStatus]]
		out := []string{
			key("enter", "switch only "+s.Name+" to ") + sBold.Render(sel.Name),
			key("i", "save the current "+s.Name+" login into ") + sBold.Render(sel.Name),
		}
		if sel.Accounts[s.ID] != nil {
			out = append(out, key("x", "forget the "+s.Name+" account saved in ")+sBold.Render(sel.Name))
		}
		out = append(out, key("←", "back to switching everything"))
		if s.Live.LoggedIn && len(m.mgr.Owners(s.ID, s.Live.Account)) == 0 {
			out = append(out, sWarn.Render("! ")+sDim.Render(s.Live.Account+" is not saved in any profile yet"))
		}
		return out
	}
	var out []string
	if sel.Name == m.mgr.Cfg.Active {
		out = append(out, sOK.Render("● ")+sDim.Render(sel.Name+" is the active profile"))
	} else {
		out = append(out, key("enter", "switch everything to ")+sBold.Render(sel.Name))
	}
	out = append(out, key("→", "pick one tool to switch or import")+dot+key("r", "run one tool as "+sel.Name))
	if len(sel.Accounts) == 0 {
		out = append(out, key("i", "import the logins above into ")+sBold.Render(sel.Name))
	}
	return out
}

func (m Model) profileCard(p *config.Profile, w int) string {
	label := func(s string) string { return sFaint.Render(pad(s, 14)) }
	rows := []string{m.profileDot(p.Name) + " " + sBold.Render(p.Name) + "  " + sDim.Render(p.Description)}
	if p.Git.IsZero() {
		rows = append(rows, label("git identity")+sFaint.Render("none · press e"))
	} else {
		rows = append(rows, label("git identity")+sBase.Render(p.Git.Name)+sDim.Render(" <"+p.Git.Email+">"))
	}
	rows = append(rows, label("rules")+sBase.Render(fmt.Sprintf("%d", len(p.Rules))))
	var newest time.Time
	for _, a := range p.Accounts {
		if a.Imported.After(newest) {
			newest = a.Imported
		}
	}
	if !newest.IsZero() {
		rows = append(rows, label("last import")+sBase.Render(humanTime(newest)))
	}
	if !p.Created.IsZero() {
		rows = append(rows, label("created")+sDim.Render(p.Created.Local().Format("2006-01-02")))
	}
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(orDefault(p.Color, string(cAccent)))).PaddingLeft(1).
		Width(min(w-2, 70)).Render(strings.Join(rows, "\n"))
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (m Model) viewRules(w int) string {
	rules := m.allRules()
	if len(rules) == 0 {
		return strings.Join([]string{
			sDim.Render("No identity rules yet."),
			"",
			sDim.Render("Rules pick the git identity per repository, by folder or"),
			sDim.Render("remote host, so a work repo always commits as work."),
			sDim.Render("Repos matching no rule cannot commit (user.useConfigOnly)."),
			"",
			sKey.Render("a") + sKeyDesc.Render(" add a rule to the selected profile"),
		}, "\n")
	}
	lines := []string{sFaint.Render(pad("PROFILE", 14) + pad("WHEN", 10) + pad("MATCH", max(10, w-50)) + "IDENTITY")}
	for i, r := range rules {
		kind := "remote"
		if r.rule.Kind == "dir" {
			kind = "folder"
		}
		row := m.profileDot(r.profile) + " " + pad(r.profile, 12) + sDim.Render(pad(kind, 10)) + sBase.Render(pad(truncate(r.rule.Pattern, max(10, w-50)-1), max(10, w-50))) + sDim.Render(truncate(r.email, 24))
		if m.focusRight && i == m.rightCursor[tabRules] {
			row = sHi.Render("▌") + row
		} else {
			row = " " + row
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", sFaint.Render("managed block in ~/.gitconfig · → focus list · x remove · a add"))
	return strings.Join(lines, "\n")
}

func (m Model) viewBackups(h int) string {
	if len(m.backups) == 0 {
		return sDim.Render("No backups yet. One is taken automatically before every switch.")
	}
	lines := []string{sFaint.Render(pad("WHEN", 22) + "REASON")}
	startIdx, limit := backupWindow(h, m.rightCursor[tabBackups])
	for i := startIdx; i < len(m.backups) && i < startIdx+limit; i++ {
		b := m.backups[i]
		row := pad(humanTime(b.Created), 22) + sDim.Render(b.Reason)
		if m.focusRight && i == m.rightCursor[tabBackups] {
			row = sHi.Render("▌") + row
		} else {
			row = " " + row
		}
		lines = append(lines, row)
	}
	lines = append(lines, "", sFaint.Render("encrypted with age · → focus list · enter restore · u restore latest"))
	return strings.Join(lines, "\n")
}

func humanTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return t.Local().Format("2006-01-02 15:04")
}

func (m Model) viewFooter() string {
	var activity string
	switch {
	case m.busy:
		var parts []string
		for _, s := range m.steps {
			parts = append(parts, stepIcon(s.state, m.spin.View())+" "+sDim.Render(s.name))
		}
		activity = m.spin.View() + " " + sBase.Render(m.busyLabel) + "  " + m.bar.View()
		if len(parts) > 0 {
			activity += "  " + strings.Join(parts, "  ")
		}
	case len(m.toasts) > 0:
		t := m.toasts[len(m.toasts)-1]
		age := time.Since(t.born).Seconds()
		st := sBase
		icon := sHi.Render("◆ ")
		switch t.kind {
		case 1:
			st, icon = sOK, sOK.Render("✓ ")
		case 2:
			st, icon = sErr, sErr.Render("✗ ")
		}
		text := t.text
		if age < 0.35 { // slide in
			n := int(float64(len([]rune(text))) * age / 0.35)
			text = string([]rune(text)[:n])
		}
		if age > 3.4 { // fade out
			st = sFaint
			icon = sFaint.Render("· ")
		}
		activity = icon + st.Render(text)
	default:
		activity = sFaint.Render("ready")
	}
	activity = truncateANSI(activity, m.w-2)
	help := m.helpLine()
	return lipgloss.NewStyle().Padding(0, 1).Render(activity + "\n" + help)
}

func stepIcon(s core.State, spin string) string {
	switch s {
	case core.Running:
		return spin
	case core.Done:
		return sOK.Render("✓")
	case core.Skipped:
		return sFaint.Render("○")
	case core.Failed:
		return sErr.Render("✗")
	}
	return sFaint.Render("·")
}

type keyHint struct{ k, v string }

// the footer's keys for the current focus, mouse.go makes the same list clickable
func (m Model) footerHints() []keyHint {
	switch {
	case m.focusRight && m.tab == tabStatus:
		return m.enabledHints([]keyHint{{"↑↓", "tool"}, {"enter", "switch this tool"}, {"i", "import this tool"}, {"x", "forget"}, {"←", "profiles"}, {"tab", "view"}})
	case m.focusRight && m.tab == tabRules:
		return m.enabledHints([]keyHint{{"↑↓", "select"}, {"x", "remove"}, {"a", "add"}, {"←", "profiles"}, {"tab", "view"}})
	case m.focusRight && m.tab == tabBackups:
		return m.enabledHints([]keyHint{{"↑↓", "select"}, {"enter", "restore"}, {"←", "profiles"}, {"tab", "view"}})
	}
	return m.enabledHints([]keyHint{{"enter", "switch"}, {"r", "run"}, {"i", "import"}, {"n", "new"}, {"e", "edit"}, {"x", "delete"}, {"a", "rule"}, {"L", "lock"}, {"u", "undo"}, {"D", "dry-run"}, {"tab", "view"}})
}

// always shown at the end of the footer
var footerTail = []keyHint{{"?", "help"}, {"q", "quit"}}

func (m Model) helpLine() string {
	sep := sFaint.Render("  ·  ")
	render := func(it keyHint) string { return sKey.Render(it.k) + " " + sKeyDesc.Render(it.v) }
	var tails []string
	for _, it := range m.enabledHints(footerTail) {
		tails = append(tails, render(it))
	}
	tail := strings.Join(tails, sep)
	out := ""
	for _, it := range m.footerHints() {
		next := render(it) + sep
		if lipgloss.Width(out+next+tail) > m.w-2 {
			break
		}
		out += next
	}
	return out + tail
}

func vaultLabel(backend string) string {
	switch backend {
	case "keyring":
		return "OS keyring"
	case "file":
		return "encrypted file vault"
	}
	return backend + " vault"
}

func pad(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func padANSI(s string, n int) string { return pad(s, n) }

func truncate(s string, n int) string {
	if n <= 1 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	return ansi.Truncate(s, n-1, "") + "…"
}

func truncateANSI(s string, n int) string { return ansi.Truncate(s, n, "…") }

// draws box centered over a dimmed bg
func overlay(bg, box string, w, h int) string {
	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < h {
		bgLines = append(bgLines, "")
	}
	boxLines := strings.Split(box, "\n")
	bw := lipgloss.Width(box)
	bh := len(boxLines)
	x := max(0, (w-bw)/2)
	y := max(0, (h-bh)/2)
	for i := range bgLines {
		bgLines[i] = sFaint.Render(ansi.Strip(bgLines[i]))
	}
	for i, bl := range boxLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		plain := ansi.Strip(bgLines[row])
		left := ansi.Truncate(plain, x, "")
		left = pad(left, x)
		right := ansi.TruncateLeft(plain, x+bw, "")
		bgLines[row] = sFaint.Render(left) + pad(bl, bw) + sFaint.Render(right)
	}
	return strings.Join(bgLines[:h], "\n")
}
