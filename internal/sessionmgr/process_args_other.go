//go:build !linux && !darwin

package sessionmgr

func readProcessArgsOS(pid string) string {
	_ = pid
	return ""
}
