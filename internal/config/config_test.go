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
	if cfg.Theme.Colors.PopupBorder != "13" || cfg.Theme.Colors.ActiveTab != "default" ||
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
	) != "all,sessions,zoxide,fd,agents" {
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
	cfg.Sources.Default = "zoxide"
	cfg.Sources.Order = []string{"sessions", "zoxide", "fd", "all"}
	cfg.Directories.FDCommand = `printf '%s\n' /tmp/project`
	cfg.Theme.Colors.PopupBorder = "#ff79c6"
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
		!strings.Contains(string(data), `zoxide`) {
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
	if loaded.DefaultSource() != sessionmgr.ModeZoxide {
		t.Fatalf("loaded default source = %v, want zoxide", loaded.DefaultSource())
	}
	if order := loaded.SourceOrder(); len(order) != 5 || order[0] != sessionmgr.ModeSessions ||
		order[3] != sessionmgr.ModeAll || order[4] != sessionmgr.ModeAgents {
		t.Fatalf("loaded source order = %#v", order)
	}
	if loaded.LoadOptions().FDCommand != `printf '%s\n' /tmp/project` {
		t.Fatalf("loaded fd command = %q", loaded.LoadOptions().FDCommand)
	}
	if loaded.Theme.Colors.PopupBorder != "#ff79c6" ||
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
	cfg.Sources.Default = "zoxide"
	cfg.Sources.Order = []string{"fd", "sessions", "fd", "bad"}
	cfg.Normalize()
	if cfg.Sources.Default != "zoxide" {
		t.Fatalf("normalized default source = %q", cfg.Sources.Default)
	}
	want := []string{"fd", "sessions", "all", "zoxide", "agents"}
	if strings.Join(cfg.Sources.Order, ",") != strings.Join(want, ",") {
		t.Fatalf("normalized source order = %#v, want %#v", cfg.Sources.Order, want)
	}
}

// TestSourceOrderDropsStaleCurrentAgentsTab proves that a config persisted on
// a prior version (where current-agents was a tab) is migrated clean: the
// current-agents entry is dropped from both the persisted order and the
// rendered tab list.
func TestSourceOrderDropsStaleCurrentAgentsTab(t *testing.T) {
	cfg := Default()
	cfg.Sources.Order = []string{"all", "sessions", "current-agents", "agents"}
	cfg.Normalize()

	// normalizeSourceOrder must have dropped current-agents from the persisted order.
	for _, name := range cfg.Sources.Order {
		if name == "current-agents" {
			t.Fatalf("current-agents still in normalized order: %#v", cfg.Sources.Order)
		}
	}

	// SourceOrder must not include ModeCurrentAgents as a tab.
	for _, mode := range cfg.SourceOrder() {
		if mode == sessionmgr.ModeCurrentAgents {
			t.Fatalf("ModeCurrentAgents in SourceOrder: %#v", cfg.SourceOrder())
		}
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
	def := Default()
	def.Normalize()
	defaults := def.Theme.Colors
	if loaded.Theme.Colors != defaults {
		t.Fatalf("theme defaults from older config = %#v, want %#v", loaded.Theme.Colors, defaults)
	}
}

func TestLoadOlderConfigFillsUnknownAgentStateDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
mode = "icons"

[icons.agent_state.idle]
icon = "○"
label = "idle"
color = "8"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.Icons.AgentState.Unknown; got != (IconConfig{Icon: "?", Label: "unknown", Color: "8"}) {
		t.Fatalf("loaded unknown agent state = %#v, want default", got)
	}
	style := loaded.IconSet().ForAgentState(sessionmgr.AgentUnknown)
	if style.Icon != "?" || style.ASCII != "unknown" || style.Color != "8" {
		t.Fatalf("projected unknown state = %#v, want icon ?, label unknown, color 8", style)
	}
}

func TestNormalizeStateDisplayMode(t *testing.T) {
	tests := map[string]string{
		"":         StateDisplayModeInherit,
		"inherit":  StateDisplayModeInherit,
		"default":  StateDisplayModeInherit,
		"icon":     StateDisplayModeIcons,
		"icons":    StateDisplayModeIcons,
		"glyphs":   StateDisplayModeIcons,
		"glyph":    StateDisplayModeIcons,
		"text":     StateDisplayModeText,
		"label":    StateDisplayModeText,
		"labels":   StateDisplayModeText,
		"none":     StateDisplayModeNone,
		"off":      StateDisplayModeNone,
		"no-icons": StateDisplayModeNone,
		"unknown":  StateDisplayModeInherit,
	}
	for in, want := range tests {
		if got := normalizeStateDisplayMode(in); got != want {
			t.Fatalf("normalizeStateDisplayMode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeStateDisplayMode(" GLYPHS "); got != StateDisplayModeIcons {
		t.Fatalf(
			"normalizeStateDisplayMode(%q) = %q, want %q",
			" GLYPHS ",
			got,
			StateDisplayModeIcons,
		)
	}
}

func TestNormalizeInputStyle(t *testing.T) {
	tests := map[string]string{
		"":            InputStylePopup,
		"popup":       InputStylePopup,
		"floating":    InputStylePopup,
		"float":       InputStylePopup,
		"box":         InputStylePopup,
		"centered":    InputStylePopup,
		"cmdline":     InputStyleCmdline,
		"cmd-line":    InputStyleCmdline,
		"commandline": InputStyleCmdline,
		"inline":      InputStyleCmdline,
		"bar":         InputStyleCmdline,
		"bottom":      InputStyleCmdline,
		"unknown":     InputStylePopup,
	}
	for in, want := range tests {
		if got := normalizeInputStyle(in); got != want {
			t.Fatalf("normalizeInputStyle(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeInputStyle(" CMDLINE "); got != InputStyleCmdline {
		t.Fatalf("normalizeInputStyle(%q) = %q, want %q", " CMDLINE ", got, InputStyleCmdline)
	}
}

func TestInputStyleDefaultIsPopup(t *testing.T) {
	cfg := Default()
	if cfg.TUI.InputStyle != InputStylePopup {
		t.Fatalf("default input_style = %q, want %q", cfg.TUI.InputStyle, InputStylePopup)
	}
}

func TestInputStyleRoundTripCmdline(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	cfg.TUI.InputStyle = InputStyleCmdline
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `[tui]`) {
		t.Fatalf("saved config missing [tui] section: %s", data)
	}
	if !strings.Contains(string(data), `input_style = "cmdline"`) {
		t.Fatalf("saved config missing input_style cmdline: %s", data)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.TUI.InputStyle != InputStyleCmdline {
		t.Fatalf("loaded input_style = %q, want cmdline", loaded.TUI.InputStyle)
	}
}

func TestDimBackgroundDefaultTrueWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte("\n[sources]\ndefault = \"sessions\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.TUI.DimBackground == nil || !*loaded.TUI.DimBackground {
		t.Fatalf("dim_background without [tui] = %v, want true", loaded.TUI.DimBackground)
	}
	if cfg := Default(); cfg.TUI.DimBackground == nil || !*cfg.TUI.DimBackground {
		t.Fatalf("default dim_background = %v, want true", cfg.TUI.DimBackground)
	}
}

func TestDimBackgroundRoundTripFalse(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	falseVal := false
	cfg.TUI.DimBackground = &falseVal
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `dim_background = false`) {
		t.Fatalf("saved config missing dim_background = false: %s", data)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.TUI.DimBackground == nil || *loaded.TUI.DimBackground {
		t.Fatalf("loaded dim_background = %v, want false", loaded.TUI.DimBackground)
	}
}

func TestInputStyleMissingSectionDefaultsPopup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte("\n[sources]\ndefault = \"sessions\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.TUI.InputStyle != InputStylePopup {
		t.Fatalf("input_style without [tui] = %q, want popup", loaded.TUI.InputStyle)
	}
}

func TestIconSetTmuxStateProjection(t *testing.T) {
	cfg := Default()
	icons := cfg.IconSet()
	if icons.TmuxStateMode != StateDisplayModeInherit {
		t.Fatalf("default tmux_state_mode = %q, want inherit", icons.TmuxStateMode)
	}
	if !icons.TmuxStateUsesIcons() || icons.TmuxStateUsesLabels() {
		t.Fatalf(
			"inherit + icons mode projection = icons:%v labels:%v",
			icons.TmuxStateUsesIcons(),
			icons.TmuxStateUsesLabels(),
		)
	}

	cfg.Icons.Mode = IconModeText
	icons = cfg.IconSet()
	if icons.TmuxStateUsesIcons() || !icons.TmuxStateUsesLabels() {
		t.Fatalf(
			"inherit + text mode projection = icons:%v labels:%v",
			icons.TmuxStateUsesIcons(),
			icons.TmuxStateUsesLabels(),
		)
	}

	cfg.Icons.TmuxStateMode = StateDisplayModeIcons
	icons = cfg.IconSet()
	if !icons.TmuxStateUsesIcons() || icons.TmuxStateUsesLabels() {
		t.Fatalf(
			"icons override + text mode projection = icons:%v labels:%v",
			icons.TmuxStateUsesIcons(),
			icons.TmuxStateUsesLabels(),
		)
	}

	cfg.Icons.Mode = IconModeIcons
	cfg.Icons.TmuxStateMode = StateDisplayModeText
	icons = cfg.IconSet()
	if icons.TmuxStateUsesIcons() || !icons.TmuxStateUsesLabels() {
		t.Fatalf(
			"tmux_state_mode=text overrides icons mode projection = icons:%v labels:%v",
			icons.TmuxStateUsesIcons(),
			icons.TmuxStateUsesLabels(),
		)
	}

	cfg.Icons.TmuxStateMode = StateDisplayModeNone
	icons = cfg.IconSet()
	if icons.TmuxStateUsesIcons() || icons.TmuxStateUsesLabels() || !icons.TmuxStateHidden() {
		t.Fatalf(
			"tmux_state_mode=none projection = icons:%v labels:%v hidden:%v",
			icons.TmuxStateUsesIcons(),
			icons.TmuxStateUsesLabels(),
			icons.TmuxStateHidden(),
		)
	}
}

func TestDefaultTmuxStateDetachedColor(t *testing.T) {
	cfg := Default()
	if got := cfg.Icons.TmuxState.Detached.Color; got != "8" {
		t.Fatalf("default detached color = %q, want 8", got)
	}
	icons := cfg.IconSet()
	if got := icons.ForTmuxState(false).Color; got != "8" {
		t.Fatalf("projected default detached color = %q, want 8", got)
	}
}

func TestLoadTmuxStateModeConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
mode = "icons"
tmux_state_mode = "text"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.TmuxStateMode != StateDisplayModeText {
		t.Fatalf("loaded tmux_state_mode = %q, want text", loaded.Icons.TmuxStateMode)
	}
}

func TestLoadPerStateTmuxStateConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
mode = "icons"

[icons.tmux_state.attached]
icon = "●"
label = "attached"
color = "10"

[icons.tmux_state.detached]
icon = "◌"
label = "detached"
color = "14"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.TmuxState.Attached.Icon != "●" {
		t.Fatalf("attached icon = %q, want ●", loaded.Icons.TmuxState.Attached.Icon)
	}
	if loaded.Icons.TmuxState.Detached.Color != "14" {
		t.Fatalf("detached color = %q, want 14", loaded.Icons.TmuxState.Detached.Color)
	}
	icons := loaded.IconSet()
	if got := icons.ForTmuxState(true).Icon; got != "●" {
		t.Fatalf("projected attached icon = %q, want ●", got)
	}
	if got := icons.ForTmuxState(false).Color; got != "14" {
		t.Fatalf("projected detached color = %q, want 14", got)
	}
}

func TestNormalizeTmuxStatePartialOverride(t *testing.T) {
	cfg := Default()
	cfg.Icons.TmuxState.Attached.Icon = "★"
	cfg.Icons.TmuxState.Attached.Label = ""
	cfg.Normalize()
	if cfg.Icons.TmuxState.Attached.Icon != "★" {
		t.Fatalf("attached icon = %q, want ★", cfg.Icons.TmuxState.Attached.Icon)
	}
	if cfg.Icons.TmuxState.Attached.Label != "attached" {
		t.Fatalf("attached label = %q, want attached", cfg.Icons.TmuxState.Attached.Label)
	}
}

func TestLoadMigratesEnabledFalseToNoneMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data := []byte(`
[icons]
enabled = false
mode = "icons"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Icons.Mode != IconModeNone {
		t.Fatalf("loaded icon mode = %q, want %q", loaded.Icons.Mode, IconModeNone)
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

func TestNormalizeThemeColorsPartial(t *testing.T) {
	cfg := Default()
	cfg.Theme.Colors.Title = "#b4befe"
	cfg.Theme.Colors.Accent = ""
	cfg.Theme.Colors.Danger = "   "
	cfg.Normalize()

	defaults := Default().Theme.Colors
	if cfg.Theme.Colors.Title != "#b4befe" {
		t.Fatalf("title = %q, want preserved override", cfg.Theme.Colors.Title)
	}
	if cfg.Theme.Colors.Accent != defaults.Accent {
		t.Fatalf("accent = %q, want default %q", cfg.Theme.Colors.Accent, defaults.Accent)
	}
	if cfg.Theme.Colors.Danger != defaults.Danger {
		t.Fatalf("danger = %q, want default %q", cfg.Theme.Colors.Danger, defaults.Danger)
	}
}

// TestNormalizeThemeColorsPopupTitleAlias verifies that popup_title inherits
// the deprecated `title` alias when unset, and that an explicit popup_title
// wins over the legacy title.
func TestNormalizeThemeColorsPopupTitleAlias(t *testing.T) {
	// Legacy `title` set, popup_title unset -> popup_title inherits title.
	cfg := Default()
	cfg.Theme.Colors.Title = "#abcdef"
	cfg.Theme.Colors.PopupTitle = ""
	cfg.Normalize()
	if cfg.Theme.Colors.PopupTitle != "#abcdef" {
		t.Fatalf("popup_title = %q, want inherited #abcdef", cfg.Theme.Colors.PopupTitle)
	}

	// Explicit popup_title wins over legacy title.
	cfg2 := Default()
	cfg2.Theme.Colors.Title = "#abcdef"
	cfg2.Theme.Colors.PopupTitle = "#112233"
	cfg2.Normalize()
	if cfg2.Theme.Colors.PopupTitle != "#112233" {
		t.Fatalf("popup_title = %q, want explicit #112233", cfg2.Theme.Colors.PopupTitle)
	}
}

func TestNormalizeThemeColorsFillsAllEmptyFields(t *testing.T) {
	cfg := Default()
	cfg.Theme.Colors = ThemeColorsConfig{}
	cfg.Normalize()

	defaults := Default().Theme.Colors
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"PopupBorder", cfg.Theme.Colors.PopupBorder, defaults.PopupBorder},
		{"ActiveTab", cfg.Theme.Colors.ActiveTab, defaults.ActiveTab},
		{"Border", cfg.Theme.Colors.Border, defaults.Border},
		{"InactiveTab", cfg.Theme.Colors.InactiveTab, defaults.InactiveTab},
		{"Title", cfg.Theme.Colors.Title, defaults.Title},
		{"PopupTitle", cfg.Theme.Colors.PopupTitle, defaults.Title},
		{"Accent", cfg.Theme.Colors.Accent, defaults.Accent},
		{"Key", cfg.Theme.Colors.Key, defaults.Key},
		{"Muted", cfg.Theme.Colors.Muted, defaults.Muted},
		{"Success", cfg.Theme.Colors.Success, defaults.Success},
		{"Info", cfg.Theme.Colors.Info, defaults.Info},
		{"Warning", cfg.Theme.Colors.Warning, defaults.Warning},
		{"Danger", cfg.Theme.Colors.Danger, defaults.Danger},
		// Per-pane tokens inherit the relevant global when unset.
		{"ListBorder", cfg.Theme.Colors.ListBorder, defaults.Border},
		{"MetadataBorder", cfg.Theme.Colors.MetadataBorder, defaults.Border},
		{"PreviewBorder", cfg.Theme.Colors.PreviewBorder, defaults.Border},
		{"ListBorderTitle", cfg.Theme.Colors.ListBorderTitle, defaults.Border},
		{"MetadataBorderTitle", cfg.Theme.Colors.MetadataBorderTitle, defaults.Border},
		{"PreviewBorderTitle", cfg.Theme.Colors.PreviewBorderTitle, defaults.Border},
		// Overview hero tiles inherit border / popup_title when unset.
		{"WorkspaceTileBorder", cfg.Theme.Colors.WorkspaceTileBorder, defaults.Border},
		{"AgentTileBorder", cfg.Theme.Colors.AgentTileBorder, defaults.Border},
		{"WorkspaceTileTitle", cfg.Theme.Colors.WorkspaceTileTitle, defaults.Title},
		{"AgentTileTitle", cfg.Theme.Colors.AgentTileTitle, defaults.Title},
	} {
		if tc.got != tc.want {
			t.Fatalf("%s = %q, want default %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestNormalizeThemeColorsPaneTokensInheritCustomGlobals(t *testing.T) {
	cfg := Default()
	cfg.Theme.Colors.PopupBorder = "#aaaaaa"
	cfg.Theme.Colors.Border = "#bbbbbb"
	cfg.Theme.Colors.Title = "#cccccc"
	// Leave the per-pane + overview tokens unset so they must inherit.
	cfg.Theme.Colors.ListBorder = ""
	cfg.Theme.Colors.MetadataBorder = ""
	cfg.Theme.Colors.PreviewBorder = ""
	cfg.Theme.Colors.ListBorderTitle = ""
	cfg.Theme.Colors.MetadataBorderTitle = ""
	cfg.Theme.Colors.PreviewBorderTitle = ""
	cfg.Normalize()

	c := cfg.Theme.Colors
	want := map[string]string{
		"ListBorder":          "#bbbbbb",
		"MetadataBorder":      "#bbbbbb",
		"PreviewBorder":       "#bbbbbb",
		"ListBorderTitle":     "#bbbbbb",
		"MetadataBorderTitle": "#bbbbbb",
		"PreviewBorderTitle":  "#bbbbbb",
		// Overview tiles: borders inherit border; titles inherit popup_title
		// (which itself inherits the legacy title here).
		"WorkspaceTileBorder": "#bbbbbb",
		"AgentTileBorder":     "#bbbbbb",
		"WorkspaceTileTitle":  "#cccccc",
		"AgentTileTitle":      "#cccccc",
	}
	got := map[string]string{
		"ListBorder":          c.ListBorder,
		"MetadataBorder":      c.MetadataBorder,
		"PreviewBorder":       c.PreviewBorder,
		"ListBorderTitle":     c.ListBorderTitle,
		"MetadataBorderTitle": c.MetadataBorderTitle,
		"PreviewBorderTitle":  c.PreviewBorderTitle,
		"WorkspaceTileBorder": c.WorkspaceTileBorder,
		"AgentTileBorder":     c.AgentTileBorder,
		"WorkspaceTileTitle":  c.WorkspaceTileTitle,
		"AgentTileTitle":      c.AgentTileTitle,
	}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("%s = %q, want inherited %q", k, got[k], w)
		}
	}
}

func TestLoadRejectsInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not = [valid\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid TOML")
	} else if !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error = %v, want path %q in message", err, path)
	}
}

func TestLoadFailsWhenConfigUnreadable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[sources]\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for unreadable config")
	}
}

func TestPrefixKeyDefaultFallback(t *testing.T) {
	cfg := Default()
	cfg.TypeFirst.Prefix = "  "
	if got := cfg.PrefixKey(); got != DefaultPrefix {
		t.Fatalf("PrefixKey() = %q, want %q", got, DefaultPrefix)
	}
	cfg.TypeFirst.Prefix = "ctrl+a"
	if got := cfg.PrefixKey(); got != "ctrl+a" {
		t.Fatalf("PrefixKey() = %q, want ctrl+a", got)
	}
}

func TestDefaultSourceInvalidFallsBackToAll(t *testing.T) {
	cfg := Default()
	cfg.Sources.Default = "not-a-source"
	if got := cfg.DefaultSource(); got != sessionmgr.ModeAll {
		t.Fatalf("DefaultSource() = %v, want all", got)
	}
}

func TestSaveFailsWhenConfigDirIsFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
	if err := Save(Default()); err == nil {
		t.Fatal("Save() expected error when config parent path is a file")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if Exists() {
		t.Fatal("Exists() = true, want false before save")
	}
	if err := Save(Default()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !Exists() {
		t.Fatal("Exists() = false, want true after save")
	}
}

func TestWorkspaceIconDefaults(t *testing.T) {
	cfg := Default()
	icons := cfg.IconSet()
	if icons.Workspace.Icon != sessionmgr.IconWorkspace+" " {
		t.Fatalf("workspace icon = %q, want %q", icons.Workspace.Icon, sessionmgr.IconWorkspace+" ")
	}
	// Session icon must now match workspace glyph.
	if icons.Session.Icon != icons.Workspace.Icon {
		t.Fatalf("session icon = %q, workspace icon = %q", icons.Session.Icon, icons.Workspace.Icon)
	}
	// Override via config.
	cfg.Icons.Workspace = IconConfig{Icon: "X ", Label: "x", Color: "9"}
	overridden := cfg.IconSet()
	if overridden.Workspace.Icon != "X " || overridden.Workspace.Color != "9" {
		t.Fatalf("overridden workspace = %+v", overridden.Workspace)
	}
}
