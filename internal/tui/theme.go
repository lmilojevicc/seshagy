package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

type palette struct {
	bg       lipgloss.TerminalColor
	fg       lipgloss.TerminalColor
	muted    lipgloss.TerminalColor
	border   lipgloss.TerminalColor
	mauve    lipgloss.TerminalColor
	peach    lipgloss.TerminalColor
	green    lipgloss.TerminalColor
	sky      lipgloss.TerminalColor
	red      lipgloss.TerminalColor
	yellow   lipgloss.TerminalColor
	lavender lipgloss.TerminalColor
}

type styles struct {
	p palette

	app                lipgloss.Style
	chipActive         lipgloss.Style
	chipIdle           lipgloss.Style
	itemName           lipgloss.Style
	pane               lipgloss.Style
	panePopup          lipgloss.Style
	paneInput          lipgloss.Style
	paneList           lipgloss.Style
	paneDetail         lipgloss.Style
	panePreview        lipgloss.Style
	listTitle          lipgloss.TerminalColor
	metadataTitle      lipgloss.TerminalColor
	previewTitle       lipgloss.TerminalColor
	tileWorkspace      lipgloss.Style
	tileAgent          lipgloss.Style
	tileSources        lipgloss.Style
	tileHelp           lipgloss.Style
	workspaceTileTitle lipgloss.TerminalColor
	agentTileTitle     lipgloss.TerminalColor
	sourcesTileTitle   lipgloss.TerminalColor
	helpTileTitle      lipgloss.TerminalColor
	title              lipgloss.Style
	muted              lipgloss.Style
	emphasis           lipgloss.Style
	key                lipgloss.Style
	selectedBG         lipgloss.Style
	bar                lipgloss.Style
	success            lipgloss.Style
	info               lipgloss.Style
	warning            lipgloss.Style
	danger             lipgloss.Style
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
		fg:       lipgloss.NoColor{},
		muted:    lipgloss.Color("8"),
		border:   lipgloss.Color("8"),
		mauve:    lipgloss.Color("13"),
		peach:    lipgloss.Color("11"),
		green:    lipgloss.Color("10"),
		sky:      lipgloss.Color("14"),
		red:      lipgloss.Color("9"),
		yellow:   lipgloss.Color("11"),
		lavender: lipgloss.Color("12"),
	}
	muted := themeColor(colors.Muted, p.muted)
	border := themeColor(colors.Border, p.border)
	popupBorder := themeColor(colors.PopupBorder, p.mauve)
	activeTab := themeColor(colors.ActiveTab, p.fg)
	inactiveTab := themeColor(colors.InactiveTab, muted)
	popupTitle := themeColor(colors.PopupTitle, p.lavender)
	accent := themeColor(colors.Accent, p.mauve)
	key := themeColor(colors.Key, p.peach)
	success := themeColor(colors.Success, p.green)
	info := themeColor(colors.Info, p.sky)
	warning := themeColor(colors.Warning, p.yellow)
	danger := themeColor(colors.Danger, p.red)

	s := styles{p: p}
	s.app = lipgloss.NewStyle().Foreground(p.fg).Background(p.bg)
	// Source-tab chips: active is the active_tab color (bold, padded), idle a
	// muted padded chip. Reuses active_tab/inactive_tab colors (no new tokens).
	s.chipActive = lipgloss.NewStyle().
		Foreground(activeTab).
		Bold(true).
		Padding(0, 1)
	s.chipIdle = lipgloss.NewStyle().
		Foreground(inactiveTab).
		Padding(0, 1)
	s.itemName = lipgloss.NewStyle().Foreground(p.fg)
	s.pane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)
	s.panePopup = s.pane.BorderForeground(popupBorder)
	// Per-pane borders and border-title colors. Each defaults to the relevant
	// global (or the pane's own border, for titles) via themeColor on top of
	// the config-layer inheritance, so unset tokens reproduce today's look.
	listBorder := themeColor(colors.ListBorder, border)
	metadataBorder := themeColor(colors.MetadataBorder, border)
	previewBorder := themeColor(colors.PreviewBorder, border)
	s.paneList = s.pane.BorderForeground(listBorder)
	s.paneDetail = s.pane.BorderForeground(metadataBorder)
	s.panePreview = s.pane.BorderForeground(previewBorder)
	s.listTitle = themeColor(colors.ListBorderTitle, listBorder)
	s.metadataTitle = themeColor(colors.MetadataBorderTitle, metadataBorder)
	s.previewTitle = themeColor(colors.PreviewBorderTitle, previewBorder)
	// Search/rename input popup border matches the dashboard tiles by default
	// (inherits the base border via config normalization) but stays themeable.
	inputBorder := themeColor(colors.InputBorder, border)
	s.paneInput = s.pane.BorderForeground(inputBorder)
	// Overview hero tiles.
	workspaceTileBorder := themeColor(colors.WorkspaceTileBorder, border)
	agentTileBorder := themeColor(colors.AgentTileBorder, border)
	s.tileWorkspace = s.pane.BorderForeground(workspaceTileBorder)
	s.tileAgent = s.pane.BorderForeground(agentTileBorder)
	s.workspaceTileTitle = themeColor(colors.WorkspaceTileTitle, popupTitle)
	s.agentTileTitle = themeColor(colors.AgentTileTitle, popupTitle)
	// SOURCES tile wraps the tab chips; reuses the base border + popup title.
	s.tileSources = s.pane
	s.sourcesTileTitle = popupTitle
	// HELP tile wraps the footer keycaps; same base border + popup title.
	s.tileHelp = s.pane
	s.helpTileTitle = popupTitle
	s.title = lipgloss.NewStyle().Foreground(popupTitle).Bold(true)
	s.muted = lipgloss.NewStyle().Foreground(muted)
	s.emphasis = lipgloss.NewStyle().Foreground(accent).Bold(true)
	s.key = lipgloss.NewStyle().Foreground(key).Bold(true)
	s.selectedBG = lipgloss.NewStyle().Reverse(true)
	s.bar = lipgloss.NewStyle().Foreground(accent)
	s.success = lipgloss.NewStyle().Foreground(success).Bold(true)
	s.info = lipgloss.NewStyle().Foreground(info).Bold(true)
	s.warning = lipgloss.NewStyle().Foreground(warning).Bold(true)
	s.danger = lipgloss.NewStyle().Foreground(danger).Bold(true)
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
