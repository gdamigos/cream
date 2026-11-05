package adapters

import (
	"testing"

	"github.com/gdamigos/cream/internal/adapter"
)

// every adapter except git can be locked
func TestAdaptersLock(t *testing.T) {
	for _, a := range All() {
		_, isLocker := a.(adapter.Locker)
		if want := a.ID() != "git"; isLocker != want {
			t.Errorf("%s: Locker = %v, want %v", a.ID(), isLocker, want)
		}
	}
}
