package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const (
	integrationPromptVersionFile = "integration-prompt-version"
	integrationPromptSeenFile    = "integration-prompt-seen"
)

func shouldStartupIntegrationPrompt() (bool, error) {
	stored, firstLaunch, err := promptVersionState()
	if err != nil {
		return false, err
	}
	current := integrations.CurrentInstallVersion()
	if stored >= current {
		return false, nil
	}
	recs := integrations.RecommendedForPrompt()
	if len(recs) == 0 {
		if firstLaunch {
			return false, writeIntegrationPromptVersion(current)
		}
		return false, nil
	}
	return true, nil
}

func recordIntegrationPromptDismissed() error {
	return writeIntegrationPromptVersion(integrations.CurrentInstallVersion())
}

func promptVersionState() (stored int, firstLaunch bool, err error) {
	versionPath := integrationPromptVersionPath()
	if data, readErr := os.ReadFile(versionPath); readErr == nil {
		return parseIntegrationPromptVersion(data), false, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return 0, false, readErr
	}

	legacyPath := integrationPromptSeenPath()
	if _, statErr := os.Stat(legacyPath); statErr == nil {
		return 0, false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return 0, false, statErr
	}

	return 0, true, nil
}

func writeIntegrationPromptVersion(version int) error {
	path := integrationPromptVersionPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", version)), 0o600)
}

func parseIntegrationPromptVersion(data []byte) int {
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return value
}

func integrationPromptVersionPath() string {
	return filepath.Join(xdg.StateHome(), "seshagy", integrationPromptVersionFile)
}

func integrationPromptSeenPath() string {
	return filepath.Join(xdg.StateHome(), "seshagy", integrationPromptSeenFile)
}
