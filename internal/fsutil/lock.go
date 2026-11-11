package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrLocked = errors.New("locked by another process")

// takes an exclusive lock on path that every process sees, waiting up to wait for it
// the os drops the lock when the process dies, so a crash never leaves it stuck
func Lock(path string, wait time.Duration) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		lerr := tryLock(f)
		if lerr == nil {
			return func() { _ = f.Close() }, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %w", ErrLocked, lerr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
