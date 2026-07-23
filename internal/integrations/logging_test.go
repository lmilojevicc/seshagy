package integrations

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/logging"
)

func startIntegrationLog(t *testing.T) (*logging.Runtime, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "integration.jsonl")
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

func integrationEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestUnknownIntegrationLoggingOmitsUserInput(t *testing.T) {
	const secret = "SecretIntegrationNameABC123"
	path := filepath.Join(t.TempDir(), "integration.jsonl")
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
	_, _ = Install(secret)
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("unknown integration leaked: %s", data)
	}
	if !strings.Contains(string(data), `"event":"integration.install"`) {
		t.Fatalf("event missing: %s", data)
	}
}

func TestIntegrationLoggingSuccessAndFailure(t *testing.T) {
	for _, action := range []string{"install", "uninstall"} {
		t.Run(action+" success", func(t *testing.T) {
			runtime, path := startIntegrationLog(t)
			t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
			var err error
			if action == "install" {
				_, err = Install("pi")
			} else {
				_, err = Uninstall("pi")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Shutdown(); err != nil {
				t.Fatal(err)
			}
			records := integrationEvents(t, path)
			wantEvent := "integration." + action
			if len(records) != 1 || records[0]["event"] != wantEvent ||
				records[0]["level"] != "INFO" || records[0]["result"] != "success" ||
				records[0]["integration"] != "pi" {
				t.Fatalf("records = %#v", records)
			}
		})
		t.Run(action+" failure", func(t *testing.T) {
			const secret = "SecretIntegrationPathAndErrorABC123"
			runtime, path := startIntegrationLog(t)
			original := integrations["pi"]
			broken := original
			broken.InstallPath = func() (string, error) { return "", errors.New(secret) }
			integrations["pi"] = broken
			t.Cleanup(func() { integrations["pi"] = original })
			if action == "install" {
				_, _ = Install("pi")
			} else {
				_, _ = Uninstall("pi")
			}
			if err := runtime.Shutdown(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), secret) {
				t.Fatalf("integration failure leaked: %s", data)
			}
			records := integrationEvents(t, path)
			wantEvent := "integration." + action
			if len(records) != 1 || records[0]["event"] != wantEvent ||
				records[0]["level"] != "ERROR" || records[0]["result"] != "failed" ||
				records[0]["error_class"] != "unknown" {
				t.Fatalf("records = %#v", records)
			}
		})
	}
}
