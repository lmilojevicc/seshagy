package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// overlay places fg on top of bg at visible column x, starting at row y. bg is
// expected to be the full-screen render (already dimmed); both strings are
// split into lines and each fg line overwrites the bg cells at the given
// column. ANSI styling on the bg portions outside the fg footprint is
// preserved via ANSI-aware truncation (charmbracelet/x/ansi), so a dimmed
// background stays dimmed around the floating popup. Wide (CJK) and combining
// runes are handled by the same width model lipgloss uses.
func overlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	for i, fgLine := range fgLines {
		bgIdx := y + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgIdx]
		fgW := lipgloss.Width(fgLine)
		// Left portion: first x visible columns of bg, ANSI styling preserved.
		var left string
		if x > 0 {
			left = ansi.Truncate(bgLine, x, "")
		}
		// Right portion: bg past column x+fgW, ANSI styling preserved.
		var right string
		if end := x + fgW; end > 0 {
			right = ansi.TruncateLeft(bgLine, end, "")
		}
		bgLines[bgIdx] = left + fgLine + right
	}
	return strings.Join(bgLines, "\n")
}
