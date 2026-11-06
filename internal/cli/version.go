package cli

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/update"
)

// the update command for how this binary was installed, or the release page
func updateHint(latest string) string {
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	home, _ := os.UserHomeDir()
	in := update.DetectInstall(exe, runtime.GOOS, runtime.GOARCH, latest, home, os.Getenv)
	if in.Command != "" {
		return "run `" + in.Command + "`"
	}
	return update.ReleaseURL(latest)
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
				a.printf("%s %s is available · %s\n", sWarn.Render("↑"), res.Latest, updateHint(res.Latest))
			} else {
				a.printf("%s up to date\n", sOK.Render("✓"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check GitHub Releases for a newer version (notify only)")
	return cmd
}
