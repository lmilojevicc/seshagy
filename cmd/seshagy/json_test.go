package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(copyDone)
	}()

	t.Cleanup(func() {
		os.Stdout = old
		_ = w.Close()
		select {
		case <-copyDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("captureStdout: timed out waiting for stdout copy during cleanup")
		}
		_ = r.Close()
	})

	var fnErr error
	func() {
		defer w.Close()
		fnErr = fn()
	}()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("captureStdout: timed out waiting for stdout copy")
	}
	return buf.String(), fnErr
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	var buf bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(copyDone)
	}()

	t.Cleanup(func() {
		os.Stderr = old
		_ = w.Close()
		select {
		case <-copyDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("captureStderr: timed out waiting for stderr copy during cleanup")
		}
		_ = r.Close()
	})

	var fnErr error
	func() {
		defer w.Close()
		fnErr = fn()
	}()

	select {
	case <-copyDone:
	case <-time.After(5 * time.Second):
		t.Fatal("captureStderr: timed out waiting for stderr copy")
	}
	return buf.String(), fnErr
}

func TestCaptureStdoutLargeOutput(t *testing.T) {
	const lines = 512
	line := strings.Repeat("x", 1024) + "\n"

	out, err := captureStdout(t, func() error {
		for range lines {
			if _, writeErr := os.Stdout.Write([]byte(line)); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fn error = %v", err)
	}
	wantMin := lines * len(line)
	if len(out) < wantMin {
		t.Fatalf("output = %d bytes, want at least %d", len(out), wantMin)
	}
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

func TestEncodeJSONRejectsUnsupportedValues(t *testing.T) {
	if err := encodeJSON(make(chan int)); err == nil {
		t.Fatal("expected marshal error for unsupported value")
	}
}

func TestEncodeSuccessRejectsUnsupportedPayload(t *testing.T) {
	err := encodeSuccess(struct {
		Ch chan int `json:"ch"`
	}{Ch: make(chan int)})
	if err == nil {
		t.Fatal("expected encodeSuccess error for unsupported payload")
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
	cliTestEnv(t)
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
	cliTestEnv(t)
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
	cliTestEnv(t)
	if err := run([]string{"--get-agents", "--json", "extra"}); err == nil {
		t.Fatal("expected usage error for extra args")
	}
}
