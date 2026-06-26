//go:build windows

package sessionmgr

// withAgentPaneLock is a no-op on Windows (tmux does not run on Windows).
func withAgentPaneLock(_ string, fn func() error) error {
	return fn()
}
