package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestAttachExecCallbackMapsErrors(t *testing.T) {
	if done := attachExecCallback(nil).(attachDoneMsg); done.err != nil {
		t.Fatalf("success callback = %#v", done)
	}
	err := errors.New("attach failed")
	if done := attachExecCallback(err).(attachDoneMsg); !errors.Is(done.err, err) {
		t.Fatalf("error callback = %#v", done)
	}
}

func TestAttachCmdReturnsExecProcess(t *testing.T) {
	if attachCmd("demo") == nil {
		t.Fatal("attachCmd() returned nil")
	}
}

func TestCreateDeleteRenameCommandsUseTmuxHooks(t *testing.T) {
	dir := t.TempDir()
	var killedSession, renamedOld, renamedNew string
	var newSessionArgs []string
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, nil
		}
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		switch {
		case len(args) >= 3 && args[0] == "kill-session":
			killedSession = strings.TrimPrefix(args[2], "=")
		case len(args) >= 4 && args[0] == "rename-session":
			renamedOld = strings.TrimPrefix(args[2], "=")
			renamedNew = args[3]
		case sessionmgr.MatchNewSession(args):
			newSessionArgs = append([]string(nil), args...)
		}
		return nil
	})

	createMsg := createSessionCmd(dir)().(createDoneMsg)
	if createMsg.err != nil || !createMsg.created {
		t.Fatalf("createSessionCmd() = %#v", createMsg)
	}
	if newSessionArgs == nil {
		t.Fatal("expected new-session call when creating session")
	}
	if len(newSessionArgs) < 6 || newSessionArgs[0] != "new-session" || newSessionArgs[1] != "-d" ||
		newSessionArgs[4] != "-c" || newSessionArgs[5] != dir {
		t.Fatalf("new-session args = %v", newSessionArgs)
	}

	deleteMsg := deleteSessionCmd("demo")().(actionDoneMsg)
	if deleteMsg.err != nil || deleteMsg.status != "killed session demo" {
		t.Fatalf("deleteSessionCmd() = %#v", deleteMsg)
	}
	if killedSession != "demo" {
		t.Fatalf("kill-session target = %q, want demo", killedSession)
	}

	renameMsg := renameCmd("old", "new")().(actionDoneMsg)
	if renameMsg.err != nil || renamedOld != "old" || renamedNew != "new" {
		t.Fatalf("renameCmd() = %#v old=%q new=%q", renameMsg, renamedOld, renamedNew)
	}
}

func TestCreateSessionCmdReusesExistingWithoutNewSession(t *testing.T) {
	dir := t.TempDir()
	raw := sessionListLine("work", dir)
	var newSessionCalled bool
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return []byte(raw), nil
		}
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		if sessionmgr.MatchNewSession(args) {
			newSessionCalled = true
		}
		return nil
	})

	createMsg := createSessionCmd(dir)().(createDoneMsg)
	if createMsg.err != nil {
		t.Fatalf("createSessionCmd() err = %v", createMsg.err)
	}
	if createMsg.created {
		t.Fatalf("createSessionCmd() created = true, want false when session exists")
	}
	if createMsg.name != "work" {
		t.Fatalf("createSessionCmd() name = %q, want work", createMsg.name)
	}
	if newSessionCalled {
		t.Fatal("new-session should not run when reusing an existing session")
	}
}

func TestCreateSessionCmdReturnsNewSessionError(t *testing.T) {
	dir := t.TempDir()
	createErr := errors.New("new-session failed")
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, nil
		}
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		if sessionmgr.MatchNewSession(args) {
			return createErr
		}
		return nil
	})

	createMsg := createSessionCmd(dir)().(createDoneMsg)
	if !errors.Is(createMsg.err, createErr) {
		t.Fatalf("createSessionCmd() err = %v, want %v", createMsg.err, createErr)
	}
	if createMsg.created {
		t.Fatalf("createSessionCmd() created = true, want false on error")
	}
}

func TestRenameCmdReturnsTmuxError(t *testing.T) {
	renameErr := errors.New("rename failed")
	sessionmgr.SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 1 && args[0] == "rename-session" {
			return renameErr
		}
		return nil
	})

	renameMsg := renameCmd("old", "new")().(actionDoneMsg)
	if !errors.Is(renameMsg.err, renameErr) {
		t.Fatalf("renameCmd() err = %v, want %v", renameMsg.err, renameErr)
	}
	if renameMsg.status != "renamed old to new" {
		t.Fatalf("renameCmd() status = %q", renameMsg.status)
	}
}

func TestPreviewCmdDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirPreview := previewCmd(sessionmgr.Item{
		Kind: sessionmgr.KindFD,
		Path: dir,
	})().(previewMsg)
	if dirPreview.err != nil || !strings.Contains(dirPreview.preview, "readme.txt") {
		t.Fatalf("directory preview = %#v", dirPreview)
	}
}

func TestPreviewCmdAgentRoutesToCaptureAgentPane(t *testing.T) {
	var capturedPane string
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[1] == "-ep" && args[2] == "-t" {
			capturedPane = args[3]
			return []byte("agent pane output\nlatest line"), nil
		}
		return nil, nil
	}, nil)

	msg := previewCmd(sessionmgr.Item{
		Kind:   sessionmgr.KindAgent,
		PaneID: "%7",
	})().(previewMsg)
	if msg.err != nil {
		t.Fatalf("agent preview err = %v", msg.err)
	}
	if capturedPane != "%7" {
		t.Fatalf("capture-pane target = %q, want %%7", capturedPane)
	}
	if !strings.Contains(msg.preview, "latest line") {
		t.Fatalf("agent preview = %q, want captured output", msg.preview)
	}
}

func TestPreviewCmdAgentEmptyPaneIDFallsBack(t *testing.T) {
	msg := previewCmd(sessionmgr.Item{
		Kind:   sessionmgr.KindAgent,
		PaneID: "",
	})().(previewMsg)
	if msg.err != nil {
		t.Fatalf("agent empty PaneID err = %v", msg.err)
	}
	if msg.preview != "no preview available" {
		t.Fatalf("agent empty PaneID preview = %q, want no preview available", msg.preview)
	}
}

func TestStartupSetupCommands(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Setup.TypeFirstPromptSeen = false
	msg := startupSetupCmd(cfg)().(setupMsg)
	if !msg.prompt || msg.err != nil {
		t.Fatalf("startupSetupCmd unseen = %#v", msg)
	}

	cfg.Setup.TypeFirstPromptSeen = true
	msg = startupSetupCmd(cfg)().(setupMsg)
	if msg.prompt || msg.err != nil {
		t.Fatalf("startupSetupCmd seen = %#v", msg)
	}
}

func TestRefreshCmdLoadsItems(t *testing.T) {
	dir := t.TempDir()
	cfg := appconfig.Default()
	cfg.Directories.FDCommand = "printf '%s\\n' " + dir
	raw := sessionListLine("demo", dir)
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return []byte(raw), nil
		}
		return nil, nil
	}, nil)

	msg := refreshCmd(sessionmgr.ModeSessions, 1, cfg.LoadOptions())().(refreshMsg)
	if msg.err != nil || len(msg.items) != 1 || msg.items[0].Name != "demo" {
		t.Fatalf("refreshCmd() = %#v", msg)
	}
}

func sessionListLine(name, path string) string {
	return strings.Join([]string{name, "100", "200", path, "0", "1"}, "\x1f")
}
