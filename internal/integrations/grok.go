package integrations

import (
	"fmt"
	"path/filepath"
)

const grokHooksRegistryName = "seshagy.json"

func installGrok(binaryPath string) ([]string, error) {
	dir := grokDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("grok config directory not found at %s", dir)
	}
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	if err := writeShellHook(TargetGrok, hookPath, binaryPath); err != nil {
		return nil, err
	}
	hooksPath := filepath.Join(dir, "hooks", grokHooksRegistryName)
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
		TargetGrok,
		grokLifecycleHooks,
		true,
	); err != nil {
		return nil, err
	}
	if err := writeJSONObject(hooksPath, root); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed Grok Build hook to %s", hookPath),
		fmt.Sprintf("updated %s", hooksPath),
	}, nil
}

func uninstallGrok() ([]string, error) {
	dir := grokDir()
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Grok Build hook", hookPath, removed)
	hooksPath := filepath.Join(dir, "hooks", grokHooksRegistryName)
	if updated, err := removeCommandsFromJSON(
		hooksPath,
		removeNestedCommands,
	); err != nil {
		return nil, err
	} else if updated {
		messages = append(
			messages,
			fmt.Sprintf("removed seshagy hook entries from %s", hooksPath),
		)
	}
	return messages, nil
}
