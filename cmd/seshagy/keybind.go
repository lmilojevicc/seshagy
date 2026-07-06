package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultTmuxKey is the default key seshagy binds in tmux.
const defaultTmuxKey = "s"

// tmuxLaunchMode picks the pane primitive that hosts seshagy. All four give
// seshagy a real controlling TTY (unlike run-shell) and rely on
// seshagy-focus-kill for dismissal when the user switches focus.
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
// runs seshagy-focus-kill inside a primitive that provides a real TTY (the
// focus-kill wrapper then dismisses the pane when focus leaves).
const (
	tmuxBindMarkerBegin = "# >>> seshagy keybind >>>"
	tmuxBindMarkerEnd   = "# <<< seshagy keybind <<<"
)

func tmuxBindLine(key string, mode tmuxLaunchMode) string {
	// seshagy-focus-kill is found on PATH because new-window/split-window/
	// display-popup inherit the tmux server's PATH (unlike run-shell, which
	// uses a minimal PATH and was the source of the original launch failure).
	cmd := "'seshagy-focus-kill seshagy'"
	switch mode {
	case tmuxModePopup:
		return fmt.Sprintf("bind-key %s display-popup -E -w 80%% -h 80%% %s", key, cmd)
	case tmuxModeWindow:
		return fmt.Sprintf("bind-key %s new-window -c '#{pane_current_path}' %s", key, cmd)
	case tmuxModeZoomed:
		// Split + zoom so seshagy fills the window; unzoom is automatic when the
		// pane is killed by focus-kill.
		return fmt.Sprintf("bind-key %s split-window -Z -c '#{pane_current_path}' %s", key, cmd)
	case tmuxModePane:
		return fmt.Sprintf("bind-key %s split-window -c '#{pane_current_path}' %s", key, cmd)
	default:
		return fmt.Sprintf("bind-key %s display-popup -E -w 80%% -h 80%% %s", key, cmd)
	}
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

// runKeybind dispatches `seshagy keybind <cmd>`.
func runKeybind(args []string) error {
	if len(args) == 0 {
		return errors.New(
			joinUsage(
				"keybind",
				"install <name> [--key <key>] [--mode popup|window|pane|pane-zoomed] | uninstall <name>",
			),
		)
	}
	switch args[0] {
	case "install":
		rest := args[1:]
		if len(rest) == 0 || rest[0] == "" {
			return errors.New(
				joinUsage("keybind", "install", "<name>", "[--key <key>]", "[--mode <mode>]"),
			)
		}
		name := rest[0]
		key := defaultTmuxKey
		mode := tmuxModePopup
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case "--key":
				if i+1 >= len(rest) {
					return errors.New("--key requires a value")
				}
				key = rest[i+1]
				i++
			case "--mode":
				if i+1 >= len(rest) {
					return errors.New("--mode requires a value")
				}
				m, err := parseTmuxLaunchMode(rest[i+1])
				if err != nil {
					return err
				}
				mode = m
				i++
			}
		}
		switch name {
		case "tmux":
			return installTmuxKeybind(key, mode)
		default:
			return fmt.Errorf("unknown keybind target: %q (only \"tmux\" is supported)", name)
		}
	case "uninstall":
		rest := args[1:]
		if len(rest) == 0 || rest[0] == "" {
			return errors.New(joinUsage("keybind", "uninstall", "<name>"))
		}
		switch rest[0] {
		case "tmux":
			return uninstallTmuxKeybind()
		default:
			return fmt.Errorf("unknown keybind target: %q (only \"tmux\" is supported)", rest[0])
		}
	default:
		return fmt.Errorf("unknown keybind command: %q", args[0])
	}
}

func installTmuxKeybind(key string, mode tmuxLaunchMode) error {
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
		) + "\n" + tmuxBindMarkerEnd + "\n"
		content = content[:lineStart] + block + content[lineEnd:]
	} else {
		block := "\n" + tmuxBindMarkerBegin + "\n" + tmuxBindLine(
			key, mode,
		) + "\n" + tmuxBindMarkerEnd + "\n"
		if !strings.HasSuffix(content, "\n") && content != "" {
			content += "\n"
		}
		content += block
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write tmux config: %w", err)
	}
	fmt.Printf("installed tmux keybind: prefix + %s → seshagy (in %s)\n", key, path)
	fmt.Printf("reload with: tmux source-file %s\n", path)
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
			fmt.Println("no tmux config found; nothing to uninstall")
			return nil
		}
		return fmt.Errorf("read tmux config: %w", err)
	}
	content := string(existing)
	idx := strings.Index(content, tmuxBindMarkerBegin)
	if idx < 0 {
		fmt.Println("no seshagy keybind found; nothing to uninstall")
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
	fmt.Printf("uninstalled seshagy keybind from %s\n", path)
	fmt.Printf("reload with: tmux source-file %s\n", path)
	return nil
}
