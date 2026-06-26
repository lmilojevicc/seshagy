package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// hooksFor returns the matcher-groups for an event in a parsed config.
func hooksFor(t *testing.T, root map[string]any, event string) []any {
	t.Helper()
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("no hooks key in config")
	}
	groups, ok := hooks[event].([]any)
	if !ok {
		return nil
	}
	return groups
}

// firstSeshagyGroup finds the first matcher-group whose command contains the
// seshagy script marker.
func firstSeshagyGroup(groups []any) map[string]any {
	for _, g := range groups {
		mg, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if groupHasSeshagy(mg) {
			return mg
		}
	}
	return nil
}

func groupCommand(mg map[string]any) string {
	hooks, _ := mg["hooks"].([]any)
	if len(hooks) == 0 {
		return ""
	}
	entry, _ := hooks[0].(map[string]any)
	cmd, _ := entry["command"].(string)
	return cmd
}

// withHome sets HOME and CODEX_HOME/CLAUDE_CONFIG_DIR under a temp dir so each
// agent's config path resolves into an isolated sandbox.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestInstallCodexMergesHooksPreservingExisting(t *testing.T) {
	home := withHome(t)
	configPath := filepath.Join(home, ".codex", "hooks.json")
	// Pre-existing herdr-style entry under PreToolUse must be preserved.
	writeJSON(t, configPath, `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "herdr --something"}]}
    ]
  }
}`)

	path, err := Install("codex")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if path != configPath {
		t.Fatalf("config path = %s, want %s", path, configPath)
	}

	scriptPath := filepath.Join(home, ".codex", "hooks", "seshagy-agent-state.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("script not installed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Fatalf("script perm = %o, want 0o755", perm)
	}

	root := readJSONMap(t, configPath)
	for _, event := range []string{"UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop"} {
		groups := hooksFor(t, root, event)
		if firstSeshagyGroup(groups) == nil {
			t.Errorf("event %s: missing seshagy entry", event)
		}
	}

	// Pre-existing herdr entry preserved.
	preGroups := hooksFor(t, root, "PreToolUse")
	foundHerdr := false
	for _, g := range preGroups {
		mg, _ := g.(map[string]any)
		if mg != nil && strings.Contains(groupCommand(mg), "herdr") {
			foundHerdr = true
		}
	}
	if !foundHerdr {
		t.Errorf("herdr entry not preserved in PreToolUse")
	}

	// PermissionRequest command must defensively decline (exit 1).
	permGroups := hooksFor(t, root, "PermissionRequest")
	mg := firstSeshagyGroup(permGroups)
	if mg == nil {
		t.Fatalf("PermissionRequest: no seshagy group")
	}
	if cmd := groupCommand(mg); !strings.Contains(cmd, "exit 1") {
		t.Errorf("PermissionRequest command = %q, want exit 1", cmd)
	}

	// Non-PermissionRequest commands must NOT contain exit 1.
	stopGroups := hooksFor(t, root, "Stop")
	stopMG := firstSeshagyGroup(stopGroups)
	if stopMG != nil && strings.Contains(groupCommand(stopMG), "exit 1") {
		t.Errorf("Stop command should not exit 1")
	}
}

func TestInstallClaudeIdempotent(t *testing.T) {
	home := withHome(t)
	configPath := filepath.Join(home, ".claude", "settings.json")

	for i := 0; i < 2; i++ {
		if _, err := Install("claude"); err != nil {
			t.Fatalf("Install pass %d: %v", i, err)
		}
	}

	root := readJSONMap(t, configPath)
	for _, event := range []string{"UserPromptSubmit", "PreToolUse", "Stop", "SessionEnd"} {
		groups := hooksFor(t, root, event)
		count := 0
		for _, g := range groups {
			mg, _ := g.(map[string]any)
			if mg != nil && groupHasSeshagy(mg) {
				count++
			}
		}
		if count != 1 {
			t.Errorf("event %s: %d seshagy groups, want 1", event, count)
		}
	}

	// Notification must have two seshagy matcher-groups (permission_prompt +
	// elicitation_dialog).
	notifGroups := hooksFor(t, root, "Notification")
	count := 0
	for _, g := range notifGroups {
		mg, _ := g.(map[string]any)
		if mg != nil && groupHasSeshagy(mg) {
			count++
		}
	}
	if count != 2 {
		t.Errorf("Notification: %d seshagy groups, want 2", count)
	}
}

func TestInstallDroidIncludesPermissionRequest(t *testing.T) {
	home := withHome(t)
	configPath := filepath.Join(home, ".factory", "settings.json")

	if _, err := Install("droid"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	root := readJSONMap(t, configPath)

	for _, event := range []string{
		"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"PreCompact", "Notification", "PermissionRequest", "PermissionDenied",
		"Stop", "SessionEnd",
	} {
		groups := hooksFor(t, root, event)
		if firstSeshagyGroup(groups) == nil {
			t.Errorf("event %s: missing seshagy entry", event)
		}
	}

	// PermissionRequest → blocked, PermissionDenied → idle, Stop → done.
	if cmd := groupCommand(
		firstSeshagyGroup(hooksFor(t, root, "PermissionRequest")),
	); !strings.Contains(
		cmd,
		"blocked",
	) {
		t.Errorf("PermissionRequest state = %q, want blocked", cmd)
	}
	if cmd := groupCommand(
		firstSeshagyGroup(hooksFor(t, root, "PermissionDenied")),
	); !strings.Contains(
		cmd,
		"idle",
	) {
		t.Errorf("PermissionDenied state = %q, want idle", cmd)
	}
	if cmd := groupCommand(
		firstSeshagyGroup(hooksFor(t, root, "Stop")),
	); !strings.Contains(
		cmd,
		"done",
	) {
		t.Errorf("Stop state = %q, want done", cmd)
	}

	// SessionStart has matcher "*".
	sessionGroups := hooksFor(t, root, "SessionStart")
	mg := firstSeshagyGroup(sessionGroups)
	if mg == nil {
		t.Fatalf("SessionStart: no seshagy group")
	}
	if m, _ := mg["matcher"].(string); m != "*" {
		t.Errorf("SessionStart matcher = %q, want *", m)
	}
}

func TestUninstallRemovesSeshagyEntriesOnly(t *testing.T) {
	home := withHome(t)
	configPath := filepath.Join(home, ".codex", "hooks.json")

	if _, err := Install("codex"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Add a non-seshagy entry manually after install.
	root := readJSONMap(t, configPath)
	hooks := root["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	stop = append(stop, map[string]any{
		"matcher": "",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": "notify-send done",
		}},
	})
	hooks["Stop"] = stop
	root["hooks"] = hooks
	data, _ := json.MarshalIndent(root, "", "  ")
	_ = os.WriteFile(configPath, data, 0o644)

	if _, err := Uninstall("codex"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	root = readJSONMap(t, configPath)
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		// hooks entirely gone is acceptable only if no non-seshagy remained;
		// we added one, so hooks must persist.
		t.Fatalf("hooks key removed but a non-seshagy entry existed")
	}
	for event, existing := range hooks {
		for _, g := range existing.([]any) {
			mg, _ := g.(map[string]any)
			if mg != nil && groupHasSeshagy(mg) {
				t.Errorf("event %s: seshagy entry still present after uninstall", event)
			}
		}
	}
	// Non-seshagy entry preserved.
	stopGroups := hooks["Stop"].([]any)
	foundOther := false
	for _, g := range stopGroups {
		mg, _ := g.(map[string]any)
		if mg != nil && strings.Contains(groupCommand(mg), "notify-send") {
			foundOther = true
		}
	}
	if !foundOther {
		t.Errorf("non-seshagy entry removed by uninstall")
	}
}

func TestShellHookSpecConfigPathEnvOverride(t *testing.T) {
	cases := []struct {
		name string
		env  string
		val  string
		spec *shellHookSpec
		file string
	}{
		{"codex", "CODEX_HOME", filepath.Join(t.TempDir(), "codex"), codexSpec, "hooks.json"},
		{
			"claude",
			"CLAUDE_CONFIG_DIR",
			filepath.Join(t.TempDir(), "claude"),
			claudeSpec,
			"settings.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.env, tc.val)
			t.Setenv("HOME", t.TempDir())
			cp, err := tc.spec.configPath()
			if err != nil {
				t.Fatalf("configPath: %v", err)
			}
			if want := filepath.Join(tc.val, tc.file); cp != want {
				t.Errorf("configPath = %s, want %s", cp, want)
			}
			sp, err := tc.spec.scriptPath()
			if err != nil {
				t.Fatalf("scriptPath: %v", err)
			}
			if want := filepath.Join(tc.val, "hooks", "seshagy-agent-state.sh"); sp != want {
				t.Errorf("scriptPath = %s, want %s", sp, want)
			}
		})
	}
}

func TestSharedHookScriptContent(t *testing.T) {
	s := sharedHookScript
	for _, want := range []string{
		`[ -n "${TMUX_PANE:-}" ] || exit 0`,
		`--pane "$TMUX_PANE"`,
		`--report-agent`,
		`--release-agent`,
		// herdr owns agent state; hook is a no-op under herdr.
		`[ "${HERDR_ENV:-}" = "1" ] && exit 0`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("shared script missing %q", want)
		}
	}
	// A state-reporting hook must NEVER fail the host agent.
	if strings.Contains(s, "set -eu") || strings.Contains(s, "set -e\n") {
		t.Errorf("shared script must not use set -e/-eu (would fail host on any error)")
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "exit 0") {
		t.Errorf("shared script must end with exit 0")
	}
	if strings.Contains(s, "/Users/milo") {
		t.Errorf("shared script contains hardcoded /Users/milo path")
	}
}

func TestAvailableIncludesOpenCode(t *testing.T) {
	for _, name := range Available() {
		if name == "opencode" {
			return
		}
	}
	t.Errorf("Available() does not include opencode")
}

func TestSeshagyOpenCodePluginContent(t *testing.T) {
	s := opencodePluginSource
	for _, want := range []string{
		"--report-agent",
		"--cwd",
		"opencode",
		"permission.ask",
		"session.idle",
		"seshagy:opencode",
		// permissionPending state machine guards (blocker #1 fix).
		"permissionPending",
		"permission.replied",
		// Event-bus mapping (mirrors herdr: question/elicitation + session.status).
		"question.asked",
		"permission.asked",
		"session.status",
		"question.replied",
		"question.rejected",
		// BigInt microseconds seq (fix #7).
		"BigInt",
		"1000n",
		// herdr owns agent state; plugin is a no-op under herdr.
		`HERDR_ENV === "1"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("opencode plugin missing %q", want)
		}
	}
	if strings.Contains(s, "/Users/milo") {
		t.Errorf("opencode plugin contains hardcoded /Users/milo path")
	}
}

func TestSeshagyPiExtensionContent(t *testing.T) {
	s := piExtensionSource
	for _, want := range []string{
		"--report-agent",
		"--release-agent",
		"seshagy:pi",
		"session_start",
		"session_shutdown",
		// herdr owns agent state; extension is a no-op under herdr.
		`HERDR_ENV === "1"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("pi extension missing %q", want)
		}
	}
	if strings.Contains(s, "/Users/milo") {
		t.Errorf("pi extension contains hardcoded /Users/milo path")
	}
}

func TestInstallOpenCodeWritesPlugin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	path, err := Install("opencode")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("plugin file not written at %s: %v", path, err)
	}

	// Uninstall removes the file.
	if _, err := Uninstall("opencode"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("plugin file still exists after uninstall (err=%v)", err)
	}
}

func TestInstallOpenCodeHonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", xdg)

	path, err := Install("opencode")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if want := filepath.Join(
		xdg,
		"opencode",
		"plugin",
		"seshagy-opencode-plugin.ts",
	); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
}

func TestLifecycleAuthorityFor(t *testing.T) {
	tests := []struct {
		agent string
		want  bool
	}{
		{"pi", true},
		{"opencode", true},
		{"codex", false},
		{"claude", false},
		{"droid", false},
		{"cursor", false},      // unregistered
		{"antigravity", false}, // unregistered
		{"grok", false},        // unregistered
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			if got := LifecycleAuthorityFor(tt.agent); got != tt.want {
				t.Fatalf("LifecycleAuthorityFor(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}
