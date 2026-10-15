package tui

import (
	"strings"
	"testing"

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
	// separator, the padding, or the next row.
	m.preview = "\x1b[41m" + strings.Repeat("x", 40)
	m.previewKey = "zoxide:/tmp/demo"

	lines := m.renderPreview(12, 3)
	if len(lines) < 2 {
		t.Fatalf("preview lines = %d, want a header and content", len(lines))
	}
	for i, line := range lines[1:] {
		if !strings.HasSuffix(line, previewReset) {
			t.Fatalf("preview content line %d does not close its style: %q", i+1, line)
		}
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
	body := m.renderBody(l)
	if len(body) != l.bodyHeight {
		t.Fatalf("body lines = %d, want %d", len(body), l.bodyHeight)
	}
	for i, line := range body {
		if strings.Contains(line, "\r") {
			t.Fatalf("body line %d contains a carriage return: %q", i, line)
		}
		if got := ansi.StringWidth(line); got != l.width {
			t.Fatalf("body line %d is %d columns, want %d: %q", i, got, l.width, line)
		}
	}
	if view := m.View(); strings.Contains(view, "\r") {
		t.Fatalf("frame contains a carriage return:\n%q", view)
	}
}
