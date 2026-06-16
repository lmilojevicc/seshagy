package integrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONObjectMissingAndInvalid(t *testing.T) {
	root, err := readJSONObject(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(root) != 0 {
		t.Fatalf("missing file = (%#v, %v), want empty map", root, err)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJSONObject(invalid); err == nil {
		t.Fatal("readJSONObject() expected parse error")
	}
}

func TestInstallMissingConfigDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, home string)
		install   func(string) ([]string, error)
		wantError string
	}{
		{
			name:      "grok",
			install:   installGrok,
			wantError: "grok config directory not found",
		},
		{
			name:      "droid",
			install:   installDroid,
			wantError: "config directory not found",
		},
		{
			name:      "codex",
			install:   installCodex,
			wantError: "codex config directory not found",
		},
		{
			name:      "copilot",
			install:   installCopilot,
			wantError: "copilot config directory not found",
		},
		{
			name:      "cursor",
			install:   installCursor,
			wantError: "cursor config directory not found",
		},
		{
			name:      "kimi",
			install:   installKimi,
			wantError: "kimi code config directory not found",
		},
		{
			name:      "hermes",
			install:   installHermes,
			wantError: "hermes config directory not found",
		},
		{
			name: "opencode",
			setup: func(t *testing.T, home string) {
				t.Helper()
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			},
			install:   installOpencode,
			wantError: "OpenCode config directory not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.setup != nil {
				tt.setup(t, home)
			}

			_, err := tt.install("/bin/seshagy")
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("install() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestInstallGrokWritesHookAndRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}

	messages, err := installGrok("/bin/seshagy")
	if err != nil {
		t.Fatalf("installGrok() error = %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("installGrok() messages = %#v", messages)
	}
	hookPath := filepath.Join(home, ".grok", "hooks", shellHookName)
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("grok hook missing at %s: %v", hookPath, err)
	}
	registryPath := filepath.Join(home, ".grok", "hooks", grokHooksRegistryName)
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("grok hooks registry missing at %s: %v", registryPath, err)
	}
}

func TestInstallCopilotWritesHookAndRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}

	messages, err := installCopilot("/bin/seshagy")
	if err != nil {
		t.Fatalf("installCopilot() error = %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("installCopilot() messages = %#v", messages)
	}
	hookPath := filepath.Join(home, ".copilot", "hooks", shellHookName)
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("copilot hook missing at %s: %v", hookPath, err)
	}
}

func TestRemoveDirMissingPath(t *testing.T) {
	removed, err := removeDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("removeDir() error = %v", err)
	}
	if !removed {
		t.Fatal("removeDir() removed = false, want true for idempotent RemoveAll")
	}
}

func TestRemoveDirDeletesExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.py"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := removeDir(dir)
	if err != nil || !removed {
		t.Fatalf("removeDir() = (%v, %v), want (true, nil)", removed, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory should be removed, stat err=%v", err)
	}
}

func TestInstallCodexPartialFailureLeavesHookWithoutRegistry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	codex := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(codex, "hooks.json")
	invalid := "{not json"
	if err := os.WriteFile(hooksPath, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := installCodex("/bin/seshagy"); err == nil {
		t.Fatal("installCodex() expected error for invalid hooks.json")
	} else if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("installCodex() error = %v, want parse failure", err)
	}

	hookPath := filepath.Join(codex, shellHookName)
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook should be written before JSON failure at %s: %v", hookPath, err)
	}
	if got := readFile(t, hooksPath); got != invalid {
		t.Fatalf("hooks.json changed on partial failure:\ngot=%q\nwant=%q", got, invalid)
	}

	rec := findRec(t, Scan(), TargetCodex)
	if rec.State != StatusCurrent {
		t.Fatalf("Scan() codex state = %q, want current after hook write", rec.State)
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook should exist for status scan: %v", err)
	}
	if _, err := readJSONObject(hooksPath); err == nil {
		t.Fatal("hooks.json should remain invalid after partial install")
	}
}
