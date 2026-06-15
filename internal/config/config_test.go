package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestLoadDefaultWhenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TypeFirst.Enabled || cfg.PrefixKey() != DefaultPrefix {
		t.Fatalf("type-first defaults = %#v", cfg.TypeFirst)
	}
	if cfg.DefaultSource() != sessionmgr.ModeAll {
		t.Fatalf("default source = %v, want all", cfg.DefaultSource())
	}
	if cfg.LoadOptions().FDCommand != sessionmgr.DefaultFDCommand {
		t.Fatalf("default fd command = %q", cfg.LoadOptions().FDCommand)
	}
	if cfg.Agents.ManifestFallback {
		t.Fatalf("default manifest fallback = %v, want false", cfg.Agents.ManifestFallback)
	}
	if cfg.LoadOptions().ManifestFallback {
		t.Fatalf(
			"default load options manifest fallback = %v, want false",
			cfg.LoadOptions().ManifestFallback,
		)
	}
	if cfg.Theme.Colors.FocusedBorder != "13" || cfg.Theme.Colors.ActiveTab != "default" ||
		cfg.Theme.Colors.Border != "8" ||
		cfg.Theme.Colors.InactiveTab != "8" ||
		cfg.Theme.Colors.Title != "12" ||
		cfg.Theme.Colors.Accent != "13" ||
		cfg.Theme.Colors.Key != "11" ||
		cfg.Theme.Colors.Muted != "8" ||
		cfg.Theme.Colors.Success != "10" ||
		cfg.Theme.Colors.Info != "14" ||
		cfg.Theme.Colors.Warning != "11" ||
		cfg.Theme.Colors.Danger != "9" {
		t.Fatalf("theme color defaults = %#v", cfg.Theme.Colors)
	}
	if got := cfg.Sources.Order; strings.Join(
		got,
		",",
	) != "all,sessions,agents,current-agents,zoxide,fd" {
		t.Fatalf("default source order = %#v", got)
	}
	if cfg.Icons.Mode != IconModeIcons {
		t.Fatalf("icon mode default = %q, want %q", cfg.Icons.Mode, IconModeIcons)
	}
	icons := cfg.IconSet()
	if got := icons.For(sessionmgr.KindSession).Text; got != sessionmgr.IconSession+" " {
		t.Fatalf("session icon = %q", got)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	cfg.Icons.Mode = IconModeText
	cfg.Icons.Session.Label = "X"
	cfg.Icons.Session.Color = "#a6e3a1"
	cfg.Sources.Default = "current-agents"
	cfg.Sources.Order = []string{"sessions", "agents", "current-agents", "zoxide", "fd", "all"}
	cfg.Directories.FDCommand = `printf '%s\n' /tmp/project`
	cfg.Theme.Colors.FocusedBorder = "#ff79c6"
	cfg.Theme.Colors.ActiveTab = "#f5c2e7"
	cfg.Theme.Colors.Border = "#313244"
	cfg.Theme.Colors.InactiveTab = "#6c7086"
	cfg.Theme.Colors.Title = "#b4befe"
	cfg.Theme.Colors.Accent = "#cba6f7"
	cfg.Theme.Colors.Key = "#f9e2af"
	cfg.Theme.Colors.Muted = "#7f849c"
	cfg.Theme.Colors.Success = "#a6e3a1"
	cfg.Theme.Colors.Info = "#89dceb"
	cfg.Theme.Colors.Warning = "#f9e2af"
	cfg.Theme.Colors.Danger = "#f38ba8"
	cfg.TypeFirst.Enabled = true
	cfg.TypeFirst.Prefix = "alt+x"
	cfg.Setup.TypeFirstPromptSeen = true
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if Path() != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "seshagy", "config.toml") {
		t.Fatalf("Path() = %q, want config.toml under XDG_CONFIG_HOME", Path())
	}
	if !strings.Contains(string(data), `[type_first]`) {
		t.Fatalf("saved config missing type_first: %s", data)
	}
	if strings.Contains(string(data), `ascii`) {
		t.Fatalf("saved config should not contain ascii keys: %s", data)
	}
	if !strings.Contains(string(data), `mode = "text"`) ||
		!strings.Contains(string(data), `label = "X"`) ||
		!strings.Contains(string(data), `#a6e3a1`) {
		t.Fatalf("saved config missing text mode, label, or hex color: %s", data)
	}
	if !strings.Contains(string(data), `[sources]`) ||
		!strings.Contains(string(data), `current-agents`) {
		t.Fatalf("saved config missing source config: %s", data)
	}
	if !strings.Contains(string(data), `[directories]`) ||
		!strings.Contains(string(data), `fd_command`) {
		t.Fatalf("saved config missing directory config: %s", data)
	}
	if !strings.Contains(string(data), `[theme.colors]`) ||
		!strings.Contains(string(data), `#ff79c6`) ||
		!strings.Contains(string(data), `#f5c2e7`) ||
		!strings.Contains(string(data), `#313244`) ||
		!strings.Contains(string(data), `#7f849c`) ||
		!strings.Contains(string(data), `#a6e3a1`) ||
		!strings.Contains(string(data), `#f38ba8`) {
		t.Fatalf("saved config missing theme colors: %s", data)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.DefaultSource() != sessionmgr.ModeCurrentAgents {
		t.Fatalf("loaded default source = %v, want current agents", loaded.DefaultSource())
	}
	if order := loaded.SourceOrder(); len(order) != 6 || order[0] != sessionmgr.ModeSessions ||
		order[2] != sessionmgr.ModeCurrentAgents ||
		order[5] != sessionmgr.ModeAll {
		t.Fatalf("loaded source order = %#v", order)
	}
	if loaded.LoadOptions().FDCommand != `printf '%s\n' /tmp/project` {
		t.Fatalf("loaded fd command = %q", loaded.LoadOptions().FDCommand)
	}
	if loaded.Theme.Colors.FocusedBorder != "#ff79c6" ||
		loaded.Theme.Colors.ActiveTab != "#f5c2e7" ||
		loaded.Theme.Colors.Border != "#313244" ||
		loaded.Theme.Colors.InactiveTab != "#6c7086" ||
		loaded.Theme.Colors.Title != "#b4befe" ||
		loaded.Theme.Colors.Accent != "#cba6f7" ||
		loaded.Theme.Colors.Key != "#f9e2af" ||
		loaded.Theme.Colors.Muted != "#7f849c" ||
		loaded.Theme.Colors.Success != "#a6e3a1" ||
		loaded.Theme.Colors.Info != "#89dceb" ||
		loaded.Theme.Colors.Warning != "#f9e2af" ||
		loaded.Theme.Colors.Danger != "#f38ba8" {
		t.Fatalf("loaded theme colors = %#v", loaded.Theme.Colors)
	}
	if !loaded.TypeFirst.Enabled || loaded.PrefixKey() != "alt+x" ||
		!loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("loaded config = %#v", loaded)
	}
	if loaded.Icons.Mode != IconModeText {
		t.Fatalf("loaded icon mode = %q, want text", loaded.Icons.Mode)
	}
	if got := loaded.IconSet().For(sessionmgr.KindSession).Text; got != "X" {
		t.Fatalf("ascii session icon = %q", got)
	}
	if got := loaded.IconSet().For(sessionmgr.KindSession).Color; got != "#a6e3a1" {
		t.Fatalf("hex session color = %q", got)
	}
}

func TestNormalizeSourceOrder(t *testing.T) {
	cfg := Default()
	cfg.Sources.Default = "current_session_agents"
	cfg.Sources.Order = []string{"fd", "agents", "fd", "bad"}
	cfg.Normalize()
	if cfg.Sources.Default != "current-agents" {
		t.Fatalf("normalized default source = %q", cfg.Sources.Default)
	}
	want := []string{"fd", "agents", "all", "sessions", "current-agents", "zoxide"}
	if strings.Join(cfg.Sources.Order, ",") != strings.Join(want, ",") {
		t.Fatalf("normalized source order = %#v, want %#v", cfg.Sources.Order, want)
	}
}

func TestLoadOlderConfigFillsThemeDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[sources]
default = "sessions"

[icons]
mode = "icons"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	defaults := Default().Theme.Colors
	if loaded.Theme.Colors != defaults {
		t.Fatalf("theme defaults from older config = %#v, want %#v", loaded.Theme.Colors, defaults)
	}
}

func TestLoadMigratesPreviousDefaultIcons(t *testing.T) {
	for _, agentIcon := range []string{"󰚩", sessionmgr.IconAgent + " "} {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := Path()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}
		data := []byte(strings.Replace(`
[icons]
mode = "icons"

[icons.session]
icon = ""

[icons.zoxide]
icon = "󰉖"

[icons.fd]
icon = "󰥩"

[icons.agent]
icon = "$AGENT"
`, "$AGENT", agentIcon, 1))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write old icon config: %v", err)
		}
		loaded, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		icons := loaded.IconSet()
		if got := icons.For(sessionmgr.KindSession).Text; got != sessionmgr.IconSession+" " {
			t.Fatalf("session icon = %q, want trailing-space default", got)
		}
		if got := icons.For(sessionmgr.KindAgent).Text; got != sessionmgr.IconAgent+"  " {
			t.Fatalf("agent icon from %q = %q, want new two-space default", agentIcon, got)
		}
	}
}

func TestIconModes(t *testing.T) {
	cfg := Default()
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != sessionmgr.IconAgent+"  " {
		t.Fatalf("default agent icon = %q", got)
	}

	cfg.Icons.Mode = IconModeText
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != "A" {
		t.Fatalf("text-mode agent label = %q", got)
	}

	cfg.Icons.Mode = IconModeNone
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != "" {
		t.Fatalf("no-icons agent text = %q, want empty", got)
	}
}

func TestNormalizeAgentStateMode(t *testing.T) {
	tests := map[string]string{
		"":        AgentStateModeInherit,
		"inherit": AgentStateModeInherit,
		"default": AgentStateModeInherit,
		"icon":    AgentStateModeIcons,
		"icons":   AgentStateModeIcons,
		"glyphs":  AgentStateModeIcons,
		"glyph":   AgentStateModeIcons,
		"text":    AgentStateModeText,
		"label":   AgentStateModeText,
		"labels":  AgentStateModeText,
		"unknown": AgentStateModeInherit,
	}
	for in, want := range tests {
		if got := normalizeAgentStateMode(in); got != want {
			t.Fatalf("normalizeAgentStateMode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeAgentStateMode(" GLYPHS "); got != AgentStateModeIcons {
		t.Fatalf("normalizeAgentStateMode(%q) = %q, want %q", " GLYPHS ", got, AgentStateModeIcons)
	}
}

func TestIconSetAgentStateProjection(t *testing.T) {
	cfg := Default()
	icons := cfg.IconSet()
	if icons.AgentStateMode != AgentStateModeInherit {
		t.Fatalf("default agent_state_mode = %q, want inherit", icons.AgentStateMode)
	}
	if !icons.AgentStateUsesIcons() || icons.AgentStateUsesLabels() {
		t.Fatalf(
			"inherit + icons mode projection = icons:%v labels:%v",
			icons.AgentStateUsesIcons(),
			icons.AgentStateUsesLabels(),
		)
	}

	cfg.Icons.Mode = IconModeText
	icons = cfg.IconSet()
	if icons.AgentStateUsesIcons() || !icons.AgentStateUsesLabels() {
		t.Fatalf(
			"inherit + text mode projection = icons:%v labels:%v",
			icons.AgentStateUsesIcons(),
			icons.AgentStateUsesLabels(),
		)
	}

	cfg.Icons.AgentStateMode = AgentStateModeIcons
	icons = cfg.IconSet()
	if !icons.AgentStateUsesIcons() || icons.AgentStateUsesLabels() {
		t.Fatalf(
			"icons override + text mode projection = icons:%v labels:%v",
			icons.AgentStateUsesIcons(),
			icons.AgentStateUsesLabels(),
		)
	}

	cfg.Icons.Mode = IconModeIcons
	cfg.Icons.AgentStateMode = AgentStateModeText
	icons = cfg.IconSet()
	if icons.AgentStateUsesIcons() || !icons.AgentStateUsesLabels() {
		t.Fatalf(
			"agent_state_mode=text overrides icons mode projection = icons:%v labels:%v",
			icons.AgentStateUsesIcons(),
			icons.AgentStateUsesLabels(),
		)
	}
}

func TestLoadAgentStateModeConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
mode = "icons"
agent_state_mode = "text"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.AgentStateMode != AgentStateModeText {
		t.Fatalf("loaded agent_state_mode = %q, want text", loaded.Icons.AgentStateMode)
	}
}

func TestLoadPerStateAgentStateConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
mode = "icons"

[icons.agent_state.working]
icon = "▶"
label = "working"
color = "10"

[icons.agent_state.blocked]
icon = "◆"
label = "blocked"
color = "11"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.AgentState.Working.Icon != "▶" {
		t.Fatalf("working icon = %q, want ▶", loaded.Icons.AgentState.Working.Icon)
	}
	if loaded.Icons.AgentState.Blocked.Color != "11" {
		t.Fatalf("blocked color = %q, want 11", loaded.Icons.AgentState.Blocked.Color)
	}
	icons := loaded.IconSet()
	if got := icons.ForState(sessionmgr.AgentWorking).Icon; got != "▶" {
		t.Fatalf("projected working icon = %q, want ▶", got)
	}
	if got := icons.ForState(sessionmgr.AgentBlocked).Color; got != "11" {
		t.Fatalf("projected blocked color = %q, want 11", got)
	}
}

func TestNormalizeAgentStatePartialOverride(t *testing.T) {
	cfg := Default()
	cfg.Icons.AgentState.Working.Icon = "★"
	cfg.Icons.AgentState.Working.Label = ""
	cfg.Normalize()
	if cfg.Icons.AgentState.Working.Icon != "★" {
		t.Fatalf("working icon = %q, want ★", cfg.Icons.AgentState.Working.Icon)
	}
	if cfg.Icons.AgentState.Working.Label != "working" {
		t.Fatalf("working label = %q, want working", cfg.Icons.AgentState.Working.Label)
	}
}

func TestLoadMigratesLegacyASCIIConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
enabled = true
ascii = true

[icons.session]
ascii = "X"
color = "#a6e3a1"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.Mode != IconModeText || loaded.Icons.Session.Label != "X" {
		t.Fatalf(
			"legacy ascii config migrated to mode=%q label=%q",
			loaded.Icons.Mode,
			loaded.Icons.Session.Label,
		)
	}
	saved, err := Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(saved), "ascii") ||
		strings.Contains(string(saved), "[icons]\n  enabled") {
		t.Fatalf("migrated config should omit legacy ascii/enabled keys: %s", saved)
	}
}
