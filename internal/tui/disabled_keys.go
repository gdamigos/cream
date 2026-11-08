package tui

import "slices"

// keys that do the same thing, disabling one disables both
var keyAliases = map[string]string{"x": "delete", "delete": "x", "enter": " ", " ": "enter"}

// ui.disabled_keys in config.toml, only checked on the main view so windows and forms keep working
func (m Model) keyDisabled(k string) bool {
	keys := m.mgr.Cfg.UI.DisabledKeys
	return slices.Contains(keys, k) || (keyAliases[k] != "" && slices.Contains(keys, keyAliases[k]))
}

func (m Model) enabledHints(hints []keyHint) []keyHint {
	return slices.DeleteFunc(slices.Clone(hints), func(h keyHint) bool { return m.keyDisabled(h.k) })
}
