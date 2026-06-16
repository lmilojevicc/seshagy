//go:build darwin

package sessionmgr

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestReadProcessArgsOSDarwin(t *testing.T) {
	got := readProcessArgsOS(strconv.Itoa(os.Getpid()))
	if got == "" || !strings.Contains(got, "test") {
		t.Fatalf("readProcessArgsOS() = %q, want current test command", got)
	}
	if got := readProcessArgsOS("0"); got != "" {
		t.Fatalf("readProcessArgsOS(0) = %q, want empty", got)
	}
}
