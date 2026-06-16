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
)

type Config struct {
	Sources     SourcesConfig     `toml:"sources"     json:"sources"`
	Directories DirectoriesConfig `toml:"directories" json:"directories"`
	Agents      AgentsConfig      `toml:"agents"      json:"agents"`
	Theme       ThemeConfig       `toml:"theme"       json:"theme"`
	Icons       IconsConfig       `toml:"icons"       json:"icons"`
	TypeFirst   TypeFirstConfig   `toml:"type_first"  json:"type_first"`
	Setup       SetupConfig       `toml:"setup"       json:"setup"`
}

type AgentsConfig struct {
	ManifestFallback   bool   `toml:"manifest_fallback"    json:"manifest_fallback"`
	ManifestAutoUpdate bool   `toml:"manifest_auto_update" json:"manifest_auto_update"`
	ManifestCatalogURL string `toml:"manifest_catalog_url" json:"manifest_catalog_url"`
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
	FocusedBorder string `toml:"focused_border" json:"focused_border"`
	ActiveTab     string `toml:"active_tab"     json:"active_tab"`
	Border        string `toml:"border"         json:"border"`
	InactiveTab   string `toml:"inactive_tab"   json:"inactive_tab"`
	Title         string `toml:"title"          json:"title"`
	Accent        string `toml:"accent"         json:"accent"`
	Key           string `toml:"key"            json:"key"`
	Muted         string `toml:"muted"          json:"muted"`
	Success       string `toml:"success"        json:"success"`
	Info          string `toml:"info"           json:"info"`
	Warning       string `toml:"warning"        json:"warning"`
	Danger        string `toml:"danger"         json:"danger"`
}

type IconsConfig struct {
	Mode           string            `toml:"mode"                       json:"mode"`
	AgentStateMode string            `toml:"agent_state_mode,omitempty" json:"agent_state_mode,omitempty"`
	TmuxStateMode  string            `toml:"tmux_state_mode,omitempty"  json:"tmux_state_mode,omitempty"`
	Enabled        *bool             `toml:"enabled,omitempty"          json:"enabled,omitempty"`
	ASCII          bool              `toml:"ascii,omitempty"            json:"ascii,omitempty"`
	Session        IconConfig        `toml:"session"                    json:"session"`
	Zoxide         IconConfig        `toml:"zoxide"                     json:"zoxide"`
	FD             IconConfig        `toml:"fd"                         json:"fd"`
	Agent          IconConfig        `toml:"agent"                      json:"agent"`
	AgentState     AgentStatesConfig `toml:"agent_state"                json:"agent_state"`
	TmuxState      TmuxStatesConfig  `toml:"tmux_state"                 json:"tmux_state"`
}

type TmuxStatesConfig struct {
	Attached IconConfig `toml:"attached" json:"attached"`
	Detached IconConfig `toml:"detached" json:"detached"`
}

type AgentStatesConfig struct {
	Working IconConfig `toml:"working" json:"working"`
	Blocked IconConfig `toml:"blocked" json:"blocked"`
	Aborted IconConfig `toml:"aborted" json:"aborted"`
	Done    IconConfig `toml:"done"    json:"done"`
	Idle    IconConfig `toml:"idle"    json:"idle"`
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
}

func Default() Config {
	return Config{
		Sources:     SourcesConfig{Default: "all", Order: defaultSourceOrderNames()},
		Directories: DirectoriesConfig{FDCommand: sessionmgr.DefaultFDCommand},
		Theme: ThemeConfig{Colors: ThemeColorsConfig{
			FocusedBorder: "13",
			ActiveTab:     "default",
			Border:        "8",
			InactiveTab:   "8",
			Title:         "12",
			Accent:        "13",
			Key:           "11",
			Muted:         "8",
			Success:       "10",
			Info:          "14",
			Warning:       "11",
			Danger:        "9",
		}},
		Icons: IconsConfig{
			Mode:       IconModeIcons,
			AgentState: defaultAgentStatesConfig(),
			TmuxState:  defaultTmuxStatesConfig(),
			Session:    IconConfig{Icon: sessionmgr.IconSession + " ", Label: "S", Color: "10"},
			Zoxide:     IconConfig{Icon: sessionmgr.IconZoxide + " ", Label: "Z", Color: "14"},
			FD:         IconConfig{Icon: sessionmgr.IconFD + " ", Label: "F", Color: "11"},
			Agent:      IconConfig{Icon: sessionmgr.IconAgent + "  ", Label: "A", Color: "13"},
		},
		TypeFirst: TypeFirstConfig{Enabled: false, Prefix: DefaultPrefix},
		Agents: AgentsConfig{
			ManifestAutoUpdate: true,
		},
	}
}

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
	c.Icons.AgentStateMode = normalizeStateDisplayMode(c.Icons.AgentStateMode)
	c.Icons.TmuxStateMode = normalizeStateDisplayMode(c.Icons.TmuxStateMode)
	if c.Icons.Enabled != nil && !*c.Icons.Enabled {
		c.Icons.Mode = IconModeNone
	} else if c.Icons.ASCII {
		c.Icons.Mode = IconModeText
	}
	c.Icons.Enabled = nil
	c.Icons.ASCII = false
	normalizeKindIcon(&c.Icons.Session, defaults.Icons.Session, sessionmgr.IconSession)
	normalizeKindIcon(&c.Icons.Zoxide, defaults.Icons.Zoxide, sessionmgr.IconZoxide)
	normalizeKindIcon(&c.Icons.FD, defaults.Icons.FD, sessionmgr.IconFD)
	normalizeKindIcon(
		&c.Icons.Agent,
		defaults.Icons.Agent,
		sessionmgr.IconAgent,
		sessionmgr.IconAgent+" ",
		"󰚩",
	)
	normalizeAgentStatesConfig(&c.Icons.AgentState, defaults.Icons.AgentState)
	normalizeTmuxStatesConfig(&c.Icons.TmuxState, defaults.Icons.TmuxState)
	if strings.TrimSpace(c.TypeFirst.Prefix) == "" {
		c.TypeFirst.Prefix = DefaultPrefix
	}
}

func normalizeThemeColors(colors *ThemeColorsConfig, defaults ThemeColorsConfig) {
	if strings.TrimSpace(colors.FocusedBorder) == "" {
		colors.FocusedBorder = defaults.FocusedBorder
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
}

func (c Config) IconSet() sessionmgr.IconSet {
	c.Normalize()
	enabled := c.Icons.Mode != IconModeNone
	return sessionmgr.IconSet{
		Enabled:        enabled,
		ASCII:          c.Icons.Mode == IconModeText,
		AgentStateMode: c.Icons.AgentStateMode,
		AgentStates:    projectAgentStateStyles(c.Icons.AgentState),
		TmuxStateMode:  c.Icons.TmuxStateMode,
		TmuxStates:     projectTmuxStateStyles(c.Icons.TmuxState),
		Session: sessionmgr.IconStyle{
			Icon:  c.Icons.Session.Icon,
			ASCII: c.Icons.Session.Label,
			Color: c.Icons.Session.Color,
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
		Agent: sessionmgr.IconStyle{
			Icon:  c.Icons.Agent.Icon,
			ASCII: c.Icons.Agent.Label,
			Color: c.Icons.Agent.Color,
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
	return modes
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
		ManifestFallback: c.Agents.ManifestFallback,
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
		seen[mode] = true
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
		sessionmgr.ModeAgents,
		sessionmgr.ModeCurrentAgents,
		sessionmgr.ModeZoxide,
		sessionmgr.ModeFD,
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
	case "agents", "agent":
		return sessionmgr.ModeAgents, true
	case "current-agents", "current-agent", "current", "current-session-agents":
		return sessionmgr.ModeCurrentAgents, true
	case "zoxide", "z":
		return sessionmgr.ModeZoxide, true
	case "fd", "f":
		return sessionmgr.ModeFD, true
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

func defaultAgentStatesConfig() AgentStatesConfig {
	return AgentStatesConfig{
		Working: IconConfig{Icon: "▶", Label: "working"},
		Blocked: IconConfig{Icon: "◆", Label: "blocked"},
		Aborted: IconConfig{Icon: "■", Label: "aborted"},
		Done:    IconConfig{Icon: "✓", Label: "done"},
		Idle:    IconConfig{Icon: "◌", Label: "idle"},
		Unknown: IconConfig{Icon: "?", Label: "unknown"},
	}
}

func normalizeAgentStatesConfig(states *AgentStatesConfig, defaults AgentStatesConfig) {
	normalizeAgentStateIcon(&states.Working, defaults.Working)
	normalizeAgentStateIcon(&states.Blocked, defaults.Blocked)
	normalizeAgentStateIcon(&states.Aborted, defaults.Aborted)
	normalizeAgentStateIcon(&states.Done, defaults.Done)
	normalizeAgentStateIcon(&states.Idle, defaults.Idle)
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
		Aborted: sessionmgr.IconStyle{
			Icon:  states.Aborted.Icon,
			ASCII: states.Aborted.Label,
			Color: states.Aborted.Color,
		},
		Done: sessionmgr.IconStyle{
			Icon:  states.Done.Icon,
			ASCII: states.Done.Label,
			Color: states.Done.Color,
		},
		Idle: sessionmgr.IconStyle{
			Icon:  states.Idle.Icon,
			ASCII: states.Idle.Label,
			Color: states.Idle.Color,
		},
		Unknown: sessionmgr.IconStyle{
			Icon:  states.Unknown.Icon,
			ASCII: states.Unknown.Label,
			Color: states.Unknown.Color,
		},
	}
}

func normalizeKindIcon(
	icon *IconConfig,
	defaults IconConfig,
	bareIcon string,
	legacyIcons ...string,
) {
	if strings.TrimSpace(icon.Icon) == "" {
		icon.Icon = defaults.Icon
	}
	if icon.Icon == bareIcon {
		icon.Icon = defaults.Icon
	}
	for _, legacy := range legacyIcons {
		if icon.Icon == legacy {
			icon.Icon = defaults.Icon
			break
		}
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
