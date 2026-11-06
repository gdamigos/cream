package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

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
	var titles []string
	if kind == formNewProfile {
		titles = append(titles, "Profile name")
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
	titles = append(titles, "Description", "Git user.name", "Git user.email", "Signing key (optional)")
	title := "New profile"
	if kind == formEditProfile {
		title = "Edit " + p.Name
	}
	m.fdata, m.formFields, m.formTitles = fd, fields, titles
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
	fields := []huh.Field{
		huh.NewSelect[string]().Title("Use "+p.Git.Email+" for repos…").Options(
			huh.NewOption("with a remote on host", "remote"),
			huh.NewOption("inside a folder", "dir"),
		).Value(&fd.ruleKind),
		huh.NewInput().Title("Host or folder").Description("e.g. gitlab.company.com or ~/work").Value(&fd.rulePattern).
			Validate(func(s string) error {
				_, err := config.ValidateRule(config.Rule{Kind: fd.ruleKind, Pattern: s})
				return err
			}),
	}
	m.fdata, m.formFields, m.formTitles = fd, fields, []string{"Use " + p.Git.Email + " for repos", "Host or folder"}
	m.form = huh.NewForm(huh.NewGroup(fields...).Title("New identity rule · " + p.Name)).
		WithTheme(formTheme()).WithWidth(min(60, max(30, m.w-12)))
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
