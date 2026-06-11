package integrations

import (
	"os"
	"path/filepath"
	"strings"
)

func piDir() string { return envDir("PI_CODING_AGENT_DIR", filepath.Join(homeDir(), ".pi", "agent")) }

func claudeDir() string  { return envDir("CLAUDE_CONFIG_DIR", filepath.Join(homeDir(), ".claude")) }
func codexDir() string   { return envDir("CODEX_HOME", filepath.Join(homeDir(), ".codex")) }
func copilotDir() string { return envDir("COPILOT_HOME", filepath.Join(homeDir(), ".copilot")) }
func droidDir() string   { return filepath.Join(homeDir(), ".factory") }
func qoderDir() string   { return envDir("QODER_CONFIG_DIR", filepath.Join(homeDir(), ".qoder")) }

func cursorDir() string   { return envDir("CURSOR_CONFIG_DIR", filepath.Join(homeDir(), ".cursor")) }
func opencodeDir() string { return filepath.Join(configHome(), "opencode") }

func envDir(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return expandHome(value)
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "."
}

func configHome() string {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return expandHome(value)
	}
	return filepath.Join(homeDir(), ".config")
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}
