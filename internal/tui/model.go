package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

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
	final, err := p.Run()
	resetTitle(final)
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
	armed        armedHint // see click_twice.go

	busy      bool
	busyLabel string
	spin      spinner.Model
	bar       progress.Model
	steps     []step
	opCh      chan tea.Msg

	modal      modalKind
	pend       *pending
	form       *huh.Form
	fdata      *formData
	formFields []huh.Field
	formTitles []string
	runOpts    []runChoice
	runCursor  int
	result     *opDoneMsg

	toasts  []toast
	upd     *update.Result
	inst    update.Install // how this binary was installed, filled when the update window opens
	tagSeed uint64
	title   string // last terminal title sent
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
		m, cmd := m.syncTitle()
		return m, tea.Batch(tick(), cmd)

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
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			return m.handleClick(msg.X, msg.Y)
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
		m.armed = armedHint{}
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
