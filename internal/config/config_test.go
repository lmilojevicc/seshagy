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
	if cfg.Theme.Colors.FocusedBorder != "13" || cfg.Theme.Colors.ActiveTab != "default" {
		t.Fatalf("theme color defaults = %#v", cfg.Theme.Colors)
	}
	if got := cfg.Sources.Order; strings.Join(got, ",") != "all,sessions,agents,current-agents,zoxide,fd" {
		t.Fatalf("default source order = %#v", got)
	}
	if cfg.Icons.Mode != IconModeIcons {
		t.Fatalf("icon mode default = %q, want %q", cfg.Icons.Mode, IconModeIcons)
	}
	icons := cfg.IconSet()
	if got := icons.For(sessionmgr.KindSession).Text; got != sessionmgr.IconSession {
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
	if !strings.Contains(string(data), `mode = "text"`) || !strings.Contains(string(data), `label = "X"`) || !strings.Contains(string(data), `#a6e3a1`) {
		t.Fatalf("saved config missing text mode, label, or hex color: %s", data)
	}
	if !strings.Contains(string(data), `[sources]`) || !strings.Contains(string(data), `current-agents`) {
		t.Fatalf("saved config missing source config: %s", data)
	}
	if !strings.Contains(string(data), `[directories]`) || !strings.Contains(string(data), `fd_command`) {
		t.Fatalf("saved config missing directory config: %s", data)
	}
	if !strings.Contains(string(data), `[theme.colors]`) || !strings.Contains(string(data), `#ff79c6`) || !strings.Contains(string(data), `#f5c2e7`) {
		t.Fatalf("saved config missing theme colors: %s", data)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.DefaultSource() != sessionmgr.ModeCurrentAgents {
		t.Fatalf("loaded default source = %v, want current agents", loaded.DefaultSource())
	}
	if order := loaded.SourceOrder(); len(order) != 6 || order[0] != sessionmgr.ModeSessions || order[2] != sessionmgr.ModeCurrentAgents || order[5] != sessionmgr.ModeAll {
		t.Fatalf("loaded source order = %#v", order)
	}
	if loaded.LoadOptions().FDCommand != `printf '%s\n' /tmp/project` {
		t.Fatalf("loaded fd command = %q", loaded.LoadOptions().FDCommand)
	}
	if loaded.Theme.Colors.FocusedBorder != "#ff79c6" || loaded.Theme.Colors.ActiveTab != "#f5c2e7" {
		t.Fatalf("loaded theme colors = %#v", loaded.Theme.Colors)
	}
	if !loaded.TypeFirst.Enabled || loaded.PrefixKey() != "alt+x" || !loaded.Setup.TypeFirstPromptSeen {
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

func TestIconModes(t *testing.T) {
	cfg := Default()
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != sessionmgr.IconAgent {
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
		t.Fatalf("legacy ascii config migrated to mode=%q label=%q", loaded.Icons.Mode, loaded.Icons.Session.Label)
	}
	saved, err := Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(saved), "ascii") || strings.Contains(string(saved), "[icons]\n  enabled") {
		t.Fatalf("migrated config should omit legacy ascii/enabled keys: %s", saved)
	}
}
