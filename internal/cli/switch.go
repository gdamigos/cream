package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

func newImportCmd(a *App) *cobra.Command {
	var only, glHost string
	cmd := &cobra.Command{
		Use:   "import <profile>",
		Short: "Capture the currently logged-in account of every tool into a profile",
		Long: `Capture the accounts you are logged into right now into a profile, so
each account only ever needs one interactive login.

Secrets go to the OS keyring (or the encrypted file vault); only account
labels such as e-mail addresses are written to config.toml.`,
		Example: "  cream import work\n  cream import personal --only claude,codex",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			var p *config.Profile
			if err := a.withPasswords(m, func() (err error) { p, err = m.Profile(args[0]); return err }); err != nil {
				return err
			}
			if glHost != "" {
				if p.Accounts["gitlab"] == nil {
					p.Accounts["gitlab"] = &config.Account{}
				}
				p.Accounts["gitlab"].Host = glHost
			}
			a.dryBanner()
			a.printf("%s %s\n", sTitle.Render("Importing into"), sBold.Render(p.Name))
			evs, err := m.Import(cmd.Context(), p.Name, splitList(only), a.DryRun, nil)
			a.printEvents(evs)
			return err
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "comma-separated adapters (claude,codex,github,gitlab,git)")
	cmd.Flags().StringVar(&glHost, "gitlab-host", "", "GitLab host to import (default: glab's default host)")
	return cmd
}

func newUseCmd(a *App) *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:     "use <profile>",
		Aliases: []string{"switch"},
		Short:   "Globally switch every tool to a profile (backup first, rollback on failure)",
		Example: "  cream use work\n  cream use personal --dry-run",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if _, err := m.Cfg.Profile(args[0]); err != nil {
				return err
			}
			a.dryBanner()
			a.printf("%s %s\n", sTitle.Render("Switching to"), sBold.Render(args[0]))
			var res *core.UseResult
			err = a.withPasswords(m, func() (err error) {
				res, err = m.Use(cmd.Context(), args[0], core.UseOptions{DryRun: a.DryRun, Only: splitList(only)}, nil)
				return err
			})
			if res != nil {
				a.printEvents(res.Events)
				if a.DryRun {
					a.printf("  %s backup of all touched files would be written to %s\n", sDim.Render("→"), m.Paths.BackupDir())
				} else if res.BackupPath != "" && err == nil {
					a.printf("%s now using %s %s\n", sOK.Render("✓"), sBold.Render(args[0]), sDim.Render("(undo with `cream restore`)"))
					a.printf("%s\n", sDim.Render("  running Claude Code / Codex sessions keep their old account until restarted"))
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "comma-separated adapters to switch")
	return cmd
}

func newRunCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <profile> <command> [args...]",
		Short: "Launch one tool under a profile in this terminal only",
		Long: `Launch a command with a profile's accounts, without touching the global
state. Claude Code gets its own CLAUDE_CONFIG_DIR and Codex its own
CODEX_HOME, so two terminals can run different accounts in parallel.
GitHub and GitLab tokens are passed via the child's environment only.`,
		Example:            "  cream run work claude\n  cream run personal codex\n  cream run work -- gh pr list",
		Args:               cobra.MinimumNArgs(2),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			profile, argv := args[0], args[1:]
			if a.DryRun {
				a.dryBanner()
				var p *config.Profile
				if err := a.withPasswords(m, func() (err error) { p, err = m.Profile(profile); return err }); err != nil {
					return err
				}
				for _, ad := range m.Adapters {
					if p.Accounts[ad.ID()] != nil && ad.ID() != "git" {
						a.printf("  %s %s session env for %s\n", sDim.Render("→"), sBold.Render("set    "), ad.Name())
					}
				}
				a.printf("  %s %s %s\n", sDim.Render("→"), sBold.Render("exec   "), strings.Join(argv, " "))
				return nil
			}
			var vars, warns []string
			var finish func()
			err = a.withPasswords(m, func() (err error) {
				vars, finish, warns, err = m.SessionEnv(cmd.Context(), profile, argv[0])
				return err
			})
			if err != nil {
				return err
			}
			for _, w := range warns {
				fmt.Fprintln(a.Err, sWarn.Render("! ")+w)
			}
			bin, err := exec.LookPath(argv[0])
			if err != nil {
				return err
			}
			c := exec.Command(bin, argv[1:]...)
			c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
			c.Env = append(os.Environ(), vars...)
			// the child handles ctrl+c, we only wait for it
			signal.Ignore(os.Interrupt)
			defer signal.Reset(os.Interrupt)
			runErr := c.Run()
			finish()
			var ee *exec.ExitError
			if errors.As(runErr, &ee) {
				return &ExitError{Code: ee.ExitCode()}
			}
			return runErr
		},
	}
	cmd.Flags().SetInterspersed(false)
	return cmd
}
