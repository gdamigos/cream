package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/gdamigos/cream/internal/core"
)

var (
	errShortPassword    = fmt.Errorf("at least %d characters", core.MinPasswordLen)
	errPasswordMismatch = errors.New("doesn't match")
)

// runs next once the profile's password is entered, right away when it has none
// the password is asked for every action, opDoneMsg and the main key handler forget it again
func (m Model) withPassword(profile string, next func(Model) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	if !m.mgr.NeedsPassword(profile) {
		return next(m)
	}
	return m.openPasswordForm(profile, next)
}

func (m Model) openPasswordForm(profile string, next func(Model) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	fd := &formData{kind: formPassword, profile: profile}
	title := "Password for " + profile
	fields := []huh.Field{huh.NewInput().Title(title).EchoMode(huh.EchoModePassword).Value(&fd.pw)}
	m.fdata, m.formFields, m.formTitles, m.afterPw = fd, fields, []string{title}, next
	m.form = huh.NewForm(huh.NewGroup(fields...).Title(profile + " is password protected")).
		WithTheme(formTheme()).WithWidth(min(60, max(30, m.w-12)))
	m.modal = modalForm
	return m, m.form.Init()
}

func (m Model) submitPassword(fd *formData) (tea.Model, tea.Cmd) {
	pw, next := fd.pw, m.afterPw
	fd.pw, m.afterPw = "", nil
	if err := m.mgr.Unseal(fd.profile, pw); err != nil {
		m.toast(2, "%s", err.Error())
		return m, nil
	}
	if next == nil {
		return m, nil
	}
	return next(m)
}

// the password fields of the profile forms, protected profiles can also drop theirs
func passwordFields(fd *formData, protected bool) ([]huh.Field, []string) {
	title, desc := "Password (optional)", "protects the profile on a shared computer, at least 8 characters"
	if protected {
		title, desc = "New password", "empty keeps the current one"
	}
	fields := []huh.Field{
		huh.NewInput().Title(title).Description(desc).EchoMode(huh.EchoModePassword).Value(&fd.pw).
			Validate(func(s string) error {
				if s != "" && len(s) < core.MinPasswordLen {
					return errShortPassword
				}
				return nil
			}),
		huh.NewInput().Title("Repeat password").EchoMode(huh.EchoModePassword).Value(&fd.pwAgain).
			Validate(func(s string) error {
				if s != fd.pw {
					return errPasswordMismatch
				}
				return nil
			}),
	}
	titles := []string{title, "Repeat password"}
	if protected {
		fields = append(fields, huh.NewConfirm().Title("Remove the password").Description("decrypts the profile again").Value(&fd.removePw))
		titles = append(titles, "Remove the password")
	}
	return fields, titles
}
