package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".tmux.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readConfig(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallTmuxKeybindFreshConfig(t *testing.T) {
	path := writeTempConfig(t, "")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s"); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, tmuxBindMarkerBegin) || !strings.Contains(got, tmuxBindMarkerEnd) {
		t.Fatalf("missing markers\n%s", got)
	}
	if !strings.Contains(got, "bind-key s run-shell 'seshagy-focus-kill seshagy'") {
		t.Fatalf("missing bind line\n%s", got)
	}
}

func TestInstallTmuxKeybindAppendsToExistingContent(t *testing.T) {
	path := writeTempConfig(t, "set -g mouse on\n")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s"); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.HasPrefix(got, "set -g mouse on\n") {
		t.Fatalf("existing content clobbered\n%s", got)
	}
	if !strings.Contains(got, tmuxBindMarkerBegin) {
		t.Fatalf("missing marker\n%s", got)
	}
}

func TestInstallTmuxKeybindIsIdempotentAndReplacesKey(t *testing.T) {
	path := writeTempConfig(t, "")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s"); err != nil {
		t.Fatal(err)
	}
	if err := installTmuxKeybind("f"); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	// exactly one marker block
	if c := strings.Count(got, tmuxBindMarkerBegin); c != 1 {
		t.Fatalf("marker blocks = %d, want 1\n%s", c, got)
	}
	if strings.Contains(got, "bind-key s ") {
		t.Fatalf("old key 's' not replaced\n%s", got)
	}
	if !strings.Contains(got, "bind-key f ") {
		t.Fatalf("new key 'f' missing\n%s", got)
	}
}

func TestUninstallTmuxKeybindRemovesBlock(t *testing.T) {
	path := writeTempConfig(t, "set -g mouse on\n")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s"); err != nil {
		t.Fatal(err)
	}
	if err := uninstallTmuxKeybind(); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if strings.Contains(got, tmuxBindMarkerBegin) {
		t.Fatalf("marker still present after uninstall\n%s", got)
	}
	if !strings.Contains(got, "set -g mouse on") {
		t.Fatalf("uninstall clobbered unrelated content\n%s", got)
	}
}

func TestUninstallTmuxKeybindWhenAbsentIsNoop(t *testing.T) {
	path := writeTempConfig(t, "set -g mouse on\n")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := uninstallTmuxKeybind(); err != nil {
		t.Fatalf("noop uninstall should not error: %v", err)
	}
	got := readConfig(t, path)
	if got != "set -g mouse on\n" {
		t.Fatalf("content changed on noop uninstall\n%s", got)
	}
}
