package core

import (
	"fmt"
	"path/filepath"

	"github.com/gdamigos/cream/internal/config"
)

// the .cream that applies to dir, checked against the profiles and tools cream knows
func (m *Manager) Folder(dir string) (*config.Folder, error) {
	f, err := config.FindFolder(dir)
	if err != nil || f == nil {
		return nil, err
	}
	if _, err := m.Cfg.Profile(f.Profile); err != nil {
		return f, fmt.Errorf("%s: %w", f.Path, err)
	}
	if _, err := m.Select(f.Only); err != nil {
		return f, fmt.Errorf("%s: %w", f.Path, err)
	}
	return f, nil
}

// writes dir/.cream, returns its path
func (m *Manager) InitFolder(dir, profile string, only []string, dryRun bool) (string, error) {
	if _, err := m.Cfg.Profile(profile); err != nil {
		return "", err
	}
	if _, err := m.Select(only); err != nil {
		return "", err
	}
	if dryRun {
		return filepath.Join(dir, config.FolderFile), nil
	}
	path, err := config.WriteFolder(dir, config.Folder{Profile: profile, Only: only})
	if err != nil {
		return path, fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func (m *Manager) doctorFolder(dir string) []Check {
	f, err := m.Folder(dir)
	c := Check{Area: "folder", Name: config.FolderFile}
	switch {
	case err != nil && f != nil:
		c.State, c.Msg, c.Fix = CheckWarn, err.Error(), "cream init <profile> --force in "+filepath.Dir(f.Path)
	case err != nil:
		c.State, c.Msg, c.Fix = CheckWarn, err.Error(), "fix the file, or delete it and run cream init <profile>"
	case f == nil:
		return nil
	case f.Profile == m.Cfg.Active:
		c.State, c.Msg = CheckOK, "uses "+f.Profile+", which is active ("+f.Path+")"
	default:
		active := m.Cfg.Active
		if active == "" {
			active = "nothing"
		}
		c.State, c.Msg, c.Fix = CheckInfo, "uses "+f.Profile+", but "+active+" is active ("+f.Path+")", "cream use"
	}
	return []Check{c}
}
