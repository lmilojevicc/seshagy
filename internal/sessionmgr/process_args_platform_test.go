//go:build !darwin && !linux

package sessionmgr

import "testing"

func TestReadProcessArgsOSPlatformGap(t *testing.T) {
	if got := readProcessArgsOS("12345"); got != "" {
		t.Fatalf("readProcessArgsOS() = %q, want empty on unsupported platforms", got)
	}
}
