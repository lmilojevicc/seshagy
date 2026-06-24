// Package integrations manages per-agent hook/extension installers.
package integrations

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/seshagy-agent-state.ts
var piExtensionSource string

// Integration describes a single agent hook installer.
type Integration struct {
	Name         string
	InstallPath  func() (string, error)
	AssetContent string
}

var piIntegration = Integration{
	Name: "pi",
	InstallPath: func() (string, error) {
		base := os.Getenv("PI_CODING_AGENT_DIR")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".pi", "agent")
		}
		return filepath.Join(base, "extensions", "seshagy-agent-state.ts"), nil
	},
	AssetContent: piExtensionSource,
}

var integrations = map[string]Integration{
	"pi": piIntegration,
}

// Install writes the integration's hook/extension file to the agent's
// configuration directory.
func Install(name string) (string, error) {
	integ, ok := integrations[name]
	if !ok {
		return "", fmt.Errorf("unknown integration: %s", name)
	}
	path, err := integ.InstallPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(integ.AssetContent), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Available returns the list of installable integrations.
func Available() []string {
	return []string{"pi"}
}
