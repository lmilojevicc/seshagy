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
	if got := cfg.Sources.Order; strings.Join(got, ",") != "all,sessions,agents,current-agents,zoxide,fd" {
		t.Fatalf("default source order = %#v", got)
	}
	if !cfg.Icons.Enabled || cfg.Icons.ASCII {
		t.Fatalf("icon mode defaults = enabled:%v ascii:%v, want nerd font icons", cfg.Icons.Enabled, cfg.Icons.ASCII)
	}
	icons := cfg.IconSet()
	if got := icons.For(sessionmgr.KindSession).Text; got != sessionmgr.IconSession {
		t.Fatalf("session icon = %q", got)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	cfg.Icons.ASCII = true
	cfg.Icons.Session.ASCII = "X"
	cfg.Icons.Session.Color = "#a6e3a1"
	cfg.Sources.Default = "current-agents"
	cfg.Sources.Order = []string{"sessions", "agents", "current-agents", "zoxide", "fd", "all"}
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
	if !strings.Contains(string(data), `ascii = true`) || !strings.Contains(string(data), `#a6e3a1`) {
		t.Fatalf("saved config missing ascii mode or hex color: %s", data)
	}
	if !strings.Contains(string(data), `[sources]`) || !strings.Contains(string(data), `current-agents`) {
		t.Fatalf("saved config missing source config: %s", data)
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
	if !loaded.TypeFirst.Enabled || loaded.PrefixKey() != "alt+x" || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("loaded config = %#v", loaded)
	}
	if !loaded.Icons.Enabled || !loaded.Icons.ASCII {
		t.Fatalf("loaded icon mode = enabled:%v ascii:%v, want ascii mode", loaded.Icons.Enabled, loaded.Icons.ASCII)
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

	cfg.Icons.ASCII = true
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != "A" {
		t.Fatalf("ascii agent icon = %q", got)
	}

	cfg.Icons.Enabled = false
	if got := cfg.IconSet().For(sessionmgr.KindAgent).Text; got != "" {
		t.Fatalf("no-icons agent text = %q, want empty", got)
	}
}
