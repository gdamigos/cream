package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/gdamigos/cream/internal/fsutil"
)

// a .cream file picks the profile for its folder and every folder below it
const FolderFile = ".cream"

type Folder struct {
	Profile string   `toml:"profile"`
	Only    []string `toml:"only,omitempty"` // tools `cream use` switches here, empty means every tool
	Path    string   `toml:"-"`              // the .cream file it came from
}

// the nearest .cream in dir or a parent, nil when there is none
func FindFolder(dir string) (*Folder, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, FolderFile)
		// a folder that happens to be called .cream isn't one
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			return parseFolder(path, b)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

func parseFolder(path string, b []byte) (*Folder, error) {
	f := &Folder{Path: path}
	if _, err := toml.Decode(string(b), f); err != nil {
		return nil, fmt.Errorf("parse %s: %w (it needs a line like profile = \"work\")", path, err)
	}
	if f.Profile == "" {
		return nil, fmt.Errorf("%s names no profile, add a line like profile = \"work\" or run `cream init <profile>` there", path)
	}
	return f, nil
}

func WriteFolder(dir string, f Folder) (string, error) {
	var buf bytes.Buffer
	buf.WriteString("# picks the cream profile for this folder, see `cream init --help`\n")
	if err := toml.NewEncoder(&buf).Encode(f); err != nil {
		return "", err
	}
	path := filepath.Join(dir, FolderFile)
	if err := fsutil.WriteFileAtomic(path, buf.Bytes(), 0o644); err != nil {
		return path, err
	}
	// a file that fails to hide still works
	_ = fsutil.Hide(path)
	return path, nil
}
