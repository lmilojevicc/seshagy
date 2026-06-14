package integrations

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestHookCapableAgent(t *testing.T) {
	for _, target := range Targets() {
		name := AgentNameForTarget(target)
		if !HookCapableAgent(name) {
			t.Fatalf("HookCapableAgent(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"gemini", "agy", "cline", ""} {
		if HookCapableAgent(name) {
			t.Fatalf("HookCapableAgent(%q) = true, want false", name)
		}
	}
}

func TestInstallKimiWritesLifecycleHooksAndPreservesUserConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kimi := filepath.Join(home, ".kimi")
	if err := os.MkdirAll(kimi, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "model = \"kimi\"\n\n[tools]\nenabled = true\n"
	configPath := filepath.Join(kimi, "config.toml")
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installKimi("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	updated := readFile(t, configPath)
	if !strings.Contains(updated, existing) {
		t.Fatalf("user config not preserved:\n%s", updated)
	}
	if !strings.Contains(updated, kimiConfigBlockBegin) ||
		!strings.Contains(updated, kimiConfigBlockEnd) {
		t.Fatalf("managed kimi block missing:\n%s", updated)
	}
	for _, hook := range kimiHookEvents {
		if !strings.Contains(updated, "event = "+strconv.Quote(hook.event)) {
			t.Fatalf("config missing hook event %q:\n%s", hook.event, updated)
		}
		if !strings.Contains(updated, " "+string(TargetKimi)+" "+hook.action) {
			t.Fatalf("config missing hook action %q:\n%s", hook.action, updated)
		}
	}
	hookContent := readFile(t, filepath.Join(kimi, "hooks", shellHookName))
	if !strings.Contains(hookContent, "SESHAGY_INTEGRATION_ID=kimi") {
		t.Fatalf("kimi hook missing integration marker:\n%s", hookContent)
	}
}

func TestUninstallKimiRemovesManagedBlockOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	kimi := filepath.Join(home, ".kimi")
	if err := os.MkdirAll(kimi, 0o755); err != nil {
		t.Fatal(err)
	}
	userConfig := "model = \"kimi\"\n"
	configPath := filepath.Join(kimi, "config.toml")
	if err := os.WriteFile(configPath, []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installKimi("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallKimi(); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, configPath)
	if strings.Contains(after, kimiConfigBlockBegin) {
		t.Fatalf("managed block should be removed:\n%s", after)
	}
	if !strings.Contains(after, userConfig) {
		t.Fatalf("user config should remain:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(kimi, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestAuthorityMapping(t *testing.T) {
	tests := []struct {
		target Target
		want   AuthorityKind
	}{
		{TargetPi, LifecycleAuthority},
		{TargetOpencode, LifecycleAuthority},
		{TargetKimi, LifecycleAuthority},
		{TargetClaude, LifecycleAuthority},
		{TargetCodex, LifecycleAuthority},
		{TargetCopilot, LifecycleAuthority},
		{TargetDroid, LifecycleAuthority},
		{TargetQodercli, LifecycleAuthority},
		{TargetCursor, LifecycleAuthority},
		{TargetGrok, LifecycleAuthority},
		{TargetKilo, LifecycleAuthority},
		{TargetHermes, LifecycleAuthority},
	}
	for _, tt := range tests {
		if got := Authority(tt.target); got != tt.want {
			t.Fatalf("Authority(%s) = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestScanIncludesAuthority(t *testing.T) {
	for _, rec := range Scan() {
		if rec.Authority == "" {
			t.Fatalf("Scan() recommendation for %s missing Authority", rec.Target)
		}
		if got := Authority(rec.Target); got != rec.Authority {
			t.Fatalf(
				"Scan() recommendation for %s has Authority=%q, want %q",
				rec.Target,
				rec.Authority,
				got,
			)
		}
	}
}

func TestInstalledV1ManagedHookIsOutdated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seshagy-agent-state.sh")
	data := []byte("# SESHAGY_INTEGRATION_ID=pi\n# SESHAGY_INTEGRATION_VERSION=1\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	state, version := installedState(path, TargetPi)
	if state != StatusOutdated || version != 1 {
		t.Fatalf("installedState(v1) = %s/%d, want outdated/1", state, version)
	}
}

func TestInstalledV2ManagedHookIsOutdated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seshagy-agent-state.sh")
	data := []byte("# SESHAGY_INTEGRATION_ID=claude\n# SESHAGY_INTEGRATION_VERSION=2\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	state, version := installedState(path, TargetClaude)
	if state != StatusOutdated || version != 2 {
		t.Fatalf("installedState(v2) = %s/%d, want outdated/2", state, version)
	}
}

func TestScanCursorRequiresCursorAgentCommand(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "cursor"))
	if rec := findRec(t, Scan(), TargetCursor); rec.AgentAvailable || !rec.Installable {
		t.Fatalf(
			"cursor editor command plus config dir should be installable but not available: %#v",
			rec,
		)
	}

	writeExecutable(t, filepath.Join(binDir, "cursor-agent"))
	if rec := findRec(t, Scan(), TargetCursor); !rec.AgentAvailable || !rec.Installable {
		t.Fatalf(
			"cursor-agent command plus config dir should make Cursor Agent available/installable: %#v",
			rec,
		)
	}
}

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
	if !strings.Contains(content, "SESHAGY_INTEGRATION_ID=pi") ||
		!strings.Contains(content, "/bin/seshagy") {
		t.Fatalf("unexpected extension content: %s", content)
	}
}

func TestInstallNestedLifecycleTargetsWriteLifecycleHooksAndCleanStale(t *testing.T) {
	tests := []struct {
		name     string
		dirName  string
		target   Target
		install  func(string) ([]string, error)
		settings string
		events   []lifecycleHook
	}{
		{
			name:     "claude",
			dirName:  ".claude",
			target:   TargetClaude,
			install:  installClaude,
			settings: "settings.json",
			events:   claudeLifecycleHooks,
		},
		{
			name:     "droid",
			dirName:  ".factory",
			target:   TargetDroid,
			install:  installDroid,
			settings: "settings.json",
			events:   droidLifecycleHooks,
		},
		{
			name:     "qoder",
			dirName:  ".qoder",
			target:   TargetQodercli,
			install:  installQodercli,
			settings: "settings.json",
			events:   qodercliLifecycleHooks,
		},
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
					map[string]any{
						"type": "command",
						"command": "bash /old/seshagy-agent-state.sh " + string(
							tt.target,
						) + " working",
					},
					map[string]any{"type": "command", "command": "echo keep"},
				}}},
				"Stop": []any{
					map[string]any{
						"hooks": []any{
							map[string]any{
								"type": "command",
								"command": "bash /old/seshagy-agent-state.sh " + string(
									tt.target,
								) + " done",
							},
						},
					},
				},
				"SessionStart": []any{
					map[string]any{
						"hooks": []any{
							map[string]any{
								"type": "command",
								"command": "bash /old/seshagy-agent-state.sh " + string(
									tt.target,
								) + " idle",
							},
						},
					},
				},
			}}
			if err := writeJSONObject(settingsPath, root); err != nil {
				t.Fatal(err)
			}

			if _, err := tt.install("/bin/seshagy"); err != nil {
				t.Fatal(err)
			}
			hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
			for _, hook := range tt.events {
				commands, matchers := nestedHookCommands(t, hooks, hook.event)
				command := managedLifecycleCommand(commands, tt.target, hook.action)
				if command == "" {
					t.Fatalf(
						"%s command = %#v, want managed %s hook",
						hook.event,
						commands,
						hook.action,
					)
				}
				if hook.event == "SessionStart" {
					if len(matchers) != 1 || matchers[0] != "*" {
						t.Fatalf("SessionStart matcher = %#v, want *", matchers)
					}
				}
			}
			userCommands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
			if !slices.Contains(userCommands, "echo keep") {
				t.Fatalf("user hook not preserved: %#v", userCommands)
			}
		})
	}
}

func TestInstallNestedLifecycleTargetsRemoveLegacySubagentStop(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		target  Target
		install func(string) ([]string, error)
	}{
		{name: "droid", dirName: ".factory", target: TargetDroid, install: installDroid},
		{name: "qoder", dirName: ".qoder", target: TargetQodercli, install: installQodercli},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, tt.dirName)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			settingsPath := filepath.Join(dir, "settings.json")
			root := map[string]any{"hooks": map[string]any{
				"SubagentStop": []any{map[string]any{"hooks": []any{
					map[string]any{
						"type": "command",
						"command": "bash /old/seshagy-agent-state.sh " + string(
							tt.target,
						) + " working",
					},
				}}},
			}}
			if err := writeJSONObject(settingsPath, root); err != nil {
				t.Fatal(err)
			}

			if _, err := tt.install("/bin/seshagy"); err != nil {
				t.Fatal(err)
			}
			hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
			if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SubagentStop")) {
				t.Fatalf(
					"legacy SubagentStop hook should be removed: %#v",
					hooks["SubagentStop"],
				)
			}
		})
	}
}

func TestInstallDroidCleansLegacyHooksJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	droid := filepath.Join(home, ".factory")
	if err := os.MkdirAll(droid, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(droid, "hooks.json")
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh droid working",
			},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(legacyPath, root); err != nil {
		t.Fatal(err)
	}

	if _, err := installDroid("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, legacyPath)["hooks"].(map[string]any)
	commands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
	if managedCommandPresent(commands) || len(commands) != 1 || commands[0] != "echo keep" {
		t.Fatalf("legacy hooks.json cleanup = %#v, want only user hook", hooks)
	}
}

func TestUninstallDroidCleansLegacyHooksJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	droid := filepath.Join(home, ".factory")
	if err := os.MkdirAll(filepath.Join(droid, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(droid, "hooks", shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(droid, "settings.json")
	legacyPath := filepath.Join(droid, "hooks.json")
	for _, path := range []string{settingsPath, legacyPath} {
		root := map[string]any{"hooks": map[string]any{
			"Stop": []any{map[string]any{"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": "bash /old/seshagy-agent-state.sh droid done",
				},
				map[string]any{"type": "command", "command": "echo keep"},
			}}},
		}}
		if err := writeJSONObject(path, root); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := uninstallDroid(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{settingsPath, legacyPath} {
		hooks := readJSON(t, path)["hooks"].(map[string]any)
		commands := nestedHookCommandsOnly(t, hooks, "Stop")
		if managedCommandPresent(commands) || len(commands) != 1 || commands[0] != "echo keep" {
			t.Fatalf("%s cleanup = %#v, want only user hook", path, hooks)
		}
	}
}

func TestInstallCodexWritesLifecycleHooksAndPreservesNestedCodexHooks(t *testing.T) {
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
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh codex working",
			},
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
	for _, hook := range codexLifecycleHooks {
		commands := nestedHookCommandsOnly(t, hooks, hook.event)
		if managedLifecycleCommand(commands, TargetCodex, hook.action) == "" {
			t.Fatalf("Codex %s = %#v, want %s hook", hook.event, commands, hook.action)
		}
	}
	if got := nestedHookCommandsOnly(
		t,
		hooks,
		"UserPromptSubmit",
	); !slices.Contains(got, "echo keep") {
		t.Fatalf("Codex user hook not preserved: %#v", got)
	}
}

func TestInstallGrokWritesLifecycleHooksAndPreservesNestedUserHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	grok := filepath.Join(home, ".grok")
	if err := os.MkdirAll(filepath.Join(grok, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh grok working",
			},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	hooksPath := filepath.Join(grok, "hooks", grokHooksRegistryName)
	if err := writeJSONObject(hooksPath, root); err != nil {
		t.Fatal(err)
	}

	if _, err := installGrok("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	hookContent := readFile(t, filepath.Join(grok, "hooks", shellHookName))
	if !strings.Contains(hookContent, "SESHAGY_INTEGRATION_ID=grok") {
		t.Fatalf("grok hook missing integration marker:\n%s", hookContent)
	}
	hooks := readJSON(t, hooksPath)["hooks"].(map[string]any)
	for _, hook := range grokLifecycleHooks {
		commands := nestedHookCommandsOnly(t, hooks, hook.event)
		if managedLifecycleCommand(commands, TargetGrok, hook.action) == "" {
			t.Fatalf("Grok %s = %#v, want %s hook", hook.event, commands, hook.action)
		}
	}
	if got := nestedHookCommandsOnly(
		t,
		hooks,
		"UserPromptSubmit",
	); !slices.Contains(got, "echo keep") {
		t.Fatalf("Grok user hook not preserved: %#v", got)
	}
}

func TestUninstallGrokRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	grok := filepath.Join(home, ".grok")
	if err := os.MkdirAll(filepath.Join(grok, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(grok, "hooks", grokHooksRegistryName)
	root := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "bash /old/seshagy-agent-state.sh grok session",
					},
				},
			},
		},
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh grok working",
			},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(hooksPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(grok, "hooks", shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallGrok(); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, hooksPath)["hooks"].(map[string]any)
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SessionStart")) ||
		managedCommandPresent(nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	userCommands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
	if len(userCommands) != 1 || userCommands[0] != "echo keep" {
		t.Fatalf("expected preserved user hook, got %#v", userCommands)
	}
	if _, err := os.Stat(filepath.Join(grok, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestScanDetectsAvailableGrok(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeExecutable(t, filepath.Join(binDir, "grok-build"))
	rec := findRec(t, Scan(), TargetGrok)
	if !rec.AgentAvailable || !rec.Installable || rec.State != StatusNotInstalled {
		t.Fatalf("unexpected grok status with config dir and grok-build: %#v", rec)
	}
	if rec.Authority != LifecycleAuthority {
		t.Fatalf("grok authority = %q, want lifecycle", rec.Authority)
	}

	messages, err := installGrok("/bin/seshagy")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 {
		t.Fatal("expected install message")
	}
	after := findRec(t, Scan(), TargetGrok)
	if after.State != StatusCurrent || after.Version != installVersion {
		t.Fatalf("unexpected after status: %#v", after)
	}
}

func TestInstallCopilotWritesLifecycleHooksAndCleansStale(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	copilot := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(copilot, 0o755); err != nil {
		t.Fatal(err)
	}
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{
			map[string]any{
				"type": "command",
				"bash": "bash /old/seshagy-agent-state.sh copilot working",
			},
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
	for _, hook := range copilotLifecycleHooks {
		commands := directHookCommands(t, hooks, hook.event)
		if managedLifecycleCommand(commands, TargetCopilot, hook.action) == "" {
			t.Fatalf("Copilot %s = %#v, want %s hook", hook.event, commands, hook.action)
		}
	}
	if got := directHookCommands(
		t,
		hooks,
		"UserPromptSubmit",
	); !slices.Contains(got, "echo keep") {
		t.Fatalf("Copilot user hook not preserved: %#v", got)
	}
}

func TestInstallCursorWritesLifecycleHooksAndCleansStale(t *testing.T) {
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
		"stop": []any{
			map[string]any{"command": "bash /old/seshagy-agent-state.sh cursor session"},
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
	for _, hook := range cursorLifecycleHooks {
		commands := simpleHookCommands(t, hooks, hook.event)
		if managedLifecycleCommand(commands, TargetCursor, hook.action) == "" {
			t.Fatalf("Cursor %s = %#v, want %s hook", hook.event, commands, hook.action)
		}
	}
	if got := simpleHookCommands(
		t,
		hooks,
		"beforeSubmitPrompt",
	); !slices.Contains(got, "echo keep") {
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
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "bash /old/seshagy-agent-state.sh claude session",
					},
				},
			},
		},
		"Stop": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "bash /old/seshagy-agent-state.sh claude done",
					},
					map[string]any{"type": "command", "command": "echo keep"},
				},
			},
		},
	}}
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(claude, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(claude, "hooks", shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallClaude(); err != nil {
		t.Fatal(err)
	}
	settings := readJSON(t, settingsPath)
	hooks := settings["hooks"].(map[string]any)
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SessionStart")) ||
		managedCommandPresent(nestedHookCommandsOnly(t, hooks, "Stop")) {
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

func TestUninstallCodexRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(codex, "hooks.json")
	root := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "bash /old/seshagy-agent-state.sh codex session",
					},
				},
			},
		},
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh codex working",
			},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(hooksPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codex, shellHookName), []byte("hook"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallCodex(); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, hooksPath)["hooks"].(map[string]any)
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SessionStart")) ||
		managedCommandPresent(nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	userCommands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
	if len(userCommands) != 1 || userCommands[0] != "echo keep" {
		t.Fatalf("expected preserved user hook, got %#v", userCommands)
	}
	if _, err := os.Stat(filepath.Join(codex, shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestUninstallCopilotRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	copilot := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(filepath.Join(copilot, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(copilot, "settings.json")
	root := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{
			map[string]any{
				"type": "command",
				"bash": "bash /old/seshagy-agent-state.sh copilot session",
			},
		},
		"UserPromptSubmit": []any{
			map[string]any{
				"type": "command",
				"bash": "bash /old/seshagy-agent-state.sh copilot working",
			},
			map[string]any{"type": "command", "bash": "echo keep"},
		},
	}}
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(copilot, "hooks", shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallCopilot(); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
	if managedCommandPresent(directHookCommands(t, hooks, "SessionStart")) ||
		managedCommandPresent(directHookCommands(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	userCommands := directHookCommands(t, hooks, "UserPromptSubmit")
	if len(userCommands) != 1 || userCommands[0] != "echo keep" {
		t.Fatalf("expected preserved user hook, got %#v", userCommands)
	}
	if _, err := os.Stat(filepath.Join(copilot, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestUninstallPiRemovesExtension(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pi := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(filepath.Join(pi, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	extPath := filepath.Join(pi, "extensions", "seshagy-agent-state.ts")
	if err := os.WriteFile(extPath, []byte("ext"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallPi(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(extPath); !os.IsNotExist(err) {
		t.Fatalf("pi extension should be removed, stat err=%v", err)
	}
}

func TestUninstallOpencodeRemovesPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	opencode := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(filepath.Join(opencode, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installOpencode("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallOpencode(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(
		filepath.Join(opencode, "plugins", "seshagy-agent-state.js"),
	); !os.IsNotExist(
		err,
	) {
		t.Fatalf("opencode plugin should be removed, stat err=%v", err)
	}
}

func TestUninstallQodercliRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	qoder := filepath.Join(home, ".qoder")
	if err := os.MkdirAll(filepath.Join(qoder, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(qoder, "settings.json")
	root := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{
			map[string]any{
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": "bash /old/seshagy-agent-state.sh qodercli session",
					},
				},
			},
		},
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "bash /old/seshagy-agent-state.sh qodercli working",
			},
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(qoder, "hooks", shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallQodercli(); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
	if managedCommandPresent(nestedHookCommandsOnly(t, hooks, "SessionStart")) ||
		managedCommandPresent(nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	userCommands := nestedHookCommandsOnly(t, hooks, "UserPromptSubmit")
	if len(userCommands) != 1 || userCommands[0] != "echo keep" {
		t.Fatalf("expected preserved user hook, got %#v", userCommands)
	}
	if _, err := os.Stat(filepath.Join(qoder, "hooks", shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestUninstallCursorRemovesManagedHookEntriesOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cursor := filepath.Join(home, ".cursor")
	if err := os.MkdirAll(cursor, 0o755); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(cursor, "hooks.json")
	root := map[string]any{"hooks": map[string]any{
		"sessionStart": []any{
			map[string]any{"command": "bash /old/seshagy-agent-state.sh cursor session"},
		},
		"beforeSubmitPrompt": []any{
			map[string]any{"command": "bash /old/seshagy-agent-state.sh cursor working"},
			map[string]any{"command": "echo keep"},
		},
	}}
	if err := writeJSONObject(hooksPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cursor, shellHookName),
		[]byte("hook"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallCursor(); err != nil {
		t.Fatal(err)
	}
	hooks := readJSON(t, hooksPath)["hooks"].(map[string]any)
	if managedCommandPresent(simpleHookCommands(t, hooks, "sessionStart")) ||
		managedCommandPresent(simpleHookCommands(t, hooks, "beforeSubmitPrompt")) {
		t.Fatalf("managed hooks should be removed: %#v", hooks)
	}
	userCommands := simpleHookCommands(t, hooks, "beforeSubmitPrompt")
	if len(userCommands) != 1 || userCommands[0] != "echo keep" {
		t.Fatalf("expected preserved user hook, got %#v", userCommands)
	}
	if _, err := os.Stat(filepath.Join(cursor, shellHookName)); !os.IsNotExist(err) {
		t.Fatalf("hook file should be removed, stat err=%v", err)
	}
}

func TestReinstallClaudeIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claude := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claude, "settings.json")
	root := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{map[string]any{"hooks": []any{
			map[string]any{"type": "command", "command": "echo keep"},
		}}},
	}}
	if err := writeJSONObject(settingsPath, root); err != nil {
		t.Fatal(err)
	}

	if _, err := installClaude("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	firstSettings := readFile(t, settingsPath)
	firstHook := readFile(t, filepath.Join(claude, "hooks", shellHookName))
	if _, err := installClaude("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, settingsPath); got != firstSettings {
		t.Fatalf("reinstall changed settings.json:\nfirst=%s\nsecond=%s", firstSettings, got)
	}
	if got := readFile(t, filepath.Join(claude, "hooks", shellHookName)); got != firstHook {
		t.Fatalf("reinstall changed hook script")
	}
	hooks := readJSON(t, settingsPath)["hooks"].(map[string]any)
	for _, hook := range claudeLifecycleHooks {
		commands := nestedHookCommandsOnly(t, hooks, hook.event)
		managed := 0
		for _, command := range commands {
			if strings.Contains(command, shellHookName) {
				managed++
			}
		}
		if managed != 1 {
			t.Fatalf("%s has %d managed hooks, want 1: %#v", hook.event, managed, commands)
		}
	}
	after := findRec(t, Scan(), TargetClaude)
	if after.State != StatusCurrent || after.Version != installVersion {
		t.Fatalf("unexpected status after reinstall: %#v", after)
	}
}

func TestShellHookSessionActionReportsUnknownWithSessionIDAndSeq(t *testing.T) {
	asset := shellHookAsset(TargetCursor, "/bin/seshagy")
	for _, want := range []string{"next_seq()", "time.time_ns()", "Time::HiRes", `date +%s`, "seshagy-seq", `seq="$(next_seq)"`, "session_id", "sessionId", "conversation_id", "conversationId", `state="unknown"`, `--state "$state"`, `--session-id "$session_id"`, `--seq "$seq"`, `--release-agent --source "$source" --seq "$seq"`, "reject_unsafe_value", "Installed path is preferred", "SESHAGY_BIN is honored only when that path is missing"} {
		if !strings.Contains(asset, want) {
			t.Fatalf("shell hook asset missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, "$$ %") {
		t.Fatalf("shell seq should not use pid modulo ordering:\n%s", asset)
	}
	if strings.Contains(asset, `date +%s%N`) {
		t.Fatalf("shell seq must not use date +%%s%%N (broken on macOS):\n%s", asset)
	}
	if strings.Contains(asset, `session|start) state="idle"`) ||
		strings.Contains(asset, `session|start)\n    state="idle"`) {
		t.Fatalf("session action should not be converted to idle:\n%s", asset)
	}
}

func TestShellHookRejectsUnsafeMessageAndSessionID(t *testing.T) {
	home := t.TempDir()
	binPath := filepath.Join(home, "seshagy")
	writeExecutable(t, binPath)
	hookPath := filepath.Join(home, "hook.sh")
	if err := os.WriteFile(
		hookPath,
		[]byte(shellHookAsset(TargetCursor, binPath)),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "TMUX_PANE=%0")

	messageCmd := exec.Command("sh", hookPath, "cursor", "working", `bad"message`)
	messageCmd.Env = env
	if err := messageCmd.Run(); err == nil {
		t.Fatal("expected hook to reject unsafe message")
	}

	sessionCmd := exec.Command("sh", hookPath, "cursor", "session")
	sessionCmd.Env = env
	sessionCmd.Stdin = strings.NewReader(`{"session_id":"bad$id"}`)
	if err := sessionCmd.Run(); err == nil {
		t.Fatal("expected hook to reject unsafe session_id")
	}
}

func TestPiExtensionUsesHerdrLikeLifecycleStateMachine(t *testing.T) {
	asset := piExtensionAsset("/bin/seshagy")
	for _, want := range []string{
		"SESHAGY_PI_IDLE_DEBOUNCE_MS",
		"SESHAGY_PI_RETRY_GRACE_MS",
		"retryableErrorPattern",
		"let reportSeq = Date.now() * 1000",
		"function nextReportSeq()",
		`"--seq", nextReportSeq()`,
		"let agentActive = false",
		"let retryHoldActive = false",
		"let failureBlocked = false",
		"function desiredState()",
		"function publishState",
		"function scheduleIdle()",
		"function holdForRetry",
		"function retryableErrorMessage",
		"if (!agentActive) return",
		`return { state: "idle"`,
	} {
		if !strings.Contains(asset, want) {
			t.Fatalf("Pi extension missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, `report("done")`) {
		t.Fatalf("Pi extension should not report done on normal lifecycle:\n%s", asset)
	}
}

func TestOpenCodePluginReportsIdleSessionIDAndSeq(t *testing.T) {
	asset := opencodePluginAsset("/bin/seshagy")
	for _, want := range []string{
		"function sessionIDFromProperties",
		"properties?.sessionID",
		"let reportSeq = Date.now() * 1000",
		"function nextReportSeq()",
		"function reportSession(sessionID)",
		`"--seq", nextReportSeq()`,
		`"--session-id", sessionID`,
		`"chat.message": async ({ sessionID, event } = {}) => report("working", sessionID || sessionIDFromProperties(event?.properties))`,
		`if (type === "session.status")`,
		`return status ? report(status, sessionID) : reportSession(sessionID)`,
		`case "session.created":`,
		`case "session.updated":`,
		`return reportSession(sessionID)`,
		`case "session.idle":`,
		`return report("idle", sessionID)`,
		`return run(["--release-agent", "--source", SOURCE, "--seq", nextReportSeq()])`,
	} {
		if !strings.Contains(asset, want) {
			t.Fatalf("OpenCode plugin missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, `report("done")`) {
		t.Fatalf("OpenCode plugin should report idle, not done:\n%s", asset)
	}
	if strings.Contains(asset, `case "session.created":
        case "session.updated":
          return sessionID ? report("idle", sessionID) : undefined;`) {
		t.Fatalf("OpenCode session identity events should not report idle state:\n%s", asset)
	}
	if strings.Contains(asset, `if (status) return report(status, sessionID);`) {
		t.Fatalf("OpenCode should only use status mapping for session.status events:\n%s", asset)
	}
}

func TestInstallKiloWritesPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	kilo := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(kilo, 0o755); err != nil {
		t.Fatal(err)
	}

	messages, err := installKilo("/bin/seshagy")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("unexpected install messages: %#v", messages)
	}

	pluginPath := filepath.Join(kilo, "plugin", kiloPluginName)
	content := readFile(t, pluginPath)
	if !strings.Contains(content, "SESHAGY_INTEGRATION_ID=kilo") {
		t.Fatalf("kilo plugin missing integration marker:\n%s", content)
	}
}

func TestUninstallKiloRemovesPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	kilo := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(kilo, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installKilo("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallKilo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(kilo, "plugin", kiloPluginName)); !os.IsNotExist(err) {
		t.Fatalf("kilo plugin should be removed, stat err=%v", err)
	}
}

func TestKiloPluginReportsIdleSessionIDAndSeq(t *testing.T) {
	asset := kiloPluginAsset("/bin/seshagy")
	for _, want := range []string{
		"const SOURCE = \"seshagy:kilo\"",
		`"--agent", "kilo"`,
		"function sessionIDFromProperties",
		"let reportSeq = Date.now() * 1000",
		`"--seq", nextReportSeq()`,
		`case "session.idle":`,
		`return report("idle", sessionID)`,
		`return run(["--release-agent", "--source", SOURCE, "--seq", nextReportSeq()])`,
	} {
		if !strings.Contains(asset, want) {
			t.Fatalf("Kilo plugin missing %q:\n%s", want, asset)
		}
	}
	if strings.Contains(asset, `report("done")`) {
		t.Fatalf("Kilo plugin should report idle, not done:\n%s", asset)
	}
}

func TestInstallHermesWritesPluginAndEnablesIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hermes := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(hermes, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(hermes, "config.yaml")
	if err := os.WriteFile(configPath, []byte("model:\n  provider: auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	messages, err := installHermes("/bin/seshagy")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("unexpected install messages: %#v", messages)
	}

	pluginDir := filepath.Join(hermes, "plugins", hermesPluginName)
	initContent := readFile(t, filepath.Join(pluginDir, hermesPluginInitName))
	if !strings.Contains(initContent, "SESHAGY_INTEGRATION_ID=hermes") {
		t.Fatalf("hermes plugin missing integration marker:\n%s", initContent)
	}
	config := readFile(t, configPath)
	if !strings.Contains(config, "plugins:\n  enabled:\n    - seshagy-agent-state") {
		t.Fatalf("hermes config missing enabled plugin:\n%s", config)
	}
}

func TestInstallHermesPreservesFlatPluginList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hermes := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(hermes, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(hermes, "config.yaml")
	existing := "plugins:\n  - platforms/discord\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installHermes("/bin/seshagy"); err != nil {
		t.Fatal(err)
	}
	config := readFile(t, configPath)
	want := "plugins:\n  - seshagy-agent-state\n  - platforms/discord\n"
	if config != want {
		t.Fatalf("config = %q, want %q", config, want)
	}
}

func TestUninstallHermesRemovesPluginAndEnabledEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hermes := filepath.Join(home, ".hermes")
	pluginDir := filepath.Join(hermes, "plugins", hermesPluginName)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(hermes, "config.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("plugins:\n  enabled:\n    - seshagy-agent-state\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(pluginDir, hermesPluginInitName),
		[]byte("x"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := uninstallHermes(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("hermes plugin dir should be removed, stat err=%v", err)
	}
	config := readFile(t, configPath)
	if strings.Contains(config, "seshagy-agent-state") {
		t.Fatalf("hermes config should not mention seshagy plugin:\n%s", config)
	}
}

func TestHermesPluginReportsLifecycleAndSessionID(t *testing.T) {
	asset := hermesPluginInitAsset("/bin/seshagy")
	for _, want := range []string{
		`_SOURCE = "seshagy:hermes"`,
		`"--agent"`,
		`_AGENT`,
		`session_id = _session_id(kwargs)`,
		`args.extend(["--session-id", session_id])`,
		`ctx.register_hook("pre_approval_request", _blocked)`,
		`ctx.register_hook("on_session_finalize", _finalize)`,
		`["--release-agent", "--source", _SOURCE, "--seq", _next_seq()]`,
	} {
		if !strings.Contains(asset, want) {
			t.Fatalf("Hermes plugin missing %q:\n%s", want, asset)
		}
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

func managedLifecycleCommand(commands []string, target Target, action string) string {
	want := " " + string(target) + " " + action
	for _, command := range commands {
		if strings.Contains(command, shellHookName) && strings.Contains(command, want) {
			return command
		}
	}
	return ""
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
