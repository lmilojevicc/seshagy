//go:build darwin

package sessionmgr

import (
	"os/exec"
	"strings"
)

func readProcessArgsOS(pid string) string {
	if pid == "" || pid == "0" {
		return ""
	}
	out, err := exec.Command("ps", "-ww", "-p", pid, "-o", "args=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
