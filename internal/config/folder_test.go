package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindFolderWalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if f, err := FindFolder(deep); err != nil || f != nil {
		t.Fatalf("no file: f=%v err=%v", f, err)
	}
	path, err := WriteFolder(root, Folder{Profile: "work", Only: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := FindFolder(deep)
	if err != nil || f == nil {
		t.Fatalf("f=%v err=%v", f, err)
	}
	if f.Profile != "work" || len(f.Only) != 1 || f.Only[0] != "claude" || f.Path != path {
		t.Fatalf("f = %+v", f)
	}

	// the nearest file wins, and replacing a hidden file works
	for range 2 {
		if _, err := WriteFolder(filepath.Join(root, "a"), Folder{Profile: "personal"}); err != nil {
			t.Fatal(err)
		}
	}
	if f, _ := FindFolder(deep); f.Profile != "personal" || f.Only != nil {
		t.Fatalf("nearest = %+v", f)
	}

	// a folder named .cream is skipped
	if err := os.MkdirAll(filepath.Join(deep, FolderFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if f, err := FindFolder(deep); err != nil || f == nil || f.Profile != "personal" {
		t.Fatalf("with a .cream folder: f=%v err=%v", f, err)
	}
}

func TestFindFolderBadFile(t *testing.T) {
	dir := t.TempDir()
	for content, want := range map[string]string{
		"profile = ":       "parse",
		"only = [\"git\"]": "names no profile",
	} {
		os.WriteFile(filepath.Join(dir, FolderFile), []byte(content), 0o600)
		if _, err := FindFolder(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: err = %v", content, err)
		}
	}
}
