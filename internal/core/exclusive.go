package core

import (
	"fmt"
	"time"

	"github.com/gdamigos/cream/internal/fsutil"
)

// how long a change waits for another cream process to finish its own
const lockWait = time.Minute

// serializes changes across cream processes, the outermost call takes the lock file and
// picks up config.toml changes made meanwhile, a dry run takes nothing so it writes no file
func (m *Manager) exclusive(dryRun bool) (release func(), err error) {
	if dryRun {
		return func() {}, nil
	}
	m.lockMu.Lock()
	defer m.lockMu.Unlock()
	if m.lockDepth == 0 {
		unlock, err := fsutil.Lock(m.Paths.LockFile(), lockWait)
		if err != nil {
			return nil, fmt.Errorf("another cream is still busy, try again when it's done (%s): %w", m.Paths.LockFile(), err)
		}
		if _, err := m.ReloadIfChanged(); err != nil {
			unlock()
			return nil, err
		}
		m.unlockFile = unlock
	}
	m.lockDepth++
	return func() {
		m.lockMu.Lock()
		defer m.lockMu.Unlock()
		if m.lockDepth--; m.lockDepth == 0 {
			m.unlockFile()
			m.unlockFile = nil
		}
	}, nil
}
