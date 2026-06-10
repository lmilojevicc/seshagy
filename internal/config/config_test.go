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
	icons := cfg.IconSet()
	if got := icons.For(sessionmgr.KindSession).Text; got != sessionmgr.IconSession {
		t.Fatalf("session icon = %q", got)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Default()
	cfg.Icons.Enabled = false
	cfg.Icons.Session.ASCII = "X"
	cfg.Icons.Session.Color = "9"
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
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !loaded.TypeFirst.Enabled || loaded.PrefixKey() != "alt+x" || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("loaded config = %#v", loaded)
	}
	if got := loaded.IconSet().For(sessionmgr.KindSession).Text; got != "X" {
		t.Fatalf("ascii session icon = %q", got)
	}
}
