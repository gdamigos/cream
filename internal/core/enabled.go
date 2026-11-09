package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

// the tools listed in config.toml adapters in display order, a missing list is filled with every tool
func enabledAdapters(all []adapter.Adapter, cfg *config.Config) ([]adapter.Adapter, error) {
	ids := make([]string, 0, len(all))
	for _, a := range all {
		ids = append(ids, a.ID())
	}
	if cfg.Adapters == nil {
		cfg.Adapters = slices.Clone(ids)
	}
	for _, id := range cfg.Adapters {
		if !slices.Contains(ids, id) {
			return nil, fmt.Errorf("config.toml lists unknown tool %q in adapters, remove it or use one of: %s", id, strings.Join(ids, ", "))
		}
	}
	out := make([]adapter.Adapter, 0, len(all))
	for _, a := range all {
		if slices.Contains(cfg.Adapters, a.ID()) {
			out = append(out, a)
		}
	}
	return out, nil
}
