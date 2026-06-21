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
	Theme       ThemeConfig       `toml:"theme"       json:"theme"`
	Icons       IconsConfig       `toml:"icons"       json:"icons"`
	TypeFirst   TypeFirstConfig   `toml:"type_first"  json:"type_first"`
	Setup       SetupConfig       `toml:"setup"       json:"setup"`
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
	Mode          string           `toml:"mode"                      json:"mode"`
	TmuxStateMode string           `toml:"tmux_state_mode,omitempty" json:"tmux_state_mode,omitempty"`
	Enabled       *bool            `toml:"enabled,omitempty"         json:"enabled,omitempty"`
	ASCII         bool             `toml:"ascii,omitempty"           json:"ascii,omitempty"`
	Session       IconConfig       `toml:"session"                   json:"session"`
	Zoxide        IconConfig       `toml:"zoxide"                    json:"zoxide"`
	FD            IconConfig       `toml:"fd"                        json:"fd"`
	TmuxState     TmuxStatesConfig `toml:"tmux_state"                json:"tmux_state"`
}

type TmuxStatesConfig struct {
	Attached IconConfig `toml:"attached" json:"attached"`
	Detached IconConfig `toml:"detached" json:"detached"`
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
			Mode:      IconModeIcons,
			TmuxState: defaultTmuxStatesConfig(),
			Session:   IconConfig{Icon: sessionmgr.IconSession + " ", Label: "S", Color: "10"},
			Zoxide:    IconConfig{Icon: sessionmgr.IconZoxide + " ", Label: "Z", Color: "14"},
			FD:        IconConfig{Icon: sessionmgr.IconFD + " ", Label: "F", Color: "11"},
		},
		TypeFirst: TypeFirstConfig{Enabled: false, Prefix: DefaultPrefix},
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
		Enabled:       enabled,
		ASCII:         c.Icons.Mode == IconModeText,
		TmuxStateMode: c.Icons.TmuxStateMode,
		TmuxStates:    projectTmuxStateStyles(c.Icons.TmuxState),
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
		FDCommand: c.Directories.FDCommand,
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
