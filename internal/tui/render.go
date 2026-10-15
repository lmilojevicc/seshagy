package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// safeWidth is one column shy of the terminal so the last column cannot trigger
// auto-wrap at the right edge.
func safeWidth(w int) int {
	if w <= 0 {
		return 0
	}
	return max(1, w-1)
}

// pad right-pads line to width visible columns.
func pad(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) >= width {
		return line
	}
	return line + strings.Repeat(" ", width-lipgloss.Width(line))
}

// clampStyledText truncates already-rendered text to width visible columns
// without splitting ANSI sequences, UTF-8, or grapheme clusters. Colors are
// preserved; only the overflow is dropped.
func clampStyledText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, "…")
}

// clampText truncates unstyled text to width columns, appending an ellipsis.
func clampText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// trimHeight truncates s to at most height lines and pads it to exactly height.
func trimHeight(s string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// visibleWindow returns the [start, end) row range to draw so cursor stays
// inside a window of the given height.
func visibleWindow(total, cursor, height int) (int, int) {
	if height <= 0 {
		return 0, 0
	}
	if total <= height {
		return 0, total
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = total - height
	}
	return start, start + height
}
