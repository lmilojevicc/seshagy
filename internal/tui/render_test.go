package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestSanitizePreviewLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "crlf line ending drops the carriage return",
			in:   "two\r",
			want: "two",
		},
		{
			name: "embedded carriage return is dropped",
			in:   "alpha\rbeta",
			want: "alphabeta",
		},
		{
			name: "tab expands to the next 8-column stop",
			in:   "ab\tc",
			want: "ab      c",
		},
		{
			name: "tab on a stop boundary moves a full stop",
			in:   "abcdefgh\tx",
			want: "abcdefgh        x",
		},
		{
			name: "tab stops count visible width, not bytes",
			in:   "\x1b[31mab\x1b[0m\tc",
			want: "\x1b[31mab\x1b[0m      c",
		},
		{
			name: "sgr colors and attributes are preserved",
			in:   "\x1b[1;31mred\x1b[0m",
			want: "\x1b[1;31mred\x1b[0m",
		},
		{
			name: "cursor moves are dropped",
			in:   "\x1b[1;1Hmove",
			want: "move",
		},
		{
			name: "erase sequences are dropped",
			in:   "text\x1b[K",
			want: "text",
		},
		{
			name: "osc titles are dropped",
			in:   "\x1b]0;title\x07text",
			want: "text",
		},
		{
			name: "osc with a string-terminator is dropped",
			in:   "\x1b]8;;https://example.com\x1b\\link",
			want: "link",
		},
		{
			name: "dcs strings are dropped",
			in:   "\x1bP1;2|payload\x1b\\after",
			want: "after",
		},
		{
			name: "apc strings are dropped",
			in:   "\x1b_Ga=T\x1b\\after",
			want: "after",
		},
		{
			name: "single-character escapes are dropped",
			in:   "\x1bctext",
			want: "text",
		},
		{
			name: "other c0 controls and del are dropped",
			in:   "a\x07\x08b\x7f",
			want: "ab",
		},
		{
			name: "an unterminated csi is dropped to the end of the line",
			in:   "text\x1b[1;",
			want: "text",
		},
		{
			name: "a bare trailing esc is dropped",
			in:   "text\x1b",
			want: "text",
		},
		{
			name: "intermediate bytes are consumed with their final byte",
			in:   "\x1b(Bansi",
			want: "ansi",
		},
		{
			name: "utf-8 text survives",
			in:   "héllo→ok",
			want: "héllo→ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizePreviewLine(tt.in); got != tt.want {
				t.Fatalf("sanitizePreviewLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPreviewLinesEndWithAReset(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo", Name: "/tmp/demo"},
	})
	// A capture that leaves a background color open must not bleed into the
	// window border, the padding, or the next row.
	m.preview = "\x1b[41m" + strings.Repeat("x", 40)
	m.previewKey = "zoxide:/tmp/demo"

	lines := m.renderPreviewContent(12, 3)
	if len(lines) != 3 {
		t.Fatalf("preview lines = %d, want 3", len(lines))
	}
	seen := 0
	for i, line := range lines {
		if line == "" {
			continue // a blank fill line carries no capture content
		}
		seen++
		if !strings.HasSuffix(line, previewReset) {
			t.Fatalf("preview content line %d does not close its style: %q", i, line)
		}
	}
	if seen == 0 {
		t.Fatal("no preview content line was drawn")
	}
}

// drawWindowExactSize asserts a window block is exactly the requested grid.
func drawWindowExactSize(t *testing.T, out []string, width, height int) {
	t.Helper()
	if len(out) != height {
		t.Fatalf("window has %d lines, want %d:\n%s", len(out), height, strings.Join(out, "\n"))
	}
	for i, line := range out {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("line %d is %d columns, want %d: %q", i, got, width, line)
		}
	}
}

func TestDrawWindowKeepsTheExactSize(t *testing.T) {
	withColor(t)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	tests := []struct {
		name          string
		width, height int
		title         string
		bottomRight   string
		lines         []string
	}{
		{
			name:        "regular window",
			width:       24,
			height:      5,
			title:       "Results",
			bottomRight: "↓ 3 more",
			lines:       []string{"row one", "row two", "row three"},
		},
		{name: "no title and no footer text", width: 20, height: 4, lines: []string{"only row"}},
		{
			name:   "ansi title",
			width:  30,
			height: 3,
			title:  bold.Render("Results · 2 need you"),
			lines:  []string{"x"},
		},
		{
			name:   "ansi title with padding",
			width:  17,
			height: 3,
			title:  bold.Render("seshagy") + " · " + bold.Render("@ agents"),
		},
		{name: "title wider than the border", width: 10, height: 3, title: strings.Repeat("t", 40)},
		{
			name:        "footer text wider than the border",
			width:       10,
			height:      3,
			bottomRight: strings.Repeat("m", 40),
		},
		{name: "more lines than rows", width: 12, height: 3, lines: []string{"a", "b", "c", "d"}},
		{name: "three columns", width: 3, height: 3, title: "x", lines: []string{"y"}},
		{name: "two columns", width: 2, height: 2},
		{name: "one column", width: 1, height: 4},
		{name: "one row", width: 8, height: 1, title: "x", bottomRight: "y"},
		{name: "one cell", width: 1, height: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := drawWindow(tt.title, tt.lines, tt.width, tt.height, border, tt.bottomRight)
			drawWindowExactSize(t, out, tt.width, tt.height)
		})
	}

	for _, dim := range [][2]int{{0, 3}, {3, 0}, {0, 0}, {-1, 5}} {
		if out := drawWindow("x", nil, dim[0], dim[1], border, ""); out != nil {
			t.Fatalf("drawWindow(%d, %d) = %v, want nil", dim[0], dim[1], out)
		}
	}
}

func TestDrawWindowPlacesTheTitleAndTheBottomRight(t *testing.T) {
	withColor(t)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render("Results")
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("↓ 3 more")

	out := drawWindow(title, nil, 21, 3, border, footer)
	drawWindowExactSize(t, out, 21, 3)
	top := sessionmgr.StripANSI(out[0])
	if top != "╭───── Results ─────╮" {
		t.Fatalf("top border = %q", top)
	}
	if !strings.Contains(out[0], title) {
		t.Fatalf("title is not rendered as given: %q", out[0])
	}
	if !strings.Contains(out[0], border.Render("╭─────")) {
		t.Fatalf("border dashes are not drawn in the border style: %q", out[0])
	}
	bottom := sessionmgr.StripANSI(out[2])
	if bottom != "╰───────── ↓ 3 more ╯" {
		t.Fatalf("bottom border = %q", bottom)
	}
	if !strings.Contains(out[2], footer) {
		t.Fatalf("bottom-right text is not rendered as given: %q", out[2])
	}

	// A title that does not fit takes the room it can and keeps both corners.
	tight := drawWindow(title, nil, 8, 3, border, footer)
	drawWindowExactSize(t, tight, 8, 3)
	if got := sessionmgr.StripANSI(
		tight[0],
	); !strings.HasPrefix(got, "╭") ||
		!strings.HasSuffix(got, "╮") {
		t.Fatalf("tight top border = %q", got)
	}
	if got := sessionmgr.StripANSI(
		tight[2],
	); !strings.HasPrefix(got, "╰") ||
		!strings.HasSuffix(got, "╯") {
		t.Fatalf("tight bottom border = %q", got)
	}
}

func TestDrawWindowPadsAndHardCutsInnerLines(t *testing.T) {
	withColor(t)
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	out := drawWindow("", []string{"short", strings.Repeat("x", 40)}, 12, 4, border, "")
	drawWindowExactSize(t, out, 12, 4)
	if got := sessionmgr.StripANSI(out[1]); got != "│short     │" {
		t.Fatalf("padded row = %q", got)
	}
	if got := sessionmgr.StripANSI(out[2]); got != "│"+strings.Repeat("x", 10)+"│" {
		t.Fatalf("hard-cut row = %q", got)
	}
	if got := sessionmgr.StripANSI(out[3]); got != "╰──────────╯" {
		t.Fatalf("missing inner lines are not blank: %q", got)
	}
	if strings.Contains(sessionmgr.StripANSI(strings.Join(out, "\n")), "…") {
		t.Fatal("inner lines are ellipsized instead of hard-cut")
	}
}

func TestViewFromACRLFCaptureKeepsTheFrameWidth(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 120, 18
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo", Windows: 2},
	})
	// The shape a `herdr pane read --ansi` / `tmux capture-pane -e` payload has:
	// CRLF endings, tabs, an erase sequence, an OSC title, and a long line.
	m.preview = "alpha\r\nsecond\tline\x1b[K\r\n" +
		"\x1b]0;title\x07\x1b[31mcolored\x1b[0m\r\n" +
		strings.Repeat("x", 200) + "\r"
	m.previewKey = "session:demo"

	l := m.viewLayout()
	rows, _, _ := m.resultsRows(m.rows(), l.listWidth(), l.listHeight())
	if len(rows) != 1 {
		t.Fatalf("result rows = %d, want 1", len(rows))
	}
	for i, line := range rows {
		if strings.Contains(line, "\r") {
			t.Fatalf("row %d contains a carriage return: %q", i, line)
		}
		if got := ansi.StringWidth(line); got != l.listWidth() {
			t.Fatalf("row %d is %d columns, want %d: %q", i, got, l.listWidth(), line)
		}
	}
	for i, line := range m.renderPreviewContent(l.preview.width-2, l.preview.height-2) {
		if strings.Contains(line, "\r") {
			t.Fatalf("preview line %d contains a carriage return: %q", i, line)
		}
		if got := ansi.StringWidth(line); got > l.preview.width-2 {
			t.Fatalf(
				"preview line %d is %d columns, want <= %d: %q",
				i,
				got,
				l.preview.width-2,
				line,
			)
		}
	}

	if view := m.View(); strings.Contains(view, "\r") {
		t.Fatalf("frame contains a carriage return:\n%q", view)
	}
	frameLines(t, m)
}
