package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sessionListLine(name, path string) string {
	return strings.Join([]string{
		name,
		"100",
		"200",
		path,
		"0",
		"1",
	}, "\x1f")
}

func TestListSessionsEmptyWhenTmuxExitOne(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, exec.Command("false").Run()
		}
		return nil, nil
	}, nil)

	items, err := ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if items != nil {
		t.Fatalf("ListSessions() = %#v, want nil on exit 1", items)
	}
}

func TestListSessionsPropagatesError(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, fmt.Errorf("tmux unavailable")
		}
		return nil, nil
	}, nil)

	if _, err := ListSessions(context.Background()); err == nil {
		t.Fatal("ListSessions() expected error")
	} else if !strings.Contains(err.Error(), "tmux list-sessions") {
		t.Fatalf("ListSessions() error = %v", err)
	}
}

func TestCurrentTmuxSession(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "display-message" && args[2] == "#S" {
			return []byte("work\n"), nil
		}
		return nil, nil
	}, nil)

	name, err := CurrentTmuxSession(context.Background())
	if err != nil || name != "work" {
		t.Fatalf("CurrentTmuxSession() = (%q, %v)", name, err)
	}
}

func TestCreateSessionFromDirReusesExisting(t *testing.T) {
	dir := t.TempDir()
	existingPath := filepath.Join(dir, "work", "api")
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "list-sessions" {
			return []byte(sessionListLine("work-api", existingPath) + "\n"), nil
		}
		return nil, nil
	}, nil)

	name, created, err := CreateSessionFromDir(context.Background(), existingPath)
	if err != nil {
		t.Fatalf("CreateSessionFromDir() error = %v", err)
	}
	if name != "work-api" || created {
		t.Fatalf("CreateSessionFromDir() = (%q, %v), want (work-api, false)", name, created)
	}
}

func TestCreateSessionFromDirCollisionQualifies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workAPI := filepath.Join(home, "work", "api")
	personalAPI := filepath.Join(home, "personal", "api")
	var renamedTo, createdName string
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "list-sessions" {
			return []byte(sessionListLine("api", workAPI) + "\n"), nil
		}
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		switch {
		case len(args) >= 4 && args[0] == "rename-session" && args[3] == "work-api":
			renamedTo = args[3]
		case len(args) >= 6 && args[0] == "new-session" && args[1] == "-d":
			createdName = args[3]
		}
		return nil
	})

	name, created, err := CreateSessionFromDir(context.Background(), personalAPI)
	if err != nil {
		t.Fatalf("CreateSessionFromDir() error = %v", err)
	}
	if !created || name != "personal-api" {
		t.Fatalf("CreateSessionFromDir() = (%q, %v), want (personal-api, true)", name, created)
	}
	if renamedTo != "work-api" {
		t.Fatalf("rename-session target = %q, want work-api", renamedTo)
	}
	if createdName != "personal-api" {
		t.Fatalf("new-session name = %q, want personal-api", createdName)
	}
}

func TestHasSession(t *testing.T) {
	known := map[string]bool{"demo": true}
	SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 3 && args[0] == "has-session" {
			name := strings.TrimPrefix(args[2], "=")
			if known[name] {
				return nil
			}
			return exec.Command("false").Run()
		}
		return nil
	})

	ok, err := HasSession(context.Background(), "demo")
	if err != nil || !ok {
		t.Fatalf("HasSession(demo) = (%v, %v), want (true, nil)", ok, err)
	}

	ok, err = HasSession(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("HasSession(missing) = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestHasSessionPropagatesError(t *testing.T) {
	SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 3 && args[0] == "has-session" {
			return fmt.Errorf("tmux unavailable")
		}
		return nil
	})

	if _, err := HasSession(context.Background(), "demo"); err == nil {
		t.Fatal("HasSession() expected error")
	} else if !strings.Contains(err.Error(), "tmux has-session") {
		t.Fatalf("HasSession() error = %v", err)
	}
}

func TestListSessionsParsesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo")
	raw := sessionListLine("demo", path) + "\n" + sessionListLine("web", path)
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "list-sessions" {
			return []byte(raw), nil
		}
		return nil, nil
	}, nil)

	items, err := ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(items) != 2 || items[0].Name != "demo" || items[1].Name != "web" {
		t.Fatalf("ListSessions() = %#v", items)
	}
	if items[0].Path != path || items[0].Kind != KindSession {
		t.Fatalf("first item = %#v", items[0])
	}
}

func TestListSessionsEmptyServer(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, exec.Command("sh", "-c", "exit 1").Run()
		}
		return nil, nil
	}, nil)

	items, err := ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if items != nil {
		t.Fatalf("ListSessions() = %#v, want nil", items)
	}
}

func TestKillSession(t *testing.T) {
	var killed string
	SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 3 && args[0] == "kill-session" {
			killed = strings.TrimPrefix(args[2], "=")
		}
		return nil
	})

	if err := KillSession(context.Background(), "demo"); err != nil {
		t.Fatalf("KillSession() error = %v", err)
	}
	if killed != "demo" {
		t.Fatalf("kill-session target = %q, want demo", killed)
	}
}

func TestRenameSession(t *testing.T) {
	var oldName, newName string
	SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 4 && args[0] == "rename-session" {
			oldName = strings.TrimPrefix(args[2], "=")
			newName = args[3]
		}
		return nil
	})

	if err := RenameSession(context.Background(), "demo", "renamed"); err != nil {
		t.Fatalf("RenameSession() error = %v", err)
	}
	if oldName != "demo" || newName != "renamed" {
		t.Fatalf("rename-session = %q -> %q, want demo -> renamed", oldName, newName)
	}
}

func TestAttachOrSwitchCommandUsesTmuxContext(t *testing.T) {
	t.Setenv("TMUX", "")
	cmd := AttachOrSwitchCommand("demo")
	if len(cmd.Args) < 3 || cmd.Args[0] != "tmux" || cmd.Args[1] != "attach-session" {
		t.Fatalf("outside tmux command = %#v", cmd.Args)
	}

	t.Setenv("TMUX", "/tmp/tmux-123/default,1,0")
	cmd = AttachOrSwitchCommand("demo")
	if len(cmd.Args) < 3 || cmd.Args[1] != "switch-client" {
		t.Fatalf("inside tmux command = %#v", cmd.Args)
	}
}

func TestFocusAgentCommandSwitchesClientInTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-123/default,1,0")
	cmd := FocusAgentCommand("work", "1", "%5")
	if len(cmd.Args) < 3 || cmd.Args[0] != "sh" || cmd.Args[1] != "-c" {
		t.Fatalf("command = %#v, want sh -c", cmd.Args)
	}
	script := cmd.Args[2]
	for _, want := range []string{
		"select-window -t 'work:1'",
		"select-pane -t '%5'",
		"switch-client -t 'work'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q\nscript: %s", want, script)
		}
	}
}

func TestFocusAgentCommandNoSwitchClientOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	cmd := FocusAgentCommand("work", "1", "%5")
	script := cmd.Args[2]
	if strings.Contains(script, "switch-client") {
		t.Errorf("script should not switch-client outside tmux\nscript: %s", script)
	}
	for _, want := range []string{"select-window", "select-pane"} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q\nscript: %s", want, script)
		}
	}
}

func TestParseSessionsSkipsMalformedLines(t *testing.T) {
	raw := []byte("only-three\x1fparts\x1fhere\n" + sessionListLine("ok", "/tmp/demo"))
	items := ParseSessions(raw)
	if len(items) != 1 || items[0].Name != "ok" {
		t.Fatalf("ParseSessions() = %#v, want one valid item", items)
	}
}

func TestCaptureSession(t *testing.T) {
	var startFlag, lineCount, target string
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "capture-pane" {
			if len(args) >= 6 {
				startFlag = args[4]
				lineCount = args[5]
			}
			target = args[3]
			return []byte("limited output\n"), nil
		}
		return nil, nil
	}, nil)

	out, err := CaptureSession(context.Background(), "demo", 10)
	if err != nil || out != "limited output\n" {
		t.Fatalf("CaptureSession() = (%q, %v)", out, err)
	}
	if startFlag != "-S" || lineCount != "-10" {
		t.Fatalf("capture args = (%q, %q), want (-S, -10)", startFlag, lineCount)
	}
	if target != "=demo:" {
		t.Fatalf("capture target = %q, want =demo:", target)
	}
}
