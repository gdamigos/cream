package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

// how often the tui asks again, the usage package also caches each answer for 5 minutes
const usageEvery = time.Minute

type usageMsg []core.Usage

// counts as a status read so config.toml isn't reloaded under it
func (m *Model) loadUsage() tea.Cmd {
	if !m.mgr.Cfg.UsageCheck || time.Since(m.usageAt) < usageEvery {
		return nil
	}
	m.usageAt = time.Now()
	m.statusLoads++
	mgr := m.mgr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return usageMsg(mgr.Usage(ctx, nil, false))
	}
}

// keeps the new usage and toasts each limit the first time it shows up
func (m Model) gotUsage(us []core.Usage) Model {
	m.statusLoads = max(0, m.statusLoads-1)
	m.usage, m.suggest = us, core.Suggest(us)
	if m.suggested == nil {
		m.suggested = map[string]bool{}
	}
	for _, s := range m.suggest {
		k := s.Adapter + ":" + s.From + ":" + s.To
		if m.suggested[k] {
			continue
		}
		m.suggested[k] = true
		until := ""
		if !s.Until.IsZero() {
			until = " until " + s.Until.Local().Format("15:04")
		}
		m.toast(0, "%s on %s is at its limit%s · S switches it to %s", s.Name, s.From, until, s.To)
	}
	return m
}

// S opens the switch preview for the first tool at its limit
func (m Model) switchLimited() (tea.Model, tea.Cmd) {
	if len(m.suggest) == 0 {
		m.toast(0, "no tool is at its limit")
		return m, nil
	}
	s := m.suggest[0]
	return m.previewSwitch(s.To, []string{s.Adapter})
}

// profile card rows: stale saved logins and plan usage of the profile
func (m Model) usageRows(p *config.Profile, label func(string) string) []string {
	var rows []string
	now := time.Now()
	for _, s := range m.mgr.StaleLogins(now) {
		if s.Profile == p.Name {
			rows = append(rows, label(s.Name)+sWarn.Render(fmt.Sprintf("! last refreshed %d days ago, may have expired", s.Days(now))))
		}
	}
	var checked time.Time
	shown := false
	for _, u := range m.usage {
		if u.Profile != p.Name {
			continue
		}
		if u.Err != "" {
			rows = append(rows, label(u.Name)+sFaint.Render(u.Err))
			continue
		}
		if u.Result == nil || len(u.Result.Windows) == 0 {
			continue
		}
		// one line per limit window, the tool's name only on the first
		for i, w := range u.Result.Windows {
			name := ""
			if i == 0 {
				name = u.Name
			}
			rows = append(rows, label(name)+windowText(w.Name, w.Used, w.ResetsAt, now))
		}
		shown = true
		if c := u.Result.CheckedAt; !c.IsZero() && (checked.IsZero() || c.Before(checked)) {
			checked = c
		}
	}
	if shown {
		src := "plan usage from Anthropic and OpenAI"
		if !checked.IsZero() {
			src += ", checked " + checked.Local().Format("15:04")
		}
		rows = append(rows, label("")+sFaint.Render(src))
	}
	return rows
}

// "24% of the 5-hour limit used · resets 06:40", or "5-hour limit reached" at 100%
func windowText(name string, used float64, resets, now time.Time) string {
	text := sBase.Render(fmt.Sprintf("%.0f%% of the %s limit used", used, windowLabel(name)))
	if used >= 100 {
		text = sWarn.Render(windowLabel(name) + " limit reached")
	}
	if !resets.IsZero() {
		at := resets.Local().Format("15:04")
		if resets.Sub(now) >= 20*time.Hour {
			at = resets.Local().Format("Mon 15:04")
		}
		text += sFaint.Render(" · resets " + at)
	}
	return text
}

// 5h is 5-hour, 7d is weekly, 7d opus is weekly Opus, 30d is 30-day
func windowLabel(name string) string {
	base, model, _ := strings.Cut(name, " ")
	n := base[:max(0, len(base)-1)]
	s := base
	switch {
	case base == "7d":
		s = "weekly"
	case strings.HasSuffix(base, "h"):
		s = n + "-hour"
	case strings.HasSuffix(base, "d"):
		s = n + "-day"
	case strings.HasSuffix(base, "m"):
		s = n + "-minute"
	}
	if model != "" {
		s += " " + strings.ToUpper(model[:1]) + model[1:]
	}
	return s
}
