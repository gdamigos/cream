package fsutil

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "cream.lock")
	unlock, err := Lock(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// a second handle conflicts like another process would
	if _, err := Lock(path, 100*time.Millisecond); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: %v", err)
	}
	unlock()
	again, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	again()
}
