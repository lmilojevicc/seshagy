package sessionmgr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/logging"
)

func TestLoadLoggingUsesSafeStructuredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "load.jsonl")
	resolved, err := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if _, err := LoadWithBackend(
		context.Background(),
		NewNoopBackend(),
		ModeSessions,
		LoadOptions{},
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record); err != nil {
		t.Fatal(err)
	}
	if record["event"] != "source.load" || record["backend"] != "none" ||
		record["source"] != "sessions" {
		t.Fatalf("record = %#v", record)
	}
}

func TestAgentReportLoggingOmitsMessageAndAgentSession(t *testing.T) {
	const secret = "SecretAgentPayloadABC123"
	path := filepath.Join(t.TempDir(), "agent.jsonl")
	resolved, _ := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	fake := NewFakeTmux()
	SetTmuxHooksForTest(t, fake.output, fake.run)
	applied, err := ReportAgent(context.Background(), AgentReport{
		Pane: "%7", Name: "pi", State: AgentWorking, Source: "test", Seq: 7,
		Message: secret, SessionID: secret,
	})
	if err != nil || !applied {
		t.Fatalf("ReportAgent() = %v, %v", applied, err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("agent payload leaked: %s", data)
	}
	if !strings.Contains(string(data), `"event":"agent.report"`) ||
		!strings.Contains(string(data), `"pane_id":"%7"`) {
		t.Fatalf("safe agent fields missing: %s", data)
	}
}

func TestAgentReportFailureLoggingUsesExactSafeContract(t *testing.T) {
	const (
		pane      = "%42-private-pane"
		message   = "SecretAgentMessageABC123"
		sessionID = "SecretAgentSessionABC123"
		rawError  = "tmux report failed: SecretRawErrorABC123"
	)
	runtime, path := startStructuredLog(t)
	fake := NewFakeTmux()
	reportErr := errors.New(rawError)
	SetTmuxHooksForTest(t, fake.output, func(context.Context, ...string) error {
		return reportErr
	})

	applied, err := NewTmuxBackend().ReportAgent(context.Background(), AgentReport{
		Pane: pane, Name: "pi", State: AgentWorking, Source: "test", Seq: 42,
		Message: message, SessionID: sessionID,
	})
	if applied || !errors.Is(err, reportErr) {
		t.Fatalf("ReportAgent() = %v, %v, want false and forced error", applied, err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{pane, message, sessionID, rawError, "Secret"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("agent report failure leaked %q: %s", forbidden, data)
		}
	}
	records := readStructuredEvents(t, path)
	var failures []map[string]any
	for _, record := range records {
		if record["event"] == "agent.report_failed" {
			failures = append(failures, record)
		}
	}
	if len(failures) != 1 {
		t.Fatalf("agent.report_failed count = %d, records = %#v", len(failures), records)
	}
	record := failures[0]
	if record["level"] != "ERROR" || record["component"] != "agents" ||
		record["backend"] != "tmux" || record["result"] != "failed" ||
		record["error_class"] != "unknown" {
		t.Fatalf("agent.report_failed record = %#v", record)
	}
	allowed := map[string]bool{
		"time": true, "level": true, "msg": true, "schema_version": true,
		"run_id": true, "app_version": true, "event": true, "component": true,
		"backend": true, "result": true, "error_class": true,
	}
	for key := range record {
		if !allowed[key] {
			t.Errorf("agent.report_failed has unexpected field %q: %#v", key, record)
		}
	}
	if len(record) != len(allowed) {
		t.Errorf("agent.report_failed fields = %#v, want exactly %#v", record, allowed)
	}
}

func startStructuredLog(t *testing.T) (*logging.Runtime, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	resolved, err := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	return runtime, path
}

func readStructuredEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid JSONL %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func findStructuredEvent(t *testing.T, records []map[string]any, event string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["event"] == event {
			return record
		}
	}
	t.Fatalf("event %q not found in %#v", event, records)
	return nil
}

func TestLoadFailureDoesNotLogRawError(t *testing.T) {
	const secret = "SecretSubprocessPayloadABC123"
	path := filepath.Join(t.TempDir(), "load.jsonl")
	resolved, _ := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	SetTmuxHooksForTest(
		t,
		func(_ context.Context, _ ...string) ([]byte, error) { return nil, errors.New(secret) },
		nil,
	)
	_, _ = LoadWithBackend(context.Background(), NewTmuxBackend(), ModeSessions, LoadOptions{})
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("raw error leaked: %s", data)
	}
	if !strings.Contains(string(data), `"event":"source.load_failed"`) {
		t.Fatalf("missing failure event: %s", data)
	}
}

type degradingMux struct{ Multiplexer }

func (degradingMux) Kind() BackendKind { return BackendNone }
func (degradingMux) ListAgents(context.Context, string) ([]Item, error) {
	return nil, errors.New("Secret;Segment;Shape")
}

func TestLoadDegradationCountsFailedSourcesNotMessagePunctuation(t *testing.T) {
	runtime, path := startStructuredLog(t)
	t.Setenv("PATH", t.TempDir())
	_, err := LoadWithBackend(context.Background(), degradingMux{NewNoopBackend()}, ModeAll,
		LoadOptions{FDCommand: "printf ''"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Secret;Segment;Shape") {
		t.Fatalf("degraded error leaked: %s", data)
	}
	records := readStructuredEvents(t, path)
	load := findStructuredEvent(t, records, "source.load")
	degradedCount := 0
	foundAgents := false
	for _, record := range records {
		if record["event"] != "source.load_degraded" {
			continue
		}
		degradedCount++
		foundAgents = foundAgents || record["failed_source"] == "agents"
	}
	if load["warning_count"] != float64(degradedCount) || load["level"] != "DEBUG" ||
		!foundAgents {
		t.Fatalf("load=%#v degraded=%#v", load, records)
	}
}

func TestSessionKillLoggingSuccessAndFailure(t *testing.T) {
	for _, tt := range []struct {
		name, secret, level, result string
		fail                        bool
	}{
		{name: "success", level: "INFO", result: "success"},
		{name: "failure", secret: "SecretKillErrorABC123", level: "ERROR", result: "failed", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime, path := startStructuredLog(t)
			SetTmuxHooksForTest(t, nil, func(_ context.Context, _ ...string) error {
				if tt.fail {
					return errors.New(tt.secret)
				}
				return nil
			})
			_ = NewTmuxBackend().KillSession(context.Background(), "SecretSessionTargetABC123")
			if err := runtime.Shutdown(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "Secret") {
				t.Fatalf("kill data leaked: %s", data)
			}
			record := findStructuredEvent(t, readStructuredEvents(t, path), "session.kill")
			if record["backend"] != "tmux" || record["level"] != tt.level ||
				record["result"] != tt.result {
				t.Fatalf("kill record = %#v", record)
			}
		})
	}
}

func TestHerdrFocusAndIgnoredAgentLogging(t *testing.T) {
	runtime, path := startStructuredLog(t)
	t.Setenv("HERDR_WORKSPACE_ID", "workspace-current")
	setHerdrHooksForTest(t, nil, func(_ context.Context, _ ...string) error { return nil })
	backend := NewHerdrBackend()
	if err := backend.KillSession(context.Background(), "SecretWorkspaceTargetABC123"); err != nil {
		t.Fatal(err)
	}
	_, _ = backend.ReportAgent(context.Background(), AgentReport{Pane: "SecretPane", Seq: 2})
	_, _ = backend.ReleaseAgent(context.Background(), AgentRelease{Pane: "SecretPane", Seq: 3})
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Secret") {
		t.Fatalf("herdr user data leaked: %s", data)
	}
	records := readStructuredEvents(t, path)
	focus := findStructuredEvent(t, records, "session.focus_restore")
	if focus["level"] != "DEBUG" || focus["result"] != "success" ||
		focus["workspace_id"] != "workspace-current" {
		t.Fatalf("focus record = %#v", focus)
	}
	if findStructuredEvent(t, records, "agent.report")["result"] != "ignored_backend" ||
		findStructuredEvent(t, records, "agent.release")["result"] != "ignored_backend" {
		t.Fatalf("ignored backend records = %#v", records)
	}
}

func TestHerdrFocusFailureOmitsWorkspaceAndRawError(t *testing.T) {
	const secret = "SecretFocusFailureABC123"
	runtime, path := startStructuredLog(t)
	t.Setenv("HERDR_WORKSPACE_ID", "SecretWorkspaceIDABC123")
	setHerdrHooksForTest(t, nil, func(_ context.Context, args ...string) error {
		if len(args) > 1 && args[1] == "focus" {
			return errors.New(secret)
		}
		return nil
	})
	if err := NewHerdrBackend().KillSession(context.Background(), "target"); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Secret") {
		t.Fatalf("focus failure leaked data: %s", data)
	}
	focus := findStructuredEvent(t, readStructuredEvents(t, path), "session.focus_restore")
	if focus["level"] != "WARN" || focus["result"] != "failed" || focus["workspace_id"] != nil {
		t.Fatalf("focus failure record = %#v", focus)
	}
}

func TestAgentReleaseLoggingSuccessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			runtime, path := startStructuredLog(t)
			fake := NewFakeTmux()
			SetTmuxHooksForTest(t, fake.output, func(ctx context.Context, args ...string) error {
				if fail {
					return errors.New("SecretReleaseFailureABC123")
				}
				return fake.run(ctx, args...)
			})
			_, _ = ReleaseAgent(
				context.Background(),
				AgentRelease{Pane: "%9", Source: "hook", Seq: 9},
			)
			if err := runtime.Shutdown(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "Secret") {
				t.Fatalf("release error leaked: %s", data)
			}
			event := "agent.release"
			level := "DEBUG"
			if fail {
				event, level = "agent.release_failed", "ERROR"
			}
			record := findStructuredEvent(t, readStructuredEvents(t, path), event)
			if record["level"] != level {
				t.Fatalf("release record = %#v", record)
			}
		})
	}
}

func TestManifestLoggingIsAggregateAndContentFree(t *testing.T) {
	isolateManifestCache(t)
	const secret = "SecretCapturedPaneTextABC123"
	runtime, path := startStructuredLog(t)
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
	fake.HandleOutput(func(args []string) bool {
		return len(args) > 0 && args[0] == "capture-pane"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("waiting for approval\nrun this command?\nskip (esc or n)\n" + secret), nil
	})
	fake.Install(t)
	items := []Item{{
		Kind: KindAgent, AgentName: "cursor", PaneID: "%5", AgentState: AgentWorking,
	}}
	ApplyManifestFallback(context.Background(), items)
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("manifest content leaked: %s", data)
	}
	records := readStructuredEvents(t, path)
	change := findStructuredEvent(t, records, "manifest.state_change")
	if change["level"] != "DEBUG" || change["state"] != "blocked" ||
		change["previous_state"] != "working" {
		t.Fatalf("state change = %#v", change)
	}
	sweep := findStructuredEvent(t, records, "manifest.sweep")
	if sweep["matched_count"] != float64(1) || sweep["changed_count"] != float64(1) {
		t.Fatalf("manifest sweep = %#v", sweep)
	}
}
