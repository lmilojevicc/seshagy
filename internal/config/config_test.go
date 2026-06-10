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
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
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
