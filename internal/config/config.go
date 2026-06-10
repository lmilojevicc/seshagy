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
)

const (
	appDirName     = "seshagy"
	configFileName = "config.toml"
	DefaultPrefix  = "ctrl+x"
)

type Config struct {
	Sources   SourcesConfig   `toml:"sources"`
	Icons     IconsConfig     `toml:"icons"`
	TypeFirst TypeFirstConfig `toml:"type_first"`
	Setup     SetupConfig     `toml:"setup"`
}

type SourcesConfig struct {
	Default string   `toml:"default"`
	Order   []string `toml:"order"`
}

type IconsConfig struct {
	Enabled bool       `toml:"enabled"`
	ASCII   bool       `toml:"ascii"`
	Session IconConfig `toml:"session"`
	Zoxide  IconConfig `toml:"zoxide"`
	FD      IconConfig `toml:"fd"`
	Agent   IconConfig `toml:"agent"`
}

type IconConfig struct {
	Icon  string `toml:"icon"`
	ASCII string `toml:"ascii"`
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
		Sources: SourcesConfig{Default: "all", Order: defaultSourceOrderNames()},
		Icons: IconsConfig{
			Enabled: true,
			ASCII:   false,
			Session: IconConfig{Icon: sessionmgr.IconSession, ASCII: "S", Color: "10"},
			Zoxide:  IconConfig{Icon: sessionmgr.IconZoxide, ASCII: "Z", Color: "14"},
			FD:      IconConfig{Icon: sessionmgr.IconFD, ASCII: "F", Color: "11"},
			Agent:   IconConfig{Icon: sessionmgr.IconAgent, ASCII: "A", Color: "13"},
		},
		TypeFirst: TypeFirstConfig{Enabled: false, Prefix: DefaultPrefix},
	}
}

func Path() string {
	return filepath.Join(configHome(), appDirName, configFileName)
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
	if strings.TrimSpace(c.Icons.Session.Icon) == "" {
		c.Icons.Session.Icon = defaults.Icons.Session.Icon
	}
	if strings.TrimSpace(c.Icons.Session.ASCII) == "" {
		c.Icons.Session.ASCII = defaults.Icons.Session.ASCII
	}
	if strings.TrimSpace(c.Icons.Session.Color) == "" {
		c.Icons.Session.Color = defaults.Icons.Session.Color
	}
	if strings.TrimSpace(c.Icons.Zoxide.Icon) == "" {
		c.Icons.Zoxide.Icon = defaults.Icons.Zoxide.Icon
	}
	if strings.TrimSpace(c.Icons.Zoxide.ASCII) == "" {
		c.Icons.Zoxide.ASCII = defaults.Icons.Zoxide.ASCII
	}
	if strings.TrimSpace(c.Icons.Zoxide.Color) == "" {
		c.Icons.Zoxide.Color = defaults.Icons.Zoxide.Color
	}
	if strings.TrimSpace(c.Icons.FD.Icon) == "" {
		c.Icons.FD.Icon = defaults.Icons.FD.Icon
	}
	if strings.TrimSpace(c.Icons.FD.ASCII) == "" {
		c.Icons.FD.ASCII = defaults.Icons.FD.ASCII
	}
	if strings.TrimSpace(c.Icons.FD.Color) == "" {
		c.Icons.FD.Color = defaults.Icons.FD.Color
	}
	if strings.TrimSpace(c.Icons.Agent.Icon) == "" {
		c.Icons.Agent.Icon = defaults.Icons.Agent.Icon
	}
	if strings.TrimSpace(c.Icons.Agent.ASCII) == "" {
		c.Icons.Agent.ASCII = defaults.Icons.Agent.ASCII
	}
	if strings.TrimSpace(c.Icons.Agent.Color) == "" {
		c.Icons.Agent.Color = defaults.Icons.Agent.Color
	}
	if strings.TrimSpace(c.TypeFirst.Prefix) == "" {
		c.TypeFirst.Prefix = DefaultPrefix
	}
}

func (c Config) IconSet() sessionmgr.IconSet {
	c.Normalize()
	return sessionmgr.IconSet{
		Enabled: c.Icons.Enabled,
		ASCII:   c.Icons.ASCII,
		Session: sessionmgr.IconStyle{Icon: c.Icons.Session.Icon, ASCII: c.Icons.Session.ASCII, Color: c.Icons.Session.Color},
		Zoxide:  sessionmgr.IconStyle{Icon: c.Icons.Zoxide.Icon, ASCII: c.Icons.Zoxide.ASCII, Color: c.Icons.Zoxide.Color},
		FD:      sessionmgr.IconStyle{Icon: c.Icons.FD.Icon, ASCII: c.Icons.FD.ASCII, Color: c.Icons.FD.Color},
		Agent:   sessionmgr.IconStyle{Icon: c.Icons.Agent.Icon, ASCII: c.Icons.Agent.ASCII, Color: c.Icons.Agent.Color},
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

func SourceModeName(mode sessionmgr.SourceMode) string {
	switch mode {
	case sessionmgr.ModeSessions:
		return "sessions"
	case sessionmgr.ModeAgents:
		return "agents"
	case sessionmgr.ModeCurrentAgents:
		return "current-agents"
	case sessionmgr.ModeZoxide:
		return "zoxide"
	case sessionmgr.ModeFD:
		return "fd"
	default:
		return "all"
	}
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

func configHome() string {
	if value := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); value != "" {
		return expandHome(value)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config")
	}
	return "."
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
		return "."
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
