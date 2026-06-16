package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
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

func remoteManifestFixture(version, contains string) string {
	return fmt.Sprintf(`
id = "codex"
version = %q
min_engine_version = 1
updated_at = "2026-06-10T12:00:00Z"

[[rules]]
id = "idle"
state = "idle"
contains = [%q]
`, version, contains)
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

func TestParseReportArgsSessionIDAndSeq(t *testing.T) {
	report, err := parseReportArgs(
		[]string{
			"--pane",
			"%1",
			"--agent",
			"opencode",
			"--state",
			"working",
			"--source",
			"seshagy:opencode",
			"--session-id",
			"session-123",
			"--seq",
			"42",
		},
	)
	if err != nil {
		t.Fatalf("parseReportArgs() error = %v", err)
	}
	if report.Pane != "%1" || report.Name != "opencode" || report.Source != "seshagy:opencode" ||
		!report.SourceSeen {
		t.Fatalf("report parsed basic fields incorrectly: %#v", report)
	}
	if report.SessionID != "session-123" || !report.SessionIDSeen {
		t.Fatalf("report session id = %q seen=%v", report.SessionID, report.SessionIDSeen)
	}
	if report.Seq != 42 || !report.SeqSeen {
		t.Fatalf("report seq = %d seen=%v", report.Seq, report.SeqSeen)
	}
}

func TestParseReportArgsRejectsInvalidSeq(t *testing.T) {
	for _, seq := range []string{"not-an-int", "-1"} {
		if _, err := parseReportArgs([]string{"--seq", seq}); err == nil {
			t.Fatalf("parseReportArgs should reject seq %q", seq)
		}
	}
}

func TestParseReleaseArgsSeq(t *testing.T) {
	release, err := parseReleaseArgs([]string{"--pane=%2", "--source=seshagy:pi", "--seq=99"})
	if err != nil {
		t.Fatalf("parseReleaseArgs() error = %v", err)
	}
	if release.Pane != "%2" || release.Source != "seshagy:pi" || !release.SourceSeen {
		t.Fatalf("release parsed basic fields incorrectly: %#v", release)
	}
	if release.Seq != 99 || !release.SeqSeen {
		t.Fatalf("release seq = %d seen=%v", release.Seq, release.SeqSeen)
	}
}

func TestParseReleaseArgsRejectsInvalidSeq(t *testing.T) {
	for _, seq := range []string{"bad", "-1"} {
		if _, err := parseReleaseArgs([]string{"--seq", seq}); err == nil {
			t.Fatalf("parseReleaseArgs should reject seq %q", seq)
		}
	}
}

func TestRunManifestRoutingErrors(t *testing.T) {
	manifestTestDirs(t)
	cases := [][]string{
		{"manifest"},
		{"manifest", "bogus"},
		{"manifest", "status", "extra"},
		{"manifest", "update", "extra"},
		{"manifest", "reload", "extra"},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) expected error, got nil", args)
		}
	}
}

func TestRunManifestStatusAndReloadNoError(t *testing.T) {
	manifestTestDirs(t)
	cases := [][]string{
		{"manifest", "status"},
		{"manifest", "reload"},
		{"manifest", "status", "--json"},
		{"manifest", "reload", "--json"},
	}
	for _, args := range cases {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) unexpected error: %v", args, err)
		}
	}
}

func TestRunManifestUpdateFromHTTPServer(t *testing.T) {
	manifestTestDirs(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.toml":
			_, _ = w.Write([]byte(`
schema_version = 1

[[agents]]
id = "codex"
path = "codex.toml"
`))
		case "/codex.toml":
			_, _ = w.Write([]byte(remoteManifestFixture("2026.06.10.3", "cli-update-ready")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("SESHAGY_AGENT_DETECTION_MANIFEST_CATALOG_URL", server.URL+"/index.toml")
	t.Setenv("SESHAGY_MANIFEST_ALLOW_HTTP_CATALOG", "1")

	if err := run([]string{"manifest", "update"}); err != nil {
		t.Fatalf("run(manifest update) unexpected error: %v", err)
	}
	if err := run([]string{"manifest", "update", "--json"}); err != nil {
		t.Fatalf("run(manifest update --json) unexpected error: %v", err)
	}

	manifestPath := sessionmgr.RemoteManifestPath("codex")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", manifestPath, err)
	}
	content := string(data)
	if !strings.Contains(content, `version = "2026.06.10.3"`) {
		t.Fatalf("manifest missing version on disk:\n%s", content)
	}
	if !strings.Contains(content, `contains = ["cli-update-ready"]`) {
		t.Fatalf("manifest missing rule on disk:\n%s", content)
	}
	status := sessionmgr.LoadManifestUpdateStatus()
	agentStatus, ok := status.AgentStatus("codex")
	if !ok || agentStatus.LastResult != "current" {
		t.Fatalf("codex status after idempotent update = %#v, ok=%v", agentStatus, ok)
	}
	if agentStatus.CachedVersion == nil || *agentStatus.CachedVersion != "2026.06.10.3" {
		t.Fatalf("codex cached version = %v, want 2026.06.10.3", agentStatus.CachedVersion)
	}
}

func TestRunReportAndReleaseAgentJSON(t *testing.T) {
	manifestTestDirs(t)
	const pane = "%10"
	f := sessionmgr.InstallAgentCLIFakeTmux(t, pane, nil)

	reportOut, err := captureStdout(t, func() error {
		return run([]string{
			"--report-agent",
			"--pane", pane,
			"--agent", "claude",
			"--state", "working",
			"--json",
		})
	})
	if err != nil {
		t.Fatalf("run(--report-agent) error = %v", err)
	}
	var reportPayload struct {
		Ok        bool   `json:"ok"`
		Applied   bool   `json:"applied"`
		Pane      string `json:"pane"`
		AgentName string `json:"agent_name"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(reportOut)), &reportPayload); err != nil {
		t.Fatalf("report json.Unmarshal() error = %v, out=%q", err, reportOut)
	}
	if !reportPayload.Ok || !reportPayload.Applied || reportPayload.Pane != pane ||
		reportPayload.AgentName != "claude" || reportPayload.State != "working" {
		t.Fatalf("report payload = %#v", reportPayload)
	}
	if got := f.Get(pane, "@agent_name"); got != "claude" {
		t.Fatalf("@agent_name after report = %q, want claude", got)
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after report = %q, want working", got)
	}

	releaseOut, err := captureStdout(t, func() error {
		return run([]string{"--release-agent", "--pane", pane, "--json"})
	})
	if err != nil {
		t.Fatalf("run(--release-agent) error = %v", err)
	}
	var releasePayload struct {
		Ok       bool   `json:"ok"`
		Released bool   `json:"released"`
		Pane     string `json:"pane"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(releaseOut)), &releasePayload); err != nil {
		t.Fatalf("release json.Unmarshal() error = %v, out=%q", err, releaseOut)
	}
	if !releasePayload.Ok || !releasePayload.Released || releasePayload.Pane != pane {
		t.Fatalf("release payload = %#v", releasePayload)
	}
	for _, opt := range []string{"@agent_name", "@agent_state", "@agent_message", "@agent_source"} {
		if got := f.Get(pane, opt); got != "" {
			t.Fatalf("release left %s = %q, want cleared", opt, got)
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

func TestRunIntegrationAliases(t *testing.T) {
	cliTestEnv(t)
	for _, args := range [][]string{
		{"integrations", "status"},
		{"hook", "status"},
		{"hooks", "status"},
	} {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) error = %v", args, err)
		}
	}
}

func TestPrintManifestStatusTextAfterUpdate(t *testing.T) {
	manifestTestDirs(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.toml":
			_, _ = w.Write([]byte(`
schema_version = 1

[[agents]]
id = "codex"
path = "codex.toml"
`))
		case "/codex.toml":
			_, _ = w.Write([]byte(remoteManifestFixture("2026.06.10.3", "status-check")))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("SESHAGY_AGENT_DETECTION_MANIFEST_CATALOG_URL", server.URL+"/index.toml")
	t.Setenv("SESHAGY_MANIFEST_ALLOW_HTTP_CATALOG", "1")

	if err := run([]string{"manifest", "update"}); err != nil {
		t.Fatalf("run(manifest update) error = %v", err)
	}

	out, err := captureStdout(t, func() error {
		return printManifestStatus(server.URL+"/index.toml", false)
	})
	if err != nil {
		t.Fatalf("printManifestStatus() error = %v", err)
	}
	if !strings.Contains(out, "last check:") || !strings.Contains(out, "last result:") {
		t.Fatalf("manifest status text missing update fields:\n%s", out)
	}
}

func TestPrintManifestUpdateResultWithCommits(t *testing.T) {
	version, err := sessionmgr.ParseManifestVersion("2026.06.10.3")
	if err != nil {
		t.Fatalf("ParseManifestVersion() error = %v", err)
	}
	output := sessionmgr.ManifestUpdateOutput{
		Updated: []sessionmgr.ManifestUpdateCommit{
			{AgentID: "codex", Version: version},
		},
	}
	output.Status.LastResult = strPtr("updated")
	out, err := captureStdout(t, func() error {
		printManifestUpdateResult(output)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "update result: updated") ||
		!strings.Contains(out, "updated codex to 2026.06.10.3") {
		t.Fatalf("printManifestUpdateResult output = %q", out)
	}
}

func TestParseReportArgsMessageStatusAndUnknownFlag(t *testing.T) {
	report, err := parseReportArgs([]string{
		"--pane", "%3",
		"--status", "idle",
		"--message", "needs input",
	})
	if err != nil {
		t.Fatalf("parseReportArgs() error = %v", err)
	}
	if report.State != sessionmgr.AgentIdle || !report.MessageSeen ||
		report.Message != "needs input" {
		t.Fatalf("report = %#v", report)
	}
	if _, err := parseReportArgs([]string{"--bogus"}); err == nil {
		t.Fatal("parseReportArgs() expected unknown flag error")
	}
}

func TestForEachFlagRequiresValue(t *testing.T) {
	err := forEachFlag(
		[]string{"--pane"},
		func(arg, key string, nextValue func() (string, error)) error {
			_, err := nextValue()
			return err
		},
	)
	if err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("forEachFlag() error = %v", err)
	}
}

func TestUnknownCommandErrorIncludesHint(t *testing.T) {
	err := unknownCommandError([]string{"frobnicate", "--json"})
	if err == nil || !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("unknownCommandError() = %v", err)
	}
}

func TestRunManifestReloadJSON(t *testing.T) {
	manifestTestDirs(t)
	out, err := captureStdout(t, func() error {
		return runManifest([]string{"reload", "--json"})
	})
	if err != nil {
		t.Fatalf("runManifest(reload --json) error = %v", err)
	}
	var payload struct {
		Ok       bool `json:"ok"`
		Reloaded int  `json:"reloaded"`
		Agents   []any
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Reloaded == 0 || len(payload.Agents) == 0 {
		t.Fatalf("reload payload = %#v", payload)
	}
}

func TestPrintManifestUpdateResultNoUpdates(t *testing.T) {
	output := sessionmgr.ManifestUpdateOutput{}
	out, err := captureStdout(t, func() error {
		printManifestUpdateResult(output)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no manifest updates") {
		t.Fatalf("printManifestUpdateResult output = %q", out)
	}
}

func strPtr(s string) *string { return &s }
