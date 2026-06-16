package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
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

func TestAttachAndFocusAgentCmdReturnExecProcess(t *testing.T) {
	if attachCmd("demo") == nil {
		t.Fatal("attachCmd() returned nil")
	}
	if focusAgentCmd("%1") == nil {
		t.Fatal("focusAgentCmd() returned nil")
	}
}

func TestCreateDeleteRenameCommandsUseTmuxHooks(t *testing.T) {
	dir := t.TempDir()
	var killedSession, renamedOld, renamedNew, killedPane string
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
		case len(args) >= 3 && args[0] == "kill-pane":
			killedPane = args[2]
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

	agentMsg := deleteAgentCmd("%4")().(actionDoneMsg)
	if agentMsg.err != nil || killedPane != "%4" {
		t.Fatalf("deleteAgentCmd() = %#v pane=%q", agentMsg, killedPane)
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

func TestRenameAgentCmdSetsLabel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	msg := renameAgentCmd("%1", "my bot", "pi")().(actionDoneMsg)
	if msg.err != nil {
		t.Fatalf("renameAgentCmd() err = %v", msg.err)
	}
	if msg.status != "renamed agent pi to my bot" {
		t.Fatalf("renameAgentCmd() status = %q", msg.status)
	}
	store, err := sessionmgr.LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get("%1", "pi"); got != "my bot" {
		t.Fatalf("stored label = %q, want my bot", got)
	}
}

func TestRenameAgentCmdClearsLabel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := sessionmgr.SetAgentDisplayName("%1", "old label", "pi"); err != nil {
		t.Fatal(err)
	}
	msg := renameAgentCmd("%1", "", "pi")().(actionDoneMsg)
	if msg.err != nil {
		t.Fatalf("renameAgentCmd() err = %v", msg.err)
	}
	if msg.status != "cleared agent label for %1" {
		t.Fatalf("renameAgentCmd() status = %q", msg.status)
	}
	store, err := sessionmgr.LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get("%1", "pi"); got != "" {
		t.Fatalf("stored label = %q, want empty", got)
	}
}

func TestPreviewCmdAgentAndDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "capture-pane" && args[3] == "%5" {
			return []byte("agent output\n"), nil
		}
		return nil, nil
	}, nil)

	agentPreview := previewCmd(sessionmgr.Item{
		Kind:   sessionmgr.KindAgent,
		PaneID: "%5",
	})().(previewMsg)
	if agentPreview.err != nil || !strings.Contains(agentPreview.preview, "agent output") {
		t.Fatalf("agent preview = %#v", agentPreview)
	}

	dirPreview := previewCmd(sessionmgr.Item{
		Kind: sessionmgr.KindFD,
		Path: dir,
	})().(previewMsg)
	if dirPreview.err != nil || !strings.Contains(dirPreview.preview, "readme.txt") {
		t.Fatalf("directory preview = %#v", dirPreview)
	}
}

func TestIntegrationCommands(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	scanMsg := integrationsCmd()().(integrationsMsg)
	if scanMsg.err != nil {
		t.Fatalf("integrationsCmd() error = %v", scanMsg.err)
	}

	installMsg := installIntegrationsCmd([]integrations.Target{integrations.TargetPi})().(integrationsInstalledMsg)
	if installMsg.err != nil || len(installMsg.messages) == 0 {
		t.Fatalf("installIntegrationsCmd() = %#v", installMsg)
	}
}

func TestInstallIntegrationsCmdPartialFailureKeepsPriorInstalls(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	msg := installIntegrationsCmd([]integrations.Target{
		integrations.TargetPi,
		integrations.TargetCodex,
	})().(integrationsInstalledMsg)
	if msg.err == nil {
		t.Fatal("installIntegrationsCmd() expected error for missing codex config")
	}
	if !strings.Contains(msg.err.Error(), "codex config directory not found") {
		t.Fatalf("installIntegrationsCmd() error = %v", msg.err)
	}
	if len(msg.messages) == 0 {
		t.Fatal("expected pi install messages before codex failure")
	}
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "seshagy-agent-state.ts")
	if _, err := os.Stat(extPath); err != nil {
		t.Fatalf("pi extension should remain after partial failure at %s: %v", extPath, err)
	}
}

func TestStartupIntegrationsCmdReturnsCheckError(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	versionPath := filepath.Join(stateDir, "seshagy", integrationPromptVersionFile)
	if err := os.MkdirAll(filepath.Dir(versionPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(versionPath, 0o700); err != nil {
		t.Fatal(err)
	}

	msg := startupIntegrationsCmd()().(integrationsMsg)
	if msg.err == nil {
		t.Fatal("expected startup integration check error")
	}
	if !strings.Contains(msg.err.Error(), "check startup hook prompt") {
		t.Fatalf("integrationsMsg.err = %v", msg.err)
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
