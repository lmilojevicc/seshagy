package tui

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/logging"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestStaleRefreshWritesCorrelatedDebugEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.jsonl")
	resolved, _ := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	model := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
		WithLogger(runtime.Logger()),
	)
	model.refreshGen[sessionmgr.ModeSessions] = 2
	model.inflightRefresh[sessionmgr.ModeSessions] = 2
	_, _ = model.handleRefreshMsg(refreshMsg{source: sessionmgr.ModeSessions, gen: 1})
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
	if record["event"] != "tui.refresh_stale" || record["generation"] != float64(1) ||
		record["current_generation"] != float64(2) {
		t.Fatalf("record = %#v", record)
	}
}

func TestCurrentRefreshDoesNotWriteStaleEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui.jsonl")
	resolved, _ := logging.Resolve(
		logging.Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown() })
	model := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
		WithLogger(runtime.Logger()),
	)
	model.refreshGen[sessionmgr.ModeSessions] = 2
	model.inflightRefresh[sessionmgr.ModeSessions] = 2
	_, _ = model.handleRefreshMsg(refreshMsg{source: sessionmgr.ModeSessions, gen: 2})
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tui.refresh_stale") {
		t.Fatalf("current generation logged stale event: %s", data)
	}
}

func TestStartupErrorIsPreservedWithInjectedDependencies(t *testing.T) {
	startupErr := errors.New("SecretMalformedConfigABC123")
	model := New(
		WithConfig(appconfig.Default()),
		WithStartupError(startupErr),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	if len(model.notifications) != 1 || model.notifications[0].text != startupErr.Error() ||
		model.notifications[0].sev != sevError {
		t.Fatalf("startup notification = %#v", model.notifications)
	}
}
