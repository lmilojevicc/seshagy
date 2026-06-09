package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const integrationPromptSeenFile = "integration-prompt-seen"

func claimStartupIntegrationPrompt() (bool, error) {
	path := integrationPromptSeenPath()
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte("seen\n"), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func integrationPromptSeenPath() string {
	return filepath.Join(stateHome(), "seshagy", integrationPromptSeenFile)
}

func stateHome() string {
	if value := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); value != "" {
		return expandStateHome(value)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state")
	}
	return "."
}

func expandStateHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return "."
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
