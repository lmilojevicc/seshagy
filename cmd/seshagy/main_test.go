package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	stateDir := filepath.Join(dir, "state")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", stateDir)
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
	cases := [][]string{
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

	for _, args := range [][]string{
		{"manifest", "update"},
		{"manifest", "update", "--json"},
	} {
		if err := run(args); err != nil {
			t.Fatalf("run(%v) unexpected error: %v", args, err)
		}
	}
}
