package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

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
			var changes []adapter.Change
			err = a.withPasswords(m, func() (err error) { changes, err = m.AddRule(args[0], r, a.DryRun); return err })
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
				if p.Protected() {
					continue
				}
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
			var changes []adapter.Change
			err = a.withPasswords(m, func() (err error) { changes, err = m.RemoveRule(args[0], n-1, a.DryRun); return err })
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
