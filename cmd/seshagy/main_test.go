package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{"--release-agent", "--seq", "-1"},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) expected error, got nil", args)
		}
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
