package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

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

	var forgetOnly string
	var forgetYes bool
	forget := &cobra.Command{
		Use:     "forget <name> --only <tools>",
		Short:   "Drop saved accounts from a profile, keeping the profile",
		Long:    "Delete the accounts a profile saved for some tools, e.g. after importing the wrong login.\nThe tools themselves stay logged in as they are now.",
		Example: "  cream profile forget work --only claude --yes",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			only := splitList(forgetOnly)
			if len(only) == 0 {
				return errors.New("need --only <tools>, e.g. --only claude (use `cream profile remove` to delete the whole profile)")
			}
			a.dryBanner()
			if !a.DryRun && !forgetYes {
				return errors.New("this deletes saved credentials; re-run with --yes to confirm")
			}
			changes, err := m.Forget(args[0], only, a.DryRun)
			if err != nil {
				return err
			}
			if a.DryRun {
				a.printChanges(changes)
				return nil
			}
			a.printf("%s %s forgot %s\n", sOK.Render("✓"), sBold.Render(args[0]), strings.Join(only, ", "))
			return nil
		},
	}
	forget.Flags().StringVar(&forgetOnly, "only", "", "comma-separated adapters (claude,codex,github,gitlab,git)")
	forget.Flags().BoolVarP(&forgetYes, "yes", "y", false, "confirm deletion")

	cmd.AddCommand(add, set, list, rm, forget)
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
