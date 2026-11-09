package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
)

func ids(ads []adapter.Adapter) []string {
	var out []string
	for _, a := range ads {
		out = append(out, a.ID())
	}
	return out
}

func TestEnabledAdapters(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Claude{}, adapters.Codex{}})
	if got := ids(m.Adapters); !slices.Equal(got, []string{"claude", "codex"}) {
		t.Fatalf("default adapters %v", got)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, m.Paths.ConfigFile()), `adapters = ["claude", "codex"]`) {
		t.Fatalf("adapters not written:\n%s", get(t, m.Paths.ConfigFile()))
	}

	// removing a tool from the file turns it off on the next reload
	cfg := strings.Replace(get(t, m.Paths.ConfigFile()), `adapters = ["claude", "codex"]`, `adapters = ["codex"]`, 1)
	put(t, m.Paths.ConfigFile(), cfg+"\n")
	if _, err := m.ReloadIfChanged(); err != nil {
		t.Fatal(err)
	}
	if got := ids(m.Adapters); !slices.Equal(got, []string{"codex"}) {
		t.Fatalf("after removing claude %v", got)
	}
	if st := m.StatusFor(ctx, nil); len(st) != 1 || st[0].ID != "codex" {
		t.Fatalf("status still shows a removed tool: %+v", st)
	}
	if _, err := m.Select([]string{"claude"}); err == nil {
		t.Fatal("--only accepted a removed tool")
	}

	// an empty list turns every tool off
	put(t, m.Paths.ConfigFile(), strings.Replace(cfg, `adapters = ["codex"]`, `adapters = []`, 1))
	if _, err := m.ReloadIfChanged(); err != nil {
		t.Fatal(err)
	}
	if len(m.Adapters) != 0 {
		t.Fatalf("empty list kept %v", ids(m.Adapters))
	}

	put(t, m.Paths.ConfigFile(), strings.Replace(cfg, `adapters = ["codex"]`, `adapters = ["codx"]`, 1)+"\n\n")
	if _, err := m.ReloadIfChanged(); err == nil || !strings.Contains(err.Error(), `"codx"`) {
		t.Fatalf("unknown tool not reported: %v", err)
	}
}
