package main

import (
	"encoding/json"
	"fmt"
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
	if !strings.Contains(out, "[directories]") || !strings.Contains(out, "fd_command") {
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
			Directories struct {
				FDCommand string `json:"fd_command"`
			} `json:"directories"`
			TUI struct {
				Layout string `json:"layout"`
			} `json:"tui"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, jsonOut)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if payload.Config.Directories.FDCommand != appconfig.Default().Directories.FDCommand {
		t.Fatalf(
			"config.directories.fd_command = %q, want %q",
			payload.Config.Directories.FDCommand,
			appconfig.Default().Directories.FDCommand,
		)
	}
	if payload.Config.TUI.Layout != appconfig.LayoutHorizontal {
		t.Fatalf(
			"config.tui.layout = %q, want %q",
			payload.Config.TUI.Layout,
			appconfig.LayoutHorizontal,
		)
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
	if !strings.Contains(string(data), "[directories]") {
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

// keyRow reads one row of `config keys` by its id column.
func keyRow(t *testing.T, out, id string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == id {
			return line
		}
	}
	t.Fatalf("config keys has no %q row:\n%s", id, out)
	return ""
}

// TestRunConfigKeysListsTheEffectiveKeyMap pins the default table `config keys`
// prints: one row per remappable action with the keys it answers, and nothing on
// stderr for a config with no overrides.
func TestRunConfigKeysListsTheEffectiveKeyMap(t *testing.T) {
	manifestTestDirs(t)
	out, err := captureStdout(t, func() error { return runConfig([]string{"keys"}) })
	if err != nil {
		t.Fatalf("runConfig(keys) error = %v", err)
	}
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows != 23 {
		t.Fatalf("config keys printed %d rows, want the 23 remappable actions:\n%s", rows, out)
	}
	for _, want := range []struct{ id, keys string }{
		{"agents", "ctrl+a"},
		{"remotes", "ctrl+e"},
		{"up", "up ctrl+p ctrl+k"},
		{"down", "down ctrl+n ctrl+j"},
		{"kill", "ctrl+x delete"},
		{"help", "f1"},
	} {
		if row := keyRow(t, out, want.id); !strings.Contains(row, want.keys) {
			t.Errorf("config keys row %q = %q, want keys %q", want.id, row, want.keys)
		}
	}
	if strings.Contains(out, "(unbound)") {
		t.Fatalf("the default table has an unbound action:\n%s", out)
	}

	stderr, err := captureStderr(t, func() error { return runConfig([]string{"keys"}) })
	if err != nil {
		t.Fatalf("runConfig(keys) error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("runConfig(keys) warned about a clean config: %q", stderr)
	}
}

// TestRunConfigKeysAppliesOverridesAndReportsIssues pins the rest of
// `config keys`: the rows follow the [tui.keys] overrides, an empty list reads
// as unbound, and every issue the picker would toast reaches stderr.
func TestRunConfigKeysAppliesOverridesAndReportsIssues(t *testing.T) {
	manifestTestDirs(t)
	cfg := appconfig.Default()
	cfg.TUI.Keys = map[string]appconfig.KeyList{
		"agents": {"f2"},
		"yazi":   {},
		"nope":   {"f3"},
	}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var out string
	stderr, err := captureStderr(t, func() error {
		var runErr error
		out, runErr = captureStdout(t, func() error { return runConfig([]string{"keys"}) })
		return runErr
	})
	if err != nil {
		t.Fatalf("runConfig(keys) error = %v", err)
	}
	if row := keyRow(t, out, "agents"); !strings.Contains(row, "f2") ||
		strings.Contains(row, "ctrl+a") {
		t.Errorf("agents row = %q, want the override f2 alone", row)
	}
	if row := keyRow(t, out, "yazi"); !strings.Contains(row, "(unbound)") {
		t.Errorf("yazi row = %q, want it reported unbound", row)
	}
	if !strings.Contains(stderr, "tui.keys.nope: unknown action; ignored") {
		t.Errorf("stderr = %q, want the unknown-action issue", stderr)
	}
}

// TestRunConfigKeysJSON pins the machine-readable form: the same rows, and the
// issues in the payload rather than on stderr.
func TestRunConfigKeysJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg := appconfig.Default()
	cfg.TUI.Keys = map[string]appconfig.KeyList{"agents": {"F2"}, "nope": {"f3"}}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var out string
	stderr, err := captureStderr(t, func() error {
		var runErr error
		out, runErr = captureStdout(t, func() error {
			return runConfig([]string{"keys", "--json"})
		})
		return runErr
	})
	if err != nil {
		t.Fatalf("runConfig(keys --json) error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("config keys --json warned on stderr: %q", stderr)
	}
	var payload struct {
		SchemaVersion int  `json:"schema_version"`
		Ok            bool `json:"ok"`
		Keys          []struct {
			ID    string   `json:"id"`
			Keys  []string `json:"keys"`
			Label string   `json:"label"`
		} `json:"keys"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if len(payload.Keys) != 23 {
		t.Fatalf("keys payload has %d rows, want 23", len(payload.Keys))
	}
	found := false
	for _, mapping := range payload.Keys {
		if mapping.ID != "agents" {
			continue
		}
		found = true
		if fmt.Sprint(mapping.Keys) != "[f2]" || mapping.Label != "agents" {
			t.Errorf("agents mapping = %#v, want normalized f2", mapping)
		}
	}
	if !found {
		t.Fatal("keys payload has no agents mapping")
	}
	if len(payload.Issues) != 1 ||
		!strings.Contains(payload.Issues[0], "tui.keys.nope: unknown action") {
		t.Errorf("issues = %q, want the unknown-action issue", payload.Issues)
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

// TestRunConfigInitWritesRemoteSection pins the generated config: the new
// [remote] section (and no [[hosts]] scaffold) is part of `config init`.
func TestRunConfigInitWritesRemoteSection(t *testing.T) {
	manifestTestDirs(t)
	if err := runConfig([]string{"init"}); err != nil {
		t.Fatalf("runConfig(init) error = %v", err)
	}
	data, err := os.ReadFile(appconfig.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "[remote]") ||
		!strings.Contains(text, "ssh_config_hosts = true") {
		t.Fatalf("generated config missing the [remote] section:\n%s", text)
	}
	if strings.Contains(text, "[[hosts]]") {
		t.Fatalf("generated config scaffolded hosts:\n%s", text)
	}
}
