package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// inputChrome is the immutable, width-specific result of rendering the copied
// controller-owned Bubbles widget. It is shell-only and never enters layoutView.
type inputChrome struct {
	Title string
	Line  string
	Help  string
}

func (m Model) projectInputChrome(available int) inputChrome {
	title, input, help := m.configuredInput(available)
	return inputChrome{
		Title: title,
		Line:  clampText(input.View(), max(1, available)),
		Help:  help,
	}
}

func renderDefaultActionsTile(view actionsView, s styles, width int) string {
	help := renderActionsHelp(view, s)
	contentW := max(1, width-4)
	return paneWithTitle(
		s.tileHelp,
		s.helpTileTitle,
		clampText(help, contentW),
		"HELP",
		width,
		0,
	)
}

func renderZenActionsLine(view actionsView, s styles, width int) string {
	usable := max(1, width)
	contentWidth := zenContentWidthForUsable(usable)
	help := clampStyledText(renderActionsHelp(view, s), contentWidth)
	line := zenCenterText(help, contentWidth)
	left := max(0, (usable-contentWidth)/2)
	return strings.Repeat(" ", left) + line
}

func renderActionsHelp(view actionsView, s styles) string {
	var help string
	if !view.Expanded {
		help = s.muted.Render("? help")
	} else {
		parts := make([]string, 0, len(view.Hints))
		for _, hint := range view.Hints {
			parts = append(parts, s.key.Render(hint.Key)+" "+hint.Label)
		}
		help = strings.Join(parts, s.muted.Render(" · "))
	}
	if view.PrefixArmed {
		help = s.warning.Bold(true).Render("PREFIX") + " " + help
	}
	return help
}

func renderPopupInput(chrome inputChrome, s styles, boxWidth int) string {
	contentW := max(1, boxWidth-4)
	content := clampText(chrome.Line, contentW) + "\n" +
		clampText(s.muted.Render(chrome.Help), contentW)
	return paneWithTitle(
		s.paneInput,
		s.helpTileTitle,
		content,
		chrome.Title,
		boxWidth,
		0,
	)
}

func renderCmdlineInput(chrome inputChrome, s styles, width int) string {
	return paneWithTitle(
		s.paneInput,
		s.helpTileTitle,
		chrome.Line,
		chrome.Title,
		width,
		0,
	)
}

func renderInputOnly(chrome inputChrome) string {
	return chrome.Line
}

func dimDashboard(frame string) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	lines := strings.Split(frame, "\n")
	for index, line := range lines {
		lines[index] = dim.Render(ansi.Strip(line))
	}
	return strings.Join(lines, "\n")
}
