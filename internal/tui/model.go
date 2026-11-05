package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/backup"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/update"
)

type Options struct {
	Version     string
	DryRun      bool
	UpdateCheck bool
	Checker     *update.Checker
	SkipIntro   bool
}

func Run(m *core.Manager, o Options) error {
	p := tea.NewProgram(New(m, o), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())
	_, err := p.Run()
	return err
}

type tab int

const (
	tabStatus tab = iota
	tabRules
	tabBackups
	tabCount
)

var tabNames = []string{"Status", "Rules", "Backups"}

type modalKind int

const (
	modalNone modalKind = iota
	modalConfirm
	modalForm
	modalRun
	modalHelp
	modalResult
	modalUpdate
)

type actionKind int

const (
	actSwitch actionKind = iota
	actImport
	actDelete
	actRestore
	actDeleteRule
	actForget
	actLock
	actUnlock
)

// how often tool state is re-read so logins made in other terminals show up
const pollEvery = 3 * time.Second

type pending struct {
	kind     actionKind
	profile  string
	only     []string // adapter ids, empty means every tool
	backupID string
	ruleIdx  int
	title    string
	lines    []string
}

type formKind int

const (
	formNewProfile formKind = iota
	formEditProfile
	formRule
)

// lives on the heap so huh keeps valid pointers when the model is copied
type formData struct {
	kind                        formKind
	profile                     string
	name, desc, gName, gEmail   string
	gKey, ruleKind, rulePattern string
}

type step struct {
	id, name string
	state    core.State
	msg      string
}

type toast struct {
	text string
	kind int // 0 info, 1 ok, 2 error
	born time.Time
}

type runChoice struct {
	label, bin string
	args       []string
}

type frameMsg time.Time
type pollMsg time.Time
type statusMsg struct {
	list  []core.AdapterStatus
	watch bool // from a background refresh, so changes came from outside cream
}
type backupsMsg []backup.Info
type updateMsg struct{ res *update.Result }
type eventMsg core.Event
type opDoneMsg struct {
	title   string
	err     error
	changes []adapter.Change
	events  []core.Event
	dry     bool
}
type previewMsg struct {
	p   pending
	err error
}
type execDoneMsg struct {
	label string
	err   error
}

type Model struct {
	mgr  *core.Manager
	opts Options

	w, h   int
	frame  int
	start  time.Time
	intro  bool
	dryRun bool

	cursor       int
	focusRight   bool
	tab          tab
	rightCursor  [tabCount]int
	status       []core.AdapterStatus
	statusLoaded bool
	statusLoads  int // status reads in flight, config is only reloaded when zero
	backups      []backup.Info

	busy      bool
	busyLabel string
	spin      spinner.Model
	bar       progress.Model
	steps     []step
	opCh      chan tea.Msg

	modal     modalKind
	pend      *pending
	form      *huh.Form
	fdata     *formData
	runOpts   []runChoice
	runCursor int
	result    *opDoneMsg

	toasts  []toast
	upd     *update.Result
	inst    update.Install // how this binary was installed, filled when the update window opens
	tagSeed uint64
}

func New(m *core.Manager, o Options) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: []string{"◜", "◠", "◝", "◞", "◡", "◟"},
		FPS:    time.Second / 12,
	}), spinner.WithStyle(sHi))
	bar := progress.New(progress.WithGradient("#4FB3A9", "#F1E3C3"), progress.WithoutPercentage(), progress.WithFillCharacters('━', '─'))
	bar.EmptyColor = string(cFaint)
	md := Model{
		mgr:     m,
		opts:    o,
		start:   time.Now(),
		intro:   !o.SkipIntro,
		dryRun:  o.DryRun,
		spin:    sp,
		bar:     bar,
		tagSeed: rand.Uint64(),
		// Init starts the first status read on a copy
		statusLoads: 1,
	}
	for i, n := range m.Cfg.Names() {
		if n == m.Cfg.Active {
			md.cursor = i
		}
	}
	return md
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), poll(), m.spin.Tick, m.loadStatus(), m.loadBackups()}
	if m.opts.UpdateCheck && m.opts.Checker != nil {
		cmds = append(cmds, checkUpdate(m.opts.Checker))
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return frameMsg(t) })
}

func poll() tea.Cmd {
	return tea.Tick(pollEvery, func(t time.Time) tea.Msg { return pollMsg(t) })
}

// re-reads config.toml if another cream changed it, then the live tool state
func (m Model) refresh() (Model, tea.Cmd) {
	if m.busy || m.modal != modalNone || m.statusLoads > 0 {
		return m, nil
	}
	if _, err := m.mgr.ReloadIfChanged(); err != nil {
		m.toast(2, "%s", firstLine(err.Error()))
	}
	if n := len(m.names()); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
	m.statusLoads++
	mgr := m.mgr
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return statusMsg{list: mgr.StatusFor(ctx, nil), watch: true}
	}
}

// saves refreshed tokens of the active profile while nothing else runs
func (m Model) syncActive() Model {
	if m.busy || m.dryRun {
		return m
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.mgr.SyncActive(ctx); err != nil {
		m.toast(2, "%s", firstLine(err.Error()))
	}
	return m
}

func (m *Model) loadStatus() tea.Cmd {
	m.statusLoads++
	mgr := m.mgr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return statusMsg{list: mgr.StatusFor(ctx, nil)}
	}
}

func (m Model) loadBackups() tea.Cmd {
	mgr := m.mgr
	return func() tea.Msg {
		l, _ := mgr.Backups.List()
		return backupsMsg(l)
	}
}

func checkUpdate(c *update.Checker) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := c.Check(ctx)
		if err != nil {
			return updateMsg{}
		}
		return updateMsg{res}
	}
}

// long ops run in a goroutine and feed this channel, re-arm after every message
func waitOp(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m Model) names() []string { return m.mgr.Cfg.Names() }

func (m Model) selected() *config.Profile {
	n := m.names()
	if len(n) == 0 {
		return nil
	}
	if m.cursor >= len(n) {
		return m.mgr.Cfg.Profiles[n[len(n)-1]]
	}
	return m.mgr.Cfg.Profiles[n[m.cursor]]
}

func (m *Model) toast(kind int, format string, args ...any) {
	m.toasts = append(m.toasts, toast{text: fmt.Sprintf(format, args...), kind: kind, born: time.Now()})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
}

func (m Model) tFrame() float64 { return float64(time.Since(m.start).Milliseconds()) / 1000 }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.bar.Width = max(10, min(40, m.w/3))
		if m.form != nil {
			m.form = m.form.WithWidth(min(60, m.w-10))
		}
		return m, nil

	case frameMsg:
		m.frame++
		now := time.Now()
		kept := m.toasts[:0]
		for _, t := range m.toasts {
			if now.Sub(t.born) < 4*time.Second {
				kept = append(kept, t)
			}
		}
		m.toasts = kept
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		pm, cmd := m.bar.Update(msg)
		m.bar = pm.(progress.Model)
		return m, cmd

	case pollMsg:
		m, cmd := m.refresh()
		return m, tea.Batch(cmd, poll())

	case tea.FocusMsg:
		return m.refresh()

	case tea.BlurMsg:
		// the user may log out of a tool in the other terminal, so save refreshed tokens now
		if m.modal == modalNone {
			m = m.syncActive()
		}
		return m, nil

	case statusMsg:
		m.statusLoads = max(0, m.statusLoads-1)
		if m.statusLoaded && msg.watch {
			m.noticeLogins(msg.list)
		}
		m.status, m.statusLoaded = msg.list, true
		if m.rightCursor[tabStatus] >= len(m.status) {
			m.rightCursor[tabStatus] = max(0, len(m.status)-1)
		}
		return m, nil

	case backupsMsg:
		m.backups = msg
		if m.rightCursor[tabBackups] >= len(m.backups) {
			m.rightCursor[tabBackups] = max(0, len(m.backups)-1)
		}
		return m, nil

	case updateMsg:
		m.upd = msg.res
		if m.upd != nil && m.upd.Newer {
			m.toast(0, "cream %s is available · press U to update", m.upd.Latest)
		}
		return m, nil

	case eventMsg:
		for i := range m.steps {
			if m.steps[i].id == msg.Adapter {
				m.steps[i].state, m.steps[i].msg = msg.State, msg.Msg
			}
		}
		done := 0
		for _, s := range m.steps {
			if s.state >= core.Done {
				done++
			}
		}
		var cmd tea.Cmd
		if len(m.steps) > 0 {
			cmd = m.bar.SetPercent(float64(done) / float64(len(m.steps)))
		}
		return m, tea.Batch(cmd, waitOp(m.opCh))

	case previewMsg:
		m.busy = false
		if msg.err != nil {
			m.toast(2, "%s", msg.err.Error())
			return m, nil
		}
		p := msg.p
		m.pend, m.modal = &p, modalConfirm
		return m, nil

	case opDoneMsg:
		m.busy, m.opCh = false, nil
		cmds := []tea.Cmd{m.loadStatus(), m.loadBackups(), m.bar.SetPercent(1)}
		if msg.err != nil {
			m.toast(2, "%s", firstLine(msg.err.Error()))
		} else if !msg.dry {
			m.toast(1, "%s", msg.title)
		}
		if msg.dry || msg.err != nil || len(msg.events) > 0 {
			res := msg
			m.result, m.modal = &res, modalResult
		}
		return m, tea.Batch(cmds...)

	case execDoneMsg:
		if msg.err != nil {
			m.toast(2, "%s: %s", msg.label, msg.err.Error())
		} else {
			m.toast(1, "%s session ended", msg.label)
		}
		cmd := m.loadStatus()
		return m, tea.Batch(cmd, tea.EnableMouseCellMotion)

	case tea.MouseMsg:
		if m.intro {
			if msg.Action == tea.MouseActionPress {
				m.intro = false
			}
			return m, nil
		}
		if m.modal != modalNone {
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m.move(-1), nil
		case tea.MouseButtonWheelDown:
			return m.move(1), nil
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.intro {
			m.intro = false
			return m, nil
		}
		return m.handleKey(msg)
	}

	if m.modal == modalForm && m.form != nil {
		return m.updateForm(msg)
	}
	return m, nil
}

// toasts logins that changed since the last read, e.g. a /login in another terminal
func (m *Model) noticeLogins(next []core.AdapterStatus) {
	for _, s := range next {
		if !s.Live.LoggedIn {
			continue
		}
		i := slices.IndexFunc(m.status, func(o core.AdapterStatus) bool { return o.ID == s.ID })
		if i >= 0 && m.status[i].Live.LoggedIn && m.status[i].Live.Account == s.Live.Account {
			continue
		}
		if owners := m.mgr.Owners(s.ID, s.Live.Account); len(owners) > 0 {
			m.toast(0, "%s is now %s · saved in %s", s.Name, s.Live.Account, strings.Join(owners, ", "))
		} else {
			m.toast(0, "%s is now %s · not saved yet, pick a profile, → to the tool, i", s.Name, s.Live.Account)
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (m Model) move(d int) Model {
	if m.focusRight {
		n := m.rightLen()
		if n > 0 {
			m.rightCursor[m.tab] = (m.rightCursor[m.tab] + d + n) % n
		}
		return m
	}
	n := len(m.names())
	if n == 0 {
		return m
	}
	m.cursor = (m.cursor + d + n) % n
	return m
}

func (m Model) rightLen() int {
	switch m.tab {
	case tabStatus:
		return len(m.status)
	case tabRules:
		return len(m.allRules())
	case tabBackups:
		return len(m.backups)
	}
	return 0
}

type ruleRef struct {
	profile string
	idx     int
	rule    config.Rule
	email   string
}

func (m Model) allRules() []ruleRef {
	var out []ruleRef
	for _, n := range m.names() {
		p := m.mgr.Cfg.Profiles[n]
		for i, r := range p.Rules {
			out = append(out, ruleRef{n, i, r, p.Git.Email})
		}
	}
	return out
}

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
		switch k.String() {
		case "y", "Y", "enter":
			p := *m.pend
			m.modal, m.pend = modalNone, nil
			return m.execute(p)
		case "n", "N", "esc", "q":
			m.modal, m.pend = modalNone, nil
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
		case "enter":
			m.modal = modalNone
			return m.launch(m.runOpts[m.runCursor])
		}
		return m, nil
	}

	if m.busy {
		if k.String() == "q" {
			return m, tea.Quit
		}
		return m, nil
	}

	sel := m.selected()
	switch k.String() {
	case "q", "esc":
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
			return m.openProfileForm(formEditProfile, sel)
		}
	case "a":
		if sel != nil {
			return m.openRuleForm(sel)
		}
	case "enter", " ":
		if m.focusRight && m.tab == tabBackups && len(m.backups) > 0 {
			b := m.backups[m.rightCursor[tabBackups]]
			return m.confirmRestore(b.ID)
		}
		if sel != nil {
			return m.previewSwitch(sel.Name, m.focusedTool())
		}
	case "i":
		if sel != nil {
			return m.confirmImport(sel, m.focusedTool())
		}
	case "r":
		if sel != nil {
			m.runOpts = m.runChoices(sel)
			m.runCursor = 0
			m.modal = modalRun
		}
	case "U":
		return m.openUpdate()
	case "L":
		if sel != nil {
			return m.confirmLock(sel.Name)
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
		}
		if sel != nil {
			m.pend = &pending{kind: actDelete, profile: sel.Name, title: "Delete profile " + sel.Name + "?",
				lines: []string{"Removes the profile and every credential saved in it.", "Tools stay logged in as they are now."}}
			m.modal = modalConfirm
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

func (m Model) previewSwitch(name string, only []string) (tea.Model, tea.Cmd) {
	if m.dryRun {
		return m.execute(pending{kind: actSwitch, profile: name, only: only})
	}
	m.busy, m.busyLabel = true, "planning switch to "+name
	mgr := m.mgr
	title := "Switch " + m.scope(only) + " to " + name + "?"
	return m, func() tea.Msg {
		res, err := mgr.Use(context.Background(), name, core.UseOptions{DryRun: true, Only: only}, nil)
		if err != nil {
			return previewMsg{err: err}
		}
		p := pending{kind: actSwitch, profile: name, only: only, title: title}
		for _, e := range res.Events {
			switch e.State {
			case core.Done:
				p.lines = append(p.lines, "✓ "+e.Name+"  →  "+e.Msg)
			case core.Skipped:
				p.lines = append(p.lines, "○ "+e.Name+"  "+e.Msg)
			case core.Failed:
				p.lines = append(p.lines, "✗ "+e.Name+"  "+e.Msg)
			}
		}
		p.lines = append(p.lines, "", "An encrypted backup is taken first; any failure rolls back.")
		return previewMsg{p: p}
	}
}

func (m Model) confirmImport(p *config.Profile, only []string) (tea.Model, tea.Cmd) {
	title := "Import current logins into " + p.Name + "?"
	if len(only) == 1 {
		title = "Import the current " + m.toolName(only[0]) + " login into " + p.Name + "?"
	}
	pd := &pending{kind: actImport, profile: p.Name, only: only, title: title}
	for _, s := range m.status {
		if len(only) > 0 && !slices.Contains(only, s.ID) {
			continue
		}
		if s.Live.LoggedIn {
			pd.lines = append(pd.lines, "● "+s.Name+"  "+s.Live.Account)
		} else {
			pd.lines = append(pd.lines, "○ "+s.Name+"  (not logged in, skipped)")
		}
	}
	pd.lines = append(pd.lines, "", "Secrets go to the "+vaultLabel(m.mgr.Env.Vault.Backend())+"; only labels go to config.toml.")
	m.pend, m.modal = pd, modalConfirm
	return m, nil
}

// shows how to update for the way cream was installed, cream itself never downloads anything
func (m Model) openUpdate() (tea.Model, tea.Cmd) {
	switch {
	case !m.opts.UpdateCheck:
		m.toast(0, "the update check is off")
		return m, nil
	case m.upd == nil:
		m.toast(0, "no update information yet")
		return m, nil
	case !m.upd.Newer:
		m.toast(1, "cream is up to date")
		return m, nil
	}
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real // homebrew links bin/cream to the caskroom
	}
	home, _ := os.UserHomeDir()
	m.inst = update.DetectInstall(exe, runtime.GOOS, runtime.GOARCH, m.upd.Latest, home, os.Getenv)
	m.modal = modalUpdate
	return m, nil
}

// OSC 52 asks the terminal itself to set the clipboard, works over ssh too
func copyToClipboard(text string) error {
	seq := osc52.New(text)
	switch {
	case os.Getenv("TMUX") != "":
		seq = seq.Tmux()
	case os.Getenv("STY") != "":
		seq = seq.Screen()
	}
	_, err := seq.WriteTo(os.Stderr)
	return err
}

// a var so tests don't open a browser
var openURL = openInBrowser

// the url is always built by update.ReleaseURL, and no shell is involved
func openInBrowser(goos, url string) error {
	var cmd *exec.Cmd
	switch goos {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// profile "" means everything, it locks or unlocks depending on the current state
func (m Model) confirmLock(profile string) (tea.Model, tea.Cmd) {
	cfg := m.mgr.Cfg
	what := "everything"
	if profile != "" {
		what = profile
	}
	if profile != "" && cfg.Lock.All {
		m.toast(2, "everything is locked · ctrl+l to unlock")
		return m, nil
	}
	unlock := cfg.Lock.All
	if profile != "" {
		unlock = slices.Contains(cfg.Lock.Profiles, profile)
	}
	if unlock {
		m.pend = &pending{kind: actUnlock, profile: profile, title: "Unlock " + what + "?",
			lines: []string{"Logs the locked tools back in and turns git identity back on."}}
		if profile == "" && len(cfg.Lock.Profiles) > 0 {
			m.pend.lines = append(m.pend.lines, "Also unlocks "+strings.Join(cfg.Lock.Profiles, ", ")+".")
		}
	} else {
		lines := []string{"Logs out every tool and turns git identity off until you unlock.", "Logins are saved first, a backup is taken."}
		if profile != "" {
			lines = []string{"Logs out the tools signed in as " + profile + "'s accounts", "and turns its git rules off. " + profile + " can't be used until unlocked."}
		}
		m.pend = &pending{kind: actLock, profile: profile, title: "Lock " + what + "?",
			lines: append(lines, "", "Running Claude Code and Codex sessions keep working until restarted.")}
	}
	m.modal = modalConfirm
	return m, nil
}

func (m Model) confirmRestore(id string) (tea.Model, tea.Cmd) {
	label := "the latest backup"
	if id != "" {
		label = "backup " + id
	}
	m.pend = &pending{kind: actRestore, backupID: id, title: "Restore " + label + "?",
		lines: []string{"Every file captured in the backup is put back.", "Your current state is backed up first."}}
	m.modal = modalConfirm
	return m, nil
}

func (m Model) startSteps(only []string) Model {
	m.steps = nil
	for _, a := range m.mgr.Adapters {
		if len(only) > 0 && !slices.Contains(only, a.ID()) {
			continue
		}
		m.steps = append(m.steps, step{id: a.ID(), name: a.Name(), state: core.Pending})
	}
	m.bar.SetPercent(0)
	return m
}

func (m Model) execute(p pending) (tea.Model, tea.Cmd) {
	mgr, dry := m.mgr, m.dryRun
	switch p.kind {
	case actSwitch, actImport:
		m = m.startSteps(p.only)
		m.busy = true
		ch := make(chan tea.Msg, 32)
		m.opCh = ch
		verb := "switching " + m.scope(p.only) + " to "
		if p.kind == actImport {
			verb = "importing into "
		}
		m.busyLabel = verb + p.profile
		doneTitle := "now using " + p.profile
		if len(p.only) == 1 {
			doneTitle = m.toolName(p.only[0]) + " now uses " + p.profile
		}
		go func() {
			defer close(ch)
			progress := func(e core.Event) { ch <- eventMsg(e) }
			if p.kind == actSwitch {
				res, err := mgr.Use(context.Background(), p.profile, core.UseOptions{DryRun: dry, Only: p.only}, progress)
				done := opDoneMsg{title: doneTitle, err: err, dry: dry}
				if res != nil {
					done.events = res.Events
					for _, e := range res.Events {
						done.changes = append(done.changes, e.Changes...)
					}
				}
				if err == nil && !dry {
					done.events = nil // success: a toast is enough
				}
				ch <- done
				return
			}
			evs, err := mgr.Import(context.Background(), p.profile, p.only, dry, progress)
			done := opDoneMsg{title: "imported into " + p.profile, err: err, dry: dry, events: evs}
			for _, e := range evs {
				done.changes = append(done.changes, e.Changes...)
			}
			ch <- done
		}()
		return m, tea.Batch(waitOp(ch), m.bar.SetPercent(0))

	case actLock, actUnlock:
		m = m.startSteps(nil)
		m.busy = true
		ch := make(chan tea.Msg, 32)
		m.opCh = ch
		what := "everything"
		if p.profile != "" {
			what = p.profile
		}
		verb := "locked "
		if p.kind == actUnlock {
			verb = "unlocked "
		}
		m.busyLabel = strings.TrimSuffix(verb, "ed ") + "ing " + what
		go func() {
			defer close(ch)
			progress := func(e core.Event) { ch <- eventMsg(e) }
			var evs []core.Event
			var err error
			if p.kind == actLock {
				evs, err = mgr.Lock(context.Background(), p.profile, dry, progress)
			} else {
				evs, err = mgr.Unlock(context.Background(), p.profile, dry, progress)
			}
			done := opDoneMsg{title: verb + what, err: err, dry: dry, events: evs}
			for _, e := range evs {
				done.changes = append(done.changes, e.Changes...)
			}
			ch <- done
		}()
		return m, tea.Batch(waitOp(ch), m.bar.SetPercent(0))

	case actRestore:
		m.busy, m.busyLabel = true, "restoring"
		return m, func() tea.Msg {
			snap, err := mgr.Restore(context.Background(), p.backupID, dry)
			d := opDoneMsg{title: "backup restored", err: err, dry: dry}
			if err == nil && dry {
				for _, f := range snap.Files {
					d.changes = append(d.changes, adapter.Change{Kind: "file", Target: f.Path, Action: "restore"})
				}
			}
			return d
		}

	case actDelete:
		if dry {
			return m, func() tea.Msg {
				return opDoneMsg{title: "delete " + p.profile, dry: true, changes: []adapter.Change{
					{Kind: "keyring", Target: "profile:" + p.profile + ":*", Action: "delete"},
					{Kind: "file", Target: mgr.Paths.ConfigFile(), Action: "patch"}}}
			}
		}
		if err := mgr.RemoveProfile(p.profile); err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if m.cursor >= len(m.names()) {
			m.cursor = max(0, len(m.names())-1)
		}
		m.toast(1, "profile %s deleted", p.profile)
		cmd := m.loadStatus()
		return m, cmd

	case actDeleteRule:
		ch, err := mgr.RemoveRule(p.profile, p.ruleIdx, dry)
		if err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if dry {
			res := opDoneMsg{title: "remove rule", dry: true, changes: ch}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		if n := len(m.allRules()); m.rightCursor[tabRules] >= n {
			m.rightCursor[tabRules] = max(0, n-1)
		}
		m.toast(1, "rule removed")
		cmd := m.loadStatus()
		return m, cmd

	case actForget:
		changes, err := mgr.Forget(p.profile, p.only, dry)
		if err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if dry {
			res := opDoneMsg{title: "forget saved account", dry: true, changes: changes}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		m.toast(1, "%s forgot its %s account", p.profile, m.scope(p.only))
		cmd := m.loadStatus()
		return m, cmd
	}
	return m, nil
}

func formTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Base = t.Focused.Base.BorderForeground(cHi)
	t.Focused.Title = t.Focused.Title.Foreground(cAccent)
	t.Focused.Description = t.Focused.Description.Foreground(cDim)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(cHi)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(cGood)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(cHi)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(cAccent)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(cHi).Foreground(cInk)
	t.Blurred.Title = t.Blurred.Title.Foreground(cDim)
	return t
}

func (m Model) openProfileForm(kind formKind, p *config.Profile) (tea.Model, tea.Cmd) {
	fd := &formData{kind: kind}
	var fields []huh.Field
	if kind == formNewProfile {
		fields = append(fields, huh.NewInput().Title("Profile name").Description("e.g. work, personal").Value(&fd.name).
			Validate(func(s string) error {
				if err := config.ValidateName(s); err != nil {
					return err
				}
				if _, ok := m.mgr.Cfg.Profiles[s]; ok {
					return fmt.Errorf("profile %q already exists", s)
				}
				return nil
			}))
	} else {
		fd.profile, fd.desc = p.Name, p.Description
		fd.gName, fd.gEmail, fd.gKey = p.Git.Name, p.Git.Email, p.Git.SigningKey
	}
	fields = append(fields,
		huh.NewInput().Title("Description").Value(&fd.desc),
		huh.NewInput().Title("Git user.name").Description("commit author for repos matching this profile's rules").Value(&fd.gName),
		huh.NewInput().Title("Git user.email").Value(&fd.gEmail).Validate(func(s string) error {
			if s != "" && !strings.Contains(s, "@") {
				return fmt.Errorf("not an e-mail address")
			}
			return nil
		}),
		huh.NewInput().Title("Signing key (optional)").Value(&fd.gKey),
	)
	title := "New profile"
	if kind == formEditProfile {
		title = "Edit " + p.Name
	}
	m.fdata = fd
	m.form = huh.NewForm(huh.NewGroup(fields...).Title(title)).
		WithTheme(formTheme()).WithWidth(min(60, max(30, m.w-12))).WithShowHelp(true)
	m.modal = modalForm
	return m, m.form.Init()
}

func (m Model) openRuleForm(p *config.Profile) (tea.Model, tea.Cmd) {
	if p.Git.IsZero() {
		m.toast(2, "give %s a git identity first (press e)", p.Name)
		return m, nil
	}
	fd := &formData{kind: formRule, profile: p.Name, ruleKind: "remote"}
	m.fdata = fd
	m.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Use "+p.Git.Email+" for repos…").Options(
			huh.NewOption("with a remote on host", "remote"),
			huh.NewOption("inside a folder", "dir"),
		).Value(&fd.ruleKind),
		huh.NewInput().Title("Host or folder").Description("e.g. gitlab.company.com or ~/work").Value(&fd.rulePattern).
			Validate(func(s string) error {
				_, err := config.ValidateRule(config.Rule{Kind: fd.ruleKind, Pattern: s})
				return err
			}),
	).Title("New identity rule · " + p.Name)).WithTheme(formTheme()).WithWidth(min(60, max(30, m.w-12)))
	m.modal = modalForm
	return m, m.form.Init()
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
		m.modal, m.form = modalNone, nil
		return m, nil
	}
	fm, cmd := m.form.Update(msg)
	if f, ok := fm.(*huh.Form); ok {
		m.form = f
	}
	switch m.form.State {
	case huh.StateAborted:
		m.modal, m.form = modalNone, nil
		return m, nil
	case huh.StateCompleted:
		m.modal, m.form = modalNone, nil
		return m.submitForm()
	}
	return m, cmd
}

func (m Model) submitForm() (tea.Model, tea.Cmd) {
	fd := m.fdata
	git := config.GitIdentity{Name: strings.TrimSpace(fd.gName), Email: strings.TrimSpace(fd.gEmail), SigningKey: strings.TrimSpace(fd.gKey)}
	switch fd.kind {
	case formNewProfile:
		if m.dryRun {
			res := opDoneMsg{title: "new profile", dry: true, changes: []adapter.Change{{Kind: "file", Target: m.mgr.Paths.ConfigFile() + " (profiles." + fd.name + ")", Action: "add"}}}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		if _, err := m.mgr.AddProfile(fd.name, strings.TrimSpace(fd.desc), git); err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		for i, n := range m.names() {
			if n == fd.name {
				m.cursor = i
			}
		}
		m.toast(1, "profile %s created · log in to your tools, then press i", fd.name)
	case formEditProfile:
		p, err := m.mgr.Cfg.Profile(fd.profile)
		if err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if m.dryRun {
			res := opDoneMsg{title: "edit profile", dry: true, changes: []adapter.Change{{Kind: "file", Target: m.mgr.Paths.ConfigFile(), Action: "patch"}}}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		p.Description, p.Git = strings.TrimSpace(fd.desc), git
		if err := m.mgr.Save(); err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if len(p.Rules) > 0 {
			if _, err := m.mgr.SyncGit(false); err != nil {
				m.toast(2, "%s", err.Error())
				return m, nil
			}
		}
		m.toast(1, "profile %s saved", p.Name)
	case formRule:
		ch, err := m.mgr.AddRule(fd.profile, config.Rule{Kind: fd.ruleKind, Pattern: fd.rulePattern}, m.dryRun)
		if err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if m.dryRun {
			res := opDoneMsg{title: "add rule", dry: true, changes: ch}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		m.tab = tabRules
		m.toast(1, "rule added · ~/.gitconfig managed block updated")
	}
	cmd := m.loadStatus()
	return m, cmd
}

func (m Model) runChoices(p *config.Profile) []runChoice {
	var out []runChoice
	for _, c := range []struct{ label, bin, adapter string }{
		{"Claude Code", "claude", "claude"},
		{"Codex", "codex", "codex"},
		{"GitHub CLI", "gh", "github"},
		{"GitLab CLI", "glab", "gitlab"},
	} {
		if _, err := exec.LookPath(c.bin); err == nil && p.Accounts[c.adapter] != nil {
			out = append(out, runChoice{label: c.label, bin: c.bin})
		}
	}
	out = append(out, shellChoice())
	return out
}

func shellChoice() runChoice {
	if runtime.GOOS == "windows" {
		for _, s := range []string{"pwsh", "powershell"} {
			if _, err := exec.LookPath(s); err == nil {
				return runChoice{label: "Shell (" + s + ")", bin: s, args: []string{"-NoLogo"}}
			}
		}
		return runChoice{label: "Shell (cmd)", bin: "cmd"}
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	return runChoice{label: "Shell (" + sh + ")", bin: sh}
}

func (m Model) launch(c runChoice) (tea.Model, tea.Cmd) {
	p := m.selected()
	if p == nil {
		return m, nil
	}
	if m.dryRun {
		res := opDoneMsg{title: "run " + c.bin, dry: true, changes: []adapter.Change{{Kind: "exec", Target: strings.Join(append([]string{c.bin}, c.args...), " ") + " (profile " + p.Name + ")", Action: "run"}}}
		m.result, m.modal = &res, modalResult
		return m, nil
	}
	vars, finish, warns, err := m.mgr.SessionEnv(context.Background(), p.Name, c.bin)
	if err != nil {
		m.toast(2, "%s", err.Error())
		return m, nil
	}
	for _, w := range warns {
		m.toast(0, "%s", w)
	}
	cmd := exec.Command(c.bin, c.args...)
	cmd.Env = append(os.Environ(), vars...)
	label := c.label + " · " + p.Name
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		finish()
		return execDoneMsg{label: label, err: err}
	})
}
