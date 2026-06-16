package xdg

import (
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	tests := map[string]string{
		"~":              "/home/testuser",
		"~/projects":     "/home/testuser/projects",
		"/absolute/path": "/absolute/path",
		"relative/path":  "relative/path",
	}
	for in, want := range tests {
		if got := ExpandHome(in); got != want {
			t.Fatalf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigHome(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	t.Run("default", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		want := filepath.Join("/home/testuser", ".config")
		if got := ConfigHome(); got != want {
			t.Fatalf("ConfigHome() = %q, want %q", got, want)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "~/custom-config")
		want := filepath.Join("/home/testuser", "custom-config")
		if got := ConfigHome(); got != want {
			t.Fatalf("ConfigHome() = %q, want %q", got, want)
		}
	})

	t.Run("absolute env override", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/etc/xdg")
		if got := ConfigHome(); got != "/etc/xdg" {
			t.Fatalf("ConfigHome() = %q, want %q", got, "/etc/xdg")
		}
	})
}

func TestStateHome(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	t.Run("default", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")
		want := filepath.Join("/home/testuser", ".local", "state")
		if got := StateHome(); got != want {
			t.Fatalf("StateHome() = %q, want %q", got, want)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "~/custom-state")
		want := filepath.Join("/home/testuser", "custom-state")
		if got := StateHome(); got != want {
			t.Fatalf("StateHome() = %q, want %q", got, want)
		}
	})

	t.Run("absolute env override", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "/var/state")
		if got := StateHome(); got != "/var/state" {
			t.Fatalf("StateHome() = %q, want %q", got, "/var/state")
		}
	})
}
