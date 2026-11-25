package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

// the on/off settings of config.toml, the General list in the settings window
var settingToggles = []struct {
	id, label string
	get       func(*config.Config) bool
	set       func(*config.Config, bool)
}{
	{"update_check", "update check (from the next start)", (*config.Config).UpdateCheckEnabled,
		func(c *config.Config, v bool) { c.UpdateCheck = &v }},
	{"usage_check", "plan usage check (sends each token to its provider)", func(c *config.Config) bool { return c.UsageCheck },
		func(c *config.Config, v bool) { c.UsageCheck = v }},
	{"log", "activity log", (*config.Config).LogEnabled,
		func(c *config.Config, v bool) { c.Log = &v }},
	{"git.strict", "git strict (no fallback identity once rules exist)", (*config.Config).GitStrict,
		func(c *config.Config, v bool) { c.Git.Strict = &v }},
	{"ui.intro", "intro animation", func(c *config.Config) bool { return c.UI.Intro },
		func(c *config.Config, v bool) { c.UI.Intro = v }},
	{"ui.dynamic_title", "profile in the tab title", func(c *config.Config) bool { return c.UI.DynamicTitle },
		func(c *config.Config, v bool) { c.UI.DynamicTitle = v }},
	{"ui.click_twice", "click a key hint twice to run it", func(c *config.Config) bool { return c.UI.ClickTwice },
		func(c *config.Config, v bool) { c.UI.ClickTwice = v }},
}

type settingsData struct {
	tools, on       []string
	staleDays, keys string
}

func (m Model) openSettings() (tea.Model, tea.Cmd) {
	cfg := m.mgr.Cfg
	fd := &formData{kind: formSettings}
	s := &fd.set
	s.tools = slices.Clone(cfg.Adapters)
	s.staleDays = strconv.Itoa(cfg.StaleLoginDays())
	s.keys = strings.Join(cfg.UI.DisabledKeys, " ")
	var tools, toggles []huh.Option[string]
	for _, a := range m.mgr.AllAdapters() {
		tools = append(tools, huh.NewOption(a.Name(), a.ID()))
	}
	for _, t := range settingToggles {
		toggles = append(toggles, huh.NewOption(t.label, t.id))
		if t.get(cfg) {
			s.on = append(s.on, t.id)
		}
	}
	titles := []string{"Adapters", "General", "Stale login warning", "Disabled keys"}
	fields := []huh.Field{
		huh.NewMultiSelect[string]().Title(titles[0]).Options(tools...).Filterable(false).Value(&s.tools),
		huh.NewMultiSelect[string]().Title(titles[1]).Options(toggles...).Filterable(false).Value(&s.on),
		huh.NewInput().Title(titles[2]).Description("days without a refresh, 0 turns it off").Value(&s.staleDays).
			Validate(func(v string) error {
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err != nil || n < 0 {
					return fmt.Errorf("a whole number of days, 0 or more")
				}
				return nil
			}),
		huh.NewInput().Title(titles[3]).Description("keys that do nothing on the main screen, e.g. x ctrl+l").Value(&s.keys),
	}
	m.fdata, m.formFields, m.formTitles = fd, fields, titles
	return m, m.showForm(huh.NewGroup(fields...).Title("Settings").Description("saved to config.toml"))
}

func (m Model) submitSettings(s settingsData) (tea.Model, tea.Cmd) {
	if m.dryRun {
		res := opDoneMsg{title: "settings", dry: true, changes: []adapter.Change{{Kind: "file", Target: m.mgr.Paths.ConfigFile(), Action: "patch"}}}
		m.result, m.modal = &res, modalResult
		return m, nil
	}
	days, _ := strconv.Atoi(strings.TrimSpace(s.staleDays))
	err := m.mgr.EditSettings(func(c *config.Config) {
		c.Adapters = slices.Clone(s.tools)
		c.StaleDays = &days
		c.UI.DisabledKeys = strings.Fields(s.keys)
		for _, t := range settingToggles {
			t.set(c, slices.Contains(s.on, t.id))
		}
	})
	if err != nil {
		m.toast(2, "%s", firstLine(err.Error()))
		return m, nil
	}
	if !m.mgr.Cfg.UpdateCheckEnabled() {
		m.opts.UpdateCheck = false
	}
	if !m.mgr.Cfg.UsageCheck {
		m.usage, m.suggest = nil, nil
	}
	m.toast(1, "settings saved")
	cmd := m.loadStatus()
	return m, cmd
}
