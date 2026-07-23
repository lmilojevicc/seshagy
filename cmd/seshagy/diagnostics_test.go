package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestDiagnosticsJSONRedactsPathsAndHasNoSideEffects(t *testing.T) {
	cliTestEnv(t)
	secret := "SecretHomePathABC123"
	custom := filepath.Join(t.TempDir(), secret, "diagnostics.jsonl")
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: custom}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error { return run([]string{"diagnostics", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("diagnostics JSON leaked path: %s", out)
	}
	var payload struct {
		Ok      bool `json:"ok"`
		Logging struct {
			PathRedacted bool `json:"path_redacted"`
			Enabled      bool `json:"enabled"`
		} `json:"logging"`
		Guidance struct {
			Upload bool `json:"upload"`
		} `json:"guidance"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Ok || !payload.Logging.PathRedacted || !payload.Logging.Enabled ||
		payload.Guidance.Upload {
		t.Fatalf("payload = %+v", payload)
	}
	if _, err := os.Stat(custom); !os.IsNotExist(err) {
		t.Fatalf("diagnostics created custom file: %v", err)
	}
}

func TestDiagnosticsHumanOutputShowsLocalPath(t *testing.T) {
	cliTestEnv(t)
	custom := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: custom}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error { return run([]string{"diagnostics"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, custom) || !strings.Contains(out, "never uploaded") {
		t.Fatalf("human diagnostics missing path/guidance: %q", out)
	}
	if _, err := os.Stat(custom); !os.IsNotExist(err) {
		t.Fatalf("human diagnostics created custom file: %v", err)
	}
}

func TestPureAndInvalidCommandsDoNotTruncateConfiguredLog(t *testing.T) {
	cliTestEnv(t)
	custom := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	const sentinel = "ExistingEvidenceABC123"
	if err := os.WriteFile(custom, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: custom}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"--version"},
		{"--help"},
		{"config", "path"},
		{"config", "show"},
		{"config", "init"},
		{"diagnostics", "--json"},
		{"--get-agents", "extra"},
		{"--report-agent", "--pane", "%1", "--source", "hook"},
		{"--release-agent", "--source", "hook"},
		{"--delete-item", "not-a-rendered-item"},
		{"integration", "install"},
		{"keybind", "install", "tmux", "--mode", "bad"},
	}
	for _, args := range commands {
		_ = run(args)
		got, err := os.ReadFile(custom)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != sentinel {
			t.Fatalf("run(%v) changed configured log: %q", args, got)
		}
	}
}

func TestOperationalCommandWritesStructuredLogWithoutChangingJSON(t *testing.T) {
	cliTestEnv(t)
	custom := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: custom}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "list-sessions" {
			return nil, nil
		}
		return nil, nil
	}, nil)
	out, err := captureStdout(t, func() error { return run([]string{"--get-sessions", "--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("machine stdout is not JSON: %v: %q", err, out)
	}
	data, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid JSONL: %v: %q", err, line)
		}
		seen[record["event"].(string)] = true
	}
	for _, event := range []string{"app.start", "source.load", "app.stop"} {
		if !seen[event] {
			t.Fatalf("missing event %s in %s", event, data)
		}
	}
}

func TestDiagnosticsJSONFailuresArePathFree(t *testing.T) {
	cliTestEnv(t)
	const secret = "SecretDiagnosticPathABC123"
	parent := filepath.Join(t.TempDir(), secret)
	if err := os.WriteFile(parent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: filepath.Join(parent, "log.jsonl")}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() error {
		diagnosticsErr := run([]string{"diagnostics", "--json"})
		if diagnosticsErr == nil {
			return errors.New("expected diagnostics failure")
		}
		return encodeJSONError(diagnosticsErr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, secret) || strings.Contains(out, "\x1b[") {
		t.Fatalf("diagnostics error leaked path or ANSI: %q", out)
	}

	const levelSecret = "SecretInvalidLogLevelABC123"
	cfg.Log = appconfig.LogConfig{Level: levelSecret}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	out, err = captureStdout(t, func() error {
		diagnosticsErr := run([]string{"diagnostics", "--json"})
		if diagnosticsErr == nil {
			return errors.New("expected invalid-level diagnostics failure")
		}
		return encodeJSONError(diagnosticsErr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, levelSecret) || strings.Contains(out, "\x1b[") {
		t.Fatalf("diagnostics error leaked level or ANSI: %q", out)
	}

	const configSecret = "SecretMalformedConfigValueABC123"
	if err := os.WriteFile(appconfig.Path(), []byte("[log\n"+configSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = captureStdout(t, func() error {
		diagnosticsErr := run([]string{"diagnostics", "--json"})
		if diagnosticsErr == nil {
			return errors.New("expected malformed-config diagnostics failure")
		}
		return encodeJSONError(diagnosticsErr)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, configSecret) || strings.Contains(out, appconfig.Path()) {
		t.Fatalf("diagnostics error leaked config details: %q", out)
	}
}

func TestDiagnosticsDoesNotModifyExistingFile(t *testing.T) {
	cliTestEnv(t)
	custom := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	before := []byte("ExistingDiagnosticEvidenceABC123")
	if err := os.WriteFile(custom, before, 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(custom, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	cfg := appconfig.Default()
	cfg.Log = appconfig.LogConfig{Level: "debug", File: custom}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := captureStdout(
			t,
			func() error { return run([]string{"diagnostics", "--json"}) },
		); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(custom)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !info.ModTime().Equal(stamp) {
		t.Fatalf("diagnostics changed file: bytes=%q mtime=%v", after, info.ModTime())
	}
}

func TestInvalidLevelAndFileAloneDoNotModifyDestination(t *testing.T) {
	for _, tt := range []struct {
		name  string
		level string
	}{
		{name: "invalid level", level: "SecretInvalidLevel"},
		{name: "file alone remains off", level: "off"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cliTestEnv(t)
			custom := filepath.Join(t.TempDir(), "diagnostics.jsonl")
			const sentinel = "ExistingEvidenceABC123"
			if err := os.WriteFile(custom, []byte(sentinel), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg := appconfig.Default()
			cfg.Log = appconfig.LogConfig{Level: tt.level, File: custom}
			if err := appconfig.Save(cfg); err != nil {
				t.Fatal(err)
			}
			err := run([]string{"--get-sessions", "--json"})
			if tt.level == "off" && err != nil {
				t.Fatal(err)
			}
			if tt.level != "off" && err == nil {
				t.Fatal("invalid level unexpectedly succeeded")
			}
			got, readErr := os.ReadFile(custom)
			if readErr != nil || string(got) != sentinel {
				t.Fatalf("destination changed: %q, %v", got, readErr)
			}
		})
	}
}

func TestPureCommandsNeverPruneDefaultLogs(t *testing.T) {
	cliTestEnv(t)
	cfg := appconfig.Default()
	cfg.Log.Level = "debug"
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "seshagy", "log")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		name := fmt.Sprintf(
			"seshagy-20250101T0000%02d.000000000Z-1-%016x.jsonl",
			i,
			i+1,
		)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"--version"}, {"config", "path"}, {"config", "show"}, {"diagnostics", "--json"}} {
		if _, err := captureStdout(t, func() error { return run(args) }); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 12 {
		t.Fatalf("pure commands pruned logs: got %d files", len(entries))
	}
}

func TestLoggingDoesNotChangeMachineStdout(t *testing.T) {
	cliTestEnv(t)
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, _ ...string) ([]byte, error) {
		return nil, nil
	}, nil)
	commands := [][]string{
		{"--version"},
		{"--version", "--json"},
		{"config", "path"},
		{"--get-sessions", "--json"},
	}
	for i, args := range commands {
		cfg := appconfig.Default()
		if err := appconfig.Save(cfg); err != nil {
			t.Fatal(err)
		}
		off, err := captureStdout(t, func() error { return run(args) })
		if err != nil {
			t.Fatalf("off run(%v): %v", args, err)
		}
		cfg.Log = appconfig.LogConfig{
			Level: "debug", File: filepath.Join(t.TempDir(), fmt.Sprintf("log-%d.jsonl", i)),
		}
		if err := appconfig.Save(cfg); err != nil {
			t.Fatal(err)
		}
		enabled, err := captureStdout(t, func() error { return run(args) })
		if err != nil {
			t.Fatalf("enabled run(%v): %v", args, err)
		}
		if off != enabled {
			t.Fatalf("run(%v) stdout changed with logging:\noff=%q\non=%q", args, off, enabled)
		}
	}
}
