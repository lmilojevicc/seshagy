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

// hardCutStyledText truncates rendered text to width visible columns with no
// ellipsis. It is the clipping a captured preview uses: a preview is a window
// onto a terminal, so a partially drawn line reads as content the pane never
// printed.
func hardCutStyledText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(text, width, "")
}

// sgrReset is the full reset that closes a styled run.
const sgrReset = "\x1b[0m"

// paintBackground draws one full-width bar behind an already-rendered line: the
// line is cut and padded to exactly width cells, the background is opened at the
// start and re-opened after every sequence inside the line that resets or
// clears it, and the bar closes with a single reset of its own. Every styled
// segment a cell carries ends with a reset, so a plain surround would leave the
// bar painted only up to the first one; re-opening behind each reset is what
// keeps the selected row one solid bar while the row's own foreground colors
// stay on top. A color profile that draws no background leaves the line plain.
func paintBackground(line string, width int, bg lipgloss.TerminalColor) string {
	line = pad(clampStyledText(line, width), width)
	open := backgroundOpen(bg)
	if open == "" {
		return line
	}
	var out strings.Builder
	out.Grow(len(line) + 2*len(open) + len(sgrReset))
	out.WriteString(open)
	for i := 0; i < len(line); {
		if line[i] != escByte {
			end := i + 1
			for end < len(line) && line[end] != escByte {
				end++
			}
			out.WriteString(line[i:end])
			i = end
			continue
		}
		// Only SGR sequences survive here, exactly as in a sanitized preview:
		// anything else would move the picker's own cursor out of the bar.
		seq, size := previewEscape(line[i:])
		out.WriteString(seq)
		if sgrClearsBackground(seq) {
			out.WriteString(open)
		}
		i += size
	}
	out.WriteString(sgrReset)
	return out.String()
}

// backgroundOpen renders the sequence that turns bg on for the renderer's
// active color profile, or "" when the configured color is none or the profile
// draws no styling at all.
func backgroundOpen(bg lipgloss.TerminalColor) string {
	probe := lipgloss.NewStyle().Background(bg).Render("x")
	open := strings.TrimSuffix(probe, sgrReset)
	if !strings.HasSuffix(open, "x") {
		return ""
	}
	return strings.TrimSuffix(open, "x")
}

// sgrClearsBackground reports whether an SGR sequence ends the background
// paintBackground drew: a full reset (ESC[0m, ESC[m, or a list carrying a bare
// 0) or the default background (ESC[49m). Foregrounds and attributes survive
// the bar re-opening behind them.
func sgrClearsBackground(seq string) bool {
	params, ok := sgrParams(seq)
	if !ok {
		return false
	}
	if len(params) == 0 {
		return true // ESC[m is ESC[0m
	}
	for i := 0; i < len(params); i++ {
		switch params[i] {
		case "", "0", "49":
			return true
		case "38", "48", "58":
			// Extended color: "5;n" (256-color) or "2;r;g;b" (truecolor). The
			// arguments are color values, where 0 is black, not a reset.
			if i+1 < len(params) && params[i+1] == "2" {
				i += 4
			} else {
				i += 2
			}
		}
	}
	return false
}

// sgrParams splits the parameters of an SGR sequence, and reports whether seq
// is one at all.
func sgrParams(seq string) ([]string, bool) {
	if len(seq) < 3 || seq[0] != escByte || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return nil, false
	}
	body := seq[2 : len(seq)-1]
	if body == "" {
		return nil, true
	}
	return strings.Split(body, ";"), true
}

// previewTabStop is the tab stop width captured preview tabs expand to.
const previewTabStop = 8

// escByte starts every escape sequence a capture can contain.
const escByte = 0x1b

// previewReset closes any color or attribute a captured preview line left open,
// so it cannot bleed into the column separator, the padding, or the next row.
const previewReset = "\x1b[0m"

// sanitizePreviewLine makes one captured preview line safe to draw inside the
// preview column. Captures are raw terminal output: they use CRLF line endings
// and carry cursor, erase, and operating-system sequences that would move the
// picker's own cursor or overwrite neighbouring cells, plus tabs that would
// shift text past the column width. Only SGR sequences survive; tabs expand to
// 8-column stops using the visible width written so far.
func sanitizePreviewLine(line string) string {
	if line == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(len(line))
	width := 0
	for i := 0; i < len(line); {
		switch b := line[i]; {
		case b == escByte:
			seq, size := previewEscape(line[i:])
			out.WriteString(seq)
			i += size
		case b == '\t':
			spaces := previewTabStop - width%previewTabStop
			out.WriteString(strings.Repeat(" ", spaces))
			width += spaces
			i++
		case b < 0x20 || b == 0x7f:
			// CR and the other C0 controls (plus DEL) move or erase cells.
			i++
		default:
			// A printable run, counted by visible width so the tab stops below
			// stay aligned with what is actually drawn. Bytes >= 0x80 stay: they
			// are UTF-8 continuation bytes, not C1 controls.
			end := i
			for end < len(line) && line[end] != escByte && line[end] != '\t' &&
				line[end] >= 0x20 && line[end] != 0x7f {
				end++
			}
			run := line[i:end]
			out.WriteString(run)
			width += ansi.StringWidth(run)
			i = end
		}
	}
	return out.String()
}

// previewEscape inspects the escape sequence starting at s[0] (an ESC) and
// returns the bytes to keep — the sequence itself only for SGR — and the number
// of bytes it spans. A sequence that is never terminated consumes the rest of
// the line instead of leaking its raw bytes into the frame.
func previewEscape(s string) (string, int) {
	if len(s) < 2 {
		return "", len(s)
	}
	switch s[1] {
	case '[': // CSI: keep colors/attributes ("...m"), drop moves, erases, modes
		end := 2
		for end < len(s) && s[end] >= 0x20 && s[end] <= 0x3f {
			end++
		}
		if end >= len(s) {
			return "", end
		}
		final := s[end]
		end++
		if final == 'm' {
			return s[:end], end
		}
		return "", end
	case ']', 'P', '^', '_', 'X': // OSC/DCS/PM/APC/SOS: string until BEL or ST
		for end := 2; end < len(s); end++ {
			if s[end] == 0x07 {
				return "", end + 1
			}
			if s[end] == escByte && end+1 < len(s) && s[end+1] == '\\' {
				return "", end + 2
			}
		}
		return "", len(s)
	default: // ESC with intermediate bytes and/or a single final byte
		end := 1
		for end < len(s) && s[end] >= 0x20 && s[end] <= 0x2f {
			end++
		}
		if end < len(s) {
			end++ // the final byte
		}
		return "", end
	}
}

// sanitizeLabelText makes a user-controlled label safe to draw in a window title
// or a toast. Unlike a preview line it keeps no styling at all: a machine label
// is drawn inside text the picker owns, so a stray escape sequence there would
// repaint the title it sits in rather than its own cell.
func sanitizeLabelText(text string) string {
	if text == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(text); {
		switch b := text[i]; {
		case b == escByte:
			_, size := previewEscape(text[i:])
			i += size
		case b < 0x20 || b == 0x7f:
			i++
		default:
			// Bytes >= 0x80 stay: they are UTF-8 continuation bytes, not C1
			// controls.
			out.WriteByte(b)
			i++
		}
	}
	return out.String()
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

// spaces is n blank cells, empty for a non-positive count.
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// windowGap is the blank column between two windows drawn side by side.
const windowGap = 1

// drawWindow renders one single-line-border window: a rounded top border with
// the title centered in it (one space of padding each side), the inner lines
// hard-cut and padded to width-2, and a rounded bottom border with bottomRight
// right-aligned in it. title and bottomRight are pre-rendered text — already
// styled by the caller — and are clamped rather than allowed to push the border
// out of shape. The result is exactly height lines of exactly width columns,
// and nil when a dimension is not positive.
func drawWindow(
	title string,
	lines []string,
	width, height int,
	border lipgloss.Style,
	bottomRight string,
) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	inner := max(0, width-2)
	if width == 1 {
		// No room for a horizontal edge: a one-column window keeps the border
		// glyphs so the frame stays rectangular.
		out := make([]string, height)
		for i := range out {
			switch i {
			case 0:
				out[i] = border.Render("╭")
			case height - 1:
				out[i] = border.Render("╰")
			default:
				out[i] = border.Render("│")
			}
		}
		return out
	}

	out := make([]string, 0, height)
	out = append(out, windowTopBorder(title, width, border))
	for i := 1; i < height-1; i++ {
		line := ""
		if i-1 < len(lines) {
			line = lines[i-1]
		}
		out = append(
			out,
			border.Render("│")+pad(hardCutStyledText(line, inner), inner)+border.Render("│"),
		)
	}
	if height > 1 {
		out = append(out, windowBottomBorder(bottomRight, width, border))
	}
	return out
}

// windowTopBorder draws a rounded top border with the title centered in it,
// one space of padding on each side. Each segment is rendered on its own so the
// title's styling never bleeds into the border and vice versa.
func windowTopBorder(title string, width int, border lipgloss.Style) string {
	inner := max(0, width-2)
	text := clampStyledText(title, max(0, inner-2))
	if text == "" {
		return border.Render("╭" + strings.Repeat("─", inner) + "╮")
	}
	text = " " + text + " "
	left := (inner - lipgloss.Width(text)) / 2
	right := inner - lipgloss.Width(text) - left
	return border.Render("╭"+strings.Repeat("─", left)) + text +
		border.Render(strings.Repeat("─", right)+"╮")
}

// windowBottomBorder draws a rounded bottom border with bottomRight flush
// against the right corner, one space of padding on each side.
func windowBottomBorder(bottomRight string, width int, border lipgloss.Style) string {
	inner := max(0, width-2)
	text := clampStyledText(bottomRight, max(0, inner-2))
	if text == "" {
		return border.Render("╰" + strings.Repeat("─", inner) + "╯")
	}
	text = " " + text + " "
	return border.Render("╰"+strings.Repeat("─", inner-lipgloss.Width(text))) + text +
		border.Render("╯")
}
