package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
	})
	err = fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), err
}

func TestStripJSONFlag(t *testing.T) {
	rest, jsonOutput := stripJSONFlag([]string{"--json", "foo", "--json"})
	if !jsonOutput {
		t.Fatal("expected json flag")
	}
	if len(rest) != 1 || rest[0] != "foo" {
		t.Fatalf("rest = %#v", rest)
	}
}

func TestVersionJSONOutput(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return run([]string{"--version", "--json"})
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.Version == "" {
		t.Fatalf("version payload = %#v", payload)
	}
}

func TestIntegrationStatusJSONOutput(t *testing.T) {
	out, err := captureStdout(t, func() error {
		return run([]string{"integration", "status", "--json"})
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var payload struct {
		Integrations []struct {
			Target string `json:"target"`
			Label  string `json:"label"`
			State  string `json:"state"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if len(payload.Integrations) == 0 {
		t.Fatal("expected integration records")
	}
}

func TestManifestStatusJSONShape(t *testing.T) {
	manifestTestDirs(t)
	out, err := captureStdout(t, func() error {
		return run([]string{"manifest", "status", "--json"})
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var payload struct {
		Catalog string `json:"catalog"`
		Status  struct {
			Agents map[string]any `json:"agents"`
		} `json:"status"`
		Agents []struct {
			AgentID      string `json:"agent_id"`
			ActiveSource struct {
				Kind string `json:"kind"`
			} `json:"active_source"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.Catalog == "" {
		t.Fatal("expected catalog in manifest status json")
	}
	if len(payload.Agents) == 0 {
		t.Fatal("expected agent summaries")
	}
	if payload.Agents[0].AgentID == "" || payload.Agents[0].ActiveSource.Kind == "" {
		t.Fatalf("unexpected agent summary payload: %#v", payload.Agents[0])
	}
}

func TestGetAgentsJSONRejectsExtraArgs(t *testing.T) {
	if err := run([]string{"--get-agents", "--json", "extra"}); err == nil {
		t.Fatal("expected usage error for extra args")
	}
}
