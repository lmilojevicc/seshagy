package integrations

import (
	"fmt"
	"os"
	"path/filepath"
)

const kiloPluginName = "seshagy-agent-state.js"

func installKilo(binaryPath string) ([]string, error) {
	dir := kiloDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("kilo config directory not found at %s", dir)
	}
	pluginsDir := filepath.Join(dir, "plugin")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(pluginsDir, kiloPluginName)
	if err := os.WriteFile(path, []byte(kiloPluginAsset(binaryPath)), 0o644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed Kilo Code plugin to %s", path)}, nil
}

func uninstallKilo() ([]string, error) {
	path := filepath.Join(kiloDir(), "plugin", kiloPluginName)
	removed, err := removeFile(path)
	return removalMessages("Kilo Code plugin", path, removed), err
}
