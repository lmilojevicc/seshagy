package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanDetectsAvailableMissingAndInstalledPi(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)
	writeExecutable(t, filepath.Join(binDir, "pi"))
	if err := os.MkdirAll(filepath.Join(home, ".pi", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	before := findRec(t, Scan(), TargetPi)
	if !before.AgentAvailable || !before.Installable || before.State != StatusNotInstalled {
		t.Fatalf("unexpected before status: %#v", before)
	}

	messages, err := installPi("/bin/seshagy")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 {
		t.Fatal("expected install message")
	}
	after := findRec(t, Scan(), TargetPi)
	if after.State != StatusCurrent || after.Version != installVersion {
		t.Fatalf("unexpected after status: %#v", after)
	}
	content := readFile(t, after.InstallPath)
	if !strings.Contains(content, "SESHAGY_INTEGRATION_ID=pi") || !strings.Contains(content, "/bin/seshagy") {
		t.Fatalf("unexpected extension content: %s", content)
	}
}

func TestInstallNestedSessionTargetsWriteSessionHookOnlyAndCleanLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		dirName     string
		target      Target
		install     func(string) ([]string, error)
		settings    string
		wantCommand string
	}{
		{name: "claude", dirName: ".claude", target: TargetClaude, install: installClaude, settings: "settings.json", wantCommand: " claude session"},
		{name: "droid", dirName: ".factory", target: TargetDroid, install: installDroid, settings: "settings.json", wantCommand: " droid session"},
		{name: "qoder", dirName: ".qoder", target: TargetQodercli, install: installQodercli, settings: "settings.json", wantCommand: " qodercli session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, tt.dirName)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			settingsPath := filepath.Join(dir, tt.settings)
			root := map[string]any{"hooks": map[string]any{
				"UserPromptSubmit": []any{map[string]any{"hooks": []any{
					map[string]any{"type": "command", "command": "bash /old/seshagy-agent-state.sh " + string(tt.target) + " working"},
					map[string]any{"type": "command", "command": "echo keep"},
				}}},
				"Stop": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "bash /old/seshagy-agent-state.sh " + string(tt.target) + " done"}}}},
			}}
			if err := writeJSONObject(settingsPath, root); err != nil {
				t.Fatal(err)
			}

			if _, err := tt.install("/bin/seshagy"); err != nil {
				t.Fatal(err)
			}
			hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
			commands, matchers := nestedHookCommands(t, hooks, "SessionStart")
			if len(commands) != 1 || !strings.Contains(commands[0], "seshagy-agent-state.sh") || !strings.Contains(commands[0], tt.wantCommand) {
				t.Fatalf("SessionStart command = %#v, want managed session hook", commands)
			}
			if len(matchers) != 1 || matchers[0] != "*" {
				t.Fatalf("SessionStart matcher = %#v, want *", matchers)
			}
			for _, event := range []string{"Stop", "SessionEnd", "PreToolUse", "PostToolUse", "PermissionRequest", "Notification"} {
				if managedCommandPresent(nestedHookCommandsOnly(t, hooks, event)) {
					t.Fatalf("stale managed hook remains for %s: %#v", event, hooks[event])
				}
			}
			userCommands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
			if len(userCommands) != 1 || userCommands[0] != "echo keep" {
				t.Fatalf("user hook not preserved: %#v", userCommands)
			}
		})
	}
}

func TestInstallCodexWritesSessionHookOnlyAndPreservesNestedCodexHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "model = \"gpt\"\n\n[features]\ncodex_hooks = true\nother = true\n\n[profiles.work.features]\ncodex_hooks = false\n\n[projects]\nfoo = \"bar\"\n"
	if err := os.WriteFile(filepath.Join(codex, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "bash /old/seshagy-agent-state.sh codex working"},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(filepath.Join(codex, "hooks.json"), root); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodex("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	updated := readFile(t, filepath.Join(codex, "config.toml"))
	for _, want := range []string{"model = \"gpt\"", "[features]", "hooks = true", "other = true", "[profiles.work.features]", "codex_hooks = false", "[projects]"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("config missing %q:\n%s", want, updated)
		}
	}
	if strings.Contains(topLevelFeaturesBlock(updated), "codex_hooks") {
		t.Fatalf("top-level codex_hooks should be removed:\n%s", updated)
	}
	hooks := readJSON(t, filepath.Join(codex, "hooks.json"))["hooks"].(map[string]any)
	commands := nestedHookCommandsOnly(t, hooks, "SessionStart")
	if len(commands) != 1 || !strings.Contains(commands[0], "seshagy-agent-state.sh") || !strings.Contains(commands[0], " codex session") {
		t.Fatalf("Codex SessionStart = %#v, want session hook", commands)
	}
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("stale Codex lifecycle hook remains: %#v", hooks["UserPromptSubmit"])
	}
	if got := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit"); len(got) != 1 || got[0] != "echo keep" {
		t.Fatalf("Codex user hook not preserved: %#v", got)
	}
}

func TestInstallCopilotWritesSessionHookOnlyAndCleansLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	copilot := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilot, 0o755); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{
			map[string]any{"type": "command", "bash": "bash /old/seshagy-agent-state.sh copilot working"},
			map[string]any{"type": "command", "bash": "echo keep"},
		},
	}}
	settingsPath := filepath.Join(copilot, "settings.json")
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}

	if _, err := installCopilot("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
	commands := directHookCommands(t, hooks, "SessionStart")
	if len(commands) != 1 || !strings.Contains(commands[0], "seshagy-agent-state.sh") || !strings.Contains(commands[0], " copilot session") {
		t.Fatalf("Copilot SessionStart = %#v, want session hook", commands)
	}
	if managedCommandPresent(directHookCommands(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("stale Copilot lifecycle hook remains: %#v", hooks["UserPromptSubmit"])
	}
	if got := directHookCommands(t, hooks, "UserPromptSubmit"); len(got) != 1 || got[0] != "echo keep" {
		t.Fatalf("Copilot user hook not preserved: %#v", got)
	}
}

func TestInstallCursorWritesSessionHookOnlyAndCleansLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursor := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursor, 0o755); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"hooks": map[string]any{
		"beforeSubmitPrompt": []any{
			map[string]any{"command": "bash /old/seshagy-agent-state.sh cursor working"},
			map[string]any{"command": "echo keep"},
		},
	}}
	hooksPath := filepath.Join(cursor, "hooks.json")
	if err := writeJSONObject(hooksPath, root); err != nil {
		t.Fatal(err)
	}

	if _, err := installCursor("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, hooksPath)["hooks"].(map[string]any)
	commands := simpleHookCommands(t, hooks, "sessionStart")
	if len(commands) != 1 || !strings.Contains(commands[0], "seshagy-agent-state.sh") || !strings.Contains(commands[0], " cursor session") {
		t.Fatalf("Cursor sessionStart = %#v, want session hook", commands)
	}
	if managedCommandPresent(simpleHookCommands(t, hooks, "beforeSubmitPrompt")) {
		t.Fatalf("stale Cursor lifecycle hook remains: %#v", hooks["beforeSubmitPrompt"])
	}
	if got := simpleHookCommands(t, hooks, "beforeSubmitPrompt"); len(got) != 1 || got[0] != "echo keep" {
		t.Fatalf("Cursor user hook not preserved: %#v", got)
	}
	if version := readJSON(t, hooksPath)["version"]; version != float64(1) {
		t.Fatalf("Cursor hooks.json version = %#v, want 1", version)
	}
}

func TestUninstallRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claude, "settings.json")
	root := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "bash /old/seshagy-agent-state.sh claude session"}}}},
		"Stop":         []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "bash /old/seshagy-agent-state.sh claude done"}, map[string]any{"type": "command", "command": "echo keep"}}}},
	}}
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(claude, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "hooks", shellHookName), []byte("hook"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallClaude(); err != nil {
		t.Fatal(err)
	}
	settings := readJSON(t, settingsPath)
	hooks := settings["hooks"].(map[string]any)
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SessionStart")) || managedCommandPresent(nestedHookCommandsOnly(t, hooks, "Stop")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	stop := nestedHookCommandsOnly(t, hooks, "Stop")
	if len(stop) != 1 || stop[0] != "echo keep" {
		t.Fatalf("expected preserved stop hook, got %#v", stop)
	}
	if _, err := os.Stat(filepath.Join(claude, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestShellHookSessionActionReportsUnknownWithSessionID(t *testing.T) {
	asset := shellHookAsset(TargetCursor, "/bin/seshagy")
	for _, want := range []string{"session_id", "sessionId", "conversation_id", "conversationId", `state="unknown"`, `--state "$state"`, `--session-id "$session_id"`} {
		if !strings.Contains(asset, want) {
			t.Fatalf("shell hook asset missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, `session|start) state="idle"`) || strings.Contains(asset, `session|start)\n    state="idle"`) {
		t.Fatalf("session action should not be converted to idle:\n%s", asset)
	}
}

func topLevelFeaturesBlock(content string) string {
	lines := strings.Split(content, "\n")
	inFeatures := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inFeatures {
				break
			}
			inFeatures = trimmed == "[features]"
		}
		if inFeatures {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func nestedHookCommandsOnly(t *testing.T, hooks map[string]any, event string) []string {
	t.Helper()
	commands, _ := nestedHookCommands(t, hooks, event)
	return commands
}

func nestedHookCommands(t *testing.T, hooks map[string]any, event string) ([]string, []string) {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		return nil, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s entries = %#v, want array", event, raw)
	}
	var commands []string
	var matchers []string
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if matcher, _ := entryObject["matcher"].(string); matcher != "" {
			matchers = append(matchers, matcher)
		}
		hookEntries, _ := entryObject["hooks"].([]any)
		for _, hook := range hookEntries {
			hookObject, ok := hook.(map[string]any)
			if !ok {
				continue
			}
			if command, _ := hookObject["command"].(string); command != "" {
				commands = append(commands, command)
			}
		}
	}
	return commands, matchers
}

func directHookCommands(t *testing.T, hooks map[string]any, event string) []string {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s entries = %#v, want array", event, raw)
	}
	var commands []string
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		command, _ := entryObject["bash"].(string)
		if command == "" {
			command, _ = entryObject["command"].(string)
		}
		if command != "" {
			commands = append(commands, command)
		}
	}
	return commands
}

func simpleHookCommands(t *testing.T, hooks map[string]any, event string) []string {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s entries = %#v, want array", event, raw)
	}
	var commands []string
	for _, entry := range entries {
		entryObject, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if command, _ := entryObject["command"].(string); command != "" {
			commands = append(commands, command)
		}
	}
	return commands
}

func managedCommandPresent(commands []string) bool {
	for _, command := range commands {
		if strings.Contains(command, shellHookName) {
			return true
		}
	}
	return false
}

func findRec(t *testing.T, recs []Recommendation, target Target) Recommendation {
	t.Helper()
	for _, rec := range recs {
		if rec.Target == target {
			return rec
		}
	}
	t.Fatalf("missing recommendation for %s", target)
	return Recommendation{}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
