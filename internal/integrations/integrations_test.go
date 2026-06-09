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

func TestInstallClaudeWritesLifecycleHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claude, "settings.json"), []byte(`{"permissions":{"allow":["Read"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installClaude("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	settings := readJSON(t, filepath.Join(claude, "settings.json"))
	hooks := settings["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop", "SessionEnd"} {
		if _, ok := hooks[event]; !ok {
			t.Fatalf("missing %s hook in %#v", event, hooks)
		}
	}
	stop := hooks["Stop"].([]any)[0].(map[string]any)
	commands := stop["hooks"].([]any)
	command := commands[0].(map[string]any)["command"].(string)
	if !strings.Contains(command, "seshagy-agent-state.sh") || !strings.Contains(command, " claude done") {
		t.Fatalf("stop hook should report done, got %q", command)
	}
}

func TestInstallCodexEnablesHooksAndPreservesOtherSections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "model = \"gpt\"\n\n[features]\nother = true\n\n[projects]\nfoo = \"bar\"\n"
	if err := os.WriteFile(filepath.Join(codex, "config.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodex("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	updated := readFile(t, filepath.Join(codex, "config.toml"))
	for _, want := range []string{"model = \"gpt\"", "[features]", "hooks = true", "other = true", "[projects]"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("config missing %q:\n%s", want, updated)
		}
	}
	hooks := readJSON(t, filepath.Join(codex, "hooks.json"))["hooks"].(map[string]any)
	if _, ok := hooks["UserPromptSubmit"]; !ok {
		t.Fatalf("missing codex UserPromptSubmit hook: %#v", hooks)
	}
}

func TestUninstallRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installClaude("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claude, "settings.json")
	settings := readJSON(t, settingsPath)
	hooks := settings["hooks"].(map[string]any)
	hooks["Stop"] = append(hooks["Stop"].([]any), map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo keep"}}})
	if err := writeJSONObject(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallClaude(); err != nil {
		t.Fatal(err)
	}
	settings = readJSON(t, settingsPath)
	hooks = settings["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	if len(stop) != 1 {
		t.Fatalf("expected one preserved stop hook, got %#v", stop)
	}
	if _, err := os.Stat(filepath.Join(claude, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
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
