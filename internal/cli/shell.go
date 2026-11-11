package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/paths"
)

var hookTools = []string{"claude", "codex", "gh", "glab"}

func newHookCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook <bash|zsh|fish|pwsh>",
		Short: "Print shell functions that run tools as the folder's profile",
		Long: `Print shell functions for claude, codex, gh and glab. Under a folder with a
.cream file they run like ` + "`cream run <command>`" + `, in that terminal only.
Everywhere else they run the tool as usual. Add one line to your shell's
startup file:

  bash   eval "$(cream hook bash)"            in ~/.bashrc
  zsh    eval "$(cream hook zsh)"             in ~/.zshrc
  fish   cream hook fish | source             in ~/.config/fish/config.fish
  pwsh   cream hook pwsh | Out-String | Invoke-Expression   in $PROFILE`,
		Example:   "  cream hook bash\n  cream hook pwsh",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "pwsh", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := hookScript(args[0])
			if err != nil {
				return err
			}
			a.printf("%s", s)
			return nil
		},
	}
	exe := &cobra.Command{
		Use:    "exec <command> [args...]",
		Short:  "Run a command as the nearest .cream file's profile, or as usual without one",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// inside cream run the session already picked a profile
			if os.Getenv("CREAM_PROFILE") == "" {
				dir, err := os.Getwd()
				if err != nil {
					return err
				}
				if found, ferr := config.FindFolder(dir); found != nil || ferr != nil {
					m, err := a.Manager()
					if err != nil {
						return err
					}
					f, err := folderProfile(m)
					if err != nil {
						return err
					}
					// the active profile is already logged in globally
					if f.Profile != m.Cfg.Active {
						return a.runAs(cmd.Context(), m, f.Profile, args)
					}
				}
			}
			return runChild(args, nil)
		},
	}
	exe.Flags().SetInterspersed(false)
	cmd.AddCommand(exe)
	return cmd
}

func hookScript(shell string) (string, error) {
	var b strings.Builder
	for _, t := range hookTools {
		switch shell {
		case "bash", "zsh":
			fmt.Fprintf(&b, "%s() { command cream hook exec %s \"$@\"; }\n", t, t)
		case "fish":
			fmt.Fprintf(&b, "function %s --wraps %s; command cream hook exec %s $argv; end\n", t, t, t)
		case "pwsh", "powershell":
			// piping $input when there is none would cut the tool off from the console
			fmt.Fprintf(&b, "function global:%s { if ($MyInvocation.ExpectingInput) { $input | cream hook exec %s @args } else { cream hook exec %s @args } }\n", t, t, t)
		default:
			return "", fmt.Errorf("unknown shell %q, use bash, zsh, fish or pwsh", shell)
		}
	}
	return b.String(), nil
}

func newPromptCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Print the profile in use for a shell prompt, with * when the folder's .cream picks another",
		Long: `Print the profile in use in this terminal: the one ` + "`cream run`" + ` started it
with, otherwise the active one. A * means the nearest .cream file picks a
different profile. Prints nothing when no profile is in use. It only reads
config.toml and the .cream file, so it is fast enough for every prompt.`,
		Example: "  cream prompt",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths.Resolve()
			if err != nil {
				return err
			}
			cfg, err := config.Load(p.ConfigFile())
			if err != nil {
				return err
			}
			// a broken .cream shows up in doctor, not in every prompt
			var f *config.Folder
			if dir, err := os.Getwd(); err == nil {
				f, _ = config.FindFolder(dir)
			}
			if s := promptText(cfg, f, os.Getenv("CREAM_PROFILE")); s != "" {
				a.printf("%s\n", s)
			}
			return nil
		},
	}
}

func promptText(cfg *config.Config, f *config.Folder, session string) string {
	name := session
	if name == "" {
		name = cfg.Active
	}
	if name != "" && f != nil && f.Profile != name {
		name += "*"
	}
	return name
}
