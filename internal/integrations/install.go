package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	return installNestedLifecycleHooks(
		TargetClaude,
		claudeDir(),
		filepath.Join(claudeDir(), "settings.json"),
		binaryPath,
		claudeLifecycleHooks,
		true,
	)
}

func installDroid(binaryPath string) ([]string, error) {
	messages, err := installNestedLifecycleHooks(
		TargetDroid,
		droidDir(),
		filepath.Join(droidDir(), "settings.json"),
		binaryPath,
		droidLifecycleHooks,
		true,
	)
	if err != nil {
		return nil, err
	}
	hooksPath := filepath.Join(droidDir(), "hooks.json")
	if updated, err := removeCommandsFromJSON(
		hooksPath,
		removeNestedCommands,
	); err != nil {
		return nil, err
	} else if updated {
		messages = append(
			messages,
			fmt.Sprintf("removed stale seshagy hook entries from %s", hooksPath),
		)
	}
	return messages, nil
}

func installQodercli(binaryPath string) ([]string, error) {
	return installNestedLifecycleHooks(
		TargetQodercli,
		qoderDir(),
		filepath.Join(qoderDir(), "settings.json"),
		binaryPath,
		qodercliLifecycleHooks,
		true,
	)
}

func installCodex(binaryPath string) ([]string, error) {
	dir := codexDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("codex config directory not found at %s", dir)
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
	removeNestedCommands(hooks, shellHookName)
	if err := ensureNestedLifecycleHooks(
		hooks,
		hookPath,
		TargetCodex,
		codexLifecycleHooks,
		false,
	); err != nil {
		return nil, err
	}
	if err := writeJSONObject(hooksPath, root); err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := ensureCodexHooksEnabled(configPath); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed Codex hook to %s", hookPath),
		fmt.Sprintf("updated %s", hooksPath),
		fmt.Sprintf("enabled Codex hooks in %s", configPath),
	}, nil
}

func installCopilot(binaryPath string) ([]string, error) {
	dir := copilotDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("copilot config directory not found at %s", dir)
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
	removeDirectCommands(hooks, shellHookName)
	for _, event := range copilotStaleLifecycleHooks {
		removeDirectCommandsForEvent(hooks, event, shellHookName)
	}
	if err := ensureDirectLifecycleHooks(
		hooks,
		hookPath,
		TargetCopilot,
		copilotLifecycleHooks,
	); err != nil {
		return nil, err
	}
	if err := writeJSONObject(settingsPath, root); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed Copilot hook to %s", hookPath),
		fmt.Sprintf("updated %s", settingsPath),
	}, nil
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
		return nil, fmt.Errorf("cursor config directory not found at %s", dir)
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
	removeSimpleCommands(hooks, shellHookName)
	for _, hook := range cursorStaleLifecycleHooks {
		removeSimpleCommandsForAction(hooks, hook.event, shellHookName, hook.action)
	}
	if _, ok := root["version"]; !ok {
		root["version"] = float64(1)
	}
	if err := ensureSimpleLifecycleHooks(
		hooks,
		hookPath,
		TargetCursor,
		cursorLifecycleHooks,
	); err != nil {
		return nil, err
	}
	if err := writeJSONObject(hooksPath, root); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed Cursor hook to %s", hookPath),
		fmt.Sprintf("updated %s", hooksPath),
	}, nil
}

func installNestedLifecycleHooks(
	target Target,
	dir, settingsPath, binaryPath string,
	events []lifecycleHook,
	matcherStar bool,
) ([]string, error) {
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
	removeNestedCommands(hooks, shellHookName)
	if err := ensureNestedLifecycleHooks(hooks, hookPath, target, events, matcherStar); err != nil {
		return nil, err
	}
	if err := writeJSONObject(settingsPath, root); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed %s hook to %s", TargetLabel(target), hookPath),
		fmt.Sprintf("updated %s", settingsPath),
	}, nil
}

func ensureNestedLifecycleHooks(
	hooks map[string]any,
	hookPath string,
	target Target,
	events []lifecycleHook,
	matcherStar bool,
) error {
	for _, hook := range events {
		command := shellHookCommand(hookPath, target, hook.action)
		if err := ensureNestedCommandHook(
			hooks,
			hook.event,
			command,
			nestedLifecycleMatcher(hook.event, matcherStar),
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectLifecycleHooks(
	hooks map[string]any,
	hookPath string,
	target Target,
	events []lifecycleHook,
) error {
	for _, hook := range events {
		command := shellHookCommand(hookPath, target, hook.action)
		if err := ensureDirectCommandHook(hooks, hook.event, command); err != nil {
			return err
		}
	}
	return nil
}

func ensureSimpleLifecycleHooks(
	hooks map[string]any,
	hookPath string,
	target Target,
	events []lifecycleHook,
) error {
	for _, hook := range events {
		command := shellHookCommand(hookPath, target, hook.action)
		if err := ensureSimpleCommandHook(hooks, hook.event, command); err != nil {
			return err
		}
	}
	return nil
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
	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(strings.Trim(trimmed, "[]"))
		}
		if section == "features" && strings.HasPrefix(trimmed, "codex_hooks") {
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
