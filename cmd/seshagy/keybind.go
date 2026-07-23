package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/cli"
)

// defaultTmuxKey is the default key seshagy binds in tmux.
const defaultTmuxKey = "s"

// tmuxLaunchMode picks the pane primitive that hosts seshagy. All four give
// seshagy a real controlling TTY (unlike run-shell).
type tmuxLaunchMode string

const (
	tmuxModePopup  tmuxLaunchMode = "popup"       // floating overlay (display-popup)
	tmuxModeWindow tmuxLaunchMode = "window"      // new full window (new-window)
	tmuxModePane   tmuxLaunchMode = "pane"        // split-window, unzoomed
	tmuxModeZoomed tmuxLaunchMode = "pane-zoomed" // split-window, then zoom
)

func parseTmuxLaunchMode(s string) (tmuxLaunchMode, error) {
	switch s {
	case "", string(tmuxModePopup):
		return tmuxModePopup, nil
	case string(tmuxModeWindow):
		return tmuxModeWindow, nil
	case string(tmuxModePane):
		return tmuxModePane, nil
	case string(tmuxModeZoomed):
		return tmuxModeZoomed, nil
	default:
		return "", fmt.Errorf(
			"unknown launch mode %q (want popup|window|pane|pane-zoomed)", s,
		)
	}
}

// tmuxBindLine is the binding for the chosen launch mode. Wrapped in a marker
// block so reinstall/uninstall can find and replace it idempotently. Each mode
// runs seshagy inside a primitive that provides a real TTY. By default,
// --ephemeral dismisses the pane when focus leaves.
const (
	tmuxBindMarkerBegin = "# >>> seshagy keybind >>>"
	tmuxBindMarkerEnd   = "# <<< seshagy keybind <<<"
)

func tmuxBindLine(key string, mode tmuxLaunchMode, persistent bool) string {
	// seshagy is found on PATH because new-window/split-window/
	// display-popup inherit the tmux server's PATH (unlike run-shell, which
	// uses a minimal PATH and was the source of the original launch failure).
	cmd := "'seshagy --ephemeral'"
	if persistent {
		cmd = "'seshagy'"
	}
	switch mode {
	case tmuxModePopup:
		return fmt.Sprintf("bind-key %s display-popup -E -w 80%% -h 80%% %s", key, cmd)
	case tmuxModeWindow:
		return fmt.Sprintf("bind-key %s new-window -c '#{pane_current_path}' %s", key, cmd)
	case tmuxModeZoomed:
		// Split + zoom so seshagy fills the window. With --ephemeral, unzoom is
		// automatic when the pane exits on focus-loss; persistent installs stay
		// zoomed until the user explicitly quits seshagy.
		return fmt.Sprintf("bind-key %s split-window -Z -c '#{pane_current_path}' %s", key, cmd)
	case tmuxModePane:
		return fmt.Sprintf("bind-key %s split-window -c '#{pane_current_path}' %s", key, cmd)
	default:
		return fmt.Sprintf("bind-key %s display-popup -E -w 80%% -h 80%% %s", key, cmd)
	}
}

const (
	herdrBindMarkerBegin = "# >>> seshagy keybind >>>"
	herdrBindMarkerEnd   = "# <<< seshagy keybind <<<"
)

// herdrLaunchMode picks the herdr primitive that hosts seshagy. herdr 0.7.4+
// supports "popup" (a session-modal floating terminal) in addition to "pane".
// The default is "pane" to preserve the original behavior.
type herdrLaunchMode string

const (
	herdrModePane           herdrLaunchMode = "pane"
	herdrModePopup          herdrLaunchMode = "popup"
	defaultHerdrPopupWidth                  = "80%"
	defaultHerdrPopupHeight                 = "80%"
)

var herdrPopupDimensionPattern = regexp.MustCompile(`^[0-9]+%?$`)

func validateHerdrPopupDimension(flag, value string) error {
	if !herdrPopupDimensionPattern.MatchString(value) {
		return fmt.Errorf("%s must be a cell count or percentage (for example, 120 or 80%%)", flag)
	}
	return nil
}

func parseHerdrLaunchMode(s string) (herdrLaunchMode, error) {
	switch s {
	case "", string(herdrModePane):
		return herdrModePane, nil
	case string(herdrModePopup):
		return herdrModePopup, nil
	default:
		return "", fmt.Errorf(
			"unknown herdr launch mode %q (want pane|popup)", s,
		)
	}
}

// herdrBindBlock returns the TOML [[keys.command]] block (with markers) that
// wires prefix+<key> to launch seshagy. "pane" (default) opens a temporary pane
// that closes on command exit; "popup" (herdr 0.7.4+) opens a session-modal
// floating terminal whose dimensions default to 80% of the terminal. Unless
// persistent is set, --ephemeral also dismisses either mode on focus-loss.
func herdrBindBlock(
	key string,
	mode herdrLaunchMode,
	width, height string,
	persistent bool,
) string {
	command := "seshagy --ephemeral"
	if persistent {
		command = "seshagy"
	}
	var body string
	switch mode {
	case herdrModePopup:
		body = fmt.Sprintf(`[[keys.command]]
  key = "prefix+%s"
  type = "popup"
  width = "%s"
  height = "%s"
  command = "%s"
  description = "seshagy session manager"`, key, width, height, command)
	default:
		body = fmt.Sprintf(`[[keys.command]]
  key = "prefix+%s"
  type = "pane"
  command = "%s"
  description = "seshagy session manager"`, key, command)
	}
	return fmt.Sprintf("%s\n%s\n%s", herdrBindMarkerBegin, body, herdrBindMarkerEnd)
}

func herdrConfigPath() (string, error) {
	if p := os.Getenv("HERDR_CONFIG_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	xdgRoot := os.Getenv("XDG_CONFIG_HOME")
	if xdgRoot == "" {
		xdgRoot = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgRoot, "herdr", "config.toml"), nil
}

// semver is a parsed major.minor.patch version used to gate herdr features.
type semver struct {
	major, minor, patch int
	prerelease          string
}

// before reports whether v sorts strictly before o. For equal core versions,
// a prerelease sorts before the stable release.
func (v semver) before(o semver) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	if v.patch != o.patch {
		return v.patch < o.patch
	}
	return v.prerelease != "" && o.prerelease == ""
}

// minHerdrPopupVersion is the first herdr release that ships the popup key
// type (herdr 0.7.4).
var minHerdrPopupVersion = semver{major: 0, minor: 7, patch: 4}

// herdrVersionOutput is the seam used to read `herdr --version`; overridden in
// tests to stub the binary. It reuses the same herdr CLI invoked by the
// sessionmgr backend (no new deps).
var herdrVersionOutput = func() ([]byte, error) {
	return exec.Command("herdr", "--version").Output()
}

// parseSemver extracts a major.minor.patch version from s (e.g. "0.7.4" or
// "v0.7.4-rc1"), retaining whether it is a prerelease while ignoring build
// metadata. Returns ok=false unless s has a 3-part numeric core.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	prerelease := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		if i == len(s)-1 {
			return semver{}, false
		}
		prerelease = s[i+1:]
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return semver{}, false
	}
	var v semver
	for i, p := range parts {
		if p == "" {
			return semver{}, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return semver{}, false
		}
		switch i {
		case 0:
			v.major = n
		case 1:
			v.minor = n
		case 2:
			v.patch = n
		}
	}
	v.prerelease = prerelease
	return v, true
}

// parseHerdrVersion extracts the first major.minor.patch triple from
// `herdr --version` output, scanning whitespace-separated fields. Returns
// ok=false when no semver token is found.
func parseHerdrVersion(out []byte) (semver, bool) {
	for _, f := range strings.Fields(string(out)) {
		if v, ok := parseSemver(f); ok {
			return v, true
		}
	}
	return semver{}, false
}

// resolveHerdrPopupMode downgrades a popup request to pane when the installed
// herdr predates popup support (0.7.4), printing a warning. If the version
// cannot be determined (herdr missing or output unparseable) the request is
// honored unchanged so newer herdr builds with unknown formats are not blocked.
func resolveHerdrPopupMode(mode herdrLaunchMode) herdrLaunchMode {
	if mode != herdrModePopup {
		return mode
	}
	out, err := herdrVersionOutput()
	if err != nil {
		return mode // herdr not runnable; honor the request (best-effort).
	}
	v, ok := parseHerdrVersion(out)
	if !ok {
		return mode // unparseable version: don't block newer builds.
	}
	if v.before(minHerdrPopupVersion) {
		version := fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
		if v.prerelease != "" {
			version += "-" + v.prerelease
		}
		cli.Warnf("herdr %s does not support popup mode (requires >= 0.7.4); "+
			"falling back to pane", version)
		return herdrModePane
	}
	return mode
}

func installHerdrKeybind(
	key string,
	mode herdrLaunchMode,
	width, height string,
	persistent bool,
) error {
	mode = resolveHerdrPopupMode(mode)
	path, err := herdrConfigPath()
	if err != nil {
		return fmt.Errorf("locate herdr config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create herdr config dir: %w", err)
	}
	existing, _ := os.ReadFile(path)
	content := string(existing)
	block := herdrBindBlock(key, mode, width, height, persistent)

	if idx := strings.Index(content, herdrBindMarkerBegin); idx >= 0 {
		end := strings.Index(content[idx:], herdrBindMarkerEnd)
		if end < 0 {
			return errors.New("malformed seshagy keybind block: missing end marker; fix manually")
		}
		lineStart := idx
		for lineStart > 0 && content[lineStart-1] != '\n' {
			lineStart--
		}
		lineEnd := idx + end + len(herdrBindMarkerEnd)
		if lineEnd < len(content) && content[lineEnd] == '\n' {
			lineEnd++
		}
		content = content[:lineStart] + block + "\n" + content[lineEnd:]
	} else {
		if !strings.HasSuffix(content, "\n") && content != "" {
			content += "\n"
		}
		content += "\n" + block + "\n"
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write herdr config: %w", err)
	}
	cli.Successf("installed herdr keybind: prefix+%s → seshagy (in %s)", key, path)
	cli.Info("reload with: herdr server reload-config")
	if mode == herdrModePopup {
		cli.Infof(
			"Popup size set to %s × %s. Adjust anytime by editing the binding in ~/.config/herdr/config.toml (width/height accept cells or %%).",
			width,
			height,
		)
	}
	return nil
}

func uninstallHerdrKeybind() error {
	path, err := herdrConfigPath()
	if err != nil {
		return fmt.Errorf("locate herdr config: %w", err)
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cli.Info("no herdr config found; nothing to uninstall")
			return nil
		}
		return fmt.Errorf("read herdr config: %w", err)
	}
	content := string(existing)
	idx := strings.Index(content, herdrBindMarkerBegin)
	if idx < 0 {
		cli.Info("no seshagy keybind found; nothing to uninstall")
		return nil
	}
	end := strings.Index(content[idx:], herdrBindMarkerEnd)
	if end < 0 {
		return errors.New("malformed seshagy keybind block: missing end marker; fix manually")
	}
	lineStart := idx
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := idx + end + len(herdrBindMarkerEnd)
	if lineEnd < len(content) && content[lineEnd] == '\n' {
		lineEnd++
	}
	content = content[:lineStart] + content[lineEnd:]
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write herdr config: %w", err)
	}
	cli.Successf("uninstalled seshagy keybind from %s", path)
	cli.Info("reload with: herdr server reload-config")
	return nil
}

func tmuxConfigPath() (string, error) {
	// 1. Explicit path override via env var.
	if p := os.Getenv("TMUX_CONF_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// 2. tmux 3.3+ honors $TMUX_CONFIG_DIR for the config directory; otherwise
	//    it falls back to $XDG_CONFIG_HOME/tmux (default ~/.config/tmux). Use
	//    the same resolution so we write where tmux actually reads.
	confDir := os.Getenv("TMUX_CONFIG_DIR")
	if confDir == "" {
		xdgRoot := os.Getenv("XDG_CONFIG_HOME")
		if xdgRoot == "" {
			xdgRoot = filepath.Join(home, ".config")
		}
		confDir = filepath.Join(xdgRoot, "tmux")
	}
	xdgConf := filepath.Join(confDir, "tmux.conf")
	if _, err := os.Stat(xdgConf); err == nil {
		return xdgConf, nil
	}
	// 3. Legacy default.
	return filepath.Join(home, ".tmux.conf"), nil
}

type keybindCommand struct {
	action     string
	target     string
	key        string
	modeText   string
	tmuxMode   tmuxLaunchMode
	herdrMode  herdrLaunchMode
	width      string
	height     string
	widthSet   bool
	heightSet  bool
	persistent bool
}

func parseKeybindCommand(args []string) (keybindCommand, error) {
	if len(args) == 0 {
		return keybindCommand{}, errors.New(
			keybindUsage("install <name> [options]", "uninstall <name>"),
		)
	}
	if args[0] == "uninstall" {
		if len(args) < 2 || args[1] == "" {
			return keybindCommand{}, errors.New(joinUsage("keybind", "uninstall", "<name>"))
		}
		if args[1] != "tmux" && args[1] != "herdr" {
			return keybindCommand{}, fmt.Errorf(
				"unknown keybind target: %q (only \"tmux\" and \"herdr\" are supported)",
				args[1],
			)
		}
		return keybindCommand{action: "uninstall", target: args[1]}, nil
	}
	if args[0] != "install" {
		return keybindCommand{}, fmt.Errorf("unknown keybind command: %q", args[0])
	}
	if len(args) < 2 || args[1] == "" {
		return keybindCommand{}, errors.New(keybindUsage("install <name> [options]"))
	}
	cmd := keybindCommand{
		action: "install", target: args[1], key: defaultTmuxKey,
		width: defaultHerdrPopupWidth, height: defaultHerdrPopupHeight,
	}
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--key", "--mode", "--width", "--height":
			if i+1 >= len(args) {
				return keybindCommand{}, fmt.Errorf("%s requires a value", args[i])
			}
			value := args[i+1]
			switch args[i] {
			case "--key":
				cmd.key = value
			case "--mode":
				cmd.modeText = value
			case "--width":
				cmd.width, cmd.widthSet = value, true
			case "--height":
				cmd.height, cmd.heightSet = value, true
			}
			i++
		case "--persistent":
			cmd.persistent = true
		}
	}
	var err error
	switch cmd.target {
	case "tmux":
		cmd.tmuxMode, err = parseTmuxLaunchMode(cmd.modeText)
	case "herdr":
		cmd.herdrMode, err = parseHerdrLaunchMode(cmd.modeText)
		if err == nil && cmd.herdrMode == herdrModePopup {
			if err = validateHerdrPopupDimension("--width", cmd.width); err == nil {
				err = validateHerdrPopupDimension("--height", cmd.height)
			}
		}
	default:
		return keybindCommand{}, fmt.Errorf(
			"unknown keybind target: %q (only \"tmux\" and \"herdr\" are supported)",
			cmd.target,
		)
	}
	return cmd, err
}

func executeKeybind(cmd keybindCommand) error {
	if cmd.action == "uninstall" {
		if cmd.target == "tmux" {
			return uninstallTmuxKeybind()
		}
		return uninstallHerdrKeybind()
	}
	if cmd.target == "tmux" {
		warnIgnoredPopupDimensions(cmd.target, cmd.modeText, cmd.widthSet, cmd.heightSet)
		return installTmuxKeybind(cmd.key, cmd.tmuxMode, cmd.persistent)
	}
	if cmd.herdrMode != herdrModePopup {
		warnIgnoredPopupDimensions(
			cmd.target,
			string(cmd.herdrMode),
			cmd.widthSet,
			cmd.heightSet,
		)
		return installHerdrKeybind(
			cmd.key,
			cmd.herdrMode,
			defaultHerdrPopupWidth,
			defaultHerdrPopupHeight,
			cmd.persistent,
		)
	}
	return installHerdrKeybind(cmd.key, cmd.herdrMode, cmd.width, cmd.height, cmd.persistent)
}

// runKeybind dispatches `seshagy keybind <cmd>`.
func runKeybind(args []string) error {
	cmd, err := parseKeybindCommand(args)
	if err != nil {
		return err
	}
	return executeKeybind(cmd)
}

func warnIgnoredPopupDimensions(target, mode string, widthSet, heightSet bool) {
	var flags []string
	if widthSet {
		flags = append(flags, "--width")
	}
	if heightSet {
		flags = append(flags, "--height")
	}
	if len(flags) == 0 {
		return
	}
	if mode == "" {
		mode = "default mode"
	}
	cli.Warnf(
		"%s only apply to herdr popup mode; ignoring for %s %s",
		strings.Join(flags, "/"),
		target,
		mode,
	)
}

func installTmuxKeybind(key string, mode tmuxLaunchMode, persistent bool) error {
	path, err := tmuxConfigPath()
	if err != nil {
		return fmt.Errorf("locate tmux config: %w", err)
	}
	existing, _ := os.ReadFile(path)
	content := string(existing)

	if idx := strings.Index(content, tmuxBindMarkerBegin); idx >= 0 {
		// Replace the existing marker block in place.
		end := strings.Index(content[idx:], tmuxBindMarkerEnd)
		if end < 0 {
			return errors.New("malformed seshagy keybind block: missing end marker; fix manually")
		}
		// Find the start of the line containing the marker (include its newline).
		lineStart := idx
		for lineStart > 0 && content[lineStart-1] != '\n' {
			lineStart--
		}
		lineEnd := idx + end + len(tmuxBindMarkerEnd)
		if lineEnd < len(content) && content[lineEnd] == '\n' {
			lineEnd++
		}
		block := tmuxBindMarkerBegin + "\n" + tmuxBindLine(
			key,
			mode,
			persistent,
		) + "\n" + tmuxBindMarkerEnd + "\n"
		content = content[:lineStart] + block + content[lineEnd:]
	} else {
		block := "\n" + tmuxBindMarkerBegin + "\n" + tmuxBindLine(
			key, mode, persistent,
		) + "\n" + tmuxBindMarkerEnd + "\n"
		if !strings.HasSuffix(content, "\n") && content != "" {
			content += "\n"
		}
		content += block
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write tmux config: %w", err)
	}
	cli.Successf("installed tmux keybind: prefix + %s → seshagy (in %s)", key, path)
	cli.Infof("reload with: tmux source-file %s", path)
	return nil
}

func uninstallTmuxKeybind() error {
	path, err := tmuxConfigPath()
	if err != nil {
		return fmt.Errorf("locate tmux config: %w", err)
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cli.Info("no tmux config found; nothing to uninstall")
			return nil
		}
		return fmt.Errorf("read tmux config: %w", err)
	}
	content := string(existing)
	idx := strings.Index(content, tmuxBindMarkerBegin)
	if idx < 0 {
		cli.Info("no seshagy keybind found; nothing to uninstall")
		return nil
	}
	end := strings.Index(content[idx:], tmuxBindMarkerEnd)
	if end < 0 {
		return errors.New("malformed seshagy keybind block: missing end marker; fix manually")
	}
	lineStart := idx
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := idx + end + len(tmuxBindMarkerEnd)
	if lineEnd < len(content) && content[lineEnd] == '\n' {
		lineEnd++
	}
	content = content[:lineStart] + content[lineEnd:]
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write tmux config: %w", err)
	}
	cli.Successf("uninstalled seshagy keybind from %s", path)
	cli.Infof("reload with: tmux source-file %s", path)
	return nil
}
