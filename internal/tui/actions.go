package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

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

func (m Model) confirmFolder(profile string) (tea.Model, tea.Cmd) {
	if m.opts.Dir == "" {
		m.toast(2, "no folder to write .cream into, run cream init %s there", profile)
		return m, nil
	}
	path := filepath.Join(m.opts.Dir, config.FolderFile)
	lines := []string{"Writes " + path + ".", "Here and below, cream use and cream run without a profile use " + profile + "."}
	if _, err := os.Stat(path); err == nil {
		lines = append(lines, "Replaces the .cream already there.")
	}
	m.pend = &pending{kind: actFolder, profile: profile, title: "Pick " + profile + " for this folder?", lines: lines}
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
			done := opDoneMsg{title: verb + what, err: err, dry: dry, events: evs, again: &p}
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
			d := opDoneMsg{title: "backup restored", err: err, dry: dry, again: &p}
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

	case actFolder:
		path, err := mgr.InitFolder(m.opts.Dir, p.profile, nil, dry)
		if err != nil {
			m.toast(2, "%s", err.Error())
			return m, nil
		}
		if dry {
			res := opDoneMsg{title: "pick " + p.profile + " for this folder", dry: true,
				changes: []adapter.Change{{Kind: "file", Target: path, Action: "write"}}}
			m.result, m.modal = &res, modalResult
			return m, nil
		}
		m.opts.Folder = &config.Folder{Profile: p.profile, Path: path}
		m.toast(1, "this folder now uses %s", p.profile)
		return m, nil
	}
	return m, nil
}
