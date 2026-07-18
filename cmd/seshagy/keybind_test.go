package main

import (
	"errors"
	"io"
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

func captureOutput(t *testing.T, stream **os.File, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := *stream
	*stream = w
	runErr := fn()
	*stream = original
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
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
		!strings.Contains(got, "seshagy --ephemeral") {
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
			// Every mode must invoke seshagy --ephemeral (built-in dismissal).
			if !strings.Contains(line, "seshagy --ephemeral") {
				t.Fatalf("mode %s: missing --ephemeral in %q", tc.mode, line)
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

// --- herdr keybind tests ---

func writeTempHerdrConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInstallHerdrKeybindFreshConfig(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)

	if err := installHerdrKeybind(
		"s",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, herdrBindMarkerBegin) || !strings.Contains(got, herdrBindMarkerEnd) {
		t.Fatalf("missing markers\n%s", got)
	}
	if !strings.Contains(got, `key = "prefix+s"`) {
		t.Fatalf("missing key line\n%s", got)
	}
	if !strings.Contains(got, `type = "pane"`) {
		t.Fatalf("missing type line\n%s", got)
	}
	if !strings.Contains(got, "seshagy --ephemeral") {
		t.Fatalf("missing command\n%s", got)
	}
}

func TestInstallHerdrKeybindAppendsToExistingKeys(t *testing.T) {
	existing := `[keys]

  [[keys.command]]
    key = "ctrl+h"
    type = "plugin_action"
    command = "herdr-splits.nav-left"
`
	path := writeTempHerdrConfig(t, existing)
	t.Setenv("HERDR_CONFIG_PATH", path)

	if err := installHerdrKeybind(
		"s",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	// Existing key entry must be untouched.
	if !strings.Contains(got, `herdr-splits.nav-left`) {
		t.Fatalf("existing key entry clobbered\n%s", got)
	}
	// Our block must be appended.
	if !strings.Contains(got, herdrBindMarkerBegin) {
		t.Fatalf("missing marker\n%s", got)
	}
	if !strings.Contains(got, `prefix+s`) {
		t.Fatalf("missing our key\n%s", got)
	}
	// Exactly one seshagy marker block.
	if c := strings.Count(got, herdrBindMarkerBegin); c != 1 {
		t.Fatalf("marker blocks = %d, want 1\n%s", c, got)
	}
}

func TestInstallHerdrKeybindIsIdempotentAndReplacesKey(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)

	if err := installHerdrKeybind(
		"s",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatal(err)
	}
	if err := installHerdrKeybind(
		"f",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if c := strings.Count(got, herdrBindMarkerBegin); c != 1 {
		t.Fatalf("marker blocks = %d, want 1\n%s", c, got)
	}
	if strings.Contains(got, `prefix+s`) {
		t.Fatalf("old key prefix+s not replaced\n%s", got)
	}
	if !strings.Contains(got, `prefix+f`) {
		t.Fatalf("new key prefix+f missing\n%s", got)
	}
}

func TestUninstallHerdrKeybindRemovesBlock(t *testing.T) {
	existing := `[keys]

  [[keys.command]]
    key = "ctrl+h"
    command = "herdr-splits.nav-left"
`
	path := writeTempHerdrConfig(t, existing)
	t.Setenv("HERDR_CONFIG_PATH", path)

	if err := installHerdrKeybind(
		"s",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatal(err)
	}
	if err := uninstallHerdrKeybind(); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if strings.Contains(got, herdrBindMarkerBegin) {
		t.Fatalf("marker still present after uninstall\n%s", got)
	}
	if !strings.Contains(got, "herdr-splits.nav-left") {
		t.Fatalf("uninstall clobbered unrelated content\n%s", got)
	}
}

func TestUninstallHerdrKeybindWhenAbsentIsNoop(t *testing.T) {
	existing := `[keys]
  [[keys.command]]
    key = "ctrl+h"
    command = "other"
`
	path := writeTempHerdrConfig(t, existing)
	t.Setenv("HERDR_CONFIG_PATH", path)

	if err := uninstallHerdrKeybind(); err != nil {
		t.Fatalf("noop uninstall should not error: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, "other") {
		t.Fatalf("content changed on noop uninstall\n%s", got)
	}
}

func TestHerdrConfigPathHonorsHERDR_CONFIG_PATH(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)

	got, err := herdrConfigPath()
	if err != nil {
		t.Fatalf("herdrConfigPath error: %v", err)
	}
	if got != path {
		t.Fatalf("herdrConfigPath = %q, want %q", got, path)
	}
}

// --- herdr launch-mode / popup tests ---

func TestParseHerdrLaunchMode(t *testing.T) {
	cases := []struct {
		in   string
		want herdrLaunchMode
		err  bool
	}{
		{"", herdrModePane, false}, // default is pane
		{"pane", herdrModePane, false},
		{"popup", herdrModePopup, false},
		{"window", "", true}, // tmux-only mode, invalid for herdr
		{"frobnicate", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseHerdrLaunchMode(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error for %q, got mode %v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("mode %q = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestHerdrBindBlockPopupEmitsPopupType(t *testing.T) {
	block := herdrBindBlock("f", herdrModePopup, defaultHerdrPopupWidth, defaultHerdrPopupHeight)
	if !strings.Contains(block, `type = "popup"`) {
		t.Fatalf("popup mode missing type=popup\n%s", block)
	}
	if !strings.Contains(block, `width = "80%"`) || !strings.Contains(block, `height = "80%"`) {
		t.Fatalf("popup mode missing width/height\n%s", block)
	}
	if !strings.Contains(block, `key = "prefix+f"`) {
		t.Fatalf("missing key line\n%s", block)
	}
	if !strings.Contains(block, "seshagy --ephemeral") {
		t.Fatalf("missing command\n%s", block)
	}
	// Must NOT also emit the pane type.
	if strings.Contains(block, `type = "pane"`) {
		t.Fatalf("popup block should not contain pane type\n%s", block)
	}
}

func TestHerdrBindBlockPaneEmitsPaneType(t *testing.T) {
	block := herdrBindBlock("s", herdrModePane, defaultHerdrPopupWidth, defaultHerdrPopupHeight)
	if !strings.Contains(block, `type = "pane"`) {
		t.Fatalf("pane mode missing type=pane\n%s", block)
	}
	// pane must not carry popup-only width/height.
	if strings.Contains(block, `width = `) || strings.Contains(block, `height = `) {
		t.Fatalf("pane block should not carry width/height\n%s", block)
	}
}

// stubHerdrVersion overrides the herdr version seam for the duration of the
// test and restores it afterward.
func stubHerdrVersion(t *testing.T, out string, err error) {
	t.Helper()
	prev := herdrVersionOutput
	herdrVersionOutput = func() ([]byte, error) { return []byte(out), err }
	t.Cleanup(func() { herdrVersionOutput = prev })
}

func TestInstallHerdrKeybindPopupWritesPopupBinding(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)
	stubHerdrVersion(t, "herdr 0.7.4\n", nil) // supports popup

	if err := installHerdrKeybind(
		"f",
		herdrModePopup,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `type = "popup"`) {
		t.Fatalf("popup type not written\n%s", got)
	}
	if !strings.Contains(got, `width = "80%"`) || !strings.Contains(got, `height = "80%"`) {
		t.Fatalf("popup dimensions not written\n%s", got)
	}
	if strings.Contains(got, `type = "pane"`) {
		t.Fatalf("should not contain pane type\n%s", got)
	}
}

func TestInstallHerdrKeybindPopupFallsBackOnOldVersion(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)
	stubHerdrVersion(t, "herdr 0.7.3\n", nil) // predates popup

	warning, err := captureOutput(t, &os.Stderr, func() error {
		return installHerdrKeybind(
			"f",
			herdrModePopup,
			defaultHerdrPopupWidth,
			defaultHerdrPopupHeight,
		)
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, want := range []string{"herdr 0.7.3", "requires >= 0.7.4", "falling back to pane"} {
		if !strings.Contains(warning, want) {
			t.Fatalf("fallback warning %q missing %q", warning, want)
		}
	}
	got := readConfig(t, path)
	// Fell back to pane (no popup type, no width/height).
	if !strings.Contains(got, `type = "pane"`) {
		t.Fatalf("expected fallback to pane type\n%s", got)
	}
	if strings.Contains(got, `type = "popup"`) {
		t.Fatalf("should not contain popup type after fallback\n%s", got)
	}
	if strings.Contains(got, `width = `) || strings.Contains(got, `height = `) {
		t.Fatalf("should not contain popup dimensions after fallback\n%s", got)
	}
}

func TestInstallHerdrKeybindPopupHonoredWhenVersionUnknown(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)
	// If herdr is missing, honor the popup request unchanged.
	stubHerdrVersion(t, "", errors.New("herdr: not found"))

	if err := installHerdrKeybind(
		"f",
		herdrModePopup,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatalf("install: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `type = "popup"`) {
		t.Fatalf("popup should be honored when version is unknown\n%s", got)
	}
}

func TestParseHerdrVersion(t *testing.T) {
	cases := []struct {
		in   string
		want semver
		ok   bool
	}{
		{"herdr 0.7.4\n", semver{major: 0, minor: 7, patch: 4}, true},
		{"herdr 0.7.4-rc1\n", semver{major: 0, minor: 7, patch: 4, prerelease: "rc1"}, true},
		{"herdr 0.7.5-rc1\n", semver{major: 0, minor: 7, patch: 5, prerelease: "rc1"}, true},
		{
			"herdr version v1.2.3-rc1\n",
			semver{major: 1, minor: 2, patch: 3, prerelease: "rc1"},
			true,
		},
		{"herdr version v1.2.3+build.7\n", semver{major: 1, minor: 2, patch: 3}, true},
		{"0.8.0", semver{major: 0, minor: 8, patch: 0}, true},
		{"no version here", semver{}, false},
		{"", semver{}, false},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.in), func(t *testing.T) {
			got, ok := parseHerdrVersion([]byte(tc.in))
			if ok != tc.ok {
				t.Fatalf("parseHerdrVersion(%q) ok=%v, want %v", tc.in, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("parseHerdrVersion(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestVersionBefore(t *testing.T) {
	if !(semver{major: 0, minor: 7, patch: 3}.before(minHerdrPopupVersion)) {
		t.Fatal("0.7.3 should be before 0.7.4")
	}
	if (semver{major: 0, minor: 7, patch: 4}.before(minHerdrPopupVersion)) {
		t.Fatal("0.7.4 should not be before 0.7.4")
	}
	if !(semver{major: 0, minor: 7, patch: 4, prerelease: "rc1"}.before(minHerdrPopupVersion)) {
		t.Fatal("0.7.4-rc1 should be before stable 0.7.4")
	}
	if (semver{major: 0, minor: 7, patch: 5, prerelease: "rc1"}.before(minHerdrPopupVersion)) {
		t.Fatal("0.7.5-rc1 should not be before 0.7.4")
	}
	if !(semver{major: 0, minor: 6, patch: 9}.before(semver{major: 0, minor: 7, patch: 0})) {
		t.Fatal("0.6.9 should be before 0.7.0")
	}
	if (semver{major: 1}.before(minHerdrPopupVersion)) {
		t.Fatal("1.0.0 should not be before 0.7.4")
	}
}

func TestResolveHerdrPopupModePrereleases(t *testing.T) {
	cases := []struct {
		version string
		want    herdrLaunchMode
	}{
		{"0.7.4-rc1", herdrModePane},
		{"0.7.5-rc1", herdrModePopup},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			stubHerdrVersion(t, "herdr "+tc.version+"\n", nil)
			got := resolveHerdrPopupMode(herdrModePopup)
			if got != tc.want {
				t.Fatalf("resolve popup for herdr %s = %s, want %s", tc.version, got, tc.want)
			}
		})
	}
}

func TestRunKeybindPopupDimensions(t *testing.T) {
	path := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", path)
	stubHerdrVersion(t, "herdr 0.7.4\n", nil)

	stdout, err := captureOutput(t, &os.Stdout, func() error {
		return runKeybind([]string{
			"install", "herdr", "--mode", "popup",
			"--width", "60%", "--height", "50%",
		})
	})
	if err != nil {
		t.Fatalf("install popup with dimensions: %v", err)
	}
	got := readConfig(t, path)
	if !strings.Contains(got, `width = "60%"`) || !strings.Contains(got, `height = "50%"`) {
		t.Fatalf("custom popup dimensions not written exactly\n%s", got)
	}
	wantNote := "Popup size set to 60% × 50%. Adjust anytime by editing the binding in ~/.config/herdr/config.toml (width/height accept cells or %)."
	if !strings.Contains(stdout, wantNote) {
		t.Fatalf("popup install output missing help note\noutput: %s\nwant: %s", stdout, wantNote)
	}
}

func TestRunKeybindPopupDimensionsRejectInvalidValues(t *testing.T) {
	stubHerdrVersion(t, "herdr 0.7.4\n", nil)
	cases := []struct {
		flag  string
		value string
	}{
		{"--width", "80px"},
		{"--height", "-1"},
		{"--width", "80 %"},
	}
	for _, tc := range cases {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			err := runKeybind([]string{"install", "herdr", "--mode", "popup", tc.flag, tc.value})
			if err == nil {
				t.Fatalf("expected invalid %s %q to fail", tc.flag, tc.value)
			}
			if !strings.Contains(err.Error(), tc.flag) ||
				!strings.Contains(err.Error(), "cell count or percentage") {
				t.Fatalf("unclear validation error: %v", err)
			}
		})
	}
}

func TestRunKeybindPopupDimensionsIgnoredOutsideHerdrPopup(t *testing.T) {
	herdrPath := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", herdrPath)
	herdrWarning, err := captureOutput(t, &os.Stderr, func() error {
		return runKeybind([]string{
			"install", "herdr", "--mode", "pane",
			"--width", "invalid", "--height", "also-invalid",
		})
	})
	if err != nil {
		t.Fatalf("herdr pane install should ignore popup dimensions: %v", err)
	}
	if !strings.Contains(herdrWarning, "--width/--height") ||
		!strings.Contains(herdrWarning, "ignoring for herdr pane") {
		t.Fatalf("herdr pane warning missing details: %q", herdrWarning)
	}
	herdrGot := readConfig(t, herdrPath)
	if strings.Contains(herdrGot, `width = `) || strings.Contains(herdrGot, `height = `) {
		t.Fatalf("herdr pane emitted popup dimensions\n%s", herdrGot)
	}

	tmuxPath := writeTempConfig(t, "")
	t.Setenv("TMUX_CONF_PATH", tmuxPath)
	tmuxWarning, err := captureOutput(t, &os.Stderr, func() error {
		return runKeybind([]string{
			"install", "tmux", "--mode", "popup",
			"--width", "invalid", "--height", "also-invalid",
		})
	})
	if err != nil {
		t.Fatalf("tmux install should ignore herdr popup dimensions: %v", err)
	}
	if !strings.Contains(tmuxWarning, "--width/--height") ||
		!strings.Contains(tmuxWarning, "ignoring for tmux popup") {
		t.Fatalf("tmux warning missing details: %q", tmuxWarning)
	}
	if got := readConfig(t, tmuxPath); strings.Contains(got, "invalid") {
		t.Fatalf("tmux binding used ignored dimension value\n%s", got)
	}
}

func TestInstallHerdrPopupMarkerLifecycle(t *testing.T) {
	existing := "[keys]\n\n# unrelated config\n"
	path := writeTempHerdrConfig(t, existing)
	t.Setenv("HERDR_CONFIG_PATH", path)
	stubHerdrVersion(t, "herdr 0.7.4\n", nil)

	if err := installHerdrKeybind(
		"s",
		herdrModePane,
		defaultHerdrPopupWidth,
		defaultHerdrPopupHeight,
	); err != nil {
		t.Fatal(err)
	}
	if err := installHerdrKeybind("f", herdrModePopup, "120", "60%"); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, path)
	if count := strings.Count(got, herdrBindMarkerBegin); count != 1 {
		t.Fatalf("begin marker count = %d, want 1\n%s", count, got)
	}
	if count := strings.Count(got, herdrBindMarkerEnd); count != 1 {
		t.Fatalf("end marker count = %d, want 1\n%s", count, got)
	}
	for _, want := range []string{`key = "prefix+f"`, `type = "popup"`, `width = "120"`, `height = "60%"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("final popup binding missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, `key = "prefix+s"`) || strings.Contains(got, `type = "pane"`) {
		t.Fatalf("reinstall left old pane binding\n%s", got)
	}

	if err := uninstallHerdrKeybind(); err != nil {
		t.Fatal(err)
	}
	got = readConfig(t, path)
	if strings.Contains(got, herdrBindMarkerBegin) || strings.Contains(got, herdrBindMarkerEnd) ||
		strings.Contains(got, `type = "popup"`) {
		t.Fatalf("popup block remains after uninstall\n%s", got)
	}
	if !strings.Contains(got, "# unrelated config") {
		t.Fatalf("uninstall removed unrelated config\n%s", got)
	}
}

func TestRunKeybindRoutesModePerBackend(t *testing.T) {
	tmuxPath := writeTempConfig(t, "")
	t.Setenv("TMUX_CONF_PATH", tmuxPath)
	herdrPath := writeTempHerdrConfig(t, "")
	t.Setenv("HERDR_CONFIG_PATH", herdrPath)
	stubHerdrVersion(t, "herdr 0.7.4\n", nil)

	// herdr + --mode popup -> popup binding.
	if err := runKeybind([]string{"install", "herdr", "--mode", "popup"}); err != nil {
		t.Fatalf("herdr popup install: %v", err)
	}
	herdrGot := readConfig(t, herdrPath)
	if !strings.Contains(herdrGot, `type = "popup"`) {
		t.Fatalf("herdr --mode popup did not write popup type\n%s", herdrGot)
	}

	// herdr + invalid mode -> error.
	if err := runKeybind([]string{"install", "herdr", "--mode", "window"}); err == nil {
		t.Fatal("herdr --mode window should error")
	}

	// tmux + --mode popup -> tmux display-popup binding (no regression).
	if err := runKeybind([]string{"install", "tmux", "--mode", "popup"}); err != nil {
		t.Fatalf("tmux popup install: %v", err)
	}
	tmuxGot := readConfig(t, tmuxPath)
	if !strings.Contains(tmuxGot, "display-popup") {
		t.Fatalf("tmux --mode popup missing display-popup\n%s", tmuxGot)
	}

	// tmux + invalid mode -> error.
	if err := runKeybind([]string{"install", "tmux", "--mode", "frobnicate"}); err == nil {
		t.Fatal("tmux --mode frobnicate should error")
	}
}
