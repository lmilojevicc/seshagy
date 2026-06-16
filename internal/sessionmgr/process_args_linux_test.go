//go:build linux

package sessionmgr

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func TestReadProcessArgsOSLinux(t *testing.T) {
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Skipf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	got := readProcessArgsOS(strconv.Itoa(cmd.Process.Pid))
	if !strings.Contains(got, "sleep") {
		t.Fatalf("readProcessArgsOS() = %q, want sleep command", got)
	}
	if got := readProcessArgsOS("0"); got != "" {
		t.Fatalf("readProcessArgsOS(0) = %q, want empty", got)
	}
}
