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

// tmuxBindLine is the binding the launcher script expects. Wrapped in a marker
// block so reinstall/uninstall can find and replace it idempotently.
const (
	tmuxBindMarkerBegin = "# >>> seshagy keybind >>>"
	tmuxBindMarkerEnd   = "# <<< seshagy keybind <<<"
)

func tmuxBindLine(key string) string {
	return fmt.Sprintf(
		"bind-key %s run-shell 'seshagy-focus-kill seshagy'",
		key,
	)
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
		return errors.New(joinUsage("keybind", "install <name> [--key <key>] | uninstall <name>"))
	}
	switch args[0] {
	case "install":
		rest := args[1:]
		if len(rest) == 0 || rest[0] == "" {
			return errors.New(joinUsage("keybind", "install", "<name>", "[--key <key>]"))
		}
		name := rest[0]
		key := defaultTmuxKey
		for i := 1; i < len(rest); i++ {
			if rest[i] == "--key" && i+1 < len(rest) {
				key = rest[i+1]
				i++
			}
		}
		switch name {
		case "tmux":
			return installTmuxKeybind(key)
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

func installTmuxKeybind(key string) error {
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
		block := tmuxBindMarkerBegin + "\n" + tmuxBindLine(key) + "\n" + tmuxBindMarkerEnd + "\n"
		content = content[:lineStart] + block + content[lineEnd:]
	} else {
		block := "\n" + tmuxBindMarkerBegin + "\n" + tmuxBindLine(
			key,
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
