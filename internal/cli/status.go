package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/config"
	"github.com/gdamigos/cream/internal/core"
)

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
	switch {
	case m.Cfg.Lock.All:
		a.printf("  %s\n", sWarn.Render("locked: everything (`cream unlock`)"))
	case len(m.Cfg.Lock.Profiles) > 0:
		a.printf("  %s\n", sWarn.Render("locked: "+strings.Join(m.Cfg.Lock.Profiles, ", ")))
	}
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
			fmt.Fprintf(a.Err, "%s\n", sDim.Render("update available: "+res.Latest+" · "+updateHint(res.Latest)))
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
		Active   string           `json:"active"`
		Vault    string           `json:"vault"`
		Locked   config.LockState `json:"locked"`
		Adapters []row            `json:"adapters"`
	}{Active: m.Cfg.Active, Vault: m.Env.Vault.Backend(), Locked: m.Cfg.Lock}
	for _, r := range m.Status(ctx) {
		out.Adapters = append(out.Adapters, row{r.ID, r.Live.Installed, r.Live.LoggedIn, r.Live.Account, r.Expected, r.Match, r.Live.Warning})
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
