package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

func TestAgentLogoUsesVendorCodepointOrder(t *testing.T) {
	tests := []struct {
		name string
		want rune
	}{
		{"claude", 0xE1A0},
		{"codex", 0xE1A1},
		{"opencode", 0xE1A2},
		{"pi", 0xE1A9},
		{"qodercli", 0xE1B6},
		{"glm", 0xE1B7},
		{"unknown-agent", 0},
	}
	for _, tt := range tests {
		got := agentLogo(tt.name)
		if tt.want == 0 {
			if got != "" {
				t.Fatalf("agentLogo(%q) = %q, want empty", tt.name, got)
			}
			continue
		}
		if got != string(tt.want) {
			t.Fatalf("agentLogo(%q) = %q (%U), want %U", tt.name, got, []rune(got), tt.want)
		}
	}
}

func TestAgentLogoAliasesAndNormalization(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"antigravity", "agy"},
		{"cursor-agent", "cursor"},
		{"kiro-cli", "kiro"},
		{"open-code", "opencode"},
		{"Claude Code", "claude"},
		{"claude:sonnet", "claude"},
		{"  codex  ", "codex"},
	}
	for _, tt := range tests {
		if got := agentKey(tt.name); got != tt.want {
			t.Fatalf("agentKey(%q) = %q, want %q", tt.name, got, tt.want)
		}
		if want := agentLogo(tt.want); want == "" {
			t.Fatalf("agentLogo(%q) has no glyph", tt.want)
		} else if got := agentLogo(tt.name); got != want {
			t.Fatalf("agentLogo(%q) = %q, want %q", tt.name, got, want)
		}
	}
}

func TestAgentColorUsesBrandHexOrDefault(t *testing.T) {
	if got, ok := agentColor("claude").(lipgloss.Color); !ok || string(got) != "#D97757" {
		t.Fatalf("agentColor(claude) = %#v, want #D97757", agentColor("claude"))
	}
	if got, ok := agentColor("opencode").(lipgloss.Color); !ok || string(got) != "#38BDF8" {
		t.Fatalf("agentColor(opencode) = %#v, want #38BDF8", agentColor("opencode"))
	}
	if _, ok := agentColor("pi").(lipgloss.NoColor); !ok {
		t.Fatalf("agentColor(pi) = %#v, want NoColor", agentColor("pi"))
	}
}

func TestShowAgentLogosHonorsEnvAndIconMode(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		mode    string
		wantLog bool
	}{
		{"font mode forces logos", "font", appconfig.IconModeIcons, true},
		{"none mode suppresses logos", "none", appconfig.IconModeIcons, false},
		{"text icon mode suppresses private-use glyphs", "font", appconfig.IconModeText, false},
		{"icons disabled suppresses logos", "font", appconfig.IconModeNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SESHAGY_AGENT_ICONS", tt.env)
			cfg := appconfig.Default()
			cfg.Icons.Mode = tt.mode
			m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
			if got := m.showAgentLogos(); got != tt.wantLog {
				t.Fatalf("showAgentLogos() = %v, want %v", got, tt.wantLog)
			}
		})
	}
}

func TestRowIconNeverPrintsPrivateUseGlyphsInTextMode(t *testing.T) {
	t.Setenv("SESHAGY_AGENT_ICONS", "font")
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	item := agentItem("claude", "%1", "frontend")
	if got := m.rowIcon(item); got != "" {
		t.Fatalf("rowIcon() = %q, want empty in text mode", got)
	}
}
