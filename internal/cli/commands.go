package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

func (a *App) dryBanner() {
	if a.DryRun {
		a.printf("%s\n", sWarn.Render("DRY RUN · nothing will be changed"))
	}
}

func (a *App) printChanges(changes []adapter.Change) {
	for _, c := range changes {
		a.printf("  %s %s %s %s\n", sDim.Render("→"), sBold.Render(fmt.Sprintf("%-7s", c.Action)), sDim.Render(fmt.Sprintf("%-9s", c.Kind)), c.Target)
	}
}

func stateIcon(s core.State) string {
	switch s {
	case core.Done:
		return sOK.Render("✓")
	case core.Skipped:
		return sDim.Render("○")
	case core.Failed:
		return sErr.Render("✗")
	}
	return "·"
}

func (a *App) printEvents(evs []core.Event) {
	for _, e := range evs {
		a.printf("  %s %-14s %s\n", stateIcon(e.State), e.Name, sDim.Render(e.Msg))
		if a.DryRun {
			a.printChanges(e.Changes)
		}
	}
}

func newProfileCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Aliases: []string{"profiles", "p"}, Short: "Create, list, edit and remove profiles"}

	var desc, gName, gEmail, gKey string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			a.dryBanner()
			if a.DryRun {
				if err := config.ValidateName(args[0]); err != nil {
					return err
				}
				a.printChanges([]adapter.Change{{Kind: "file", Target: m.Paths.ConfigFile() + " (profiles." + args[0] + ")", Action: "add"}})
				return nil
			}
			p, err := m.AddProfile(args[0], desc, config.GitIdentity{Name: gName, Email: gEmail, SigningKey: gKey})
			if err != nil {
				return err
			}
			a.printf("%s profile %s created\n", sOK.Render("✓"), sBold.Render(p.Name))
			a.printf("%s\n", sDim.Render("  next: log in to your tools, then `cream import "+p.Name+"`"))
			return nil
		},
	}
	add.Flags().StringVarP(&desc, "description", "d", "", "short description")
	add.Flags().StringVar(&gName, "git-name", "", "git user.name for this profile")
	add.Flags().StringVar(&gEmail, "git-email", "", "git user.email for this profile")
	add.Flags().StringVar(&gKey, "signing-key", "", "git user.signingkey for this profile")

	set := &cobra.Command{
		Use:   "set <name>",
		Short: "Edit a profile's description or git identity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			p, err := m.Cfg.Profile(args[0])
			if err != nil {
				return err
			}
			f := cmd.Flags()
			if f.Changed("description") {
				p.Description = desc
			}
			if f.Changed("git-name") {
				p.Git.Name = gName
			}
			if f.Changed("git-email") {
				p.Git.Email = gEmail
			}
			if f.Changed("signing-key") {
				p.Git.SigningKey = gKey
			}
			a.dryBanner()
			changes, err := m.SyncGit(true)
			if err != nil {
				return err
			}
			if a.DryRun {
				a.printChanges(append([]adapter.Change{{Kind: "file", Target: m.Paths.ConfigFile(), Action: "patch"}}, onlyIncludeFiles(changes, p.Name)...))
				return nil
			}
			if err := m.Save(); err != nil {
				return err
			}
			if len(p.Rules) > 0 {
				if _, err := m.SyncGit(false); err != nil {
					return err
				}
			}
			a.printf("%s profile %s updated\n", sOK.Render("✓"), sBold.Render(p.Name))
			return nil
		},
	}
	set.Flags().StringVarP(&desc, "description", "d", "", "short description")
	set.Flags().StringVar(&gName, "git-name", "", "git user.name")
	set.Flags().StringVar(&gEmail, "git-email", "", "git user.email")
	set.Flags().StringVar(&gKey, "signing-key", "", "git user.signingkey")

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if len(m.Cfg.Profiles) == 0 {
				a.printf("%s\n", sDim.Render("no profiles yet - create one with `cream profile add work`"))
				return nil
			}
			for _, n := range m.Cfg.Names() {
				p := m.Cfg.Profiles[n]
				mark := "  "
				if n == m.Cfg.Active {
					mark = sOK.Render("● ")
				}
				a.printf("%s%s %s\n", mark, sBold.Render(n), sDim.Render(p.Description))
				for _, ad := range m.Adapters {
					if acct := p.Accounts[ad.ID()]; acct != nil {
						a.printf("    %-14s %s\n", sDim.Render(ad.Name()), acct.Label)
					}
				}
				if len(p.Rules) > 0 {
					var rs []string
					for _, r := range p.Rules {
						rs = append(rs, r.String())
					}
					a.printf("    %-14s %s\n", sDim.Render("rules"), strings.Join(rs, ", "))
				}
			}
			return nil
		},
	}

	var yes bool
	rm := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm", "delete"},
		Short:   "Delete a profile and its saved credentials",
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
			if a.DryRun {
				var ch []adapter.Change
				for _, ad := range m.Adapters {
					ch = append(ch, adapter.Change{Kind: "keyring", Target: "profile:" + args[0] + ":" + ad.ID(), Action: "delete"})
				}
				ch = append(ch, adapter.Change{Kind: "file", Target: m.Paths.ConfigFile() + " (profiles." + args[0] + ")", Action: "remove"})
				a.printChanges(ch)
				return nil
			}
			if !yes {
				return errors.New("this deletes the profile's saved credentials; re-run with --yes to confirm")
			}
			if err := m.RemoveProfile(args[0]); err != nil {
				return err
			}
			a.printf("%s profile %s removed\n", sOK.Render("✓"), sBold.Render(args[0]))
			return nil
		},
	}
	rm.Flags().BoolVarP(&yes, "yes", "y", false, "confirm deletion")

	cmd.AddCommand(add, set, list, rm)
	return cmd
}

func onlyIncludeFiles(ch []adapter.Change, profile string) []adapter.Change {
	var out []adapter.Change
	for _, c := range ch {
		if strings.Contains(c.Target, profile+".gitconfig") {
			out = append(out, c)
		}
	}
	return out
}

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
			p, err := m.Cfg.Profile(args[0])
			if err != nil {
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
			res, err := m.Use(cmd.Context(), args[0], core.UseOptions{DryRun: a.DryRun, Only: splitList(only)}, nil)
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
				p, err := m.Cfg.Profile(profile)
				if err != nil {
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
			vars, finish, warns, err := m.SessionEnv(cmd.Context(), profile, argv[0])
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

func newStatusCmd(a *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the active account per tool",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if asJSON {
				return a.printStatusJSON(cmd.Context(), m)
			}
			return a.printStatus(cmd.Context(), m, true)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func (a *App) printStatus(ctx context.Context, m *core.Manager, notify bool) error {
	active := m.Cfg.Active
	if active == "" {
		active = sDim.Render("(none)")
	} else {
		active = sBold.Render(active)
	}
	a.printf("%s %s   %s %s\n", sTitle.Render("CRE.A.M"), sDim.Render("active profile:"), active, sDim.Render("· vault: "+m.Env.Vault.Backend()))
	for _, r := range m.Status(ctx) {
		icon := sDim.Render("○")
		acct := sDim.Render("not logged in")
		if !r.Live.Installed {
			acct = sDim.Render("not installed")
		}
		if r.Live.LoggedIn {
			icon, acct = sOK.Render("●"), r.Live.Account
		}
		match := ""
		if r.Expected != "" && !r.Match {
			match = sWarn.Render("  ≠ profile expects " + r.Expected)
		}
		a.printf("  %s %-14s %s%s\n", icon, r.Name, acct, match)
		if r.Live.Warning != "" {
			a.printf("    %s\n", sWarn.Render("! "+r.Live.Warning))
		}
	}
	if notify && a.UpdateCheckAllowed(m) {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if res, err := a.UpdateChecker(m).Check(ctx); err == nil && res.Newer {
			fmt.Fprintf(a.Err, "%s\n", sDim.Render("update available: "+res.Latest+" (update via scoop / brew / apt or "+res.URL+")"))
		}
	}
	return nil
}

func (a *App) printStatusJSON(ctx context.Context, m *core.Manager) error {
	type row struct {
		Adapter   string `json:"adapter"`
		Installed bool   `json:"installed"`
		LoggedIn  bool   `json:"logged_in"`
		Account   string `json:"account,omitempty"`
		Expected  string `json:"expected,omitempty"`
		Match     bool   `json:"match"`
		Warning   string `json:"warning,omitempty"`
	}
	out := struct {
		Active   string `json:"active"`
		Vault    string `json:"vault"`
		Adapters []row  `json:"adapters"`
	}{Active: m.Cfg.Active, Vault: m.Env.Vault.Backend()}
	for _, r := range m.Status(ctx) {
		out.Adapters = append(out.Adapters, row{r.ID, r.Live.Installed, r.Live.LoggedIn, r.Live.Account, r.Expected, r.Match, r.Live.Warning})
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func newRuleCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "rule", Aliases: []string{"rules"}, Short: "Git identity rules (folder / remote host → profile)"}
	var dir, remote string
	add := &cobra.Command{
		Use:   "add <profile> (--dir <path> | --remote <host>)",
		Short: "Use the profile's git identity for repos in a folder or with a remote host",
		Long: `Add an identity rule. CRE.A.M writes a managed includeIf block at the end of
~/.gitconfig and sets user.useConfigOnly = true, so a repository that
matches no rule cannot commit with a wrong identity. Requires git 2.36+
for --remote rules.`,
		Example: "  cream rule add work --remote gitlab.company.com\n  cream rule add personal --dir ~/code/personal",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			var r config.Rule
			switch {
			case dir != "" && remote != "":
				return errors.New("use either --dir or --remote, not both")
			case dir != "":
				r = config.Rule{Kind: "dir", Pattern: dir}
			case remote != "":
				r = config.Rule{Kind: "remote", Pattern: remote}
			default:
				return errors.New("need --dir <path> or --remote <host>")
			}
			a.dryBanner()
			changes, err := m.AddRule(args[0], r, a.DryRun)
			if err != nil {
				return err
			}
			if a.DryRun {
				a.printChanges(changes)
				return nil
			}
			a.printf("%s rule added to %s\n", sOK.Render("✓"), sBold.Render(args[0]))
			return nil
		},
	}
	add.Flags().StringVar(&dir, "dir", "", "folder whose repos use this identity (e.g. ~/work)")
	add.Flags().StringVar(&remote, "remote", "", "remote host whose repos use this identity (e.g. gitlab.company.com)")

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List identity rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			n := 0
			for _, name := range m.Cfg.Names() {
				p := m.Cfg.Profiles[name]
				for i, r := range p.Rules {
					a.printf("  %s #%d  %-6s %-40s → %s\n", sBold.Render(name), i+1, r.Kind, r.Pattern, sDim.Render(p.Git.Email))
					n++
				}
			}
			if n == 0 {
				a.printf("%s\n", sDim.Render("no rules yet - e.g. `cream rule add work --remote gitlab.company.com`"))
			}
			return nil
		},
	}
	rm := &cobra.Command{
		Use:     "remove <profile> <number>",
		Aliases: []string{"rm"},
		Short:   "Remove a rule (numbers from `cream rule list`)",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(strings.TrimPrefix(args[1], "#"))
			if err != nil {
				return fmt.Errorf("rule number: %w", err)
			}
			a.dryBanner()
			changes, err := m.RemoveRule(args[0], n-1, a.DryRun)
			if err != nil {
				return err
			}
			if a.DryRun {
				a.printChanges(changes)
				return nil
			}
			a.printf("%s rule #%d removed from %s\n", sOK.Render("✓"), n, sBold.Render(args[0]))
			return nil
		},
	}
	cmd.AddCommand(add, list, rm)
	return cmd
}

func newRestoreCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "restore [backup-id]",
		Short: "Roll back the last switch (or a specific backup)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			a.dryBanner()
			snap, err := m.Restore(cmd.Context(), id, a.DryRun)
			if err != nil {
				return err
			}
			if a.DryRun {
				for _, f := range snap.Files {
					act := "restore"
					if !f.Exists {
						act = "delete"
					}
					a.printChanges([]adapter.Change{{Kind: "file", Target: f.Path, Action: act}})
				}
				for ad := range snap.Extra {
					a.printChanges([]adapter.Change{{Kind: "keyring", Target: ad + " keychain item", Action: "restore"}})
				}
				return nil
			}
			from := snap.From
			if from == "" {
				from = "(no profile)"
			}
			a.printf("%s restored backup %s · active profile: %s\n", sOK.Render("✓"), sDim.Render(snap.ID), sBold.Render(from))
			return nil
		},
	}
}

func newBackupsCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "backups",
		Short: "List encrypted backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			list, err := m.Backups.List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				a.printf("%s\n", sDim.Render("no backups yet"))
			}
			for _, b := range list {
				a.printf("  %s  %s  %s\n", sBold.Render(b.ID), b.Created.Local().Format("2006-01-02 15:04:05"), sDim.Render(b.Reason))
			}
			return nil
		},
	}
}

func newVersionCmd(a *App) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			a.printf("cream %s (commit %s, built %s)\n", a.Build.Version, a.Build.Commit, a.Build.Date)
			if !check {
				return nil
			}
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if !a.UpdateCheckAllowed(m) {
				a.printf("%s\n", sDim.Render("update check disabled"))
				return nil
			}
			res, err := a.UpdateChecker(m).Check(cmd.Context())
			if err != nil {
				return err
			}
			if res.Newer {
				a.printf("%s %s is available: %s\n", sWarn.Render("↑"), res.Latest, res.URL)
			} else {
				a.printf("%s up to date\n", sOK.Render("✓"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check GitHub Releases for a newer version (notify only)")
	return cmd
}
