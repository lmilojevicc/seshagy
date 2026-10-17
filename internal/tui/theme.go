package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

// palette holds the terminal's ANSI colors the picker draws with by default.
// The picker follows the user's terminal theme (default fg/bg plus the ANSI
// palette) and lets [theme.colors] override individual accents.
type palette struct {
	bg     lipgloss.TerminalColor
	fg     lipgloss.TerminalColor
	muted  lipgloss.TerminalColor
	mauve  lipgloss.TerminalColor
	peach  lipgloss.TerminalColor
	green  lipgloss.TerminalColor
	sky    lipgloss.TerminalColor
	red    lipgloss.TerminalColor
	yellow lipgloss.TerminalColor

	// selection is the background bar under the selected result row.
	selection lipgloss.TerminalColor
}

type styles struct {
	p palette

	app        lipgloss.Style
	itemName   lipgloss.Style
	title      lipgloss.Style
	muted      lipgloss.Style
	emphasis   lipgloss.Style
	key        lipgloss.Style
	bar        lipgloss.Style
	match      lipgloss.Style
	rule       lipgloss.Style
	chipActive lipgloss.Style
	chipIdle   lipgloss.Style
	success    lipgloss.Style
	info       lipgloss.Style
	warning    lipgloss.Style
	danger     lipgloss.Style
	pane       lipgloss.Style
	panePopup  lipgloss.Style

	// selectionBG is the selected row's bar color; the rows paint it
	// themselves with paintBackground so inner resets cannot break the bar.
	selectionBG lipgloss.TerminalColor

	// Telescope chrome: window borders and titles, and the needs-you accent.
	border       lipgloss.Style
	titlePrompt  lipgloss.Style
	titleResults lipgloss.Style
	titlePreview lipgloss.Style
	needsYou     lipgloss.Style
}

func stylesFromConfig(cfg appconfig.Config) styles {
	cfg.Normalize()
	colors := cfg.Theme.Colors

	p := palette{
		bg:     lipgloss.NoColor{},
		fg:     lipgloss.NoColor{},
		muted:  lipgloss.Color("8"),
		mauve:  lipgloss.Color("13"),
		peach:  lipgloss.Color("11"),
		green:  lipgloss.Color("10"),
		sky:    lipgloss.Color("14"),
		red:    lipgloss.Color("9"),
		yellow: lipgloss.Color("11"),

		selection: lipgloss.Color("236"),
	}
	muted := themeColor(colors.Muted, p.muted)
	border := themeColor(colors.Border, p.muted)
	accent := themeColor(colors.Accent, p.mauve)
	popupTitle := themeColor(colors.PopupTitle, p.mauve)
	key := themeColor(colors.Key, p.peach)
	success := themeColor(colors.Success, p.green)
	info := themeColor(colors.Info, p.sky)
	warning := themeColor(colors.Warning, p.yellow)
	danger := themeColor(colors.Danger, p.red)

	s := styles{p: p}
	s.app = lipgloss.NewStyle().Foreground(p.fg).Background(p.bg)
	s.itemName = lipgloss.NewStyle().Foreground(p.fg)
	s.title = lipgloss.NewStyle().Foreground(popupTitle).Bold(true)
	s.muted = lipgloss.NewStyle().Foreground(muted)
	s.emphasis = lipgloss.NewStyle().Foreground(accent).Bold(true)
	s.key = lipgloss.NewStyle().Foreground(key).Bold(true)
	s.bar = lipgloss.NewStyle().Foreground(accent)
	s.match = lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true)
	s.rule = lipgloss.NewStyle().Foreground(muted)
	s.chipActive = lipgloss.NewStyle().Foreground(accent).Bold(true)
	s.chipIdle = lipgloss.NewStyle().Foreground(muted)
	s.success = lipgloss.NewStyle().Foreground(success).Bold(true)
	s.info = lipgloss.NewStyle().Foreground(info).Bold(true)
	s.warning = lipgloss.NewStyle().Foreground(warning).Bold(true)
	s.danger = lipgloss.NewStyle().Foreground(danger).Bold(true)
	s.selectionBG = themeColor(colors.SelectionBG, p.selection)
	s.pane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)
	s.panePopup = s.pane.BorderForeground(themeColor(colors.PopupBorder, popupTitle))
	s.border = lipgloss.NewStyle().Foreground(border)
	s.titlePrompt = lipgloss.NewStyle().Foreground(info).Bold(true)
	s.titleResults = lipgloss.NewStyle().Foreground(danger).Bold(true)
	s.titlePreview = lipgloss.NewStyle().Foreground(success).Bold(true)
	s.needsYou = lipgloss.NewStyle().Foreground(danger).Bold(true)
	return s
}

func themeColor(value string, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if strings.EqualFold(value, "default") {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(value)
}
