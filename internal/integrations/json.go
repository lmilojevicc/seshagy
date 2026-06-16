package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, nil
}

func writeJSONObject(path string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func hooksObject(root map[string]any) (map[string]any, error) {
	raw, ok := root["hooks"]
	if !ok || raw == nil {
		hooks := map[string]any{}
		root["hooks"] = hooks
		return hooks, nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("hooks must be a JSON object")
	}
	return hooks, nil
}

func ensureNestedCommandHook(hooks map[string]any, event, command string, matcher string) error {
	entries, err := hookArray(hooks, event)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hookEntries, _ := entryObject["hooks"].([]any)
		for _, hook := range hookEntries {
			hookObject, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if hookObject["type"] == "command" && hookObject["command"] == command {
				return nil
			}
		}
	}
	entry := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command, "timeout": float64(10)},
		},
	}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	entries = append(entries, entry)
	hooks[event] = entries
	return nil
}

func ensureDirectCommandHook(hooks map[string]any, event, command string) error {
	entries, err := hookArray(hooks, event)
	if err != nil {
		return err
	}
	for i, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if entryObject["type"] == "command" &&
			(entryObject["bash"] == command || entryObject["command"] == command) {
			entryObject["bash"] = command
			entryObject["timeoutSec"] = float64(10)
			delete(entryObject, "command")
			entries[i] = entryObject
			hooks[event] = entries
			return nil
		}
	}
	entries = append(
		entries,
		map[string]any{"type": "command", "bash": command, "timeoutSec": float64(10)},
	)
	hooks[event] = entries
	return nil
}

func ensureSimpleCommandHook(hooks map[string]any, event, command string) error {
	entries, err := hookArray(hooks, event)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if ok && entryObject["command"] == command {
			return nil
		}
	}
	entries = append(entries, map[string]any{"command": command})
	hooks[event] = entries
	return nil
}

func hookArray(hooks map[string]any, event string) ([]any, error) {
	raw, ok := hooks[event]
	if !ok || raw == nil {
		return []any{}, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("hook entries for %s must be an array", event)
	}
	return entries, nil
}

func removeNestedCommands(hooks map[string]any, commandPrefix string) bool {
	updated := false
	for event, raw := range hooks {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		keptEntries := entries[:0]
		for _, entry := range entries {
			entryObject, ok := entry.(map[string]any)
			if !ok {
				keptEntries = append(keptEntries, entry)
				continue
			}
			hookEntries, _ := entryObject["hooks"].([]any)
			keptHooks := hookEntries[:0]
			for _, hook := range hookEntries {
				hookObject, ok := hook.(map[string]any)
				command, _ := hookObject["command"].(string)
				if ok && strings.Contains(command, commandPrefix) {
					updated = true
					continue
				}
				keptHooks = append(keptHooks, hook)
			}
			if len(hookEntries) > 0 {
				entryObject["hooks"] = keptHooks
			}
			if len(keptHooks) > 0 || len(hookEntries) == 0 {
				keptEntries = append(keptEntries, entryObject)
			} else {
				updated = true
			}
		}
		if len(keptEntries) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keptEntries
		}
	}
	return updated
}

func removeDirectCommands(hooks map[string]any, commandPrefix string) bool {
	updated := false
	for event, raw := range hooks {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := entries[:0]
		for _, entry := range entries {
			entryObject, ok := entry.(map[string]any)
			if !ok {
				kept = append(kept, entry)
				continue
			}
			command, _ := entryObject["bash"].(string)
			if command == "" {
				command, _ = entryObject["command"].(string)
			}
			if strings.Contains(command, commandPrefix) {
				updated = true
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	return updated
}

func removeSimpleCommands(hooks map[string]any, commandPrefix string) bool {
	updated := false
	for event, raw := range hooks {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := entries[:0]
		for _, entry := range entries {
			entryObject, ok := entry.(map[string]any)
			command, _ := entryObject["command"].(string)
			if ok && strings.Contains(command, commandPrefix) {
				updated = true
				continue
			}
			kept = append(kept, entry)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	return updated
}

func removeDirectCommandsForEvent(hooks map[string]any, event string) bool {
	raw, ok := hooks[event]
	if !ok {
		return false
	}
	entries, ok := raw.([]any)
	if !ok {
		return false
	}
	kept := entries[:0]
	updated := false
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			kept = append(kept, entry)
			continue
		}
		command, _ := entryObject["bash"].(string)
		if command == "" {
			command, _ = entryObject["command"].(string)
		}
		if strings.Contains(command, shellHookName) {
			updated = true
			continue
		}
		kept = append(kept, entry)
	}
	if !updated {
		return false
	}
	if len(kept) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = kept
	}
	return true
}

func removeSimpleCommandsForAction(
	hooks map[string]any,
	event, action string,
) bool {
	raw, ok := hooks[event]
	if !ok {
		return false
	}
	entries, ok := raw.([]any)
	if !ok {
		return false
	}
	kept := entries[:0]
	updated := false
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			kept = append(kept, entry)
			continue
		}
		command, _ := entryObject["command"].(string)
		if strings.Contains(command, shellHookName) &&
			strings.Contains(command, " "+action) {
			updated = true
			continue
		}
		kept = append(kept, entry)
	}
	if !updated {
		return false
	}
	if len(kept) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = kept
	}
	return true
}
