package sessionmgr

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func tmuxCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Env = withLocale(os.Environ())
	return cmd
}

// tmuxOutput and tmuxRun are the single seam through which the low-level pane
// helpers invoke tmux. Tests override these to simulate a tmux server in
// memory without spawning real processes.
var (
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		return tmuxCommand(ctx, args...).Output()
	}
	tmuxRun = func(ctx context.Context, args ...string) error {
		return tmuxCommand(ctx, args...).Run()
	}
)

func plainCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = withLocale(os.Environ())
	return cmd
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = withLocale(os.Environ())
	return cmd
}

func withLocale(env []string) []string {
	for _, e := range env {
		if strings.HasPrefix(e, "LC_ALL=") || strings.HasPrefix(e, "LC_CTYPE=") ||
			strings.HasPrefix(e, "LANG=") {
			return env
		}
	}
	return append(env, "LC_ALL=C.UTF-8")
}

func InTmux() bool { return os.Getenv("TMUX") != "" }

func InTmuxPopup(ctx context.Context) (bool, error) {
	if !InTmux() {
		return false, nil
	}
	out, err := tmuxOutput(ctx, "display-message", "-p", "#{pane_id}")
	if err != nil {
		return false, err
	}
	return detectTmuxPopup(os.Getenv("TMUX_PANE"), strings.TrimSpace(string(out))), nil
}

func detectTmuxPopup(envPane, currentPane string) bool {
	envPane = strings.TrimSpace(envPane)
	currentPane = strings.TrimSpace(currentPane)
	if currentPane == "" {
		return false
	}
	if envPane == "" {
		return true
	}
	return envPane != currentPane
}

func showPaneOption(ctx context.Context, pane, opt string) (string, error) {
	out, err := tmuxOutput(ctx, "show-option", "-qvpt", pane, opt)
	return strings.TrimSpace(string(out)), err
}

func setPaneOption(ctx context.Context, pane, opt, value string) error {
	return tmuxRun(ctx, "set-option", "-qpt", pane, opt, value)
}

func unsetPaneOption(ctx context.Context, pane, opt string) error {
	return tmuxRun(ctx, "set-option", "-qupt", pane, opt)
}

func CurrentTmuxSession(ctx context.Context) (string, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "#S")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
