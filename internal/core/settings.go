package core

import (
	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/config"
)

// every tool cream knows, including the ones config.toml turns off
func (m *Manager) AllAdapters() []adapter.Adapter { return m.all }

// changes the settings in config.toml, edit runs on the latest config and must replace slices, not modify them
func (m *Manager) EditSettings(edit func(*config.Config)) error {
	release, err := m.exclusive(false)
	if err != nil {
		return err
	}
	defer release()
	old := *m.Cfg
	edit(m.Cfg)
	enabled, err := enabledAdapters(m.all, m.Cfg)
	if err == nil {
		err = m.Save()
	}
	if err != nil {
		*m.Cfg = old
		return err
	}
	m.Adapters = enabled
	// strict decides whether the active profile is written as the global identity
	if old.GitStrict() != m.Cfg.GitStrict() {
		if _, err := m.SyncGit(false); err != nil {
			return err
		}
	}
	m.Log("settings edit")
	return nil
}
