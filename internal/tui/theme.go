package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

type palette struct {
	bg       lipgloss.TerminalColor
	bgAlt    lipgloss.TerminalColor
	fg       lipgloss.TerminalColor
	muted    lipgloss.TerminalColor
	border   lipgloss.TerminalColor
	mauve    lipgloss.TerminalColor
	peach    lipgloss.TerminalColor
	green    lipgloss.TerminalColor
	teal     lipgloss.TerminalColor
	sky      lipgloss.TerminalColor
	red      lipgloss.TerminalColor
	yellow   lipgloss.TerminalColor
	lavender lipgloss.TerminalColor
}

type styles struct {
	p palette

	app         lipgloss.Style
	tabActive   lipgloss.Style
	tabInactive lipgloss.Style
	pane        lipgloss.Style
	paneFocus   lipgloss.Style
	title       lipgloss.Style
	subtitle    lipgloss.Style
	muted       lipgloss.Style
	emphasis    lipgloss.Style
	key         lipgloss.Style
	iconSession lipgloss.Style
	iconZoxide  lipgloss.Style
	iconFD      lipgloss.Style
	iconAgent   lipgloss.Style
	selectedBG  lipgloss.Style
	bar         lipgloss.Style
	status      lipgloss.Style
	success     lipgloss.Style
	info        lipgloss.Style
	warning     lipgloss.Style
	danger      lipgloss.Style
}

func defaultStyles() styles {
	return stylesFromConfig(appconfig.Default())
}

func stylesFromConfig(cfg appconfig.Config) styles {
	cfg.Normalize()
	colors := cfg.Theme.Colors

	// Use terminal-default foreground/background plus the terminal's ANSI color
	// palette for accents. This lets seshagy follow the user's terminal theme
	// instead of painting a fixed Catppuccin surface over it.
	p := palette{
		bg:       lipgloss.NoColor{},
		bgAlt:    lipgloss.NoColor{},
		fg:       lipgloss.NoColor{},
		muted:    lipgloss.Color("8"),
		border:   lipgloss.Color("8"),
		mauve:    lipgloss.Color("13"),
		peach:    lipgloss.Color("11"),
		green:    lipgloss.Color("10"),
		teal:     lipgloss.Color("6"),
		sky:      lipgloss.Color("14"),
		red:      lipgloss.Color("9"),
		yellow:   lipgloss.Color("11"),
		lavender: lipgloss.Color("12"),
	}
	focusedBorder := themeColor(colors.FocusedBorder, p.mauve)
	activeTab := themeColor(colors.ActiveTab, p.fg)

	s := styles{p: p}
	s.app = lipgloss.NewStyle().Foreground(p.fg).Background(p.bg)
	s.tabActive = lipgloss.NewStyle().Foreground(activeTab).Bold(true)
	s.tabInactive = lipgloss.NewStyle().Foreground(p.muted)
	s.pane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.border).Padding(0, 1)
	s.paneFocus = s.pane.BorderForeground(focusedBorder)
	s.title = lipgloss.NewStyle().Foreground(p.lavender).Bold(true)
	s.subtitle = lipgloss.NewStyle().Foreground(p.muted)
	s.muted = lipgloss.NewStyle().Foreground(p.muted)
	s.emphasis = lipgloss.NewStyle().Foreground(p.mauve).Bold(true)
	s.key = lipgloss.NewStyle().Foreground(p.peach).Bold(true)
	s.iconSession = lipgloss.NewStyle().Foreground(p.green).Bold(true)
	s.iconZoxide = lipgloss.NewStyle().Foreground(p.sky).Bold(true)
	s.iconFD = lipgloss.NewStyle().Foreground(p.peach).Bold(true)
	s.iconAgent = lipgloss.NewStyle().Foreground(p.mauve).Bold(true)
	s.selectedBG = lipgloss.NewStyle().Reverse(true)
	s.bar = lipgloss.NewStyle().Foreground(p.mauve)
	s.status = lipgloss.NewStyle().Foreground(p.fg).Background(p.bgAlt).Padding(0, 1)
	s.success = lipgloss.NewStyle().Foreground(p.green).Bold(true)
	s.info = lipgloss.NewStyle().Foreground(p.sky).Bold(true)
	s.warning = lipgloss.NewStyle().Foreground(p.yellow).Bold(true)
	s.danger = lipgloss.NewStyle().Foreground(p.red).Bold(true)
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
