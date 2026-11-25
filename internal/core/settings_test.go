package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
)

func TestEditSettings(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.Codex{}})
	err := m.EditSettings(func(c *config.Config) {
		c.Adapters = []string{"codex"}
		c.UI.DisabledKeys = []string{"x"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(m.Adapters); !slices.Equal(got, []string{"codex"}) {
		t.Fatalf("adapters after edit %v", got)
	}
	if got := ids(m.AllAdapters()); len(got) != 2 {
		t.Fatalf("all adapters %v", got)
	}
	file := get(t, m.Paths.ConfigFile())
	if !strings.Contains(file, `adapters = ["codex"]`) || !strings.Contains(file, `disabled_keys = ["x"]`) {
		t.Fatalf("settings not written:\n%s", file)
	}

	// an unknown tool is refused and nothing changes
	err = m.EditSettings(func(c *config.Config) { c.Adapters, c.UsageCheck = []string{"nope"}, true })
	if err == nil {
		t.Fatal("unknown tool accepted")
	}
	if m.Cfg.UsageCheck || !slices.Equal(m.Cfg.Adapters, []string{"codex"}) {
		t.Fatal("a refused edit changed the config")
	}
	if strings.Contains(get(t, m.Paths.ConfigFile()), "nope") {
		t.Fatal("a refused edit was saved")
	}
}
