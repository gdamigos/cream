package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// temp file plus rename so nobody sees a half written file, keeps existing permissions
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".cream-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func ReadFileIfExists(path string) ([]byte, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// size and mtime, so a reader notices when another process rewrote the file, empty when it is missing
func Stamp(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprint(fi.Size(), fi.ModTime().UnixNano())
}

func RemoveIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
