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
)

type Config struct {
	Sources     SourcesConfig     `toml:"sources"`
	Directories DirectoriesConfig `toml:"directories"`
	Theme       ThemeConfig       `toml:"theme"`
	Icons       IconsConfig       `toml:"icons"`
	TypeFirst   TypeFirstConfig   `toml:"type_first"`
	Setup       SetupConfig       `toml:"setup"`
}

type SourcesConfig struct {
	Default string   `toml:"default"`
	Order   []string `toml:"order"`
}

type DirectoriesConfig struct {
	FDCommand string `toml:"fd_command"`
}

type ThemeConfig struct {
	Colors ThemeColorsConfig `toml:"colors"`
}

type ThemeColorsConfig struct {
	FocusedBorder string `toml:"focused_border"`
	ActiveTab     string `toml:"active_tab"`
	Border        string `toml:"border"`
	InactiveTab   string `toml:"inactive_tab"`
	Title         string `toml:"title"`
	Accent        string `toml:"accent"`
	Key           string `toml:"key"`
	Muted         string `toml:"muted"`
	Success       string `toml:"success"`
	Info          string `toml:"info"`
	Warning       string `toml:"warning"`
	Danger        string `toml:"danger"`
}

type IconsConfig struct {
	Mode    string     `toml:"mode"`
	Enabled *bool      `toml:"enabled,omitempty"`
	ASCII   bool       `toml:"ascii,omitempty"`
	Session IconConfig `toml:"session"`
	Zoxide  IconConfig `toml:"zoxide"`
	FD      IconConfig `toml:"fd"`
	Agent   IconConfig `toml:"agent"`
}

type IconConfig struct {
	Icon  string `toml:"icon"`
	Label string `toml:"label"`
	ASCII string `toml:"ascii,omitempty"`
	Color string `toml:"color"`
}

type TypeFirstConfig struct {
	Enabled bool   `toml:"enabled"`
	Prefix  string `toml:"prefix"`
}

type SetupConfig struct {
	TypeFirstPromptSeen bool `toml:"type_first_prompt_seen"`
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
			Mode:    IconModeIcons,
			Session: IconConfig{Icon: sessionmgr.IconSession + " ", Label: "S", Color: "10"},
			Zoxide:  IconConfig{Icon: sessionmgr.IconZoxide + " ", Label: "Z", Color: "14"},
			FD:      IconConfig{Icon: sessionmgr.IconFD + " ", Label: "F", Color: "11"},
			Agent:   IconConfig{Icon: sessionmgr.IconAgent + "  ", Label: "A", Color: "13"},
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
	if c.Icons.Enabled != nil && !*c.Icons.Enabled {
		c.Icons.Mode = IconModeNone
	} else if c.Icons.ASCII {
		c.Icons.Mode = IconModeText
	}
	c.Icons.Enabled = nil
	c.Icons.ASCII = false
	if strings.TrimSpace(c.Icons.Session.Icon) == "" {
		c.Icons.Session.Icon = defaults.Icons.Session.Icon
	}
	if c.Icons.Session.Icon == sessionmgr.IconSession {
		c.Icons.Session.Icon = defaults.Icons.Session.Icon
	}
	if legacy := strings.TrimSpace(
		c.Icons.Session.ASCII,
	); legacy != "" &&
		(strings.TrimSpace(c.Icons.Session.Label) == "" || c.Icons.Session.Label == defaults.Icons.Session.Label) {
		c.Icons.Session.Label = legacy
	}
	if strings.TrimSpace(c.Icons.Session.Label) == "" {
		c.Icons.Session.Label = strings.TrimSpace(c.Icons.Session.ASCII)
	}
	if strings.TrimSpace(c.Icons.Session.Label) == "" {
		c.Icons.Session.Label = defaults.Icons.Session.Label
	}
	c.Icons.Session.ASCII = ""
	if strings.TrimSpace(c.Icons.Session.Color) == "" {
		c.Icons.Session.Color = defaults.Icons.Session.Color
	}
	if strings.TrimSpace(c.Icons.Zoxide.Icon) == "" {
		c.Icons.Zoxide.Icon = defaults.Icons.Zoxide.Icon
	}
	if c.Icons.Zoxide.Icon == sessionmgr.IconZoxide {
		c.Icons.Zoxide.Icon = defaults.Icons.Zoxide.Icon
	}
	if legacy := strings.TrimSpace(
		c.Icons.Zoxide.ASCII,
	); legacy != "" &&
		(strings.TrimSpace(c.Icons.Zoxide.Label) == "" || c.Icons.Zoxide.Label == defaults.Icons.Zoxide.Label) {
		c.Icons.Zoxide.Label = legacy
	}
	if strings.TrimSpace(c.Icons.Zoxide.Label) == "" {
		c.Icons.Zoxide.Label = strings.TrimSpace(c.Icons.Zoxide.ASCII)
	}
	if strings.TrimSpace(c.Icons.Zoxide.Label) == "" {
		c.Icons.Zoxide.Label = defaults.Icons.Zoxide.Label
	}
	c.Icons.Zoxide.ASCII = ""
	if strings.TrimSpace(c.Icons.Zoxide.Color) == "" {
		c.Icons.Zoxide.Color = defaults.Icons.Zoxide.Color
	}
	if strings.TrimSpace(c.Icons.FD.Icon) == "" {
		c.Icons.FD.Icon = defaults.Icons.FD.Icon
	}
	if c.Icons.FD.Icon == sessionmgr.IconFD {
		c.Icons.FD.Icon = defaults.Icons.FD.Icon
	}
	if legacy := strings.TrimSpace(
		c.Icons.FD.ASCII,
	); legacy != "" &&
		(strings.TrimSpace(c.Icons.FD.Label) == "" || c.Icons.FD.Label == defaults.Icons.FD.Label) {
		c.Icons.FD.Label = legacy
	}
	if strings.TrimSpace(c.Icons.FD.Label) == "" {
		c.Icons.FD.Label = strings.TrimSpace(c.Icons.FD.ASCII)
	}
	if strings.TrimSpace(c.Icons.FD.Label) == "" {
		c.Icons.FD.Label = defaults.Icons.FD.Label
	}
	c.Icons.FD.ASCII = ""
	if strings.TrimSpace(c.Icons.FD.Color) == "" {
		c.Icons.FD.Color = defaults.Icons.FD.Color
	}
	if strings.TrimSpace(c.Icons.Agent.Icon) == "" {
		c.Icons.Agent.Icon = defaults.Icons.Agent.Icon
	}
	if c.Icons.Agent.Icon == sessionmgr.IconAgent ||
		c.Icons.Agent.Icon == sessionmgr.IconAgent+" " ||
		c.Icons.Agent.Icon == "󰚩" {
		c.Icons.Agent.Icon = defaults.Icons.Agent.Icon
	}
	if legacy := strings.TrimSpace(
		c.Icons.Agent.ASCII,
	); legacy != "" &&
		(strings.TrimSpace(c.Icons.Agent.Label) == "" || c.Icons.Agent.Label == defaults.Icons.Agent.Label) {
		c.Icons.Agent.Label = legacy
	}
	if strings.TrimSpace(c.Icons.Agent.Label) == "" {
		c.Icons.Agent.Label = strings.TrimSpace(c.Icons.Agent.ASCII)
	}
	if strings.TrimSpace(c.Icons.Agent.Label) == "" {
		c.Icons.Agent.Label = defaults.Icons.Agent.Label
	}
	c.Icons.Agent.ASCII = ""
	if strings.TrimSpace(c.Icons.Agent.Color) == "" {
		c.Icons.Agent.Color = defaults.Icons.Agent.Color
	}
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
		Enabled: enabled,
		ASCII:   c.Icons.Mode == IconModeText,
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
	return sessionmgr.LoadOptions{FDCommand: c.Directories.FDCommand}
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
