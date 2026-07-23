package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func cliTestEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	configDir := filepath.Join(dir, "config")
	stateDir := filepath.Join(dir, "state")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", stateDir)
	// CLI tests mock tmux via SetTmuxHooksForTest; force TMUX so Detect() picks
	// the tmux backend regardless of the real environment.
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	t.Setenv("HERDR_ENV", "")
	t.Setenv("SESHAGY_LOG_LEVEL", "")
	t.Setenv("SESHAGY_LOG_FILE", "")
}

func manifestTestDirs(t *testing.T) {
	t.Helper()
	cliTestEnv(t)
}

func TestRunRoutingNoError(t *testing.T) {
	cliTestEnv(t)
	cases := [][]string{
		{"--help"},
		{"-h"},
		{"help"},
		{"--version"},
		{"version"},
		{"config", "path"},
		{"config"},
	}
	for _, args := range cases {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) unexpected error: %v", args, err)
		}
	}
}

func TestRunRoutingErrors(t *testing.T) {
	cliTestEnv(t)
	cases := [][]string{
		{"bogus"},
		{"--json"},
		{"config", "bogus"},
		{"config", "init", "bad"},
		{"agent"},
		{"agent", "frobnicate", "%1"},
		{"integration", "install"},
		{"integration", "frobnicate", "x"},
		{"--delete-item"},
		{"--report-agent", "--bogus"},
		{"--report-agent", "--state", "working"},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) expected error, got nil", args)
		}
	}
}

func TestMalformedAgentFlagsPreserveStderrUsageWithoutOpeningLog(t *testing.T) {
	cliTestEnv(t)
	t.Setenv("SESHAGY_LOG_LEVEL", "debug")

	tests := []struct {
		name         string
		command      string
		descriptions []string
	}{
		{
			name:    "report",
			command: "--report-agent",
			descriptions: []string{
				"agent name",
				"target by working directory (alternative to --pane)",
				"JSON output",
				"optional status message",
				"target pane id (e.g. %5)",
				"monotonic sequence number",
				"optional agent session id",
				"report source",
				"agent state",
			},
		},
		{
			name:    "release",
			command: "--release-agent",
			descriptions: []string{
				"target by working directory (alternative to --pane)",
				"JSON output",
				"target pane id (e.g. %5)",
				"monotonic sequence number",
				"report source",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), tt.name+".jsonl")
			t.Setenv("SESHAGY_LOG_FILE", logPath)
			stderr, err := captureStderr(t, func() error {
				return run([]string{tt.command, "--bogus"})
			})
			if err == nil {
				t.Fatal("malformed flag unexpectedly succeeded")
			}
			for _, want := range append([]string{
				"flag provided but not defined: -bogus",
				"Usage of " + tt.command + ":",
			}, tt.descriptions...) {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr missing %q:\n%s", want, stderr)
				}
			}
			if count := strings.Count(stderr, "Usage of "+tt.command+":"); count != 1 {
				t.Errorf("usage count = %d, want 1:\n%s", count, stderr)
			}
			if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
				t.Fatalf("malformed flags touched log path: %v", statErr)
			}
		})
	}
}

func TestRunConfigPathJSON(t *testing.T) {
	manifestTestDirs(t)
	out, err := captureStdout(t, func() error {
		return run([]string{"config", "path", "--json"})
	})
	if err != nil {
		t.Fatalf("run(config path --json) error = %v", err)
	}
	var payload struct {
		Ok   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Path == "" {
		t.Fatalf("config path payload = %#v", payload)
	}
}

func TestUnknownCommandErrorIncludesHint(t *testing.T) {
	err := unknownCommandError([]string{"frobnicate", "--json"})
	if err == nil || !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("unknownCommandError() = %v", err)
	}
}

func TestRunGetAgentsRoutes(t *testing.T) {
	cliTestEnv(t)
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			return nil, nil
		}
		return nil, nil
	}, nil)
	if err := run([]string{"--get-agents"}); err != nil {
		t.Fatalf("run(--get-agents) unexpected error: %v", err)
	}
}

func TestRunGetCurrentSessionAgentsRoutes(t *testing.T) {
	cliTestEnv(t)
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "display-message" {
			return []byte("mysession\n"), nil
		}
		return nil, nil
	}, nil)
	if err := run([]string{"--get-current-session-agents"}); err != nil {
		t.Fatalf("run(--get-current-session-agents) unexpected error: %v", err)
	}
}

func TestRunGetAgentsJSONIncludesAgentFields(t *testing.T) {
	cliTestEnv(t)
	line := strings.Join([]string{
		"%1", "seshagy", "1", "2", "/home/user/proj", "pi", "12345", "0",
	}, "\x1f")
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			return []byte(line), nil
		}
		return nil, nil
	}, nil)
	out, err := captureStdout(t, func() error {
		return run([]string{"--get-agents", "--json"})
	})
	if err != nil {
		t.Fatalf("run(--get-agents --json) error = %v", err)
	}
	var payload struct {
		Ok    bool `json:"ok"`
		Items []struct {
			Kind      string `json:"kind"`
			AgentName string `json:"agent_name"`
			State     string `json:"agent_state"`
			Location  string `json:"location"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || len(payload.Items) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	item := payload.Items[0]
	if item.Kind != "agent" || item.AgentName != "pi" {
		t.Fatalf("item = %+v", item)
	}
	if item.State != "idle" {
		t.Fatalf("agent_state = %q, want idle", item.State)
	}
	if item.Location != "seshagy:1.2" {
		t.Fatalf("location = %q, want seshagy:1.2", item.Location)
	}
}
