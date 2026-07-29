package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// titledTopEdge builds the top border line of a rounded pane at display width
// w with title overlaid fieldset-style: ╭─ title ──╮. The corners and dashes
// are rendered in borderFG; the title text is rendered in titleFG, so a pane
// can give its title a distinct color from its border. When titleFG == borderFG
// the edge is monochrome (the default). lipgloss v1.1.0 has no native
// border-title API, so this is composed by hand. Empty titles and very narrow
// widths fall back to a plain dashed edge in borderFG.
func titledTopEdge(title string, w int, borderFG, titleFG lipgloss.TerminalColor) string {
	if title == "" || w < 7 {
		return lipgloss.NewStyle().Foreground(borderFG).
			Render("╭" + strings.Repeat("─", max(0, w-2)) + "╮")
	}
	// Layout: ╭─ (3) + title + space (1) + dashes + ╮ (1); keep >= 1 trailing dash.
	clamped := clampText(title, w-6)
	dashes := w - 5 - lipgloss.Width(clamped)
	if dashes < 1 {
		dashes = 1
	}
	border := lipgloss.NewStyle().Foreground(borderFG)
	return border.Render("╭─ ") +
		lipgloss.NewStyle().Foreground(titleFG).Render(clamped) +
		border.Render(" "+strings.Repeat("─", dashes)+"╮")
}

// titledBottomEdge is the bottom-border mirror of titledTopEdge: ╰─ title ──╯.
func titledBottomEdge(title string, w int, borderFG, titleFG lipgloss.TerminalColor) string {
	if title == "" || w < 7 {
		return lipgloss.NewStyle().Foreground(borderFG).
			Render("╰" + strings.Repeat("─", max(0, w-2)) + "╯")
	}
	clamped := clampText(title, w-6)
	dashes := w - 5 - lipgloss.Width(clamped)
	if dashes < 1 {
		dashes = 1
	}
	border := lipgloss.NewStyle().Foreground(borderFG)
	return border.Render("╰─ ") +
		lipgloss.NewStyle().Foreground(titleFG).Render(clamped) +
		border.Render(" "+strings.Repeat("─", dashes)+"╯")
}

// paneWithTitle renders a pane via style (width/height applied as for the
// other pane renderers) and overlays title onto its top border. The border
// line color comes from the style's own border foreground; the title text
// color comes from titleFG, which by default matches the pane border (via
// theme inheritance) so the edge is monochrome unless a distinct title color
// is configured.
func paneWithTitle(
	style lipgloss.Style,
	titleFG lipgloss.TerminalColor,
	content, title string,
	width, height int,
) string {
	boxStyle := style.Width(width - 2)
	if height > 0 {
		boxStyle = boxStyle.Height(height - 2)
	}
	box := boxStyle.Render(content)
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	w := lipgloss.Width(lines[0])
	if w < 3 {
		return box
	}
	lines[0] = titledTopEdge(title, w, style.GetBorderTopForeground(), titleFG)
	return strings.Join(lines, "\n")
}

func pad(line string, width int) string {
	if lipgloss.Width(line) >= width {
		return line
	}
	return line + strings.Repeat(" ", width-lipgloss.Width(line))
}

// clampStyledText truncates terminal output without splitting ANSI sequences,
// UTF-8, or grapheme clusters. Use clampText before applying styles when
// possible; this helper is the final width guard for already-rendered text.
func clampStyledText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, "…")
}

// joinFrame stacks UI blocks without lipgloss.JoinVertical, which pads every
// line to the widest line in the frame and can push the tab bar past the pane.
func joinFrame(header, body, footer string, width, height int) string {
	safeW := safeWidth(width)
	lines := make([]string, 0, height)
	appendBlock := func(block string) {
		if block == "" {
			return
		}
		for _, line := range strings.Split(block, "\n") {
			if len(lines) >= height {
				return
			}
			if lipgloss.Width(line) > safeW {
				line = clampStyledText(line, safeW)
			}
			lines = append(lines, line)
		}
	}
	appendBlock(header)
	appendBlock(body)
	appendBlock(footer)
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func trimHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
