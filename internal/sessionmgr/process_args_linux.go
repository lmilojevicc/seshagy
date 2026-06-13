//go:build linux

package sessionmgr

import (
	"os"
	"strings"
)

func readProcessArgsOS(pid string) string {
	if pid == "" || pid == "0" {
		return ""
	}
	data, err := os.ReadFile("/proc/" + pid + "/cmdline")
	if err != nil || len(data) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
}
