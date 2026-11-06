package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdamigos/cream/internal/update"
)

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

func (m Model) viewUpdate() string {
	if m.upd == nil {
		return ""
	}
	lines := []string{
		sAccent.Render("cream " + m.upd.Latest + " is available"),
		sDim.Render("you have " + m.opts.Version + " · installed with " + m.inst.Label),
		"",
	}
	key := func(k, desc string) string { return sKey.Render(k) + sKeyDesc.Render(" "+desc) }
	if m.inst.Command != "" {
		// wrapped for display only, c copies it as one line
		cmd := ansi.Wrap(m.inst.Command, max(20, min(76, m.w-16)), " ")
		lines = append(lines, sDim.Render("Quit cream and run:"), "", sBold.Render(cmd), "",
			key("c", "copy the command")+"   "+key("o", "open the release page"))
	} else {
		lines = append(lines, sDim.Render("Download the new version from the release page."), "",
			key("o", "open the release page")+"   "+key("c", "copy the link"))
	}
	lines = append(lines, key("esc", "close"), "", sFaint.Render("cream never downloads or installs anything itself"))
	return sModal.Render(strings.Join(lines, "\n"))
}
