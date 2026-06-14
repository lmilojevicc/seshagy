package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestHasJSONFlag(t *testing.T) {
	if hasJSONFlag([]string{"--get-agents"}) {
		t.Fatal("expected no json flag")
	}
	if !hasJSONFlag([]string{"--get-agents", "--json"}) {
		t.Fatal("expected trailing json flag")
	}
	if !hasJSONFlag([]string{"--json", "--get-agents"}) {
		t.Fatal("expected leading json flag")
	}
	if !hasJSONFlag([]string{"--delete-item", "line with --json", "--json"}) {
		t.Fatal("expected json flag in delete-item args")
	}
}

func TestParseDeleteItemArgsPreservesJSONInLineText(t *testing.T) {
	line, jsonOutput := parseDeleteItemArgs([]string{"session", "foo", "--json", "bar"})
	if jsonOutput {
		t.Fatal("expected jsonOutput=false when --json is not trailing")
	}
	if line != "session foo --json bar" {
		t.Fatalf("line = %q, want %q", line, "session foo --json bar")
	}
}

func TestParseDeleteItemArgsStripsTrailingJSON(t *testing.T) {
	line, jsonOutput := parseDeleteItemArgs([]string{"session", "foo", "--json"})
	if !jsonOutput {
		t.Fatal("expected jsonOutput=true for trailing --json")
	}
	if line != "session foo" {
		t.Fatalf("line = %q, want %q", line, "session foo")
	}
}

func TestEncodeJSONErrorUsageVsErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{
			name: "usage",
			err:  errors.New("usage: seshagy --get-agents [--json]"),
			code: "usage",
		},
		{
			name: "error",
			err:  errors.New("config already exists"),
			code: "error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := captureStdout(t, func() error {
				return encodeJSONError(tc.err)
			})
			if err != nil {
				t.Fatalf("encodeJSONError() error = %v", err)
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
				t.Fatalf("payload = %#v", payload)
			}
			if payload.Error != tc.err.Error() {
				t.Fatalf("error = %q, want %q", payload.Error, tc.err.Error())
			}
			if payload.Code != tc.code {
				t.Fatalf("code = %q, want %q", payload.Code, tc.code)
			}
		})
	}
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
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Version       string `json:"version"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
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
		SchemaVersion int  `json:"schema_version"`
		Ok            bool `json:"ok"`
		Integrations  []struct {
			Target string `json:"target"`
			Label  string `json:"label"`
			State  string `json:"state"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
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
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Catalog       string `json:"catalog"`
		Status        struct {
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
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
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
