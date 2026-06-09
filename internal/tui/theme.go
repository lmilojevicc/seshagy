package tui

import "github.com/charmbracelet/lipgloss"

type palette struct {
	bg       lipgloss.Color
	bgAlt    lipgloss.Color
	fg       lipgloss.Color
	muted    lipgloss.Color
	border   lipgloss.Color
	selected lipgloss.Color
	mauve    lipgloss.Color
	peach    lipgloss.Color
	green    lipgloss.Color
	teal     lipgloss.Color
	sky      lipgloss.Color
	red      lipgloss.Color
	yellow   lipgloss.Color
	lavender lipgloss.Color
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
	selectedBG  lipgloss.Style
	bar         lipgloss.Style
	status      lipgloss.Style
	success     lipgloss.Style
	info        lipgloss.Style
	warning     lipgloss.Style
	danger      lipgloss.Style
}

func defaultStyles() styles {
	p := palette{
		bg:       "#1e1e2e",
		bgAlt:    "#181825",
		fg:       "#cdd6f4",
		muted:    "#7f849c",
		border:   "#45475a",
		selected: "#313244",
		mauve:    "#cba6f7",
		peach:    "#fab387",
		green:    "#a6e3a1",
		teal:     "#94e2d5",
		sky:      "#89dceb",
		red:      "#f38ba8",
		yellow:   "#f9e2af",
		lavender: "#b4befe",
	}
	s := styles{p: p}
	s.app = lipgloss.NewStyle().Foreground(p.fg).Background(p.bg)
	s.tabActive = lipgloss.NewStyle().Foreground(p.fg).Bold(true)
	s.tabInactive = lipgloss.NewStyle().Foreground(p.muted)
	s.pane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.border).Padding(0, 1)
	s.paneFocus = s.pane.BorderForeground(p.mauve)
	s.title = lipgloss.NewStyle().Foreground(p.lavender).Bold(true)
	s.subtitle = lipgloss.NewStyle().Foreground(p.muted)
	s.muted = lipgloss.NewStyle().Foreground(p.muted)
	s.emphasis = lipgloss.NewStyle().Foreground(p.mauve).Bold(true)
	s.key = lipgloss.NewStyle().Foreground(p.peach).Bold(true)
	s.selectedBG = lipgloss.NewStyle().Background(p.selected)
	s.bar = lipgloss.NewStyle().Foreground(p.mauve)
	s.status = lipgloss.NewStyle().Foreground(p.fg).Background(p.bgAlt).Padding(0, 1)
	s.success = lipgloss.NewStyle().Foreground(p.green).Bold(true)
	s.info = lipgloss.NewStyle().Foreground(p.sky).Bold(true)
	s.warning = lipgloss.NewStyle().Foreground(p.yellow).Bold(true)
	s.danger = lipgloss.NewStyle().Foreground(p.red).Bold(true)
	return s
}
