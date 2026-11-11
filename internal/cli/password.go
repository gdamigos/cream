package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/core"
)

// runs fn, and when it needs a protected profile's password asks for it once and runs it again
func (a *App) withPasswords(m *core.Manager, fn func() error) error {
	asked := map[string]bool{}
	for {
		err := fn()
		var pr core.PasswordRequired
		if !errors.As(err, &pr) || asked[pr.Profile] {
			return err
		}
		asked[pr.Profile] = true
		pw, perr := a.password("Password for " + pr.Profile + ": ")
		if perr != nil {
			return perr
		}
		if err := m.Unseal(pr.Profile, pw); err != nil {
			return err
		}
	}
}

func (a *App) password(prompt string) (string, error) {
	if a.ReadPassword != nil {
		return a.ReadPassword(prompt)
	}
	if v := os.Getenv("CREAM_PROFILE_PASSWORD"); v != "" {
		return v, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("this needs a profile password: run it in a terminal or set CREAM_PROFILE_PASSWORD")
	}
	fmt.Fprint(a.Err, prompt)
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(a.Err)
	return string(b), err
}

func (a *App) newPassword() (string, error) {
	pw, err := a.password("New password: ")
	if err != nil {
		return "", err
	}
	if len(pw) < core.MinPasswordLen {
		return "", fmt.Errorf("the password needs at least %d characters", core.MinPasswordLen)
	}
	again, err := a.password("Repeat it: ")
	if err != nil {
		return "", err
	}
	if again != pw {
		return "", errors.New("the passwords don't match")
	}
	return pw, nil
}

func newPasswordCmd(a *App) *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "password <name>",
		Short: "Set, change or remove a profile's password",
		Long: `Protect a profile with a password, so people sharing this computer can't
switch to it, run tools as it or see its accounts, git identity and rules.
Its saved logins and settings are encrypted with a key only the password opens.
A forgotten password can't be recovered.`,
		Example: "  cream profile password alice\n  cream profile password alice --remove",
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
			if remove && !p.Protected() {
				return fmt.Errorf("profile %q has no password", p.Name)
			}
			a.dryBanner()
			old := ""
			if p.Protected() {
				if old, err = a.password("Current password for " + p.Name + ": "); err != nil {
					return err
				}
			}
			var changes []adapter.Change
			if remove {
				changes, err = m.RemovePassword(cmd.Context(), p.Name, old, a.DryRun)
			} else {
				next, perr := a.newPassword()
				if perr != nil {
					return perr
				}
				changes, err = m.SetPassword(cmd.Context(), p.Name, old, next, a.DryRun)
			}
			if a.DryRun {
				a.printChanges(changes)
				return err
			}
			if err != nil {
				return err
			}
			if remove {
				a.printf("%s %s has no password anymore\n", sOK.Render("✓"), sBold.Render(p.Name))
			} else {
				a.printf("%s %s is protected by its password\n", sOK.Render("✓"), sBold.Render(p.Name))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the password and decrypt the profile again")
	return cmd
}
