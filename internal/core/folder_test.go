package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
	"github.com/gdamigos/cream/internal/adapters"
	"github.com/gdamigos/cream/internal/config"
)

func TestInitFolder(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Codex{}})
	m.AddProfile("work", "", config.GitIdentity{})
	dir := t.TempDir()

	if _, err := m.InitFolder(dir, "ghost", nil, false); err == nil {
		t.Fatal("unknown profile accepted")
	}
	if _, err := m.InitFolder(dir, "work", []string{"nope"}, false); err == nil {
		t.Fatal("unknown tool accepted")
	}
	path, err := m.InitFolder(dir, "work", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("dry run wrote the file")
	}
	if _, err := m.InitFolder(dir, "work", []string{"codex"}, false); err != nil {
		t.Fatal(err)
	}
	f, err := m.Folder(filepath.Join(dir, "sub"))
	if err != nil || f == nil || f.Profile != "work" {
		t.Fatalf("f=%v err=%v", f, err)
	}
}

func TestDoctorFolder(t *testing.T) {
	m, _ := newManager(t, []adapter.Adapter{adapters.Codex{}})
	m.AddProfile("work", "", config.GitIdentity{})
	dir := t.TempDir()
	if got := m.doctorFolder(dir); got != nil {
		t.Fatalf("no file: %+v", got)
	}

	put(t, filepath.Join(dir, config.FolderFile), `profile = "ghost"`)
	c := m.doctorFolder(dir)
	if len(c) != 1 || c[0].State != CheckWarn || !strings.Contains(c[0].Msg, "unknown profile") {
		t.Fatalf("unknown profile: %+v", c)
	}

	put(t, filepath.Join(dir, config.FolderFile), `profile = "work"`)
	if c := m.doctorFolder(dir); c[0].State != CheckInfo || c[0].Fix != "cream use" {
		t.Fatalf("inactive: %+v", c)
	}
	m.Cfg.Active = "work"
	if c := m.doctorFolder(dir); c[0].State != CheckOK {
		t.Fatalf("active: %+v", c)
	}
}
