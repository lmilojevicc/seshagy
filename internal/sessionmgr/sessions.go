package sessionmgr

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const sessionFormat = "#{session_name}\x1f#{session_created}\x1f#{session_activity}\x1f#{session_path}\x1f#{session_attached}\x1f#{session_windows}"

func exactSession(name string) string { return "=" + name }
func exactPane(name string) string    { return "=" + name + ":" }

func ListSessions(ctx context.Context) ([]Item, error) {
	out, err := tmuxCommand(ctx, "list-sessions", "-F", sessionFormat).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	return ParseSessions(out), nil
}

func ParseSessions(raw []byte) []Item {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	items := make([]Item, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x1f")
		if len(parts) < 6 {
			continue
		}
		items = append(items, Item{
			Kind:     KindSession,
			Name:     parts[0],
			Target:   parts[0],
			Created:  unixTime(parts[1]),
			Activity: unixTime(parts[2]),
			Path:     parts[3],
			Attached: parts[4] != "0" && parts[4] != "",
			Windows:  atoi(parts[5]),
		})
	}
	return items
}

func HasSession(ctx context.Context, name string) (bool, error) {
	err := tmuxCommand(ctx, "has-session", "-t", exactSession(name)).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("tmux has-session: %w", err)
}

func CreateSessionFromDir(ctx context.Context, dir string) (string, bool, error) {
	dir = ExpandHome(dir)
	name := SessionNameFromDir(dir)
	exists, err := HasSession(ctx, name)
	if err != nil {
		return name, false, err
	}
	if exists {
		return name, false, nil
	}
	cmd := tmuxCommand(ctx, "new-session", "-d", "-s", name, "-c", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return name, false, fmt.Errorf(
			"tmux new-session: %w (%s)",
			err,
			strings.TrimSpace(string(out)),
		)
	}
	return name, true, nil
}

func KillSession(ctx context.Context, name string) error {
	if out, err := tmuxCommand(
		ctx,
		"kill-session",
		"-t",
		exactSession(name),
	).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func RenameSession(ctx context.Context, oldName, newName string) error {
	newName = sanitizeSessionName(newName)
	if out, err := tmuxCommand(
		ctx,
		"rename-session",
		"-t",
		exactSession(oldName),
		newName,
	).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux rename-session: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func CaptureSession(ctx context.Context, name string, lines int) (string, error) {
	args := []string{"capture-pane", "-ep", "-t", exactPane(name)}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	out, err := tmuxCommand(ctx, args...).Output()
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return string(out), nil
}

func AttachOrSwitchCommand(name string) *exec.Cmd {
	if InTmux() {
		return exec.Command("tmux", "switch-client", "-t", exactSession(name))
	}
	return exec.Command("tmux", "attach-session", "-t", name)
}

func unixTime(s string) time.Time {
	n := atoi(s)
	if n <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(n), 0)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
