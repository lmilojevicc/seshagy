// Package integrations manages per-agent hook/extension installers.
package integrations

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/seshagy-agent-state.ts
var piExtensionSource string

//go:embed assets/seshagy-agent-state.sh
var sharedHookScript string

//go:embed assets/seshagy-opencode-plugin.ts
var opencodePluginSource string

// scriptMarker identifies seshagy-managed entries inside an agent's hooks
// config so install (refresh) and uninstall can find them deterministically.
const scriptMarker = "seshagy-agent-state.sh"

// Integration describes a single agent hook installer.
type Integration struct {
	Name         string
	InstallPath  func() (string, error)
	AssetContent string
	// ShellHook describes a shell-hook integration (codex/claude/droid). Nil
	// for asset-only integrations (pi).
	ShellHook *shellHookSpec
	// LifecycleAuthority is true when the integration's hooks/plugins emit
	// the FULL lifecycle (idle, working, blocked, done). For such agents the
	// detection engine suppresses screen-manifest fallback when hooks are
	// fresh. Agents with partial hooks (codex/claude/droid) or no hooks
	// (cursor/agy/grok) are false — screen manifest always runs so it can
	// overwrite stale hook state (herdr authority model).
	LifecycleAuthority bool
}

// hookEvent maps one agent hook event to a seshagy state.
type hookEvent struct {
	event   string // e.g. "PreToolUse", "Notification"
	matcher string // "" = omit matcher field; "permission_prompt", "*" etc.
	state   string // working|blocked|done|idle|release|session
}

// shellHookSpec describes a shell-hook integration (codex/claude/droid).
type shellHookSpec struct {
	agentName  string
	configPath func() (string, error) // target settings/hooks JSON path
	scriptPath func() (string, error) // where to install the .sh script
	events     []hookEvent
}

func codexBase() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func claudeBase() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func factoryBase() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".factory")
}

var codexSpec = &shellHookSpec{
	agentName: "codex",
	configPath: func() (string, error) {
		return filepath.Join(codexBase(), "hooks.json"), nil
	},
	scriptPath: func() (string, error) {
		return filepath.Join(codexBase(), "hooks", "seshagy-agent-state.sh"), nil
	},
	events: []hookEvent{
		{event: "UserPromptSubmit", state: "working"},
		{event: "PreToolUse", state: "working"},
		{event: "PermissionRequest", state: "blocked"},
		{event: "Stop", state: "done"},
	},
}

var claudeSpec = &shellHookSpec{
	agentName: "claude",
	configPath: func() (string, error) {
		return filepath.Join(claudeBase(), "settings.json"), nil
	},
	scriptPath: func() (string, error) {
		return filepath.Join(claudeBase(), "hooks", "seshagy-agent-state.sh"), nil
	},
	events: []hookEvent{
		{event: "UserPromptSubmit", state: "working"},
		{event: "PreToolUse", state: "working"},
		{event: "Notification", matcher: "permission_prompt", state: "blocked"},
		{event: "Notification", matcher: "elicitation_dialog", state: "blocked"},
		{event: "Stop", state: "done"},
		{event: "SessionEnd", state: "release"},
	},
}

var droidSpec = &shellHookSpec{
	agentName: "droid",
	configPath: func() (string, error) {
		return filepath.Join(factoryBase(), "settings.json"), nil
	},
	scriptPath: func() (string, error) {
		return filepath.Join(factoryBase(), "hooks", "seshagy-agent-state.sh"), nil
	},
	events: []hookEvent{
		{event: "SessionStart", matcher: "*", state: "session"},
		{event: "UserPromptSubmit", state: "working"},
		{event: "PreToolUse", state: "working"},
		{event: "PostToolUse", state: "working"},
		{event: "PreCompact", state: "working"},
		{event: "Notification", state: "blocked"},
		{event: "PermissionRequest", state: "blocked"},
		{event: "PermissionDenied", state: "idle"},
		{event: "Stop", state: "done"},
		{event: "SessionEnd", state: "release"},
	},
}

var piIntegration = Integration{
	Name:               "pi",
	LifecycleAuthority: true,
	InstallPath: func() (string, error) {
		base := os.Getenv("PI_CODING_AGENT_DIR")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".pi", "agent")
		}
		return filepath.Join(base, "extensions", "seshagy-agent-state.ts"), nil
	},
	AssetContent: piExtensionSource,
}

// opencodeConfigBase resolves the opencode config directory, honoring
// $XDG_CONFIG_HOME (opencode follows the XDG convention).
func opencodeConfigBase() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "opencode")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opencode")
}

var opencodeIntegration = Integration{
	Name:               "opencode",
	LifecycleAuthority: true,
	InstallPath: func() (string, error) {
		// opencode auto-discovers {plugin,plugins}/*.{ts,js} relative to its
		// config dir; dropping the .ts here is enough — no opencode.json patch.
		return filepath.Join(opencodeConfigBase(), "plugin", "seshagy-opencode-plugin.ts"), nil
	},
	AssetContent: opencodePluginSource,
}

var integrations = map[string]Integration{
	"pi":       piIntegration,
	"codex":    {Name: "codex", ShellHook: codexSpec},   // partial hooks
	"claude":   {Name: "claude", ShellHook: claudeSpec}, // partial hooks
	"droid":    {Name: "droid", ShellHook: droidSpec},   // partial hooks
	"opencode": opencodeIntegration,
}

// LifecycleAuthorityFor returns true when the named agent's integration
// emits the full lifecycle (idle/working/blocked/done). Unregistered agents
// (cursor, antigravity, grok) default to false — their state comes entirely
// from the screen manifest.
func LifecycleAuthorityFor(agentName string) bool {
	integ, ok := integrations[agentName]
	if !ok {
		return false
	}
	return integ.LifecycleAuthority
}

// Install writes the integration's hook/extension file to the agent's
// configuration directory.
func Install(name string) (string, error) {
	integ, ok := integrations[name]
	if !ok {
		return "", fmt.Errorf("unknown integration: %s", name)
	}
	if integ.ShellHook != nil {
		path, err := installShellHook(integ.ShellHook)
		if err != nil {
			return "", err
		}
		if name == "codex" {
			warnCodexFeatureFlag()
		}
		return path, nil
	}
	path, err := integ.InstallPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(integ.AssetContent), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Uninstall removes seshagy-managed hook entries from an agent's hooks config
// and deletes the installed shell script.
func Uninstall(name string) (string, error) {
	integ, ok := integrations[name]
	if !ok {
		return "", fmt.Errorf("unknown integration: %s", name)
	}
	if integ.ShellHook == nil {
		// Asset-only integration (pi, opencode): remove the installed file.
		path, err := integ.InstallPath()
		if err != nil {
			return "", err
		}
		_ = os.Remove(path)
		return path, nil
	}
	spec := integ.ShellHook
	configPath, err := spec.configPath()
	if err != nil {
		return "", err
	}
	if err := removeSeshagyHooks(configPath); err != nil {
		return "", err
	}
	scriptPath, err := spec.scriptPath()
	if err == nil {
		_ = os.Remove(scriptPath)
	}
	return configPath, nil
}

// installShellHook writes the shared script and merges the event hooks into the
// agent's config JSON, preserving existing non-seshagy entries.
func installShellHook(spec *shellHookSpec) (string, error) {
	scriptPath, err := spec.scriptPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(scriptPath, []byte(sharedHookScript), 0o755); err != nil {
		return "", err
	}
	configPath, err := spec.configPath()
	if err != nil {
		return "", err
	}
	if err := mergeShellHooks(configPath, scriptPath, spec); err != nil {
		return "", err
	}
	return configPath, nil
}

// commandFor builds the hook command string for one event. Codex
// PermissionRequest appends `; exit 1` defensively: codex treats exit 0 as
// auto-approve, so the hook must decline-to-decide to let the normal approval
// UI show. All other events exit 0 via the script's own default.
func commandFor(scriptPath, agent, state, event string) string {
	cmd := fmt.Sprintf("bash '%s' %s %s", scriptPath, agent, state)
	if agent == "codex" && event == "PermissionRequest" {
		cmd += "; exit 1"
	}
	return cmd
}

// matcherGroup builds the JSON-decodable matcher-group for one event.
type matcherGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// mergeShellHooks writes the seshagy matcher-groups for each event into the
// agent's config JSON, preserving existing non-seshagy entries. Events sharing
// the same event key (e.g. claude's two Notification matchers) are accumulated
// into the same matcher-group list before replacing.
func mergeShellHooks(configPath, scriptPath string, spec *shellHookSpec) error {
	root := readConfigRoot(configPath)
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}
	// Group events by key so multiple matchers on one event are added together.
	type pending struct {
		evs []hookEvent
	}
	byEvent := make(map[string]*pending)
	order := []string{}
	for _, ev := range spec.events {
		p, ok := byEvent[ev.event]
		if !ok {
			p = &pending{}
			byEvent[ev.event] = p
			order = append(order, ev.event)
		}
		p.evs = append(p.evs, ev)
	}
	for _, event := range order {
		p := byEvent[event]
		groups := []map[string]any{}
		for _, ev := range p.evs {
			mg := matcherGroup{
				Hooks: []hookEntry{{
					Type:    "command",
					Command: commandFor(scriptPath, spec.agentName, ev.state, ev.event),
					Timeout: 10,
				}},
			}
			if ev.matcher != "" {
				mg.Matcher = ev.matcher
			}
			encoded, _ := json.Marshal(mg)
			var decoded map[string]any
			_ = json.Unmarshal(encoded, &decoded)
			groups = append(groups, decoded)
		}
		hooks[event] = replaceSeshagyGroupsMulti(hooks[event], groups)
	}
	root["hooks"] = hooks
	return writeConfigRoot(configPath, root)
}

// replaceSeshagyGroupsMulti drops existing seshagy matcher-groups from a list
// (refresh) and appends the new ones, preserving non-seshagy entries.
func replaceSeshagyGroupsMulti(existing any, newGroups []map[string]any) []any {
	var out []any
	if list, ok := existing.([]any); ok {
		for _, item := range list {
			mg, ok := item.(map[string]any)
			if !ok {
				out = append(out, item)
				continue
			}
			if groupHasSeshagy(mg) {
				continue
			}
			out = append(out, item)
		}
	}
	for _, g := range newGroups {
		out = append(out, g)
	}
	return out
}

func groupHasSeshagy(mg map[string]any) bool {
	hooks, ok := mg["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooks {
		entry, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, _ := entry["command"].(string); strings.Contains(cmd, scriptMarker) {
			return true
		}
	}
	return false
}

func removeSeshagyHooks(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	for event, existing := range hooks {
		list, ok := existing.([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, item := range list {
			mg, ok := item.(map[string]any)
			if !ok {
				kept = append(kept, item)
				continue
			}
			if groupHasSeshagy(mg) {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		root["hooks"] = hooks
	}
	return writeConfigRoot(configPath, root)
}

func readConfigRoot(configPath string) map[string]any {
	root := make(map[string]any)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return root
	}
	_ = json.Unmarshal(data, &root)
	return root
}

func writeConfigRoot(configPath string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath + ".seshagy-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

// warnCodexFeatureFlag prints a stderr warning if codex's [features] hooks flag
// is not enabled in $CODEX_HOME/config.toml. It does not modify the file.
func warnCodexFeatureFlag() {
	configPath := filepath.Join(codexBase(), "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"seshagy: codex hooks require [features] hooks = true in %s; enable it and restart codex\n",
			configPath,
		)
		return
	}
	if strings.Contains(string(data), "hooks = true") {
		return
	}
	fmt.Fprintf(os.Stderr,
		"seshagy: codex hooks require [features] hooks = true in %s; enable it and restart codex\n",
		configPath)
}

// Available returns the list of installable integrations.
func Available() []string {
	return []string{"pi", "codex", "claude", "droid", "opencode"}
}
