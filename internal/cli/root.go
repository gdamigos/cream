// every tui action must also exist here
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/gdamigos/cream/internal/core"
	"github.com/gdamigos/cream/internal/update"
)

type BuildInfo struct {
	Version, Commit, Date string
}

type App struct {
	Build         BuildInfo
	DryRun        bool
	NoUpdateCheck bool
	Out           io.Writer
	Err           io.Writer
	Open          func() (*core.Manager, error)
	// injected by main to avoid an import cycle with tui
	RunTUI func(*App, *core.Manager) error
	// asks for a profile password, nil reads CREAM_PROFILE_PASSWORD or the terminal
	ReadPassword func(prompt string) (string, error)

	mgr *core.Manager
}

func (a *App) Manager() (*core.Manager, error) {
	if a.mgr != nil {
		return a.mgr, nil
	}
	m, err := a.Open()
	if err != nil {
		return nil, err
	}
	a.mgr = m
	return m, nil
}

func (a *App) UpdateCheckAllowed(m *core.Manager) bool {
	return !a.NoUpdateCheck && !update.DisabledByEnv() && m.Cfg.UpdateCheckEnabled()
}

func (a *App) UpdateChecker(m *core.Manager) *update.Checker {
	return &update.Checker{CachePath: m.Paths.UpdateCache(), Current: a.Build.Version}
}

func DefaultOpen() (*core.Manager, error) {
	return core.Open(core.Options{Passphrase: promptPassphrase})
}

func promptPassphrase() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("no OS keyring available; set CREAM_VAULT_PASSPHRASE to unlock the encrypted file vault")
	}
	fmt.Fprint(os.Stderr, "CRE.A.M vault passphrase (no OS keyring found): ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var (
	text   = lipgloss.Color("#FAF6EC")
	accent = lipgloss.Color("#4FB3A9")
	good   = lipgloss.Color("#7BD88F")
	bad    = lipgloss.Color("#F2777A")
	dim    = lipgloss.Color("#8A9796")

	sTitle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	sOK    = lipgloss.NewStyle().Foreground(good)
	sErr   = lipgloss.NewStyle().Foreground(bad)
	sDim   = lipgloss.NewStyle().Foreground(dim)
	sBold  = lipgloss.NewStyle().Bold(true).Foreground(text)
	sWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8C468"))
)

func (a *App) printf(format string, args ...any) { fmt.Fprintf(a.Out, format, args...) }

func NewRoot(a *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "cream",
		Short: "CRE.A.M - one profile switch for every CLI login you own",
		Long: `CRE.A.M (CREdentiAls Master) switches Claude Code, Codex, GitHub CLI,
GitLab CLI and your git identity between profiles such as "work" and
"personal" - with one keypress in the TUI or one command in scripts.

Run without arguments to open the full-screen TUI.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       a.Build.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
				return a.printStatus(cmd.Context(), m, false)
			}
			return a.RunTUI(a, m)
		},
	}
	root.SetVersionTemplate(fmt.Sprintf("cream {{.Version}} (commit %s, built %s, %s/%s)\n", a.Build.Commit, a.Build.Date, runtime.GOOS, runtime.GOARCH))
	root.PersistentFlags().BoolVarP(&a.DryRun, "dry-run", "n", false, "show exactly which files and keyring entries would change, change nothing")
	root.PersistentFlags().BoolVar(&a.NoUpdateCheck, "no-update-check", false, "skip the GitHub Releases update check (also CREAM_NO_UPDATE_CHECK=1)")
	root.SetOut(a.Out)
	root.SetErr(a.Err)

	root.AddCommand(
		newProfileCmd(a),
		newImportCmd(a),
		newUseCmd(a),
		newRunCmd(a),
		newLockCmd(a, true),
		newLockCmd(a, false),
		newStatusCmd(a),
		newUsageCmd(a),
		newDoctorCmd(a),
		newRuleCmd(a),
		newRestoreCmd(a),
		newBackupsCmd(a),
		newVersionCmd(a),
	)
	return root
}

func Execute(ctx context.Context, a *App, args []string) int {
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	if a.Open == nil {
		a.Open = DefaultOpen
	}
	root := NewRoot(a)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			return ee.Code
		}
		fmt.Fprintln(a.Err, sErr.Render("✗ ")+err.Error())
		return 1
	}
	return 0
}

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
