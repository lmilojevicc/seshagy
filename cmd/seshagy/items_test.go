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
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "list-sessions" {
			return []byte(sessionLine), nil
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
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	sessionmgr.NewStrictFakeTmux(t, sessionmgr.NewFakeTmux()).
		AllowPaneOptions().
		AllowOutput(sessionmgr.MatchListSessions).
		AllowOutput(sessionmgr.MatchListPanes).
		HandleOutput(sessionmgr.MatchListSessions, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(sessionLine), nil
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
	if kinds["session"] != 1 || kinds["fd"] != 1 {
		t.Fatalf("items = %#v, want session and fd", kinds)
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
	err = run(args)
	if err == nil {
		t.Fatal("run() error = nil, want kill-session error")
	}
	if !strings.Contains(err.Error(), "tmux kill-session") {
		t.Fatalf("error = %q, want tmux kill-session wrapper", err.Error())
	}
	out, encErr := captureStdout(t, func() error {
		return encodeJSONError(err)
	})
	if encErr != nil {
		t.Fatalf("encodeJSONError() error = %v", encErr)
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
