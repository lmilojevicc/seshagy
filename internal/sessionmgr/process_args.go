package sessionmgr

var readProcessArgsHook func(pid string) string

func readProcessArgs(pid string) string {
	if readProcessArgsHook != nil {
		return readProcessArgsHook(pid)
	}
	return readProcessArgsOS(pid)
}
