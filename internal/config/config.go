package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const (
	appDirName     = "seshagy"
	configFileName = "config.toml"
	DefaultPrefix  = "ctrl+x"
	IconModeIcons  = "icons"
	IconModeText   = "text"
	IconModeNone   = "none"

	StateDisplayModeInherit = "inherit"
	StateDisplayModeIcons   = "icons"
	StateDisplayModeText    = "text"
	StateDisplayModeNone    = "none"

	InputStylePopup   = "popup"
	InputStyleCmdline = "cmdline"
)

type Config struct {
	Sources     SourcesConfig     `toml:"sources"     json:"sources"`
	Directories DirectoriesConfig `toml:"directories" json:"directories"`
	Theme       ThemeConfig       `toml:"theme"       json:"theme"`
	Icons       IconsConfig       `toml:"icons"       json:"icons"`
	TypeFirst   TypeFirstConfig   `toml:"type_first"  json:"type_first"`
	Setup       SetupConfig       `toml:"setup"       json:"setup"`
	Agents      AgentsConfig      `toml:"agents"      json:"agents"`
	TUI         TUIConfig         `toml:"tui"         json:"tui"`
	Log         LogConfig         `toml:"log"         json:"log"`
}

// LogConfig controls opt-in, file-only structured diagnostics.
type LogConfig struct {
	Level string `toml:"level" json:"level"`
	File  string `toml:"file"  json:"file"`
}

// TUIConfig holds TUI-only rendering toggles.
type TUIConfig struct {
	InputStyle    string `toml:"input_style"    json:"input_style"`
	DimBackground *bool  `toml:"dim_background" json:"dim_background"`
	Preview       *bool  `toml:"preview"        json:"preview"`
}

type SourcesConfig struct {
	Default string   `toml:"default" json:"default"`
	Order   []string `toml:"order"   json:"order"`
}

type DirectoriesConfig struct {
	FDCommand string `toml:"fd_command" json:"fd_command"`
}

type ThemeConfig struct {
	Colors ThemeColorsConfig `toml:"colors" json:"colors"`
}

type ThemeColorsConfig struct {
	PopupBorder         string `toml:"popup_border"          json:"popup_border"`
	ActiveTab           string `toml:"active_tab"            json:"active_tab"`
	Border              string `toml:"border"                json:"border"`
	InactiveTab         string `toml:"inactive_tab"          json:"inactive_tab"`
	PopupTitle          string `toml:"popup_title"           json:"popup_title"`
	Title               string `toml:"title"                 json:"title"` // deprecated alias for popup_title
	Accent              string `toml:"accent"                json:"accent"`
	Key                 string `toml:"key"                   json:"key"`
	Muted               string `toml:"muted"                 json:"muted"`
	Success             string `toml:"success"               json:"success"`
	Info                string `toml:"info"                  json:"info"`
	Warning             string `toml:"warning"               json:"warning"`
	Danger              string `toml:"danger"                json:"danger"`
	ListBorder          string `toml:"list_border"           json:"list_border"`
	MetadataBorder      string `toml:"metadata_border"       json:"metadata_border"`
	PreviewBorder       string `toml:"preview_border"        json:"preview_border"`
	ListBorderTitle     string `toml:"list_border_title"     json:"list_border_title"`
	MetadataBorderTitle string `toml:"metadata_border_title" json:"metadata_border_title"`
	PreviewBorderTitle  string `toml:"preview_border_title"  json:"preview_border_title"`
	InputBorder         string `toml:"input_border"          json:"input_border"`

	// Overview hero tiles (workspaces + agents).
	WorkspaceTileBorder string `toml:"workspace_tile_border" json:"workspace_tile_border"`
	WorkspaceTileTitle  string `toml:"workspace_tile_title"  json:"workspace_tile_title"`
	AgentTileBorder     string `toml:"agent_tile_border"     json:"agent_tile_border"`
	AgentTileTitle      string `toml:"agent_tile_title"      json:"agent_tile_title"`
}

type IconsConfig struct {
	Mode           string            `toml:"mode"                       json:"mode"`
	TmuxStateMode  string            `toml:"tmux_state_mode,omitempty"  json:"tmux_state_mode,omitempty"`
	AgentStateMode string            `toml:"agent_state_mode,omitempty" json:"agent_state_mode,omitempty"`
	Enabled        *bool             `toml:"enabled,omitempty"          json:"enabled,omitempty"`
	ASCII          bool              `toml:"ascii,omitempty"            json:"ascii,omitempty"`
	Session        IconConfig        `toml:"session"                    json:"session"`
	Workspace      IconConfig        `toml:"workspace"                  json:"workspace"`
	Zoxide         IconConfig        `toml:"zoxide"                     json:"zoxide"`
	FD             IconConfig        `toml:"fd"                         json:"fd"`
	TmuxState      TmuxStatesConfig  `toml:"tmux_state"                 json:"tmux_state"`
	AgentState     AgentStatesConfig `toml:"agent_state"                json:"agent_state"`
}

type TmuxStatesConfig struct {
	Attached IconConfig `toml:"attached" json:"attached"`
	Detached IconConfig `toml:"detached" json:"detached"`
}

type AgentStatesConfig struct {
	Idle    IconConfig `toml:"idle"    json:"idle"`
	Working IconConfig `toml:"working" json:"working"`
	Blocked IconConfig `toml:"blocked" json:"blocked"`
	Done    IconConfig `toml:"done"    json:"done"`
	Unknown IconConfig `toml:"unknown" json:"unknown"`
}

type IconConfig struct {
	Icon  string `toml:"icon"            json:"icon"`
	Label string `toml:"label"           json:"label"`
	ASCII string `toml:"ascii,omitempty" json:"ascii,omitempty"`
	Color string `toml:"color"           json:"color"`
}

type TypeFirstConfig struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Prefix  string `toml:"prefix"  json:"prefix"`
}

type SetupConfig struct {
	TypeFirstPromptSeen bool `toml:"type_first_prompt_seen" json:"type_first_prompt_seen"`
	InstallMenuSeen     bool `toml:"install_menu_seen"      json:"install_menu_seen"`
}

// AgentsConfig holds agent-detection options. ManifestFallback enables the
// capture-pane screen-rule backstop for hook-less agents (opencode, cursor,
// antigravity, grok). It defaults to true so detection works out of the box.
// AGENTS.md describes manifest_fallback as opt-in; that wording predates the
// default-on decision and will be reconciled in a future docs pass.
type AgentsConfig struct {
	ManifestFallback *bool  `toml:"manifest_fallback,omitempty" json:"manifest_fallback,omitempty"`
	CatalogURL       string `toml:"catalog_url,omitempty"       json:"catalog_url,omitempty"`
}

// ManifestFallback reports whether the capture-pane manifest backstop is
// enabled. Defaults to true (nil pointer).
func (c Config) ManifestFallback() bool {
	if c.Agents.ManifestFallback == nil {
		return true
	}
	return *c.Agents.ManifestFallback
}

// CatalogURL returns the manifest catalog URL. Defaults to the herdr public
// catalog when empty.
func (c Config) CatalogURL() string {
	if u := strings.TrimSpace(c.Agents.CatalogURL); u != "" {
		return u
	}
	return sessionmgr.DefaultManifestCatalogURL()
}

func Default() Config {
	return Config{
		Sources:     SourcesConfig{Default: "all", Order: defaultSourceOrderNames()},
		Directories: DirectoriesConfig{FDCommand: sessionmgr.DefaultFDCommand},
		Theme: ThemeConfig{Colors: ThemeColorsConfig{
			PopupBorder: "13",
			ActiveTab:   "default",
			Border:      "8",
			InactiveTab: "8",
			Title:       "12",
			Accent:      "13",
			Key:         "11",
			Muted:       "8",
			Success:     "10",
			Info:        "14",
			Warning:     "11",
			Danger:      "9",
		}},
		Icons: IconsConfig{
			Mode:       IconModeIcons,
			TmuxState:  defaultTmuxStatesConfig(),
			AgentState: defaultAgentStatesConfig(),
			Session:    IconConfig{Icon: sessionmgr.IconSession + " ", Label: "S", Color: "10"},
			Workspace:  IconConfig{Icon: sessionmgr.IconWorkspace + " ", Label: "W", Color: "10"},
			Zoxide:     IconConfig{Icon: sessionmgr.IconZoxide + " ", Label: "Z", Color: "14"},
			FD:         IconConfig{Icon: sessionmgr.IconFD + " ", Label: "F", Color: "11"},
		},
		TypeFirst: TypeFirstConfig{Enabled: false, Prefix: DefaultPrefix},
		Log:       LogConfig{Level: "off"},
		TUI: TUIConfig{
			InputStyle:    InputStylePopup,
			DimBackground: ptrBool(true),
			Preview:       ptrBool(true),
		},
	}
}

func ptrBool(b bool) *bool { return &b }

func Path() string {
	return filepath.Join(xdg.ConfigHome(), appDirName, configFileName)
}

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

func Load() (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("read %s: %w", Path(), err)
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(cfg Config) error {
	cfg.Normalize()
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func Marshal(cfg Config) ([]byte, error) {
	cfg.Normalize()
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(cfg)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Config) Normalize() {
	defaults := Default()
	c.Sources.Order = normalizeSourceOrder(c.Sources.Order)
	if mode, ok := sourceModeFromName(c.Sources.Default); ok {
		c.Sources.Default = SourceModeName(mode)
	} else {
		c.Sources.Default = defaults.Sources.Default
	}
	if strings.TrimSpace(c.Directories.FDCommand) == "" {
		c.Directories.FDCommand = defaults.Directories.FDCommand
	}
	normalizeThemeColors(&c.Theme.Colors, defaults.Theme.Colors)
	c.Icons.Mode = normalizeIconMode(c.Icons.Mode)
	c.Icons.TmuxStateMode = normalizeStateDisplayMode(c.Icons.TmuxStateMode)
	c.Icons.AgentStateMode = normalizeStateDisplayMode(c.Icons.AgentStateMode)
	if c.Icons.Enabled != nil && !*c.Icons.Enabled {
		c.Icons.Mode = IconModeNone
	} else if c.Icons.ASCII {
		c.Icons.Mode = IconModeText
	}
	c.Icons.Enabled = nil
	c.Icons.ASCII = false
	normalizeKindIcon(&c.Icons.Session, defaults.Icons.Session, sessionmgr.IconSession)
	normalizeKindIcon(&c.Icons.Workspace, defaults.Icons.Workspace, sessionmgr.IconWorkspace)
	normalizeKindIcon(&c.Icons.Zoxide, defaults.Icons.Zoxide, sessionmgr.IconZoxide)
	normalizeKindIcon(&c.Icons.FD, defaults.Icons.FD, sessionmgr.IconFD)
	normalizeTmuxStatesConfig(&c.Icons.TmuxState, defaults.Icons.TmuxState)
	normalizeAgentStatesConfig(&c.Icons.AgentState, defaults.Icons.AgentState)
	if strings.TrimSpace(c.TypeFirst.Prefix) == "" {
		c.TypeFirst.Prefix = DefaultPrefix
	}
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	if c.Log.Level == "" {
		c.Log.Level = "off"
	}
	c.Log.File = strings.TrimSpace(c.Log.File)
	c.TUI.InputStyle = normalizeInputStyle(c.TUI.InputStyle)
	if c.TUI.DimBackground == nil {
		c.TUI.DimBackground = ptrBool(true)
	}
	if c.TUI.Preview == nil {
		c.TUI.Preview = ptrBool(true)
	}
}

func normalizeThemeColors(colors *ThemeColorsConfig, defaults ThemeColorsConfig) {
	if strings.TrimSpace(colors.PopupBorder) == "" {
		colors.PopupBorder = defaults.PopupBorder
	}
	if strings.TrimSpace(colors.ActiveTab) == "" {
		colors.ActiveTab = defaults.ActiveTab
	}
	if strings.TrimSpace(colors.Border) == "" {
		colors.Border = defaults.Border
	}
	if strings.TrimSpace(colors.InactiveTab) == "" {
		colors.InactiveTab = defaults.InactiveTab
	}
	if strings.TrimSpace(colors.Title) == "" {
		colors.Title = defaults.Title
	}
	// popup_title colors popup/dialog headings; the legacy `title` key is a
	// deprecated alias, so popup_title inherits it when unset.
	if strings.TrimSpace(colors.PopupTitle) == "" {
		colors.PopupTitle = colors.Title
	}
	if strings.TrimSpace(colors.Accent) == "" {
		colors.Accent = defaults.Accent
	}
	if strings.TrimSpace(colors.Key) == "" {
		colors.Key = defaults.Key
	}
	if strings.TrimSpace(colors.Muted) == "" {
		colors.Muted = defaults.Muted
	}
	if strings.TrimSpace(colors.Success) == "" {
		colors.Success = defaults.Success
	}
	if strings.TrimSpace(colors.Info) == "" {
		colors.Info = defaults.Info
	}
	if strings.TrimSpace(colors.Warning) == "" {
		colors.Warning = defaults.Warning
	}
	if strings.TrimSpace(colors.Danger) == "" {
		colors.Danger = defaults.Danger
	}
	// Per-pane borders and border titles inherit the relevant global (or the
	// pane's own border, for titles) when unset, so omitting them preserves
	// today's look. The globals above are already resolved, so these read the
	// user's final values rather than the hardcoded defaults.
	if strings.TrimSpace(colors.ListBorder) == "" {
		colors.ListBorder = colors.Border
	}
	if strings.TrimSpace(colors.MetadataBorder) == "" {
		colors.MetadataBorder = colors.Border
	}
	if strings.TrimSpace(colors.PreviewBorder) == "" {
		colors.PreviewBorder = colors.Border
	}
	if strings.TrimSpace(colors.ListBorderTitle) == "" {
		colors.ListBorderTitle = colors.ListBorder
	}
	if strings.TrimSpace(colors.MetadataBorderTitle) == "" {
		colors.MetadataBorderTitle = colors.MetadataBorder
	}
	if strings.TrimSpace(colors.PreviewBorderTitle) == "" {
		colors.PreviewBorderTitle = colors.PreviewBorder
	}
	// Search/rename input popup border defaults to the base border so it matches
	// the dashboard tiles out of the box while staying independently themeable.
	if strings.TrimSpace(colors.InputBorder) == "" {
		colors.InputBorder = colors.Border
	}
	// Overview hero tiles inherit border / popup_title so the default look is
	// unchanged unless themed.
	if strings.TrimSpace(colors.WorkspaceTileBorder) == "" {
		colors.WorkspaceTileBorder = colors.Border
	}
	if strings.TrimSpace(colors.AgentTileBorder) == "" {
		colors.AgentTileBorder = colors.Border
	}
	if strings.TrimSpace(colors.WorkspaceTileTitle) == "" {
		colors.WorkspaceTileTitle = colors.PopupTitle
	}
	if strings.TrimSpace(colors.AgentTileTitle) == "" {
		colors.AgentTileTitle = colors.PopupTitle
	}
}

func (c Config) IconSet() sessionmgr.IconSet {
	c.Normalize()
	enabled := c.Icons.Mode != IconModeNone
	return sessionmgr.IconSet{
		Enabled:        enabled,
		ASCII:          c.Icons.Mode == IconModeText,
		TmuxStateMode:  c.Icons.TmuxStateMode,
		AgentStateMode: c.Icons.AgentStateMode,
		TmuxStates:     projectTmuxStateStyles(c.Icons.TmuxState),
		AgentStates:    projectAgentStateStyles(c.Icons.AgentState),
		Session: sessionmgr.IconStyle{
			Icon:  c.Icons.Session.Icon,
			ASCII: c.Icons.Session.Label,
			Color: c.Icons.Session.Color,
		},
		Workspace: sessionmgr.IconStyle{
			Icon:  c.Icons.Workspace.Icon,
			ASCII: c.Icons.Workspace.Label,
			Color: c.Icons.Workspace.Color,
		},
		Zoxide: sessionmgr.IconStyle{
			Icon:  c.Icons.Zoxide.Icon,
			ASCII: c.Icons.Zoxide.Label,
			Color: c.Icons.Zoxide.Color,
		},
		FD: sessionmgr.IconStyle{
			Icon:  c.Icons.FD.Icon,
			ASCII: c.Icons.FD.Label,
			Color: c.Icons.FD.Color,
		},
	}
}

func (c Config) PrefixKey() string {
	prefix := strings.TrimSpace(c.TypeFirst.Prefix)
	if prefix == "" {
		return DefaultPrefix
	}
	return prefix
}

func (c Config) SourceOrder() []sessionmgr.SourceMode {
	c.Normalize()
	modes := make([]sessionmgr.SourceMode, 0, len(c.Sources.Order))
	for _, name := range c.Sources.Order {
		if mode, ok := sourceModeFromName(name); ok {
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		return defaultSourceOrder()
	}
	// ModeCurrentAgents is CLI-only (--get-current-session-agents passes the
	// mode directly and bypasses SourceOrder). It must never render as a tab,
	// even when a stale config persists it in [sources].order.
	filtered := modes[:0]
	for _, mode := range modes {
		if mode == sessionmgr.ModeCurrentAgents {
			continue
		}
		filtered = append(filtered, mode)
	}
	return filtered
}

func (c Config) DefaultSource() sessionmgr.SourceMode {
	c.Normalize()
	if mode, ok := sourceModeFromName(c.Sources.Default); ok {
		return mode
	}
	return sessionmgr.ModeAll
}

func (c Config) LoadOptions() sessionmgr.LoadOptions {
	c.Normalize()
	return sessionmgr.LoadOptions{
		FDCommand:        c.Directories.FDCommand,
		ManifestFallback: c.ManifestFallback(),
	}
}

func SourceModeName(mode sessionmgr.SourceMode) string {
	return mode.Names().ConfigToken
}

func normalizeSourceOrder(names []string) []string {
	seen := map[sessionmgr.SourceMode]bool{}
	order := make([]string, 0, len(defaultSourceOrderNames()))
	for _, name := range names {
		mode, ok := sourceModeFromName(name)
		if !ok || seen[mode] {
			continue
		}
		// ModeCurrentAgents is never a tab; mark it seen so it is dropped from
		// the persisted order on Save() without being re-added by defaults.
		seen[mode] = true
		if mode == sessionmgr.ModeCurrentAgents {
			continue
		}
		order = append(order, SourceModeName(mode))
	}
	for _, mode := range defaultSourceOrder() {
		if seen[mode] {
			continue
		}
		order = append(order, SourceModeName(mode))
	}
	return order
}

func defaultSourceOrder() []sessionmgr.SourceMode {
	return []sessionmgr.SourceMode{
		sessionmgr.ModeAll,
		sessionmgr.ModeSessions,
		sessionmgr.ModeZoxide,
		sessionmgr.ModeFD,
		sessionmgr.ModeAgents,
	}
}

func defaultSourceOrderNames() []string {
	modes := defaultSourceOrder()
	names := make([]string, 0, len(modes))
	for _, mode := range modes {
		names = append(names, SourceModeName(mode))
	}
	return names
}

func sourceModeFromName(name string) (sessionmgr.SourceMode, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	switch normalized {
	case "all":
		return sessionmgr.ModeAll, true
	case "sessions", "session":
		return sessionmgr.ModeSessions, true
	case "zoxide", "z":
		return sessionmgr.ModeZoxide, true
	case "fd", "f":
		return sessionmgr.ModeFD, true
	case "agents", "agent":
		return sessionmgr.ModeAgents, true
	case "current-agents", "current-agent":
		return sessionmgr.ModeCurrentAgents, true
	default:
		return sessionmgr.ModeAll, false
	}
}

func normalizeIconMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "icon", "icons", "nerd", "nerd-font", "nerdfont":
		return IconModeIcons
	case "text", "label", "labels", "plain", "letters", "ascii":
		return IconModeText
	case "none", "off", "disabled", "disable", "no-icons", "noicons":
		return IconModeNone
	default:
		return IconModeIcons
	}
}

func normalizeKindIcon(
	icon *IconConfig,
	defaults IconConfig,
	bareIcon string,
) {
	if strings.TrimSpace(icon.Icon) == "" {
		icon.Icon = defaults.Icon
	}
	if icon.Icon == bareIcon {
		icon.Icon = defaults.Icon
	}
	if legacy := strings.TrimSpace(icon.ASCII); legacy != "" &&
		(strings.TrimSpace(icon.Label) == "" || icon.Label == defaults.Label) {
		icon.Label = legacy
	}
	if strings.TrimSpace(icon.Label) == "" {
		icon.Label = strings.TrimSpace(icon.ASCII)
	}
	if strings.TrimSpace(icon.Label) == "" {
		icon.Label = defaults.Label
	}
	icon.ASCII = ""
	if strings.TrimSpace(icon.Color) == "" {
		icon.Color = defaults.Color
	}
}

func normalizeInputStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "popup", "floating", "float", "box", "centered":
		return InputStylePopup
	case "cmdline", "cmd-line", "commandline", "inline", "bar", "bottom":
		return InputStyleCmdline
	default:
		return InputStylePopup
	}
}

func normalizeStateDisplayMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "inherit", "default":
		return StateDisplayModeInherit
	case "icon", "icons", "glyphs", "glyph":
		return StateDisplayModeIcons
	case "text", "label", "labels":
		return StateDisplayModeText
	case "none", "off", "disabled", "disable", "no-icons", "noicons":
		return StateDisplayModeNone
	default:
		return StateDisplayModeInherit
	}
}

func defaultTmuxStatesConfig() TmuxStatesConfig {
	return TmuxStatesConfig{
		Attached: IconConfig{Icon: "●", Label: "attached", Color: "10"},
		Detached: IconConfig{Icon: "◌", Label: "detached", Color: "8"},
	}
}

func normalizeTmuxStatesConfig(states *TmuxStatesConfig, defaults TmuxStatesConfig) {
	normalizeTmuxStateIcon(&states.Attached, defaults.Attached)
	normalizeTmuxStateIcon(&states.Detached, defaults.Detached)
}

func normalizeTmuxStateIcon(state *IconConfig, defaults IconConfig) {
	if strings.TrimSpace(state.Icon) == "" {
		state.Icon = defaults.Icon
	}
	if strings.TrimSpace(state.Label) == "" {
		state.Label = defaults.Label
	}
}

func projectTmuxStateStyles(states TmuxStatesConfig) sessionmgr.TmuxStateStyles {
	return sessionmgr.TmuxStateStyles{
		Attached: sessionmgr.IconStyle{
			Icon:  states.Attached.Icon,
			ASCII: states.Attached.Label,
			Color: states.Attached.Color,
		},
		Detached: sessionmgr.IconStyle{
			Icon:  states.Detached.Icon,
			ASCII: states.Detached.Label,
			Color: states.Detached.Color,
		},
	}
}

func defaultAgentStatesConfig() AgentStatesConfig {
	return AgentStatesConfig{
		Working: IconConfig{Icon: "●", Label: "working", Color: "10"},
		Blocked: IconConfig{Icon: "◐", Label: "blocked", Color: "11"},
		Done:    IconConfig{Icon: "◉", Label: "done", Color: "14"},
		Unknown: IconConfig{Icon: "?", Label: "unknown", Color: "8"},
		Idle:    IconConfig{Icon: "○", Label: "idle", Color: "8"},
	}
}

func normalizeAgentStatesConfig(states *AgentStatesConfig, defaults AgentStatesConfig) {
	normalizeAgentStateIcon(&states.Idle, defaults.Idle)
	normalizeAgentStateIcon(&states.Working, defaults.Working)
	normalizeAgentStateIcon(&states.Blocked, defaults.Blocked)
	normalizeAgentStateIcon(&states.Done, defaults.Done)
	normalizeAgentStateIcon(&states.Unknown, defaults.Unknown)
}

func normalizeAgentStateIcon(state *IconConfig, defaults IconConfig) {
	if strings.TrimSpace(state.Icon) == "" {
		state.Icon = defaults.Icon
	}
	if strings.TrimSpace(state.Label) == "" {
		state.Label = defaults.Label
	}
}

func projectAgentStateStyles(states AgentStatesConfig) sessionmgr.AgentStateStyles {
	return sessionmgr.AgentStateStyles{
		Idle: sessionmgr.IconStyle{
			Icon:  states.Idle.Icon,
			ASCII: states.Idle.Label,
			Color: states.Idle.Color,
		},
		Working: sessionmgr.IconStyle{
			Icon:  states.Working.Icon,
			ASCII: states.Working.Label,
			Color: states.Working.Color,
		},
		Blocked: sessionmgr.IconStyle{
			Icon:  states.Blocked.Icon,
			ASCII: states.Blocked.Label,
			Color: states.Blocked.Color,
		},
		Done: sessionmgr.IconStyle{
			Icon:  states.Done.Icon,
			ASCII: states.Done.Label,
			Color: states.Done.Color,
		},
		Unknown: sessionmgr.IconStyle{
			Icon:  states.Unknown.Icon,
			ASCII: states.Unknown.Label,
			Color: states.Unknown.Color,
		},
	}
}
