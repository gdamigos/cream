package cli

import (
	"fmt"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/core"
)

func (a *App) dryBanner() {
	if a.DryRun {
		a.printf("%s\n", sWarn.Render("DRY RUN · nothing will be changed"))
	}
}

func (a *App) printChanges(changes []adapter.Change) {
	for _, c := range changes {
		a.printf("  %s %s %s %s\n", sDim.Render("→"), sBold.Render(fmt.Sprintf("%-7s", c.Action)), sDim.Render(fmt.Sprintf("%-9s", c.Kind)), c.Target)
	}
}

func stateIcon(s core.State) string {
	switch s {
	case core.Done:
		return sOK.Render("✓")
	case core.Skipped:
		return sDim.Render("○")
	case core.Failed:
		return sErr.Render("✗")
	}
	return "·"
}

func (a *App) printEvents(evs []core.Event) {
	for _, e := range evs {
		a.printf("  %s %-14s %s\n", stateIcon(e.State), e.Name, sDim.Render(e.Msg))
		if a.DryRun {
			a.printChanges(e.Changes)
		}
	}
}
