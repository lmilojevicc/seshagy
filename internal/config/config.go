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
	Icons     IconsConfig     `toml:"icons"`
	TypeFirst TypeFirstConfig `toml:"type_first"`
	Setup     SetupConfig     `toml:"setup"`
}

type IconsConfig struct {
	Enabled bool       `toml:"enabled"`
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
		Icons: IconsConfig{
			Enabled: true,
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
