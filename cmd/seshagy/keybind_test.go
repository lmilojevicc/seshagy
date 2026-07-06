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

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, tmuxBindMarkerBegin) || !strings.Contains(got, tmuxBindMarkerEnd) {
		t.Fatalf("missing markers\n%s", got)
	}
	if !strings.Contains(got, "bind-key s display-popup") ||
		!strings.Contains(got, "seshagy-focus-kill seshagy") {
		t.Fatalf("missing bind line\n%s", got)
	}
}

func TestInstallTmuxKeybindAppendsToExistingContent(t *testing.T) {
	path := writeTempConfig(t, "set -g mouse on\n")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
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

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatal(err)
	}
	if err := installTmuxKeybind("f", tmuxModePopup); err != nil {
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

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
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

// TestInstallTmuxKeybindPrefersExistingXDGConfig verifies that when
// ~/.config/tmux/tmux.conf already exists and TMUX_CONF_PATH is unset, the
// binding lands in the XDG location (where tmux 3.2+ actually reads).
func TestInstallTmuxKeybindPrefersExistingXDGConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TMUX_CONF_PATH", "")
	t.Setenv("TMUX_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	// Create the XDG config so it's picked up over ~/.tmux.conf.
	xdgDir := filepath.Join(home, ".config", "tmux")
	if err := os.MkdirAll(xdgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	xdgConf := filepath.Join(xdgDir, "tmux.conf")
	if err := os.WriteFile(xdgConf, []byte("set -g mouse on\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatalf("install: %v", err)
	}
	// Binding must be in the XDG file.
	xdgGot := readConfig(t, xdgConf)
	if !strings.Contains(xdgGot, tmuxBindMarkerBegin) {
		t.Fatalf("XDG config missing marker\n%s", xdgGot)
	}
	// ~/.tmux.conf must NOT have been created.
	if _, err := os.Stat(filepath.Join(home, ".tmux.conf")); err == nil {
		t.Fatal("~/.tmux.conf was created; should have preferred the XDG path")
	}
}

// TestInstallTmuxKeybindFallsBackToLegacyPath verifies that when no env var is
// set and no XDG config exists, the legacy ~/.tmux.conf is created.
func TestInstallTmuxKeybindFallsBackToLegacyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TMUX_CONF_PATH", "")
	t.Setenv("TMUX_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatalf("install: %v", err)
	}
	legacy := filepath.Join(home, ".tmux.conf")
	got := readConfig(t, legacy)
	if !strings.Contains(got, tmuxBindMarkerBegin) {
		t.Fatalf("legacy config missing marker\n%s", got)
	}
}

// TestInstallTmuxKeybindHonorsTMUX_CONFIG_DIR verifies that when TMUX_CONFIG_DIR
// is set and its tmux.conf exists, that path wins (tmux 3.3+ reads this env).
func TestInstallTmuxKeybindHonorsTMUX_CONFIG_DIR(t *testing.T) {
	confDir := t.TempDir()
	t.Setenv("TMUX_CONFIG_DIR", confDir)
	t.Setenv("TMUX_CONF_PATH", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	conf := filepath.Join(confDir, "tmux.conf")
	if err := os.WriteFile(conf, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, conf)
	if !strings.Contains(got, tmuxBindMarkerBegin) {
		t.Fatalf("TMUX_CONFIG_DIR tmux.conf missing marker\n%s", got)
	}
}

// TestInstallTmuxKeybindHonorsXDGConfig_HOME verifies that when XDG_CONFIG_HOME
// is set (and TMUX_CONFIG_DIR is not) and the tmux config exists there, that
// path wins.
func TestInstallTmuxKeybindHonorsXDGConfig_HOME(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	t.Setenv("TMUX_CONFIG_DIR", "")
	t.Setenv("TMUX_CONF_PATH", "")
	tmuxDir := filepath.Join(xdgRoot, "tmux")
	if err := os.MkdirAll(tmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	xdgConf := filepath.Join(tmuxDir, "tmux.conf")
	if err := os.WriteFile(xdgConf, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := installTmuxKeybind("s", tmuxModePopup); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, xdgConf)
	if !strings.Contains(got, tmuxBindMarkerBegin) {
		t.Fatalf("XDG_CONFIG_HOME tmux.conf missing marker\n%s", got)
	}
}

func TestTmuxBindLineModes(t *testing.T) {
	cases := []struct {
		mode     tmuxLaunchMode
		contains string
	}{
		{tmuxModePopup, "display-popup -E -w 80% -h 80%"},
		{tmuxModeWindow, "new-window -c"},
		{tmuxModePane, "split-window -c"},
		{tmuxModeZoomed, "split-window -Z -c"},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			line := tmuxBindLine("s", tc.mode)
			if !strings.Contains(line, tc.contains) {
				t.Fatalf("mode %s: line %q missing %q", tc.mode, line, tc.contains)
			}
			// Every mode must invoke seshagy-focus-kill (the dismissal wrapper).
			if !strings.Contains(line, "seshagy-focus-kill seshagy") {
				t.Fatalf("mode %s: missing focus-kill wrapper in %q", tc.mode, line)
			}
			// No mode should use run-shell (it has no controlling TTY — that
			// was the original bug).
			if strings.Contains(line, "run-shell") {
				t.Fatalf("mode %s: must not use run-shell (no TTY): %q", tc.mode, line)
			}
		})
	}
}

func TestParseTmuxLaunchModeRejectsUnknown(t *testing.T) {
	if _, err := parseTmuxLaunchMode("frobnicate"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
	// Default empty string maps to popup.
	m, err := parseTmuxLaunchMode("")
	if err != nil || m != tmuxModePopup {
		t.Fatalf("empty mode = %v err=%v, want popup", m, err)
	}
}

func TestInstallTmuxKeybindWritesSelectedMode(t *testing.T) {
	path := writeTempConfig(t, "")
	t.Setenv("TMUX_CONF_PATH", path)

	if err := installTmuxKeybind("s", tmuxModeWindow); err != nil {
		t.Fatalf("install window: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, "new-window") || strings.Contains(got, "display-popup") {
		t.Fatalf("window mode not written\n%s", got)
	}
}
