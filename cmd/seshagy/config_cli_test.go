package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

func TestRunConfigShowTextAndJSON(t *testing.T) {
	manifestTestDirs(t)
	if err := runConfig([]string{"init"}); err != nil {
		t.Fatalf("runConfig(init) error = %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runConfig([]string{"show"})
	})
	if err != nil {
		t.Fatalf("runConfig(show) error = %v", err)
	}
	if !strings.Contains(out, "[sources]") || !strings.Contains(out, `default = "all"`) {
		t.Fatalf("show text output missing config sections:\n%s", out)
	}

	jsonOut, err := captureStdout(t, func() error {
		return runConfig([]string{"show", "--json"})
	})
	if err != nil {
		t.Fatalf("runConfig(show --json) error = %v", err)
	}
	var payload struct {
		SchemaVersion int `json:"schema_version"`
		Ok            bool
		Config        struct {
			Sources struct {
				Default string `json:"default"`
			} `json:"sources"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if payload.Config.Sources.Default != "all" {
		t.Fatalf("config.sources.default = %q, want all", payload.Config.Sources.Default)
	}
}

func TestRunConfigInitCreates(t *testing.T) {
	manifestTestDirs(t)

	out, err := captureStdout(t, func() error {
		return runConfig([]string{"init"})
	})
	if err != nil {
		t.Fatalf("runConfig(init) error = %v", err)
	}
	path := strings.TrimSpace(out)
	if path != appconfig.Path() {
		t.Fatalf("printed path = %q, want %q", path, appconfig.Path())
	}
	if !appconfig.Exists() {
		t.Fatalf("config file not created at %s", appconfig.Path())
	}
}

func TestRunConfigInitForce(t *testing.T) {
	manifestTestDirs(t)
	if err := runConfig([]string{"init"}); err != nil {
		t.Fatalf("runConfig(init) error = %v", err)
	}
	if err := os.WriteFile(appconfig.Path(), []byte("custom = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	jsonOut, err := captureStdout(t, func() error {
		return runConfig([]string{"init", "--force", "--json"})
	})
	if err != nil {
		t.Fatalf("runConfig(init --force --json) error = %v", err)
	}
	var payload struct {
		Ok      bool   `json:"ok"`
		Path    string `json:"path"`
		Created bool   `json:"created"`
		Forced  bool   `json:"forced"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if !payload.Ok || payload.Path != appconfig.Path() || payload.Created || !payload.Forced {
		t.Fatalf("payload = %#v", payload)
	}

	data, err := os.ReadFile(appconfig.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "custom = true") {
		t.Fatalf("forced init did not replace config:\n%s", data)
	}
	if !strings.Contains(string(data), "[sources]") {
		t.Fatalf("forced init did not write default config:\n%s", data)
	}
}

func TestRunConfigBareJSON(t *testing.T) {
	manifestTestDirs(t)
	out, err := captureStdout(t, func() error {
		return runConfig([]string{"--json"})
	})
	if err != nil {
		t.Fatalf("runConfig(--json) error = %v", err)
	}
	var payload struct {
		Ok   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Path != appconfig.Path() {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRunConfigInitAlreadyExistsError(t *testing.T) {
	manifestTestDirs(t)
	if err := runConfig([]string{"init"}); err != nil {
		t.Fatalf("runConfig(init) error = %v", err)
	}
	if err := runConfig([]string{"init"}); err == nil {
		t.Fatal("runConfig(init) expected error when config already exists")
	} else if !strings.Contains(err.Error(), "config already exists") {
		t.Fatalf("runConfig(init) error = %v, want config already exists", err)
	}
}
