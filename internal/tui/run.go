package tui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

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
