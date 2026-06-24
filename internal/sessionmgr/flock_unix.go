//go:build !windows

package sessionmgr

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// withAgentPaneLock acquires an exclusive flock on a per-pane lockfile, runs
// fn, then releases. It is best-effort: if the lockfile cannot be opened or
// the lock cannot be acquired, fn runs without the lock (reports still work).
func withAgentPaneLock(pane string, fn func() error) error {
	lockPath := filepath.Join(os.TempDir(), "seshagy-agent-"+sanitizePaneID(pane)+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fn()
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		return fn()
	}
	defer func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }()
	return fn()
}

// sanitizePaneID strips non-alphanumeric characters from a pane id (%NN → NN)
// for use in a filename.
func sanitizePaneID(pane string) string {
	var b strings.Builder
	for _, r := range pane {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
