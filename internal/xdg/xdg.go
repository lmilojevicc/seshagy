// Package xdg centralizes home-directory and XDG base-directory resolution
// shared across the config, integrations, and sessionmgr packages.
package xdg

import (
	"os"
	"path/filepath"
	"strings"
)

// Home returns the user's home directory, falling back to "." when it cannot
// be determined.
func Home() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

// ConfigHome returns $XDG_CONFIG_HOME when set, otherwise ~/.config.
func ConfigHome() string {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return ExpandHome(value)
	}
	return filepath.Join(Home(), ".config")
}

// ExpandHome expands a leading ~ or ~/ in path to the user's home directory.
func ExpandHome(path string) string {
	if path == "~" {
		return Home()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(Home(), strings.TrimPrefix(path, "~/"))
	}
	return path
}
