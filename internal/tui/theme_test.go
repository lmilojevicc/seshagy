package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

// TestStylesFromConfigChromeDefaults pins the telescope chrome styles to the
// terminal palette: muted borders, a blue prompt title, a red results title, a
// green preview title, a faint gutter, and a bold red needs-you accent.
func TestStylesFromConfigChromeDefaults(t *testing.T) {
	s := stylesFromConfig(appconfig.Default())
	tests := []struct {
		name string
		got  lipgloss.TerminalColor
		want lipgloss.TerminalColor
	}{
		{"border", s.border.GetForeground(), lipgloss.Color("8")},
		{"titlePrompt", s.titlePrompt.GetForeground(), lipgloss.Color("14")},
		{"titleResults", s.titleResults.GetForeground(), lipgloss.Color("9")},
		{"titlePreview", s.titlePreview.GetForeground(), lipgloss.Color("10")},
		{"gutter", s.gutter.GetForeground(), lipgloss.Color("8")},
		{"needsYou", s.needsYou.GetForeground(), lipgloss.Color("9")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("%s foreground = %v, want %v", test.name, test.got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name  string
		style lipgloss.Style
	}{
		{"titlePrompt", s.titlePrompt},
		{"titleResults", s.titleResults},
		{"titlePreview", s.titlePreview},
		{"needsYou", s.needsYou},
	} {
		if !test.style.GetBold() {
			t.Fatalf("%s is not bold", test.name)
		}
	}
	if !s.gutter.GetFaint() {
		t.Fatal("gutter is not faint")
	}
	if s.border.GetBold() || s.gutter.GetBold() {
		t.Fatal("border and gutter must not be bold")
	}
}

// TestStylesFromConfigSelectionBackground pins the bar color under the selected
// result row: the 256-color dark grey default, an ANSI index, a hex override,
// and "default" for a row with no bar at all.
func TestStylesFromConfigSelectionBackground(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  lipgloss.TerminalColor
	}{
		{name: "default dark grey", value: "", want: lipgloss.Color("236")},
		{name: "ansi index", value: "4", want: lipgloss.Color("4")},
		{name: "hex", value: "#283457", want: lipgloss.Color("#283457")},
		{
			name:  "the default keyword leaves the bar unpainted",
			value: "default",
			want:  lipgloss.NoColor{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.Theme.Colors.SelectionBG = test.value
			if got := stylesFromConfig(cfg).selectionBG; got != test.want {
				t.Fatalf("selectionBG = %v, want %v", got, test.want)
			}
		})
	}
}

// TestStylesFromConfigChromeOverrides verifies [theme.colors] still rethemes the
// chrome: each new style follows the existing key it inherits from.
func TestStylesFromConfigChromeOverrides(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*appconfig.ThemeColorsConfig)
		style  func(styles) lipgloss.Style
		want   lipgloss.TerminalColor
	}{
		{
			name:   "border follows theme.colors.border",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Border = "#111111" },
			style:  func(s styles) lipgloss.Style { return s.border },
			want:   lipgloss.Color("#111111"),
		},
		{
			name:   "titlePrompt follows theme.colors.info",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Info = "#222222" },
			style:  func(s styles) lipgloss.Style { return s.titlePrompt },
			want:   lipgloss.Color("#222222"),
		},
		{
			name:   "titleResults follows theme.colors.danger",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Danger = "#333333" },
			style:  func(s styles) lipgloss.Style { return s.titleResults },
			want:   lipgloss.Color("#333333"),
		},
		{
			name:   "titlePreview follows theme.colors.success",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Success = "#444444" },
			style:  func(s styles) lipgloss.Style { return s.titlePreview },
			want:   lipgloss.Color("#444444"),
		},
		{
			name:   "gutter follows theme.colors.muted",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Muted = "#555555" },
			style:  func(s styles) lipgloss.Style { return s.gutter },
			want:   lipgloss.Color("#555555"),
		},
		{
			name:   "needsYou follows theme.colors.danger",
			mutate: func(c *appconfig.ThemeColorsConfig) { c.Danger = "#333333" },
			style:  func(s styles) lipgloss.Style { return s.needsYou },
			want:   lipgloss.Color("#333333"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := appconfig.Default()
			test.mutate(&cfg.Theme.Colors)
			if got := test.style(stylesFromConfig(cfg)).GetForeground(); got != test.want {
				t.Fatalf("foreground = %v, want %v", got, test.want)
			}
		})
	}
}
