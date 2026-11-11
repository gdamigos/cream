package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

func newInitCmd(a *App) *cobra.Command {
	var only string
	var force bool
	cmd := &cobra.Command{
		Use:   "init [profile]",
		Short: "Write a .cream file that picks a profile for this folder",
		Long: `Write a .cream file into the current folder. In this folder and every
folder below it, ` + "`cream use`" + ` and ` + "`cream run <command>`" + ` without a profile use the
one it names, and the TUI starts on it. Without a profile it uses the
active one. --only limits which tools ` + "`cream use`" + ` switches here.`,
		Example: "  cream init work\n  cream init personal --only claude,codex",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			profile := m.Cfg.Active
			if len(args) > 0 {
				profile = args[0]
			}
			if profile == "" {
				return errors.New("no active profile, name one: cream init <profile>")
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			path := filepath.Join(dir, config.FolderFile)
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists, add --force to replace it", path)
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			a.dryBanner()
			if path, err = m.InitFolder(dir, profile, splitList(only), a.DryRun); err != nil {
				return err
			}
			if a.DryRun {
				a.printf("  %s %s %s\n", sDim.Render("→"), sBold.Render("write  "), path)
				return nil
			}
			a.printf("%s %s picks %s here %s\n", sOK.Render("✓"), path, sBold.Render(profile), sDim.Render("(switch with `cream use`)"))
			return nil
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "comma-separated adapters `cream use` switches in this folder")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing .cream file")
	return cmd
}

// the nearest .cream, an error when there is none
func folderProfile(m *core.Manager) (*config.Folder, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	f, err := m.Folder(dir)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, errors.New("no profile given and no .cream file in this folder or above, name a profile or pick one for this folder with `cream init <profile>`")
	}
	return f, nil
}
