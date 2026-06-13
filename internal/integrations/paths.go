package integrations

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

func piDir() string { return envDir("PI_CODING_AGENT_DIR", filepath.Join(xdg.Home(), ".pi", "agent")) }

func claudeDir() string  { return envDir("CLAUDE_CONFIG_DIR", filepath.Join(xdg.Home(), ".claude")) }
func codexDir() string   { return envDir("CODEX_HOME", filepath.Join(xdg.Home(), ".codex")) }
func copilotDir() string { return envDir("COPILOT_HOME", filepath.Join(xdg.Home(), ".copilot")) }
func droidDir() string   { return filepath.Join(xdg.Home(), ".factory") }
func qoderDir() string   { return envDir("QODER_CONFIG_DIR", filepath.Join(xdg.Home(), ".qoder")) }

func cursorDir() string   { return envDir("CURSOR_CONFIG_DIR", filepath.Join(xdg.Home(), ".cursor")) }
func opencodeDir() string { return filepath.Join(xdg.ConfigHome(), "opencode") }

func envDir(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return xdg.ExpandHome(value)
}
