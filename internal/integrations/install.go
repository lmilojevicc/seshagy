package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var nestedLifecycle = []struct{ event, state string }{
	{"SessionStart", "idle"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"PermissionRequest", "blocked"},
	{"Notification", "blocked"},
	{"Stop", "done"},
	{"SessionEnd", "release"},
}

var directLifecycle = []struct{ event, state string }{
	{"SessionStart", "idle"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"notification", "blocked"},
	{"Stop", "done"},
	{"SessionEnd", "release"},
}

var cursorLifecycle = []struct{ event, state string }{
	{"sessionStart", "idle"},
	{"beforeSubmitPrompt", "working"},
	{"beforeShellExecution", "working"},
	{"beforeMCPExecution", "working"},
	{"notification", "blocked"},
	{"stop", "done"},
	{"sessionEnd", "release"},
}

func installPi(binaryPath string) ([]string, error) {
	dir := filepath.Join(piDir(), "extensions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "seshagy-agent-state.ts")
	if err := os.WriteFile(path, []byte(piExtensionAsset(binaryPath)), 0o644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed Pi extension to %s", path)}, nil
}

func installClaude(binaryPath string) ([]string, error) {
	return installNestedSettingsHook(TargetClaude, claudeDir(), filepath.Join(claudeDir(), "settings.json"), binaryPath, true)
}

func installDroid(binaryPath string) ([]string, error) {
	return installNestedSettingsHook(TargetDroid, droidDir(), filepath.Join(droidDir(), "settings.json"), binaryPath, false)
}

func installQodercli(binaryPath string) ([]string, error) {
	return installNestedSettingsHook(TargetQodercli, qoderDir(), filepath.Join(qoderDir(), "settings.json"), binaryPath, false)
}

func installCodex(binaryPath string) ([]string, error) {
	dir := codexDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("Codex config directory not found at %s", dir)
	}
	hookPath := filepath.Join(dir, shellHookName)
	if err := writeShellHook(TargetCodex, hookPath, binaryPath); err != nil {
		return nil, err
	}
	hooksPath := filepath.Join(dir, "hooks.json")
	root, err := readJSONObject(hooksPath)
	if err != nil {
		return nil, err
	}
	hooks, err := hooksObject(root)
	if err != nil {
		return nil, err
	}
	for _, item := range nestedLifecycle {
		if err := ensureNestedCommandHook(hooks, item.event, hookCommand(hookPath, TargetCodex, item.state), ""); err != nil {
			return nil, err
		}
	}
	if err := writeJSONObject(hooksPath, root); err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := ensureCodexHooksEnabled(configPath); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed Codex hook to %s", hookPath), fmt.Sprintf("updated %s", hooksPath), fmt.Sprintf("enabled Codex hooks in %s", configPath)}, nil
}

func installCopilot(binaryPath string) ([]string, error) {
	dir := copilotDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("Copilot config directory not found at %s", dir)
	}
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	if err := writeShellHook(TargetCopilot, hookPath, binaryPath); err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(dir, "settings.json")
	root, err := readJSONObject(settingsPath)
	if err != nil {
		return nil, err
	}
	hooks, err := hooksObject(root)
	if err != nil {
		return nil, err
	}
	for _, item := range directLifecycle {
		if err := ensureDirectCommandHook(hooks, item.event, hookCommand(hookPath, TargetCopilot, item.state)); err != nil {
			return nil, err
		}
	}
	if err := writeJSONObject(settingsPath, root); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed Copilot hook to %s", hookPath), fmt.Sprintf("updated %s", settingsPath)}, nil
}

func installOpencode(binaryPath string) ([]string, error) {
	dir := opencodeDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("OpenCode config directory not found at %s", dir)
	}
	pluginsDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(pluginsDir, "seshagy-agent-state.js")
	if err := os.WriteFile(path, []byte(opencodePluginAsset(binaryPath)), 0o644); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed OpenCode plugin to %s", path)}, nil
}

func installCursor(binaryPath string) ([]string, error) {
	dir := cursorDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("Cursor config directory not found at %s", dir)
	}
	hookPath := filepath.Join(dir, shellHookName)
	if err := writeShellHook(TargetCursor, hookPath, binaryPath); err != nil {
		return nil, err
	}
	hooksPath := filepath.Join(dir, "hooks.json")
	root, err := readJSONObject(hooksPath)
	if err != nil {
		return nil, err
	}
	hooks, err := hooksObject(root)
	if err != nil {
		return nil, err
	}
	for _, item := range cursorLifecycle {
		if err := ensureSimpleCommandHook(hooks, item.event, hookCommand(hookPath, TargetCursor, item.state)); err != nil {
			return nil, err
		}
	}
	if err := writeJSONObject(hooksPath, root); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed Cursor hook to %s", hookPath), fmt.Sprintf("updated %s", hooksPath)}, nil
}

func installNestedSettingsHook(target Target, dir, settingsPath, binaryPath string, matcherStar bool) ([]string, error) {
	if !configDirExists(dir) {
		return nil, fmt.Errorf("%s config directory not found at %s", TargetLabel(target), dir)
	}
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	if err := writeShellHook(target, hookPath, binaryPath); err != nil {
		return nil, err
	}
	root, err := readJSONObject(settingsPath)
	if err != nil {
		return nil, err
	}
	hooks, err := hooksObject(root)
	if err != nil {
		return nil, err
	}
	matcher := ""
	if matcherStar {
		matcher = "*"
	}
	for _, item := range nestedLifecycle {
		if err := ensureNestedCommandHook(hooks, item.event, hookCommand(hookPath, target, item.state), matcher); err != nil {
			return nil, err
		}
	}
	if err := writeJSONObject(settingsPath, root); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("installed %s hook to %s", TargetLabel(target), hookPath), fmt.Sprintf("updated %s", settingsPath)}, nil
}

func writeShellHook(target Target, path, binaryPath string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(shellHookAsset(target, binaryPath)), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func hookCommand(hookPath string, target Target, state string) string {
	return "bash " + shellQuoteLiteral(hookPath) + " " + string(target) + " " + state
}

func ensureCodexHooksEnabled(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	updated := buildCodexConfigWithHooks(content)
	if updated == content && err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func buildCodexConfigWithHooks(content string) string {
	lines := strings.Split(content, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "codex_hooks") {
			continue
		}
		filtered = append(filtered, line)
	}
	lines = filtered
	features := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "[features]" {
			features = i
			break
		}
	}
	if features == -1 {
		base := strings.TrimRight(strings.Join(lines, "\n"), "\n")
		if base != "" {
			base += "\n\n"
		}
		return base + "[features]\nhooks = true\n"
	}
	for i := features + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			break
		}
		if strings.HasPrefix(trimmed, "hooks") {
			lines[i] = "hooks = true"
			return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
		}
	}
	inserted := make([]string, 0, len(lines)+1)
	inserted = append(inserted, lines[:features+1]...)
	inserted = append(inserted, "hooks = true")
	inserted = append(inserted, lines[features+1:]...)
	lines = inserted
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}
