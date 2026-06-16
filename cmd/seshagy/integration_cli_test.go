package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIntegrationInstallPiJSON(t *testing.T) {
	cliTestEnv(t)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	jsonOut, err := captureStdout(t, func() error {
		return runIntegration([]string{"install", "pi", "--json"})
	})
	if err != nil {
		t.Fatalf("runIntegration(install pi --json) error = %v", err)
	}
	var payload struct {
		Ok       bool     `json:"ok"`
		Target   string   `json:"target"`
		Action   string   `json:"action"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || payload.Target != "pi" || payload.Action != "install" ||
		len(payload.Messages) == 0 {
		t.Fatalf("install payload = %#v", payload)
	}
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "seshagy-agent-state.ts")
	if _, err := os.Stat(extPath); err != nil {
		t.Fatalf("pi extension missing at %s: %v", extPath, err)
	}
}

func TestRunIntegrationBareStatus(t *testing.T) {
	cliTestEnv(t)
	out, err := captureStdout(t, func() error {
		return runIntegration(nil)
	})
	if err != nil {
		t.Fatalf("runIntegration(nil) error = %v", err)
	}
	if !strings.Contains(out, "pi") {
		t.Fatalf("bare integration status missing pi:\n%s", out)
	}
}

func TestRunIntegrationStatusTextAndJSON(t *testing.T) {
	cliTestEnv(t)
	t.Setenv("PATH", t.TempDir())
	textOut, err := captureStdout(t, func() error {
		return runIntegration([]string{"status"})
	})
	if err != nil {
		t.Fatalf("runIntegration(status) error = %v", err)
	}
	for _, target := range []string{"pi", "cursor", "hermes"} {
		if !strings.Contains(textOut, target) {
			t.Fatalf("status text missing target %q:\n%s", target, textOut)
		}
	}

	jsonOut, err := captureStdout(t, func() error {
		return runIntegration([]string{"status", "--json"})
	})
	if err != nil {
		t.Fatalf("runIntegration(status --json) error = %v", err)
	}
	var payload struct {
		SchemaVersion int  `json:"schema_version"`
		Ok            bool `json:"ok"`
		Integrations  []struct {
			Target         string   `json:"target"`
			Label          string   `json:"label"`
			State          string   `json:"state"`
			AgentAvailable bool     `json:"agent_available"`
			Installable    bool     `json:"installable"`
			Authority      string   `json:"authority"`
			Reason         string   `json:"reason"`
			InstallPath    string   `json:"install_path"`
			ConfigDir      string   `json:"config_dir"`
			Commands       []string `json:"commands"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if len(payload.Integrations) == 0 {
		t.Fatal("expected integration records in JSON output")
	}

	byTarget := make(map[string]struct {
		State          string
		AgentAvailable bool
		Installable    bool
		Authority      string
		Label          string
	}, len(payload.Integrations))
	for _, rec := range payload.Integrations {
		if rec.Target == "" || rec.Label == "" || rec.State == "" || rec.Authority == "" {
			t.Fatalf("integration record missing fields: %#v", rec)
		}
		if rec.InstallPath == "" && rec.Installable {
			t.Fatalf("installable target %q missing install_path", rec.Target)
		}
		byTarget[rec.Target] = struct {
			State          string
			AgentAvailable bool
			Installable    bool
			Authority      string
			Label          string
		}{
			State:          rec.State,
			AgentAvailable: rec.AgentAvailable,
			Installable:    rec.Installable,
			Authority:      rec.Authority,
			Label:          rec.Label,
		}
	}

	for _, target := range []string{"pi", "cursor", "hermes", "claude", "codex"} {
		rec, ok := byTarget[target]
		if !ok {
			t.Fatalf("status JSON missing target %q", target)
		}
		switch target {
		case "pi":
			if rec.Label != "Pi" || rec.Authority != "lifecycle" {
				t.Fatalf("pi record = %#v", rec)
			}
			if rec.State != "not-installed" && rec.State != "current" && rec.State != "outdated" {
				t.Fatalf("pi state = %q", rec.State)
			}
		case "cursor":
			if rec.Label != "Cursor Agent" {
				t.Fatalf("cursor label = %q", rec.Label)
			}
			if rec.AgentAvailable {
				t.Fatal("cursor should not be agent_available without cursor-agent on PATH")
			}
			if rec.Installable {
				t.Fatal("cursor should not be installable without cursor-agent on PATH")
			}
			if rec.State != "not-installed" {
				t.Fatalf("cursor state = %q, want not-installed", rec.State)
			}
		case "hermes":
			if rec.Label != "Hermes Agent" || rec.Authority != "lifecycle" {
				t.Fatalf("hermes record = %#v", rec)
			}
		}
	}
}

func TestRunIntegrationInstallMissingTargetError(t *testing.T) {
	cliTestEnv(t)
	err := runIntegration([]string{"install", "not-a-target"})
	if err == nil {
		t.Fatal("runIntegration(install not-a-target) expected error, got nil")
	}
	if !strings.Contains(err.Error(), `unknown integration target "not-a-target"`) {
		t.Fatalf("runIntegration() error = %v, want unknown integration target", err)
	}
}

func TestRunIntegrationInstallAndUninstallPiCLI(t *testing.T) {
	cliTestEnv(t)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	installOut, err := captureStdout(t, func() error {
		return runIntegration([]string{"install", "pi"})
	})
	if err != nil {
		t.Fatalf("runIntegration(install pi) error = %v", err)
	}
	if !strings.Contains(installOut, "installed Pi extension") {
		t.Fatalf("install output = %q", installOut)
	}
	extPath := filepath.Join(home, ".pi", "agent", "extensions", "seshagy-agent-state.ts")
	if _, err := os.Stat(extPath); err != nil {
		t.Fatalf("pi extension missing at %s: %v", extPath, err)
	}

	jsonOut, err := captureStdout(t, func() error {
		return runIntegration([]string{"uninstall", "pi", "--json"})
	})
	if err != nil {
		t.Fatalf("runIntegration(uninstall pi --json) error = %v", err)
	}
	var payload struct {
		Ok       bool     `json:"ok"`
		Target   string   `json:"target"`
		Action   string   `json:"action"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || payload.Target != "pi" || payload.Action != "uninstall" {
		t.Fatalf("uninstall payload = %#v", payload)
	}
	if _, err := os.Stat(extPath); !os.IsNotExist(err) {
		t.Fatalf("pi extension should be removed at %s, stat err=%v", extPath, err)
	}
}
