package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClickTwiceDefault(t *testing.T) {
	load := func(content string) *Config {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if !load("version = 1\n[ui]\nintro = true\n").UI.ClickTwice {
		t.Fatal("a config without click_twice should keep it on")
	}
	if load("version = 1\n[ui]\nclick_twice = false\n").UI.ClickTwice {
		t.Fatal("click_twice = false was ignored")
	}
}
