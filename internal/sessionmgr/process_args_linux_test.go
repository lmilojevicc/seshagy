//go:build linux

package sessionmgr

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestReadProcessArgsOSLinux(t *testing.T) {
	// ponytail: use current PID — /proc/PID/cmdline can be empty briefly after spawn on CI.
	got := readProcessArgsOS(strconv.Itoa(os.Getpid()))
	if got == "" || !strings.Contains(got, "test") {
		t.Fatalf("readProcessArgsOS() = %q, want current test command", got)
	}
	if got := readProcessArgsOS("0"); got != "" {
		t.Fatalf("readProcessArgsOS(0) = %q, want empty", got)
	}
}
