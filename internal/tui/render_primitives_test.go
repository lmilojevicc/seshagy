package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// TestTitledTopEdge covers the hand-composed border title: exact display
// width, fieldset layout, clamping, the empty/narrow fallbacks, and the
// multi-color edge (title text colored separately from the border).
func TestTitledTopEdge(t *testing.T) {
	borderFG := lipgloss.Color("9")
	titleFG := lipgloss.Color("12")
	// Force a color profile so the multi-color assertion can observe SGR
	// sequences (the test environment strips color from the default profile).
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })
	cases := []struct {
		name              string
		title             string
		w                 int
		want              []string // substrings the plain edge must contain
		borderFG, titleFG lipgloss.TerminalColor
	}{
		{
			name:     "normal",
			title:    "All (3 · 2 agents)",
			w:        40,
			want:     []string{"╭─ ", "All (3 · 2 agents)", "─╮"},
			borderFG: borderFG,
			titleFG:  borderFG,
		},
		{
			name:     "long clamped",
			title:    "All (1145 · 12 workspaces · 8 agents · 1125 dirs)",
			w:        26,
			want:     []string{"╭─ ", "…", "─╮"},
			borderFG: borderFG,
			titleFG:  borderFG,
		},
		{
			name:     "empty plain edge",
			title:    "",
			w:        20,
			want:     []string{"╭", "╮"},
			borderFG: borderFG,
			titleFG:  borderFG,
		},
		{
			name:     "narrow plain edge",
			title:    "Preview",
			w:        5,
			want:     []string{"╭", "╮"},
			borderFG: borderFG,
			titleFG:  borderFG,
		},
		{
			name:     "two colors",
			title:    "Preview",
			w:        30,
			want:     []string{"╭─ ", "Preview", "─╮"},
			borderFG: borderFG,
			titleFG:  titleFG,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			edge := titledTopEdge(tt.title, tt.w, tt.borderFG, tt.titleFG)
			clean := sessionmgr.StripANSI(edge)
			if got := lipgloss.Width(clean); got != tt.w {
				t.Fatalf("display width = %d, want %d (%q)", got, tt.w, clean)
			}
			for _, want := range tt.want {
				if !strings.Contains(clean, want) {
					t.Fatalf("edge %q missing %q", clean, want)
				}
			}
			if tt.name == "long clamped" && strings.Contains(clean, "workspaces") {
				t.Fatalf("long title not clamped: %q", clean)
			}
			if tt.name == "narrow plain edge" && strings.Contains(clean, "Preview") {
				t.Fatalf("fallback edge leaked title: %q", clean)
			}
			// When the title color differs from the border, the edge must carry
			// both sequences: border for corners+dashes, title for the text.
			if tt.borderFG != tt.titleFG {
				borderSet, titleSet := sgrPrefix(tt.borderFG), sgrPrefix(tt.titleFG)
				if borderSet == titleSet {
					t.Fatalf("test colors collide: %q", borderSet)
				}
				if !strings.Contains(edge, borderSet) || !strings.Contains(edge, titleSet) {
					t.Fatalf(
						"two-color edge missing a color sequence:\nedge=%q\nborder=%q\ntitle=%q",
						edge,
						borderSet,
						titleSet,
					)
				}
			}
		})
	}
}

func TestTitledBottomEdge(t *testing.T) {
	borderFG := lipgloss.Color("9")
	titleFG := lipgloss.Color("12")
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	for _, tt := range []struct {
		name  string
		title string
		w     int
	}{
		{name: "normal two colors", title: "filter", w: 20},
		{name: "empty plain edge", title: "", w: 12},
		{name: "narrow plain edge", title: "filter", w: 6},
		{name: "long title clamped", title: "a-filter-query-that-is-too-long", w: 18},
	} {
		t.Run(tt.name, func(t *testing.T) {
			edge := titledBottomEdge(tt.title, tt.w, borderFG, titleFG)
			clean := sessionmgr.StripANSI(edge)
			if got := lipgloss.Width(clean); got != tt.w {
				t.Fatalf("display width = %d, want %d (%q)", got, tt.w, clean)
			}
			if !strings.HasPrefix(clean, "╰") || !strings.HasSuffix(clean, "╯") {
				t.Fatalf("bottom edge lacks rounded corners: %q", clean)
			}
			switch tt.name {
			case "normal two colors":
				if !strings.Contains(clean, "╰─ filter ") ||
					!strings.Contains(edge, sgrPrefix(borderFG)) ||
					!strings.Contains(edge, sgrPrefix(titleFG)) {
					t.Fatalf("normal edge lacks fieldset layout or two colors: %q", edge)
				}
			case "empty plain edge", "narrow plain edge":
				if strings.Contains(clean, tt.title) && tt.title != "" {
					t.Fatalf("plain fallback leaked title: %q", clean)
				}
				if clean != "╰"+strings.Repeat("─", tt.w-2)+"╯" {
					t.Fatalf("plain fallback = %q", clean)
				}
			case "long title clamped":
				if !strings.Contains(clean, "…") || strings.Contains(clean, "too-long") {
					t.Fatalf("long title was not clamped: %q", clean)
				}
			}
		})
	}
}

func TestPadDisplayWidth(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	for _, tt := range []struct {
		name  string
		line  string
		width int
		want  string
	}{
		{name: "ansi", line: lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("x"), width: 3},
		{name: "wide unicode", line: "界", width: 4, want: "界  "},
		{name: "already wide", line: "abcd", width: 3, want: "abcd"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := pad(tt.line, tt.width)
			if lipgloss.Width(got) != max(lipgloss.Width(tt.line), tt.width) {
				t.Fatalf("display width = %d", lipgloss.Width(got))
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("pad() = %q, want %q", got, tt.want)
			}
			if tt.name == "ansi" && !strings.HasPrefix(got, tt.line) {
				t.Fatalf("pad() lost styled prefix: %q", got)
			}
		})
	}
}

func TestTrimHeight(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		height int
		want   string
	}{
		{name: "clip", input: "a\nb\nc", height: 2, want: "a\nb"},
		{name: "pad", input: "a", height: 3, want: "a\n\n"},
		{name: "exact", input: "a\nb", height: 2, want: "a\nb"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimHeight(tt.input, tt.height); got != tt.want {
				t.Fatalf("trimHeight() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinFrameOrderingClampFillAndTruncate(t *testing.T) {
	styledWide := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("界界界")
	got := joinFrame("header", styledWide+"\nbody-2", "footer", 6, 5)
	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Fatalf("line count = %d, want 5: %q", len(lines), got)
	}
	if lines[0] != "head…" {
		t.Fatalf("header clamp = %q", lines[0])
	}
	if lipgloss.Width(lines[1]) > safeWidth(6) {
		t.Fatalf("wide styled body width = %d", lipgloss.Width(lines[1]))
	}
	if lines[2] != "body…" || lines[3] != "foot…" || lines[4] != "" {
		t.Fatalf("ordering/fill = %#v", lines)
	}

	truncated := joinFrame("h", "b1\nb2", "f", 20, 2)
	if truncated != "h\nb1" {
		t.Fatalf("truncate = %q", truncated)
	}
}

func TestPaneWithTitlePreservesWidthHeightAndStyles(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	borderFG := lipgloss.Color("9")
	titleFG := lipgloss.Color("12")
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderFG)
	got := paneWithTitle(style, titleFG, "content", "Title", 24, 6)
	lines := strings.Split(got, "\n")
	if len(lines) != 6 {
		t.Fatalf("height = %d, want 6\n%s", len(lines), got)
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 24 {
			t.Fatalf("line %d width = %d, want 24: %q", i, lipgloss.Width(line), line)
		}
	}
	if !strings.Contains(lines[0], sgrPrefix(borderFG)) ||
		!strings.Contains(lines[0], sgrPrefix(titleFG)) {
		t.Fatalf("top edge missing border/title style: %q", lines[0])
	}
	if !strings.Contains(sessionmgr.StripANSI(lines[0]), "Title") {
		t.Fatalf("top edge missing title: %q", lines[0])
	}
}
