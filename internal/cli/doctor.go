package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/core"
)

func newDoctorCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check every tool's setup and the current repo's git identity",
		Long: `Check that each tool is set up and logged in as the active profile expects,
that every saved account is still in the vault, that the managed ~/.gitconfig
block is up to date, and, inside a git repo, that commits here use the email of
the profile that owns the repo, and that a .cream file here names a real
profile. Read only, and exits with 1 when something is broken.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			dir, _ := os.Getwd()
			checks := m.Doctor(cmd.Context(), dir)
			titles := map[string]string{"tools": "Tools", "accounts": "Saved accounts", "git": "Git", "repo": "This repo", "folder": "This folder"}
			failed, area := 0, ""
			for _, c := range checks {
				if c.Area != area {
					area = c.Area
					a.printf("%s\n", sTitle.Render(titles[area]))
				}
				icon := sOK.Render("✓")
				msg := c.Msg
				switch c.State {
				case core.CheckInfo:
					icon, msg = sDim.Render("○"), sDim.Render(c.Msg)
				case core.CheckWarn:
					icon = sWarn.Render("!")
				case core.CheckFail:
					icon = sErr.Render("✗")
					failed++
				}
				a.printf("  %s %-16s %s\n", icon, c.Name, msg)
				if c.Fix != "" {
					a.printf("    %s %s\n", sDim.Render("fix:"), c.Fix)
				}
			}
			if failed > 0 {
				a.printf("\n%s\n", sErr.Render(fmt.Sprintf("%d problem(s) found", failed)))
				return &ExitError{Code: 1}
			}
			a.printf("\n%s\n", sOK.Render("✓ no problems found"))
			return nil
		},
	}
}
