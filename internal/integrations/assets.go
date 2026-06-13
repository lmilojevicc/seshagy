package integrations

import (
	_ "embed"
	"fmt"
	"strings"
)

const shellHookName = "seshagy-agent-state.sh"

// Hook/plugin asset bodies are kept as separate template files (under assets/)
// and embedded here. They are fmt.Sprintf format strings: literal percent signs
// in shell snippets are escaped as %% in the template files.
var (
	//go:embed assets/seshagy-agent-state.sh.tmpl
	shellHookTemplate string
	//go:embed assets/pi-extension.ts.tmpl
	piExtensionTemplate string
	//go:embed assets/opencode-plugin.ts.tmpl
	opencodePluginTemplate string
)

func shellHookAsset(target Target, binaryPath string) string {
	return fmt.Sprintf(
		shellHookTemplate,
		target,
		installVersion,
		target,
		shellQuoteLiteral(binaryPath),
	)
}

func piExtensionAsset(binaryPath string) string {
	return fmt.Sprintf(piExtensionTemplate, installVersion, binaryPath)
}

func opencodePluginAsset(binaryPath string) string {
	return fmt.Sprintf(opencodePluginTemplate, installVersion, binaryPath)
}

func shellQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
