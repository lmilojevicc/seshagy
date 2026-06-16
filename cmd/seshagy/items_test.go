package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func installDeleteItemTmuxRecorder(
	t *testing.T,
	wantCmd string,
	onMatch func(args []string),
) {
	t.Helper()
	sessionmgr.SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 1 && args[0] == wantCmd {
			onMatch(args)
			return nil
		}
		return fmt.Errorf("unexpected tmux call: %v", args)
	})
}

func writeFDTestConfig(t *testing.T, fdDir string) {
	t.Helper()
	cfg := appconfig.Default()
	cfg.Directories.FDCommand = fmt.Sprintf("printf '%%s\\n' %s", fdDir)
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestPrintItemsJSONEnvelope(t *testing.T) {
	manifestTestDirs(t)
	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return printItems(context.Background(), sessionmgr.ModeFD, true)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Mode          string `json:"mode"`
		Items         []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if payload.Mode != "fd" {
		t.Fatalf("mode = %q, want fd", payload.Mode)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "fd" {
		t.Fatalf("items = %#v, want one fd item", payload.Items)
	}
}

func TestDeleteItemSessionJSONEnvelope(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)

	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), line, true)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Deleted       bool   `json:"deleted"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok || !payload.Deleted {
		t.Fatalf("envelope = %#v", payload)
	}
	if payload.Kind != string(sessionmgr.KindSession) || payload.Name != "demo" {
		t.Fatalf("delete payload = %#v", payload)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestPrintItemsTextOutput(t *testing.T) {
	manifestTestDirs(t)
	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return printItems(context.Background(), sessionmgr.ModeFD, false)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}
	if !strings.Contains(out, fdDir) {
		t.Fatalf("text output missing fd path:\n%s", out)
	}
}

func TestPrintItemsWritesWarningToStderr(t *testing.T) {
	manifestTestDirs(t)
	const pane = "%1"
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	agentLine := strings.Join([]string{
		pane, "work", "1", "0", t.TempDir(), "1", "1", "1", "0",
		"claude", "", "claude", "working", "", "123", "seshagy:claude", "", "42", "12345",
	}, "\x1f")
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 3 && args[0] == "list-sessions":
			return []byte(sessionLine), nil
		case len(args) >= 4 && args[0] == "list-panes" && args[1] == "-a":
			return []byte(agentLine + "\n"), nil
		}
		return nil, nil
	}, nil)

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	cfg.Directories.FDCommand = "sleep 30"
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("config.Save() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stderr, err := captureStderr(t, func() error {
		return printItems(ctx, sessionmgr.ModeAll, false)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}
	if !strings.Contains(stderr, "seshagy: warning:") || !strings.Contains(stderr, "fd command") {
		t.Fatalf("stderr = %q, want fd warning", stderr)
	}
}

func TestGetAllJSONIncludesZoxideWarning(t *testing.T) {
	manifestTestDirs(t)
	const pane = "%3"
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	agentLine := strings.Join([]string{
		pane, "work", "1", "0", t.TempDir(), "1", "1", "1", "0",
		"claude", "", "claude", "working", "", "123", "seshagy:claude", "", "42", "12345",
	}, "\x1f")
	sessionmgr.NewStrictFakeTmux(t, sessionmgr.NewFakeTmux()).
		AllowPaneOptions().
		AllowOutput(sessionmgr.MatchListSessions).
		AllowOutput(sessionmgr.MatchListPanesAgents).
		HandleOutput(sessionmgr.MatchListSessions, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(sessionLine), nil
		}).
		HandleOutput(sessionmgr.MatchListPanesAgents, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(agentLine + "\n"), nil
		}).
		Install(t)

	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Executable on PATH but bad interpreter → start error (exit errors are ignored).
	if err := os.WriteFile(
		filepath.Join(binDir, "zoxide"),
		[]byte("#!/nonexistent-zoxide-interpreter\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return run([]string{"--get-all", "--json"})
	})
	if err != nil {
		t.Fatalf("run(--get-all --json) error = %v", err)
	}
	var payload struct {
		Ok      bool   `json:"ok"`
		Mode    string `json:"mode"`
		Warning string `json:"warning"`
		Items   []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Mode != "all" {
		t.Fatalf("envelope = %#v", payload)
	}
	if !strings.Contains(payload.Warning, "zoxide query") {
		t.Fatalf("warning = %q, want zoxide failure", payload.Warning)
	}
	kinds := map[string]int{}
	for _, item := range payload.Items {
		kinds[item.Kind]++
	}
	if kinds["session"] != 1 || kinds["agent"] != 1 || kinds["fd"] != 1 {
		t.Fatalf("items = %#v, want session agent and fd", kinds)
	}
}

func TestDeleteItemSessionNonJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), line, false)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("non-json delete should be silent, got %q", out)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestDeleteItemAgentJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindAgent, PaneID: "%9", AgentName: "claude"},
		cfg.IconSet(),
	)
	var killedPane string
	installDeleteItemTmuxRecorder(t, "kill-pane", func(args []string) {
		if len(args) >= 3 {
			killedPane = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), line, true)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}
	var payload struct {
		Ok        bool   `json:"ok"`
		Deleted   bool   `json:"deleted"`
		Kind      string `json:"kind"`
		PaneID    string `json:"pane_id"`
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || !payload.Deleted || payload.Kind != "agent" ||
		payload.PaneID != "%9" || payload.AgentName != "claude" {
		t.Fatalf("delete payload = %#v", payload)
	}
	if killedPane != "%9" {
		t.Fatalf("kill-pane target = %q, want %%9", killedPane)
	}
}

func TestDeleteItemNonDeletableKind(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
		cfg.IconSet(),
	)
	err = deleteItem(context.Background(), line, false)
	if err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("deleteItem() error = %v, want cannot be deleted", err)
	}
}

func TestDeleteItemAgentNonJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindAgent, PaneID: "%9", AgentName: "claude"},
		cfg.IconSet(),
	)
	var killedPane string
	installDeleteItemTmuxRecorder(t, "kill-pane", func(args []string) {
		if len(args) >= 3 {
			killedPane = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), line, false)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("non-json delete should be silent, got %q", out)
	}
	if killedPane != "%9" {
		t.Fatalf("kill-pane target = %q, want %%9", killedPane)
	}
}

func TestRunGetSessionsTextOutput(t *testing.T) {
	manifestTestDirs(t)
	dir := t.TempDir()
	raw := strings.Join([]string{"demo", "100", "200", dir, "0", "1"}, "\x1f") + "\n"
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return []byte(raw), nil
		}
		return nil, nil
	}, nil)

	out, err := captureStdout(t, func() error {
		return run([]string{"--get-sessions"})
	})
	if err != nil {
		t.Fatalf("run(--get-sessions) error = %v", err)
	}
	if !strings.Contains(out, "demo") {
		t.Fatalf("sessions output missing demo:\n%s", out)
	}
}

func TestRunGetAgentsAndAllJSON(t *testing.T) {
	cliTestEnv(t)
	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	const pane = "%11"
	fields := strings.Join([]string{
		pane, "work", "1", "0", t.TempDir(), "1", "1", "1", "0",
		"claude", "", "claude", "working", "", "123", "seshagy:claude", "", "42", "12345",
	}, "\x1f")
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "list-sessions":
			return nil, nil
		case sessionmgr.MatchListPanesAgents(args):
			return []byte(fields + "\n"), nil
		default:
			if len(args) >= 1 && args[0] == "list-panes" {
				t.Fatalf("list-panes args = %v, want -a -F agentFormat", args)
			}
		}
		return nil, nil
	}, nil)

	agentsOut, err := captureStdout(t, func() error {
		return run([]string{"--get-agents", "--json"})
	})
	if err != nil {
		t.Fatalf("run(--get-agents) error = %v", err)
	}
	var agentsPayload struct {
		Ok    bool `json:"ok"`
		Items []struct {
			Kind      string `json:"kind"`
			AgentName string `json:"agent_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(agentsOut)), &agentsPayload); err != nil {
		t.Fatalf("json.Unmarshal(--get-agents) error = %v, out=%q", err, agentsOut)
	}
	if !agentsPayload.Ok {
		t.Fatalf("--get-agents payload = %#v", agentsPayload)
	}
	if len(agentsPayload.Items) != 1 {
		t.Fatalf("--get-agents len(items) = %d, want 1", len(agentsPayload.Items))
	}
	if agentsPayload.Items[0].Kind != "agent" || agentsPayload.Items[0].AgentName != "claude" {
		t.Fatalf("--get-agents items = %#v", agentsPayload.Items)
	}

	allOut, err := captureStdout(t, func() error {
		return run([]string{"--get-all", "--json"})
	})
	if err != nil {
		t.Fatalf("run(--get-all) error = %v", err)
	}
	var allPayload struct {
		Ok    bool `json:"ok"`
		Items []struct {
			Kind      string `json:"kind"`
			AgentName string `json:"agent_name"`
			Path      string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(allOut)), &allPayload); err != nil {
		t.Fatalf("json.Unmarshal(--get-all) error = %v, out=%q", err, allOut)
	}
	if !allPayload.Ok {
		t.Fatalf("--get-all payload = %#v", allPayload)
	}
	if len(allPayload.Items) < 2 {
		t.Fatalf("--get-all len(items) = %d, want at least agent and fd", len(allPayload.Items))
	}
	kinds := map[string]int{}
	for _, item := range allPayload.Items {
		kinds[item.Kind]++
	}
	if kinds["agent"] != 1 || kinds["fd"] != 1 {
		t.Fatalf("--get-all kinds = %#v, want one agent and one fd", kinds)
	}
	for _, item := range allPayload.Items {
		if item.Kind == "agent" && item.AgentName != "claude" {
			t.Fatalf("--get-all agent item = %#v, want claude", item)
		}
		if item.Kind == "fd" && item.Path != fdDir {
			t.Fatalf("--get-all fd item path = %q, want %q", item.Path, fdDir)
		}
	}
}

func TestRunDeleteItemViaCLI(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return run([]string{"--delete-item", line, "--json"})
	})
	if err != nil {
		t.Fatalf("run(--delete-item) error = %v", err)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Deleted       bool   `json:"deleted"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok || !payload.Deleted {
		t.Fatalf("envelope = %#v", payload)
	}
	if payload.Kind != string(sessionmgr.KindSession) || payload.Name != "demo" {
		t.Fatalf("delete payload = %#v", payload)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestDeleteItemUnrecognizedLine(t *testing.T) {
	manifestTestDirs(t)
	err := deleteItem(context.Background(), "not a valid item line", false)
	if err == nil || !strings.Contains(err.Error(), "unrecognized item line") {
		t.Fatalf("deleteItem() error = %v, want unrecognized item line", err)
	}
}

func TestDeleteItemKillFailure(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	killErr := fmt.Errorf("tmux refused kill")
	tests := []struct {
		name    string
		wantCmd string
		line    string
	}{
		{
			name:    "session",
			wantCmd: "kill-session",
			line: sessionmgr.FormatLineWithIcons(
				sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
				cfg.IconSet(),
			),
		},
		{
			name:    "agent",
			wantCmd: "kill-pane",
			line: sessionmgr.FormatLineWithIcons(
				sessionmgr.Item{Kind: sessionmgr.KindAgent, PaneID: "%9", AgentName: "claude"},
				cfg.IconSet(),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionmgr.SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
				if len(args) >= 1 && args[0] == tt.wantCmd {
					return killErr
				}
				return fmt.Errorf("unexpected tmux call: %v", args)
			})
			err := deleteItem(context.Background(), tt.line, false)
			if err == nil {
				t.Fatalf("deleteItem() expected error for %s failure", tt.wantCmd)
			}
			if !strings.Contains(err.Error(), "tmux "+tt.wantCmd) {
				t.Fatalf("deleteItem() error = %v, want tmux %s wrapper", err, tt.wantCmd)
			}
		})
	}
}

func TestDeleteItemKillFailureJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	killErr := fmt.Errorf("tmux refused kill")
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	sessionmgr.SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) >= 1 && args[0] == "kill-session" {
			return killErr
		}
		return fmt.Errorf("unexpected tmux call: %v", args)
	})

	args := []string{"--delete-item", line, "--json"}
	out, err := captureStdout(t, func() error {
		return runWithCLIJSONHandling(args)
	})
	if err != nil {
		t.Fatalf("runWithCLIJSONHandling() error = %v", err)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Error         string `json:"error"`
		Code          string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || payload.Ok {
		t.Fatalf("envelope = %#v, want ok=false", payload)
	}
	if !strings.Contains(payload.Error, "tmux kill-session") {
		t.Fatalf("error = %q, want tmux kill-session wrapper", payload.Error)
	}
	if payload.Code != "error" {
		t.Fatalf("code = %q, want error", payload.Code)
	}
}
