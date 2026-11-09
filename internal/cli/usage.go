package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gdamigos/cream/internal/core"
)

func newUsageCmd(a *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show how much of their plan limits the saved Claude Code and Codex logins used",
		Long: `Ask Anthropic and OpenAI how much of each plan's limits the saved
Claude Code and Codex logins have used. This sends each login's token to
its own provider, so it only runs with usage_check = true in config.toml.
Results are cached for 5 minutes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := a.Manager()
			if err != nil {
				return err
			}
			if !m.Cfg.UsageCheck {
				return errors.New("the usage check is off, set usage_check = true in " + m.Paths.ConfigFile() + " to turn it on")
			}
			us := m.Usage(cmd.Context(), nil, false)
			if asJSON {
				return a.printUsageJSON(us)
			}
			if len(us) == 0 {
				a.printf("%s\n", sDim.Render("no saved Claude Code or Codex logins yet, e.g. `cream import work`"))
				return nil
			}
			for _, u := range us {
				live := "  "
				if u.Live {
					live = sOK.Render("● ")
				}
				a.printf("%s%-12s %-12s %s\n", live, u.Profile, u.Name, usageLine(u))
			}
			a.printSuggestions(core.Suggest(us))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func usageLine(u core.Usage) string {
	if u.Err != "" {
		return sDim.Render(u.Err)
	}
	if u.Result == nil || len(u.Result.Windows) == 0 {
		return sDim.Render("no usage information")
	}
	var parts []string
	for _, w := range u.Result.Windows {
		s := fmt.Sprintf("%s %.0f%%", w.Name, w.Used)
		if !w.ResetsAt.IsZero() {
			s += " (resets " + resetTime(w.ResetsAt) + ")"
		}
		if w.Used >= 100 {
			s = sWarn.Render(s)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, sDim.Render(" · "))
}

func resetTime(t time.Time) string {
	t = t.Local()
	if time.Until(t) < 20*time.Hour {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func (a *App) printSuggestions(ss []core.Suggestion) {
	for _, s := range ss {
		until := ""
		if !s.Until.IsZero() {
			until = " until " + resetTime(s.Until)
		}
		a.printf("%s %s on %s is at its limit%s, switch it with `cream use %s --only %s`\n", sWarn.Render("!"), s.Name, s.From, until, s.To, s.Adapter)
	}
}

func (a *App) printUsageJSON(us []core.Usage) error {
	type row struct {
		Profile string `json:"profile"`
		Adapter string `json:"adapter"`
		Live    bool   `json:"live"`
		Usage   any    `json:"usage,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	out := []row{}
	for _, u := range us {
		r := row{Profile: u.Profile, Adapter: u.Adapter, Live: u.Live, Error: u.Err}
		if u.Result != nil {
			r.Usage = u.Result
		}
		out = append(out, r)
	}
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// stale logins and, with usage_check on, switch hints for tools at their limit
func (a *App) printWarnings(ctx context.Context, m *core.Manager) {
	now := time.Now()
	for _, s := range m.StaleLogins(now) {
		a.printf("  %s\n", sWarn.Render("! "+s.Msg(now)+" (cream doctor)"))
	}
	if m.Cfg.UsageCheck {
		a.printSuggestions(core.Suggest(m.Usage(ctx, nil, false)))
	}
}
