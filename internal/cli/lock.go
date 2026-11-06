package cli

import (
	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/core"
)

func newLockCmd(a *App, lock bool) *cobra.Command {
	use, short, example := "unlock [profile]", "Log the locked tools back in and turn git identity back on", "  cream unlock\n  cream unlock work"
	if lock {
		use, short, example = "lock [profile]", "Log every tool out on purpose and turn git identity off, until `cream unlock`", "  cream lock\n  cream lock work"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `.

Without a profile it covers every tool. With one it only covers the tools
logged in as that profile's accounts, and the profile can't be used until it is
unlocked. Logins are saved to the vault first and an encrypted backup is taken.
It is a guardrail against mistakes, not a security boundary.`,
		Example: example,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			profile := ""
			if len(args) == 1 {
				profile = args[0]
			}
			a.dryBanner()
			what := "everything"
			if profile != "" {
				what = profile
			}
			var evs []core.Event
			if lock {
				a.printf("%s %s\n", sTitle.Render("Locking"), sBold.Render(what))
				evs, err = m.Lock(cmd.Context(), profile, a.DryRun, nil)
			} else {
				a.printf("%s %s\n", sTitle.Render("Unlocking"), sBold.Render(what))
				evs, err = m.Unlock(cmd.Context(), profile, a.DryRun, nil)
			}
			a.printEvents(evs)
			if err == nil && !a.DryRun {
				if lock {
					a.printf("%s locked %s %s\n", sOK.Render("✓"), sBold.Render(what), sDim.Render("(undo with `cream unlock`)"))
					a.printf("%s\n", sDim.Render("  running Claude Code / Codex sessions keep working until restarted"))
				} else {
					a.printf("%s unlocked %s\n", sOK.Render("✓"), sBold.Render(what))
				}
			}
			return err
		},
	}
}
