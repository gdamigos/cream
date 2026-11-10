package cli

import (
	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/backup"
)

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
			var snap *backup.Snapshot
			err = a.withPasswords(m, func() (err error) { snap, err = m.Restore(cmd.Context(), id, a.DryRun); return err })
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
