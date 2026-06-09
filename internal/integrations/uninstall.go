package integrations

import (
	"fmt"
	"os"
	"path/filepath"
)

func uninstallPi() ([]string, error) {
	path := filepath.Join(piDir(), "extensions", "seshagy-agent-state.ts")
	removed, err := removeFile(path)
	return removalMessages("Pi extension", path, removed), err
}

func uninstallClaude() ([]string, error) {
	return uninstallNestedSettingsHook(TargetClaude, claudeDir(), filepath.Join(claudeDir(), "settings.json"))
}

func uninstallDroid() ([]string, error) {
	return uninstallNestedSettingsHook(TargetDroid, droidDir(), filepath.Join(droidDir(), "settings.json"))
}

func uninstallQodercli() ([]string, error) {
	return uninstallNestedSettingsHook(TargetQodercli, qoderDir(), filepath.Join(qoderDir(), "settings.json"))
}

func uninstallCodex() ([]string, error) {
	dir := codexDir()
	hookPath := filepath.Join(dir, shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Codex hook", hookPath, removed)
	hooksPath := filepath.Join(dir, "hooks.json")
	if updated, err := removeCommandsFromJSON(hooksPath, shellHookName, removeNestedCommands); err != nil {
		return nil, err
	} else if updated {
		messages = append(messages, fmt.Sprintf("removed seshagy hook entries from %s", hooksPath))
	}
	return messages, nil
}

func uninstallCopilot() ([]string, error) {
	dir := copilotDir()
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Copilot hook", hookPath, removed)
	settingsPath := filepath.Join(dir, "settings.json")
	if updated, err := removeCommandsFromJSON(settingsPath, shellHookName, removeDirectCommands); err != nil {
		return nil, err
	} else if updated {
		messages = append(messages, fmt.Sprintf("removed seshagy hook entries from %s", settingsPath))
	}
	return messages, nil
}

func uninstallOpencode() ([]string, error) {
	path := filepath.Join(opencodeDir(), "plugins", "seshagy-agent-state.js")
	removed, err := removeFile(path)
	return removalMessages("OpenCode plugin", path, removed), err
}

func uninstallCursor() ([]string, error) {
	dir := cursorDir()
	hookPath := filepath.Join(dir, shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Cursor hook", hookPath, removed)
	hooksPath := filepath.Join(dir, "hooks.json")
	if updated, err := removeCommandsFromJSON(hooksPath, shellHookName, removeSimpleCommands); err != nil {
		return nil, err
	} else if updated {
		messages = append(messages, fmt.Sprintf("removed seshagy hook entries from %s", hooksPath))
	}
	return messages, nil
}

func uninstallNestedSettingsHook(target Target, dir, settingsPath string) ([]string, error) {
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages(TargetLabel(target)+" hook", hookPath, removed)
	if updated, err := removeCommandsFromJSON(settingsPath, shellHookName, removeNestedCommands); err != nil {
		return nil, err
	} else if updated {
		messages = append(messages, fmt.Sprintf("removed seshagy hook entries from %s", settingsPath))
	}
	return messages, nil
}

func removeCommandsFromJSON(path, commandPrefix string, remove func(map[string]any, string) bool) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	root, err := readJSONObject(path)
	if err != nil {
		return false, err
	}
	hooks, err := hooksObject(root)
	if err != nil {
		return false, err
	}
	updated := remove(hooks, commandPrefix)
	if updated {
		return true, writeJSONObject(path, root)
	}
	return false, nil
}

func removeFile(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func removalMessages(label, path string, removed bool) []string {
	if removed {
		return []string{fmt.Sprintf("removed %s at %s", label, path)}
	}
	return []string{fmt.Sprintf("no %s found at %s", label, path)}
}
