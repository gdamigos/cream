package paths

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Home      string
	CreamHome string // all cream state lives under here
}

// $CREAM_HOME, then $XDG_CONFIG_HOME/cream, then ~/.config/cream on every os
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	p := Paths{Home: home}
	switch {
	case os.Getenv("CREAM_HOME") != "":
		p.CreamHome = os.Getenv("CREAM_HOME")
	case os.Getenv("XDG_CONFIG_HOME") != "":
		p.CreamHome = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "cream")
	default:
		p.CreamHome = filepath.Join(home, ".config", "cream")
	}
	return p, nil
}

func (p Paths) ConfigFile() string { return filepath.Join(p.CreamHome, "config.toml") }

func (p Paths) BackupDir() string { return filepath.Join(p.CreamHome, "backups") }

func (p Paths) SessionDir(profile, tool string) string {
	return filepath.Join(p.CreamHome, "sessions", profile, tool)
}

func (p Paths) GitDir() string { return filepath.Join(p.CreamHome, "git") }

func (p Paths) VaultFile() string { return filepath.Join(p.CreamHome, "vault.age") }

func (p Paths) UpdateCache() string { return filepath.Join(p.CreamHome, "update-check.json") }

func (p Paths) UsageCache() string { return filepath.Join(p.CreamHome, "usage.json") }

func (p Paths) LogFile() string { return filepath.Join(p.CreamHome, "cream.log") }

func (p Paths) LockFile() string { return filepath.Join(p.CreamHome, "cream.lock") }

func (p Paths) InHome(elem ...string) string {
	return filepath.Join(append([]string{p.Home}, elem...)...)
}
