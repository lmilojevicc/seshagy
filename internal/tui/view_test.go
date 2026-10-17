package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func pickerItems() []sessionmgr.Item {
	blocked := agentItem("claude", "%1", "frontend")
	blocked.AgentState = sessionmgr.AgentBlocked
	blocked.AgentDisplayName = "claude-refactor"
	working := agentItem("codex", "%2", "api")
	working.AgentState = sessionmgr.AgentWorking
	working.AgentDisplayName = "review-spec"
	return []sessionmgr.Item{
		blocked,
		working,
		{Kind: sessionmgr.KindSession, Name: "seshagy", Target: "seshagy", Windows: 3},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo", Name: "~/code/demo"},
	}
}

// frameLines is the rendered frame's stripped lines, checked to be exactly the
// frame's width and height: every window border, row, and the footer must land
// on the planned grid.
func frameLines(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(sessionmgr.StripANSI(m.View()), "\n")
	if len(lines) != m.height {
		t.Fatalf(
			"frame has %d lines, want %d:\n%s",
			len(lines),
			m.height,
			strings.Join(lines, "\n"),
		)
	}
	width := m.viewLayout().width
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("line %d is %d columns, want %d: %q", i, got, width, line)
		}
	}
	return lines
}

// resultRowLine is the content of the first framed column of the line that holds
// text — the results window's row for a row that carries text — with the window
// borders and the row's padding removed. The caller decides whether the line is
// stripped or still styled.
func resultRowLine(t *testing.T, frame, text string) string {
	t.Helper()
	for _, line := range strings.Split(frame, "\n") {
		start := strings.Index(line, "│")
		if start < 0 {
			continue
		}
		end := strings.Index(line[start+1:], "│")
		if end < 0 {
			continue
		}
		if row := line[start+1 : start+1+end]; strings.Contains(row, text) {
			return strings.TrimRight(row, " ")
		}
	}
	t.Fatalf("no results row contains %q:\n%s", text, frame)
	return ""
}

// promptContentLine is the ANSI-stripped content row of the prompt window.
func promptContentLine(t *testing.T, m Model) string {
	t.Helper()
	l := m.viewLayout()
	if l.prompt.height < promptWindowRows {
		t.Fatalf("frame has no prompt window content row (height %d)", l.prompt.height)
	}
	return frameLines(t, m)[l.prompt.y+1]
}

func TestViewSnapshotSanityAcrossWidths(t *testing.T) {
	for _, width := range []int{40, 80, 140} {
		m := newTestModel(t)
		m.width, m.height = width, 30
		m.loadedOnce = true
		m = withItems(m, pickerItems())

		out := sessionmgr.StripANSI(m.View())
		lines := frameLines(t, m)

		// The prompt window is the first thing every layout draws, and its title
		// carries the active scope; the query row sits inside it.
		for _, text := range []string{"seshagy · all"} {
			if !strings.Contains(lines[0], text) {
				t.Fatalf("width %d: prompt window title missing %q: %q", width, text, lines[0])
			}
		}
		if !strings.HasPrefix(lines[0], "╭") {
			t.Fatalf("width %d: prompt window has no top border: %q", width, lines[0])
		}
		promptQuery := lines[m.viewLayout().prompt.y+1]
		for _, text := range []string{"❯", placeholderText} {
			if !strings.Contains(promptQuery, text) {
				t.Fatalf("width %d: prompt content missing %q: %q", width, text, promptQuery)
			}
		}

		want := []string{"Results", "seshagy", "↑↓", "4"}
		if width >= 60 {
			// Narrow frames clip titles and the placeholder; keep the full-text
			// assertions to widths where nothing has to be clipped.
			want = append(want, "claude-refactor", "review-spec", "need you")
		}
		for _, text := range want {
			if !strings.Contains(out, text) {
				t.Fatalf("width %d: view missing %q\n%s", width, text, out)
			}
		}
		if strings.Contains(promptQuery, "#") {
			t.Fatalf("width %d: hosts sigil hinted without host items: %q", width, promptQuery)
		}
	}
}

// TestFrameWindowsAcrossLayoutsAndSizes pins the frame's grid: for every layout
// and preview state, the frame is exactly the terminal height and every line is
// exactly the frame width, with the preview window present only when it is on.
func TestFrameWindowsAcrossLayoutsAndSizes(t *testing.T) {
	sizes := []struct{ width, height int }{
		{120, 34},
		{80, 24},
		{60, 20},
	}
	layouts := []string{
		appconfig.LayoutHorizontal,
		appconfig.LayoutVertical,
		appconfig.LayoutDropdown,
	}
	for _, size := range sizes {
		for _, layout := range layouts {
			for _, preview := range []bool{true, false} {
				name := fmt.Sprintf("%dx%d/%s/preview=%v", size.width, size.height, layout, preview)
				t.Run(name, func(t *testing.T) {
					m := newTestModel(t)
					m.width, m.height = size.width, size.height
					m.layout = layout
					m.showPreview = preview
					m = withItems(m, pickerItems())

					lines := frameLines(t, m)
					l := m.viewLayout()
					if l.layout == appconfig.LayoutHorizontal && size.width < horizontalMinWidth {
						t.Fatalf("horizontal kept below the %d column floor", horizontalMinWidth)
					}
					stacked := l.layout != appconfig.LayoutHorizontal
					if l.prompt.y != 0 || l.results.y != l.prompt.height {
						t.Fatalf("the prompt window does not sit above the results: %+v", l)
					}
					bottom := l.height
					if l.footer {
						bottom--
					}
					if stacked {
						if l.prompt.width != l.width || l.results.width != l.width {
							t.Fatalf("stacked windows are not full width: %+v", l)
						}
					} else if preview && (l.prompt.width >= l.width || l.results.width != l.prompt.width) {
						t.Fatalf("the horizontal left column is not split off: %+v", l)
					}
					if !preview {
						if l.preview.height != 0 {
							t.Fatalf("preview window drawn with the preview off: %+v", l.preview)
						}
						if l.prompt.width != l.width {
							t.Fatalf("the windows did not take the full width: %+v", l)
						}
						if l.results.y+l.results.height != bottom {
							t.Fatalf("the results did not take the full height: %+v", l)
						}
					} else {
						if l.preview.x+l.preview.width != l.width {
							t.Fatalf(
								"the preview window does not reach the right edge: %+v",
								l.preview,
							)
						}
						if l.preview.y+l.preview.height != bottom {
							t.Fatalf("the preview window does not reach the footer: %+v", l)
						}
						if l.preview.height < 2 {
							t.Fatalf("preview window height = %d", l.preview.height)
						}
						if l.preview.x == 0 {
							if !stacked || l.preview.y != l.results.y+l.results.height {
								t.Fatalf(
									"the stacked preview does not sit under the results: %+v",
									l,
								)
							}
						} else {
							if l.preview.y != 0 || l.preview.width < previewMinCols {
								t.Fatalf("the preview column is misplaced: %+v", l.preview)
							}
							if l.prompt.width+windowGap+l.preview.width != l.width {
								t.Fatalf("the horizontal columns do not tile the frame: %+v", l)
							}
						}
					}
					if !l.footer {
						t.Fatalf("%dx%d dropped the footer", size.width, size.height)
					}
					if footer := lines[l.height-1]; !strings.Contains(footer, "↑↓ move") {
						t.Fatalf("footer line = %q", footer)
					}
				})
			}
		}
	}
}

// TestPromptWindowTitleCarriesTheScope pins the prompt window title: "seshagy"
// in the prompt accent, then the active scope, centered in the top border.
func TestPromptWindowTitleCarriesTheScope(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents

	l := m.viewLayout()
	block := m.renderPromptWindow(l)
	title := sessionmgr.StripANSI(block[0])
	if !strings.Contains(title, " seshagy · @ agents ") {
		t.Fatalf("prompt title row = %q", title)
	}
	if !strings.HasPrefix(title, "╭") || ansi.StringWidth(title) != l.prompt.width {
		t.Fatalf("prompt title row is not a window border: %q", title)
	}
	if !strings.Contains(block[0], m.styles.titlePrompt.Render("seshagy")) {
		t.Fatalf("seshagy is not drawn in the prompt accent: %q", block[0])
	}
	if !strings.Contains(block[0], m.styles.emphasis.Render("@ agents")) {
		t.Fatalf("scope label is not drawn in the accent: %q", block[0])
	}
}

func TestViewHidesAndShowsTheHostsSigil(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindHost, Name: "box", Target: "box"}})
	if out := promptContentLine(t, m); !strings.Contains(out, "#") {
		t.Fatalf("hosts sigil missing from the hint with a host item\n%s", out)
	}
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "dev", Target: "dev"}})
	if out := promptContentLine(t, m); strings.Contains(out, "#") {
		t.Fatalf("hosts sigil hinted without host items\n%s", out)
	}
}

func TestResultsWindowTitleCountsWhatNeedsYou(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())

	l := m.viewLayout()
	block := m.renderResultsWindow(l)
	title := sessionmgr.StripANSI(block[0])
	if !strings.Contains(title, " Results · 1 need you ") {
		t.Fatalf("results title = %q, want the needs-you count", title)
	}
	if !strings.Contains(block[0], m.styles.needsYou.Render("1 need you")) {
		t.Fatalf("needs-you count is not drawn in the red accent: %q", block[0])
	}

	// No agents waiting: the count disappears but the title stays.
	m = withItems(m, []sessionmgr.Item{{
		Kind:     sessionmgr.KindSession,
		Name:     "dev",
		Target:   "dev",
		Attached: true,
	}})
	title = sessionmgr.StripANSI(m.renderResultsWindow(m.viewLayout())[0])
	if !strings.Contains(title, " Results ") || strings.Contains(title, "need you") {
		t.Fatalf("results title = %q, want the plain title", title)
	}
}

func TestViewShowsSpinnerOnlyWhileLoading(t *testing.T) {
	m := newTestModel(t)
	m.spinnerActive = true
	if out := sessionmgr.StripANSI(m.View()); !strings.Contains(out, "⠋") {
		t.Fatalf("loading view has no spinner\n%s", out)
	}
	m.loadedOnce = true
	if out := sessionmgr.StripANSI(m.View()); strings.Contains(out, "⠋") {
		t.Fatalf("spinner still shown after the first results\n%s", out)
	}
}

func TestEmptyStateMessages(t *testing.T) {
	tests := []struct {
		name  string
		scope scope
		query string
		items []sessionmgr.Item
		want  []string
	}{
		{
			name:  "empty scope filter",
			scope: scopeAgents,
			want:  []string{"nothing in", "@ agents", "⇥ next filter · ⌫ back to all"},
		},
		{
			name:  "no matches for the query",
			scope: scopeAll,
			query: "zzz",
			items: pickerItems(),
			want:  []string{`no matches for "zzz"`},
		},
		{
			name: "nothing at all",
			want: []string{"nothing to jump to"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m = withItems(m, tt.items)
			m.scope = tt.scope
			m.query = tt.query
			out := sessionmgr.StripANSI(m.View())
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("empty state missing %q\n%s", want, out)
				}
			}
		})
	}
}

func TestPromptShowsShortPlaceholderOnly(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	plain := sessionmgr.StripANSI(m.View())
	if !strings.Contains(plain, placeholderText) {
		t.Fatalf("placeholder missing\n%s", plain)
	}
	if strings.Contains(placeholderText, "@") {
		t.Fatal("placeholder lists sigils; the hint is the only place they appear")
	}
}

// cursorColumn is the column the reverse-video cursor block starts in.
func cursorColumn(t *testing.T, line string) int {
	t.Helper()
	index := strings.Index(line, "\x1b[7m")
	if index < 0 {
		t.Fatalf("no cursor block in %q", line)
	}
	return ansi.StringWidth(line[:index])
}

func TestPromptCursorSitsInTheInputNotAfterThePlaceholder(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())

	// The row carries a one-column inset before the prompt, so the prompt starts
	// in the row's second cell and the cursor sits two cells behind it.
	empty := m.renderPromptContent(80)
	if got := cursorColumn(t, empty); got != promptInset+2 {
		t.Fatalf("empty query: cursor column = %d, want %d: %q", got, promptInset+2, empty)
	}
	if strings.Index(empty, "\x1b[7m") > strings.Index(empty, placeholderText) {
		t.Fatalf("empty query: placeholder precedes the cursor: %q", empty)
	}

	m.query = "api"
	typed := m.renderPromptContent(80)
	if got := cursorColumn(t, typed); got != promptInset+5 {
		t.Fatalf("typed query: cursor column = %d, want %d: %q", got, promptInset+5, typed)
	}
}

// TestPromptContentKeepsItsInsetAndCount pins the prompt content row's chrome:
// one column of inset before the accent prompt — the inset the results rows
// carry — and a count that always reads shown/total.
func TestPromptContentKeepsItsInsetAndCount(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())

	row := sessionmgr.StripANSI(m.renderPromptContent(60))
	if !strings.HasPrefix(row, " ❯") {
		t.Fatalf("prompt row = %q, want a one-column inset before the prompt", row)
	}
	if !strings.HasSuffix(row, "4/4") {
		t.Fatalf("prompt row = %q, want the shown/total count", row)
	}

	m.query = "ses"
	if row := sessionmgr.StripANSI(m.renderPromptContent(60)); !strings.HasSuffix(row, "1/4") {
		t.Fatalf("filtered prompt row = %q, want the shown/total count", row)
	}
}

func TestPromptContentShowsTheHintAndCount(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents

	wide := m.renderPromptContent(78)
	plain := sessionmgr.StripANSI(wide)
	for _, sigil := range []string{"@", "!", "$", "*", "~", "/"} {
		if !strings.Contains(plain, sigil) {
			t.Fatalf("sigil hint missing %q: %q", sigil, plain)
		}
	}
	if !strings.HasSuffix(plain, "2/4") {
		t.Fatalf("the shown/total count is not right-aligned: %q", plain)
	}
	if !strings.Contains(wide, m.styles.chipActive.Render("@")) {
		t.Fatalf("the active scope's sigil is not accented: %q", wide)
	}

	// A narrow prompt window drops the hint but keeps the count.
	narrow := sessionmgr.StripANSI(m.renderPromptContent(20))
	if strings.Contains(narrow, "@") {
		t.Fatalf("hint kept in a narrow prompt window: %q", narrow)
	}
	if !strings.Contains(narrow, "2/4") {
		t.Fatalf("count dropped in a narrow prompt window: %q", narrow)
	}

	// The query wins: a long query leaves no room for the hint.
	m.query = strings.Repeat("q", 40)
	if row := sessionmgr.StripANSI(m.renderPromptContent(30)); strings.Contains(row, "@") {
		t.Fatalf("hint overlaps a long query: %q", row)
	}
}

func TestFooterCounterAndHintDropping(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents
	m.query = "claude"

	// The shown/total count lives in the prompt window, not the footer.
	prompt := sessionmgr.StripANSI(m.renderPromptContent(m.viewLayout().listWidth()))
	if !strings.Contains(prompt, "1/4") {
		t.Fatalf("prompt content = %q, want the 1/4 count", prompt)
	}

	wide := sessionmgr.StripANSI(m.renderFooter(m.width))
	if !strings.Contains(wide, "yazi") {
		t.Fatalf("wide footer dropped hints: %q", wide)
	}
	if strings.Contains(wide, "1/4") {
		t.Fatalf("footer still shows the count: %q", wide)
	}

	narrow := sessionmgr.StripANSI(m.renderFooter(40))
	if strings.Contains(narrow, "yazi") {
		t.Fatalf("narrow footer kept a hint that must drop from the end: %q", narrow)
	}
	if !strings.Contains(narrow, "↑↓ move") {
		t.Fatalf("narrow footer dropped every hint: %q", narrow)
	}
}

func TestKillConfirmationRendersInThePromptWindow(t *testing.T) {
	m := newTestModel(t)
	m = withItems(
		m,
		[]sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"}},
	)
	m, _ = press(t, m, "ctrl+x")

	prompt := sessionmgr.StripANSI(m.renderPromptContent(m.viewLayout().listWidth()))
	if !strings.Contains(prompt, "kill session demo?") {
		t.Fatalf("prompt window = %q, want the confirmation question", prompt)
	}
	if !strings.Contains(prompt, "⏎/y confirm · any other key cancels") {
		t.Fatalf("prompt window = %q, want the confirmation keys", prompt)
	}
	footer := sessionmgr.StripANSI(m.renderFooter(m.width))
	if !strings.Contains(footer, "confirm") {
		t.Fatalf("footer = %q, want the confirmation keys", footer)
	}
	if strings.Contains(footer, "yazi") {
		t.Fatalf("footer = %q, want the key hints replaced", footer)
	}
}

func TestFooterShowsRenameHintWhileRenaming(t *testing.T) {
	m := newTestModel(t)
	m = withItems(
		m,
		[]sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"}},
	)
	m, _ = press(t, m, "ctrl+r")
	footer := sessionmgr.StripANSI(m.renderFooter(m.width))
	if !strings.Contains(footer, "save") || !strings.Contains(footer, "cancel") {
		t.Fatalf("rename footer = %q", footer)
	}
	prompt := sessionmgr.StripANSI(m.renderPromptContent(m.viewLayout().listWidth()))
	if !strings.Contains(prompt, "rename") || !strings.Contains(prompt, "demo") {
		t.Fatalf("rename prompt window = %q", prompt)
	}
}

func TestRenderPreviewBottomAnchoredForCapturesAndTopForDirs(t *testing.T) {
	content := "line00\nline01\nline02\nline03\nline04\nline05\nline06\nline07\nline08\nline09"
	tests := []struct {
		name      string
		item      sessionmgr.Item
		wantLast  bool
		wantFirst bool
	}{
		{
			name:     "session capture keeps the tail",
			item:     sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
			wantLast: true,
		},
		{
			name: "directory listing keeps the head",
			item: sessionmgr.Item{
				Kind: sessionmgr.KindZoxide,
				Path: "/tmp/demo",
				Name: "/tmp/demo",
			},
			wantFirst: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{tt.item})
			m.preview = content
			m.previewKey = tt.item.Key()

			lines := m.renderPreviewContent(40, 6)
			if len(lines) != 6 {
				t.Fatalf("preview lines = %d, want exactly 6", len(lines))
			}
			out := strings.Join(lines, "\n")
			if tt.wantLast &&
				(!strings.Contains(out, "line09") || strings.Contains(out, "line00")) {
				t.Fatalf("bottom-anchored preview = %q", out)
			}
			if tt.wantFirst &&
				(!strings.Contains(out, "line00") || strings.Contains(out, "line09")) {
				t.Fatalf("top-anchored preview = %q", out)
			}
		})
	}
}

func TestPreviewClippingKeepsANSIAndWidth(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	red := "\x1b[31m"
	m.preview = red + strings.Repeat("x", 60) + "\x1b[0m"

	lines := m.renderPreviewContent(20, 4)
	if len(lines) != 4 {
		t.Fatalf("preview lines = %d, want 4", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, red) {
		t.Fatalf("preview colors were stripped:\n%q", joined)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 20 {
			t.Fatalf("preview line wider than the column (%d): %q", got, line)
		}
	}
}

// TestLayoutResolvesPreviewAndThresholds pins the layout split itself: the
// horizontal split needs 100 columns and falls back to the stacked layout below
// it, the preview column keeps its floor, and the stacked results keep their
// share of the rows.
func TestLayoutResolvesPreviewAndThresholds(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		layout     string
		preview    bool
		wantLayout string
		wantSplit  bool
	}{
		{
			"horizontal keeps the split",
			120,
			34,
			appconfig.LayoutHorizontal,
			true,
			appconfig.LayoutHorizontal,
			true,
		},
		{
			"horizontal falls back below 100",
			99,
			34,
			appconfig.LayoutHorizontal,
			true,
			appconfig.LayoutVertical,
			false,
		},
		{
			"vertical stays stacked",
			120,
			34,
			appconfig.LayoutVertical,
			true,
			appconfig.LayoutVertical,
			false,
		},
		{
			"dropdown stays stacked",
			120,
			34,
			appconfig.LayoutDropdown,
			true,
			appconfig.LayoutDropdown,
			false,
		},
		{
			"preview off takes the width",
			120,
			34,
			appconfig.LayoutHorizontal,
			false,
			appconfig.LayoutHorizontal,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := computeViewLayout(tt.width, tt.height, tt.layout, tt.preview)
			if l.layout != tt.wantLayout {
				t.Fatalf("effective layout = %q, want %q", l.layout, tt.wantLayout)
			}
			if tt.wantSplit {
				if l.preview.width < previewMinCols || l.preview.x != l.results.width+windowGap {
					t.Fatalf("preview column = %+v", l.preview)
				}
				return
			}
			if l.preview.height != 0 && !tt.preview {
				t.Fatalf("preview window drawn with the preview off: %+v", l.preview)
			}
			if tt.preview && l.preview.height == 0 {
				t.Fatalf("stacked preview window missing: %+v", l)
			}
		})
	}

	// The stacked split keeps the results window's share of the rows.
	l := computeViewLayout(80, 24, appconfig.LayoutVertical, true)
	rows := 24 - 1 - promptWindowRows
	wantResults := max(minResultsRows, rows*resultsPercent/100)
	if l.results.height != wantResults {
		t.Fatalf("stacked results height = %d, want %d", l.results.height, wantResults)
	}
	if l.preview.y != l.results.y+l.results.height {
		t.Fatalf("preview window does not follow the results: %+v", l)
	}
	if l.prompt.height != promptWindowRows {
		t.Fatalf("prompt window height = %d, want %d", l.prompt.height, promptWindowRows)
	}
}

func TestAgentStateGlyphWordAndColorComeFromTheIconSet(t *testing.T) {
	tests := []struct {
		state sessionmgr.AgentState
		glyph string
		word  string
		// want are the styled runs the badge draws: the glyph and the word each
		// in the state's color, except on the blocked badge, whose glyph and word
		// share one red block.
		want []string
	}{
		{sessionmgr.AgentBlocked, "∅", "blocked", []string{"\x1b[101m ∅ blocked "}},
		{sessionmgr.AgentWorking, "●", "working", []string{"\x1b[92m●", "\x1b[92mworking"}},
		{sessionmgr.AgentDone, "◉", "done", []string{"\x1b[96m◉", "\x1b[96mdone"}},
		{sessionmgr.AgentIdle, "○", "idle", []string{"\x1b[90m○", "\x1b[90midle"}},
		{sessionmgr.AgentUnknown, "?", "unknown", []string{"\x1b[90m?", "\x1b[90munknown"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			withColor(t)
			m := newTestModel(t)
			item := agentItem("claude", "%1", "frontend")
			item.AgentState = tt.state
			m = withItems(m, []sessionmgr.Item{item})

			out := m.View()
			if plain := sessionmgr.StripANSI(out); !strings.Contains(plain, tt.glyph) {
				t.Fatalf("%s glyph missing:\n%s", tt.state, plain)
			}
			if plain := sessionmgr.StripANSI(out); !strings.Contains(plain, tt.word) {
				t.Fatalf("%s word missing:\n%s", tt.state, plain)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("%s badge missing %q:\n%q", tt.state, want, out)
				}
			}
		})
	}
}

func TestAgentStateStyleOverrideFromConfig(t *testing.T) {
	withColor(t)
	cfg := defaultTestConfig()
	cfg.Icons.AgentState.Working = appconfig.IconConfig{
		Icon: "!", Label: "busy", Color: "#ff0000",
	}
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32
	item := agentItem("claude", "%1", "frontend")
	item.AgentState = sessionmgr.AgentWorking
	m = withItems(m, []sessionmgr.Item{item})

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	out := m.View()
	plain := sessionmgr.StripANSI(out)
	if !strings.Contains(plain, "!") || !strings.Contains(plain, "busy") {
		t.Fatalf("configured working glyph/label missing:\n%s", plain)
	}
	if strings.Contains(plain, "working") || strings.Contains(plain, "●") {
		t.Fatalf("built-in working vocabulary still drawn:\n%s", plain)
	}
	if !strings.Contains(out, style.Render("!")) || !strings.Contains(out, style.Render("busy")) {
		t.Fatalf("configured working color not applied:\n%q", out)
	}
}

func TestAgentStateTextModeOmitsTheGlyphColumn(t *testing.T) {
	withColor(t)
	cfg := defaultTestConfig()
	cfg.Icons.Mode = appconfig.IconModeText
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32
	item := agentItem("claude", "%1", "frontend")
	item.AgentState = sessionmgr.AgentBlocked
	m = withItems(m, []sessionmgr.Item{item})

	plain := sessionmgr.StripANSI(m.View())
	if strings.Contains(plain, "∅") {
		t.Fatalf("glyph column drawn in text mode:\n%s", plain)
	}
	if !strings.Contains(plain, "blocked") {
		t.Fatalf("state word missing in text mode:\n%s", plain)
	}
}

func TestAgentStateNoneModeHidesGlyphAndWord(t *testing.T) {
	withColor(t)
	cfg := defaultTestConfig()
	cfg.Icons.AgentStateMode = appconfig.StateDisplayModeNone
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32
	item := agentItem("claude", "%1", "frontend")
	item.AgentState = sessionmgr.AgentBlocked
	m = withItems(m, []sessionmgr.Item{item})

	plain := sessionmgr.StripANSI(m.View())
	if strings.Contains(plain, "∅") || strings.Contains(plain, "blocked") {
		t.Fatalf("agent state drawn in none mode:\n%s", plain)
	}
}

// itemLines renders one item, found by key, as the results window draws it at
// width: the styled lines and the same lines stripped. A test checks colors on
// the styled lines and text on the plain ones.
//
//nolint:unparam // test helper; the width and selection are constant so far
func itemLines(
	t *testing.T,
	m Model,
	key string,
	width int,
	selected bool,
) (styled, plain []string) {
	t.Helper()
	for _, r := range m.rows() {
		if r.item.Key() != key {
			continue
		}
		styled = m.renderItem(r, width, selected)
		for _, line := range styled {
			plain = append(plain, sessionmgr.StripANSI(line))
		}
		return styled, plain
	}
	t.Fatalf("no item with key %q in %v", key, m.rows())
	return nil, nil
}

// itemMix is one item of every kind, for the tests that render the whole
// mixed-height results window.
func itemMix() []sessionmgr.Item {
	agent := agentItem("claude", "%1", "frontend")
	agent.AgentDisplayName = "claude"
	agent.AgentState = sessionmgr.AgentBlocked
	agent.StartedAt = time.Now().Add(-3 * time.Minute)
	return []sessionmgr.Item{
		agent,
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo", Name: "~/code/demo"},
		{Kind: sessionmgr.KindSession, Name: "seshagy", Target: "seshagy"},
		{Kind: sessionmgr.KindHost, Name: "termux", Target: "seshagy-termux", Location: "ssh"},
	}
}

// TestAgentItemDrawsBoldNameStateBadgeAndContext pins the agent item: line 1 is
// the icon, the bold display name and the state badge right after it — the
// configured glyph and word in the state's color, a blocked agent on the design's
// red block — and line 2 is the dim workspace and age indented under the name.
func TestAgentItemDrawsBoldNameStateBadgeAndContext(t *testing.T) {
	withColor(t)
	tests := []struct {
		state sessionmgr.AgentState
		glyph string
		word  string
	}{
		{sessionmgr.AgentBlocked, "∅", "blocked"},
		{sessionmgr.AgentWorking, "●", "working"},
		{sessionmgr.AgentDone, "◉", "done"},
		{sessionmgr.AgentIdle, "○", "idle"},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			item := agentItem("claude", "%1", "frontend")
			item.AgentState = tt.state
			item.StartedAt = time.Now().Add(-3 * time.Minute)
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{item})

			styled, plain := itemLines(t, m, item.Key(), 60, false)
			if len(styled) != 2 {
				t.Fatalf("agent item = %d lines, want 2:\n%s",
					len(styled), strings.Join(plain, "\n"))
			}
			bold := m.styles.itemName.Bold(true)
			if !strings.Contains(styled[0], bold.Render("claude")) {
				t.Fatalf("line 1 = %q, want the name bold", styled[0])
			}
			// The badge sits two columns after the name, with either the state's
			// own colors or the blocked block.
			if !strings.Contains(plain[0], "claude  "+tt.glyph+" "+tt.word) {
				t.Fatalf("line 1 = %q, want %q two spaces after the name",
					plain[0], tt.glyph+" "+tt.word)
			}
			if tt.state == sessionmgr.AgentBlocked {
				badge := m.styles.badgeBlocked.Render(" " + tt.glyph + " " + tt.word + " ")
				if !strings.Contains(styled[0], badge) {
					t.Fatalf("line 1 = %q, want the red blocked badge %q", styled[0], badge)
				}
			} else {
				style := m.agentStateStyle(tt.state)
				if !strings.Contains(styled[0], style.Render(tt.glyph)+" "+style.Render(tt.word)) {
					t.Fatalf("line 1 = %q, want the glyph and word in the state color", styled[0])
				}
			}
			// Line 2 is the dim workspace and age, indented under the name.
			if !strings.Contains(plain[1], "frontend · 3m") {
				t.Fatalf("line 2 = %q, want the workspace and age", plain[1])
			}
			if !strings.HasPrefix(plain[1], strings.Repeat(" ", m.rowPrefixWidth())+"frontend") {
				t.Fatalf("line 2 = %q, want it indented under the name", plain[1])
			}
			for _, part := range []string{"frontend", "3m"} {
				if !strings.Contains(styled[1], m.styles.muted.Render(part)) {
					t.Fatalf("line 2 = %q, want %q dim", styled[1], part)
				}
			}
			for _, line := range styled {
				if got := ansi.StringWidth(line); got != 60 {
					t.Fatalf("line = %d columns, want 60: %q", got, line)
				}
			}
		})
	}
}

// TestAgentItemNamesTheAgentTypeWithoutALogo pins the fallback the dim context
// line carries: with the logo font in use the mark says which agent this is, and
// without one the brand key is the only place the type can show. An agent whose
// name is already the title repeats it nowhere.
func TestAgentItemNamesTheAgentTypeWithoutALogo(t *testing.T) {
	m := newTestModel(t)
	t.Setenv("SESHAGY_AGENT_ICONS", "font")
	item := agentItem("claude", "%1", "skills")
	item.AgentDisplayName = "review-spec"
	m = withItems(m, []sessionmgr.Item{item})

	_, plain := itemLines(t, m, item.Key(), 60, false)
	if len(plain) != 2 || !strings.Contains(strings.Join(plain, "\n"), "skills") {
		t.Fatalf("item = %q, want the workspace", strings.Join(plain, "\n"))
	}
	if strings.Contains(strings.Join(plain, " "), "claude") {
		t.Fatalf("the agent type is named beside its logo:\n%s", strings.Join(plain, "\n"))
	}

	t.Setenv("SESHAGY_AGENT_ICONS", "none")
	_, plain = itemLines(t, m, item.Key(), 60, false)
	if !strings.Contains(plain[1], "claude") {
		t.Fatalf("line 2 = %q, want the agent type without a logo", plain[1])
	}

	named := item
	named.AgentDisplayName = "claude"
	m = withItems(m, []sessionmgr.Item{named})
	_, plain = itemLines(t, m, named.Key(), 60, false)
	if strings.Count(strings.Join(plain, " "), "claude") != 1 {
		t.Fatalf("item = %q, want the name once", strings.Join(plain, "\n"))
	}
}

// TestSessionItemDrawsAttachedFlagAndAgentTally pins the session item: line 1 is
// the icon, the bold name and the green attached flag one column after it — and
// nothing at all while the session is detached — and line 2 is the window count
// followed by the agents inside the session, one space between a state glyph and
// its count.
func TestSessionItemDrawsAttachedFlagAndAgentTally(t *testing.T) {
	withColor(t)
	working := agentItem("pi", "%1", "seshagy")
	working.AgentState = sessionmgr.AgentWorking
	working.Session = "seshagy"
	idle := agentItem("codex", "%2", "seshagy")
	idle.AgentState = sessionmgr.AgentIdle
	idle.Session = "seshagy"
	attached := sessionmgr.Item{
		Kind:     sessionmgr.KindSession,
		Name:     "seshagy",
		Target:   "seshagy",
		Attached: true,
		Windows:  7,
	}
	detached := sessionmgr.Item{
		Kind:    sessionmgr.KindSession,
		Name:    "fff-cli",
		Target:  "fff-cli",
		Windows: 1,
	}

	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{working, idle, attached, detached})
	m.terms = sessionmgr.TmuxTerms()

	styled, plain := itemLines(t, m, attached.Key(), 60, false)
	if len(styled) != 2 {
		t.Fatalf("session item = %d lines, want 2:\n%s", len(styled), strings.Join(plain, "\n"))
	}
	if !strings.Contains(styled[0], m.styles.itemName.Bold(true).Render("seshagy")) {
		t.Fatalf("line 1 = %q, want the name bold", styled[0])
	}
	if !strings.Contains(plain[0], "seshagy • attached") {
		t.Fatalf("line 1 = %q, want the attached flag one space after the name", plain[0])
	}
	if !strings.Contains(styled[0], m.styles.success.Render("• attached")) {
		t.Fatalf("line 1 = %q, want the attached flag in the success accent", styled[0])
	}
	if !strings.Contains(plain[1], "7 windows · ● 1 ○ 1") {
		t.Fatalf("line 2 = %q, want the window count and the agent tally", plain[1])
	}
	for _, glyph := range []struct {
		state sessionmgr.AgentState
		text  string
	}{
		{sessionmgr.AgentWorking, "●"},
		{sessionmgr.AgentIdle, "○"},
	} {
		want := m.agentStateStyle(glyph.state).Render(glyph.text)
		if !strings.Contains(styled[1], want+" 1") {
			t.Fatalf("line 2 = %q, want the %s glyph colored before its count",
				styled[1], glyph.state)
		}
	}

	// A detached session shows no flag, and a session with no agents drops the
	// tally rather than the window count.
	_, detachedPlain := itemLines(t, m, detached.Key(), 60, false)
	if strings.Contains(detachedPlain[0], "attached") {
		t.Fatalf("line 1 = %q, want no flag on a detached session", detachedPlain[0])
	}
	if !strings.Contains(detachedPlain[1], "1 window") ||
		strings.Contains(detachedPlain[1], "·") {
		t.Fatalf("line 2 = %q, want the single window count alone", detachedPlain[1])
	}
}

// TestSessionItemFollowsTheBackendVocabulary pins the window wording on the
// second line: the session's own backend names its windows, so herdr counts tabs
// and tmux counts windows, and the tally never spells a kind word.
func TestSessionItemFollowsTheBackendVocabulary(t *testing.T) {
	tests := []struct {
		name  string
		terms sessionmgr.Terms
		want  string
	}{
		{name: "herdr tabs", terms: sessionmgr.HerdrTerms(), want: "2 tabs"},
		{name: "tmux windows", terms: sessionmgr.TmuxTerms(), want: "2 windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := sessionmgr.Item{
				Kind:    sessionmgr.KindSession,
				Name:    "skills",
				Target:  "skills",
				Windows: 2,
			}
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{session})
			m.terms = tt.terms

			_, plain := itemLines(t, m, session.Key(), 60, false)
			if !strings.Contains(plain[1], tt.want) {
				t.Fatalf("line 2 = %q, want %q", plain[1], tt.want)
			}
		})
	}
}

// TestDirItemIsOneDimParentBrightBasenameLine pins the directory item: a single
// line of the dim parent path, trailing slash included, and the bright basename
// it is about. The parent path is not repeated in a second line and no source
// tag stands in for it.
func TestDirItemIsOneDimParentBrightBasenameLine(t *testing.T) {
	withColor(t)
	for _, kind := range []sessionmgr.Kind{sessionmgr.KindZoxide, sessionmgr.KindFD} {
		t.Run(string(kind), func(t *testing.T) {
			item := sessionmgr.Item{
				Kind: kind,
				Path: "~/.pi/agent/extensions",
				Name: "~/.pi/agent/extensions",
			}
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{item})

			styled, plain := itemLines(t, m, item.Key(), 60, false)
			if len(styled) != 1 {
				t.Fatalf("directory item = %d lines, want 1:\n%s",
					len(styled), strings.Join(plain, "\n"))
			}
			if !strings.Contains(styled[0], m.styles.muted.Render("~/.pi/agent/")) {
				t.Fatalf("line = %q, want the parent path dim", styled[0])
			}
			if !strings.Contains(styled[0], m.styles.itemName.Render("extensions")) {
				t.Fatalf("line = %q, want the basename bright", styled[0])
			}
			if !strings.Contains(plain[0], "~/.pi/agent/extensions") {
				t.Fatalf("line = %q, want the whole path", plain[0])
			}
			for _, absent := range []string{"zoxide", "fd", "find"} {
				if strings.Contains(plain[0], absent) {
					t.Fatalf("line = %q, want no %q tag", plain[0], absent)
				}
			}
		})
	}
}

// TestHostItemShowsTransportAndDropsItsOwnAlias pins the host item: a single line
// of the bright name and the dim transport and alias, with the alias left out
// when it only repeats the name (a host discovered in ~/.ssh/config uses its own
// name).
func TestHostItemShowsTransportAndDropsItsOwnAlias(t *testing.T) {
	withColor(t)
	registered := sessionmgr.Item{
		Kind:     sessionmgr.KindHost,
		Name:     "termux",
		Target:   "seshagy-termux",
		Location: "ssh",
	}
	discovered := sessionmgr.Item{
		Kind:     sessionmgr.KindHost,
		Name:     "gitbox",
		Target:   "gitbox",
		Location: "ssh",
	}
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{registered, discovered})

	styled, plain := itemLines(t, m, registered.Key(), 60, false)
	if len(styled) != 1 {
		t.Fatalf("host item = %d lines, want 1:\n%s", len(styled), strings.Join(plain, "\n"))
	}
	if !strings.Contains(plain[0], "termux  ssh · seshagy-termux") {
		t.Fatalf("line = %q, want the transport and alias two spaces after the name", plain[0])
	}
	if !strings.Contains(styled[0], m.styles.itemName.Render("termux")) {
		t.Fatalf("line = %q, want the name bright", styled[0])
	}
	if !strings.Contains(styled[0], m.styles.muted.Render("ssh · seshagy-termux")) {
		t.Fatalf("line = %q, want the transport and alias dim", styled[0])
	}

	_, plain = itemLines(t, m, discovered.Key(), 60, false)
	if !strings.HasSuffix(strings.TrimRight(plain[0], " "), "gitbox  ssh") {
		t.Fatalf("line = %q, want the transport alone", plain[0])
	}
	if strings.Contains(plain[0], "·") {
		t.Fatalf("line = %q, want no alias column", plain[0])
	}
}

// TestItemLinesSpellNoKindWords pins the wording of the results items: the kind
// is what the icon and the name already say, so no item repeats it.
func TestItemLinesSpellNoKindWords(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, itemMix())
	out := sessionmgr.StripANSI(m.View())
	for _, word := range []string{" · agent", " · session", " · host", " · dir"} {
		if strings.Contains(out, word) {
			t.Fatalf("results window spells a kind word %q:\n%s", word, out)
		}
	}
}

// TestItemLinesAreExactlyTheResultsWidth pins the width of every item line in
// every layout: each line the results window draws is exactly its inner width, so
// the frame cannot wrap.
func TestItemLinesAreExactlyTheResultsWidth(t *testing.T) {
	sizes := []struct{ width, height int }{{120, 34}, {80, 24}}
	for _, size := range sizes {
		for _, layout := range []string{
			appconfig.LayoutHorizontal,
			appconfig.LayoutVertical,
			appconfig.LayoutDropdown,
		} {
			for _, preview := range []bool{true, false} {
				t.Run(fmt.Sprintf("%dx%d/%s/preview=%v", size.width, size.height, layout, preview),
					func(t *testing.T) {
						m := newTestModel(t)
						m.width, m.height = size.width, size.height
						m.layout = layout
						m.showPreview = preview
						m = withItems(m, itemMix())

						l := m.viewLayout()
						if l.listWidth() <= 0 || l.listHeight() <= 0 {
							t.Fatalf("results window has no room: %+v", l.results)
						}
						for _, r := range m.rows() {
							for i, line := range m.renderItem(r, l.listWidth(), r.item.Key() == m.selectedKey()) {
								if got := ansi.StringWidth(line); got != l.listWidth() {
									t.Fatalf("item %s line %d = %d columns, want %d: %q",
										r.item.Key(), i, got, l.listWidth(), line)
								}
							}
						}
						frameLines(t, m)
					})
			}
		}
	}
}

func TestTextIconModeHidesPrivateUseAgentGlyphs(t *testing.T) {
	t.Setenv("SESHAGY_AGENT_ICONS", "font")
	cfg := defaultTestConfig()
	cfg.Icons.Mode = "text"
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 30
	m = withItems(m, []sessionmgr.Item{agentItem("claude", "%1", "frontend")})

	plain := sessionmgr.StripANSI(m.View())
	if strings.ContainsRune(plain, rune(agentLogoBase)) {
		t.Fatalf("private-use logo printed in text icon mode:\n%s", plain)
	}
}

// titleSegment is one style run of an expected title.
type titleSegment struct {
	text  string
	style func(m Model) lipgloss.Style
}

// TestDirRowSplitsParentAndBasename pins the directory row: the path splits at
// its last slash into the dim parent, trailing slash included, and the bright
// basename the row is about. The match runes are emphasized on whichever side of
// the split they fall.
func TestDirRowSplitsParentAndBasename(t *testing.T) {
	withColor(t)
	muted := func(m Model) lipgloss.Style { return m.styles.muted }
	name := func(m Model) lipgloss.Style { return m.styles.itemName }
	match := func(m Model) lipgloss.Style { return m.styles.match }
	tests := []struct {
		name    string
		path    string
		matches []int
		want    []titleSegment
	}{
		{
			name: "nested path",
			path: "internal/tui/view.go",
			want: []titleSegment{{"internal/tui/", muted}, {"view.go", name}},
		},
		{
			name: "home relative",
			path: "~/code/demo",
			want: []titleSegment{{"~/code/", muted}, {"demo", name}},
		},
		{
			name: "absolute two levels",
			path: "/tmp/demo",
			want: []titleSegment{{"/tmp/", muted}, {"demo", name}},
		},
		{
			name: "root level basename",
			path: "/demo",
			want: []titleSegment{{"/", muted}, {"demo", name}},
		},
		{
			name: "basename only",
			path: "demo",
			want: []titleSegment{{"demo", name}},
		},
		{
			name:    "match in the basename",
			path:    "internal/tui/view.go",
			matches: []int{13},
			want: []titleSegment{
				{"internal/tui/", muted}, {"v", match}, {"iew.go", name},
			},
		},
		{
			name:    "match in the parent",
			path:    "internal/tui/view.go",
			matches: []int{2},
			want: []titleSegment{
				{"in", muted}, {"t", match}, {"ernal/tui/", muted}, {"view.go", name},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			item := sessionmgr.Item{Kind: sessionmgr.KindFD, Path: tt.path, Name: tt.path}
			var want strings.Builder
			for _, seg := range tt.want {
				want.WriteString(seg.style(m).Render(seg.text))
			}
			got := m.renderTitle(row{item: item, matches: tt.matches}, 80)
			if got != want.String() {
				t.Fatalf("title = %q, want %q", got, want.String())
			}
		})
	}
}

// TestResultsWindowCountersLiveInTheBottomBorder pins the results window frame:
// every inner line belongs to an item and the scroll counters are drawn in the
// bottom border instead of taking lines away from the list. The counters count
// items even though a session spends two lines of the window.
func TestResultsWindowCountersLiveInTheBottomBorder(t *testing.T) {
	items := make([]sessionmgr.Item, 40)
	for i := range items {
		items[i] = sessionmgr.Item{
			Kind:   sessionmgr.KindSession,
			Name:   fmt.Sprintf("s%02d", i),
			Target: fmt.Sprintf("s%02d", i),
		}
	}
	tests := []struct {
		name         string
		cursor       int
		wantAbove    string
		wantBelow    string
		wantSelected string
	}{
		{name: "top only", cursor: 0, wantBelow: "↓ 33 more", wantSelected: "s00"},
		{
			name:         "both",
			cursor:       20,
			wantAbove:    "↑ 14 more · ↓ 19 more",
			wantSelected: "s20",
		},
		{name: "bottom only", cursor: 39, wantAbove: "↑ 33 more", wantSelected: "s39"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width, m.height = 120, 20
			m.showPreview = false
			m = withItems(m, items)
			m = m.setCursor(tt.cursor)

			l := m.viewLayout()
			block := m.renderResultsWindow(l)
			if len(block) != l.results.height {
				t.Fatalf("results window is %d lines, want %d", len(block), l.results.height)
			}
			if l.listHeight() != 14 {
				t.Fatalf("results inner height = %d, want 14", l.listHeight())
			}
			bottom := sessionmgr.StripANSI(block[len(block)-1])
			if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
				t.Fatalf("results bottom border = %q", bottom)
			}
			if want := tt.wantAbove + tt.wantBelow; want != "" && !strings.Contains(bottom, want) {
				t.Fatalf("results bottom border = %q, want %q", bottom, want)
			}
			// No inner line is spent on a counter, and the selected item is one
			// solid bar over both of its lines. The bar is a background sequence,
			// which a profile that draws no styling cannot emit: that one falls
			// back to the accent marker.
			barOpen := backgroundOpen(m.styles.selectionBG)
			var selected []string
			for _, line := range block[1 : len(block)-1] {
				plain := sessionmgr.StripANSI(line)
				if strings.Contains(plain, "more") {
					t.Fatalf("counter drawn inside the results window: %q", plain)
				}
				isSelected := strings.Contains(plain, "▌")
				if barOpen != "" {
					isSelected = strings.Contains(line, barOpen)
				}
				if isSelected {
					selected = append(selected, plain)
				}
			}
			if len(selected) != 2 {
				t.Fatalf("selection bar covers %d lines, want the item's 2", len(selected))
			}
			if !strings.Contains(selected[0], tt.wantSelected) {
				t.Fatalf("selected item = %q, want %q", selected[0], tt.wantSelected)
			}
		})
	}

	// No counters when every row fits.
	m := newTestModel(t)
	m.width, m.height = 120, 20
	m = withItems(m, items[:4])
	block := m.renderResultsWindow(m.viewLayout())
	if out := sessionmgr.StripANSI(strings.Join(block, "\n")); strings.Contains(out, "more") {
		t.Fatalf("counter drawn without hidden rows:\n%s", out)
	}
}

// rowsFor builds the rows a list of items draws as, in item order. The rows all
// carry one section (the zero group), so the frame draws no header for them: a
// lone section has none.
func rowsFor(items []sessionmgr.Item) []row {
	rows := make([]row, len(items))
	for i, item := range items {
		rows[i] = row{item: item}
	}
	return rows
}

// drawnItems lists the items a window draws, in order, checking that none of them
// is drawn split apart.
func drawnItems(t *testing.T, win itemWindow) []int {
	t.Helper()
	items := make([]int, 0, win.shown)
	for i := 0; i < len(win.lines); i++ {
		line := win.lines[i]
		if line.item < 0 {
			continue
		}
		if line.detail {
			if i == 0 || win.lines[i-1].item != line.item {
				t.Fatalf(
					"item %d's continuation line is drawn on its own: %#v",
					line.item,
					win.lines,
				)
			}
			continue
		}
		if i+1 < len(win.lines) && win.lines[i+1].item == line.item {
			if !win.lines[i+1].detail {
				t.Fatalf("item %d is drawn split apart: %#v", line.item, win.lines)
			}
			i++
		}
		items = append(items, line.item)
	}
	return items
}

// drawnHeaders lists the sections whose header line a window draws, in order.
func drawnHeaders(win itemWindow) []group {
	groups := make([]group, 0, groupCount)
	for _, line := range win.lines {
		if line.item < 0 {
			groups = append(groups, line.group)
		}
	}
	return groups
}

// mixedHeightItems is a list of one- and two-line items in item order: sessions
// and agents take two lines, hosts and directories one, so the window's line
// budget and its item counters diverge. (A model ranks and groups the same items
// before drawing them.)
func mixedHeightItems() []sessionmgr.Item {
	session := func(name string) sessionmgr.Item {
		return sessionmgr.Item{Kind: sessionmgr.KindSession, Name: name, Target: name}
	}
	dir := func(path string) sessionmgr.Item {
		return sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: path, Name: path}
	}
	return []sessionmgr.Item{
		session("s0"),
		dir("~/a"),
		agentItem("pi", "%1", "s0"),
		session("s3"),
		{Kind: sessionmgr.KindHost, Name: "termux", Target: "termux", Location: "ssh"},
		dir("~/b"),
		session("s6"),
	}
}

// TestItemFlowWindowKeepsWholeItemsAroundTheCursor pins the scroll math: the
// window is measured in lines, so it never opens on an item's second line and
// never draws half an item, the cursor's item is always inside it, and the items
// hidden above and below are still counted in items.
func TestItemFlowWindowKeepsWholeItemsAroundTheCursor(t *testing.T) {
	flow := newItemFlow(rowsFor(mixedHeightItems()), false)
	tests := []struct {
		name                    string
		cursor, height          int
		wantStart, wantEnd      int
		wantAbove, wantBelow    int
		wantLines               int
		clippedCursorItemTooBig bool
	}{
		{
			name: "cursor at the top", cursor: 0, height: 5,
			wantStart: 0, wantEnd: 3, wantBelow: 4, wantLines: 5,
		},
		{
			// The next item is two lines tall and only one line is left over, so
			// the last line stays blank rather than showing half an item.
			name:      "scrolling down from the top keeps the cursor at the bottom edge",
			cursor:    3,
			height:    5,
			wantStart: 1,
			wantEnd:   4,
			wantAbove: 1,
			wantBelow: 3,
			wantLines: 5,
		},
		{
			// The item above the window is two lines tall and only one line is
			// left over, so that line stays blank rather than showing half an item.
			name: "cursor at the bottom leaves one line blank", cursor: 6, height: 5,
			wantStart: 4, wantEnd: 7, wantAbove: 4, wantLines: 4,
		},
		{
			name: "every item fits", cursor: 3, height: 11,
			wantStart: 0, wantEnd: 7, wantLines: 11,
		},
		{
			// A window too short for the cursor's own two-line item still shows
			// it, clipped, so the list is never blank.
			name: "two-line item in a one-line window", cursor: 6, height: 1,
			wantStart: 6, wantEnd: 7, wantAbove: 6,
			clippedCursorItemTooBig: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			win := flow.window(tt.cursor, 0, tt.height)
			items := drawnItems(t, win)
			if len(items) == 0 || items[0] != tt.wantStart || items[len(items)-1] != tt.wantEnd-1 {
				t.Fatalf("window draws items %v, want [%d, %d)", items, tt.wantStart, tt.wantEnd)
			}
			if win.above != tt.wantAbove || win.below != tt.wantBelow {
				t.Fatalf("hidden items = (%d above, %d below), want (%d, %d)",
					win.above, win.below, tt.wantAbove, tt.wantBelow)
			}
			if win.above > 0 && items[0] != win.above {
				t.Fatalf("window hides %d items above but opens on item %d", win.above, items[0])
			}
			if win.below > 0 && items[len(items)-1] != flow.items-1-win.below {
				t.Fatalf(
					"window hides %d items below but closes on item %d",
					win.below,
					items[len(items)-1],
				)
			}
			if !slices.Contains(items, tt.cursor) {
				t.Fatalf("window %v does not hold the cursor item %d", items, tt.cursor)
			}
			if got := win.above + win.shown + win.below; got != flow.items {
				t.Fatalf("window accounts for %d of %d items", got, flow.items)
			}
			lines := 0
			for _, item := range items {
				lines += flow.heights[item]
			}
			if tt.clippedCursorItemTooBig {
				if lines <= tt.height {
					t.Fatalf("window draws %d lines, want more than the %d it has",
						lines, tt.height)
				}
				return
			}
			if lines != tt.wantLines {
				t.Fatalf("window draws %d lines, want %d", lines, tt.wantLines)
			}
			if lines > tt.height {
				t.Fatalf("window draws %d lines in a %d line frame", lines, tt.height)
			}
		})
	}
}

// TestResultsWindowScrollsMixedHeightItemsByLines draws the window over a
// one- and two-line list the model ranks and groups: the cursor's item is whole
// wherever it sits, a leftover line is left blank rather than half drawn (a
// section header counts against the room like an item line), and the counters
// still count items.
func TestResultsWindowScrollsMixedHeightItemsByLines(t *testing.T) {
	tests := []struct {
		name              string
		cursor, height    int
		wantAbove         int
		wantBelow         int
		wantLines         int
		wantCursorClipped bool
	}{
		{
			// The sessions header and the row under it need three of the five
			// lines, so the next row cannot be drawn and one line stays blank.
			name: "cursor at the top with an odd leftover line", cursor: 0, height: 5,
			wantBelow: 6, wantLines: 3,
		},
		{
			name: "cursor below the first window scrolls just far enough", cursor: 3, height: 5,
			wantAbove: 2, wantBelow: 3, wantLines: 4,
		},
		{
			name: "cursor at the bottom", cursor: 6, height: 5,
			wantAbove: 4, wantLines: 5,
		},
		{
			name: "a one-line item in a two-line window", cursor: 5, height: 2,
			wantAbove: 5, wantBelow: 1, wantLines: 2,
		},
		{
			name: "a one-line window clips the cursor's item", cursor: 3, height: 1,
			wantAbove: 3, wantBelow: 3, wantLines: 1, wantCursorClipped: true,
		},
		{name: "every item fits", cursor: 3, height: 15, wantLines: 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const width = 60
			m := newTestModel(t)
			m = withItems(m, mixedHeightItems())
			m = m.setCursor(tt.cursor)
			rows := m.rows()

			lines, above, below := m.resultsRows(rows, width, tt.height)
			if above != tt.wantAbove || below != tt.wantBelow {
				t.Fatalf("counters = (%d above, %d below), want (%d, %d)",
					above, below, tt.wantAbove, tt.wantBelow)
			}
			if len(lines) != tt.wantLines {
				t.Fatalf("window drew %d lines, want %d:\n%s",
					len(lines), tt.wantLines, strings.Join(lines, "\n"))
			}
			if len(lines) > tt.height {
				t.Fatalf("window drew %d lines in a %d line frame", len(lines), tt.height)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got != width {
					t.Fatalf("line %d = %d columns, want %d: %q", i, got, width, line)
				}
			}
			// The cursor's item is drawn whole, unless the frame is too short for
			// it: then its first line is drawn rather than nothing at all.
			item := m.renderItem(rows[tt.cursor], width, true)
			drawn := 0
			for _, want := range item {
				for _, got := range lines {
					if got == want {
						drawn++
					}
				}
			}
			wantDrawn := len(item)
			if tt.wantCursorClipped {
				wantDrawn = 1
			}
			if drawn != wantDrawn {
				t.Fatalf("the cursor item drew %d of %d lines:\n%s",
					drawn, len(item), strings.Join(lines, "\n"))
			}
			// A blank line only comes from a row that cannot be drawn whole: not
			// the next row below with its header, and not the one above.
			if len(lines) < tt.height {
				room := tt.height - len(lines)
				if below > 0 {
					if n := linesNeededFor(rows, len(rows)-below, m.grouped(rows)); n <= room {
						t.Fatalf("window left %d lines blank with room for the row below", room)
					}
				}
				if above > 0 && linesNeededFor(rows, above-1, false) <= room {
					t.Fatalf("window left %d lines blank with room for the row above", room)
				}
			}
		})
	}
}

// linesNeededFor is the lines a frame spends on the row at index: the row's own
// lines, plus the header that titles it when the list draws headers and the row
// opens a section.
func linesNeededFor(rows []row, index int, grouped bool) int {
	if index < 0 || index >= len(rows) {
		return 0
	}
	need := itemHeight(rows[index].item)
	if grouped && (index == 0 || rows[index].group != rows[index-1].group) {
		need++
	}
	return need
}

// groupedItems holds every section but "find", so an empty-query list draws one
// header per section it holds.
func groupedItems() []sessionmgr.Item {
	blocked := agentItem("claude", "%1", "frontend")
	blocked.AgentState = sessionmgr.AgentBlocked
	done := agentItem("codex", "%2", "api")
	done.AgentState = sessionmgr.AgentDone
	return []sessionmgr.Item{
		blocked,
		done,
		agentItem("pi", "%3", "skills"),
		{Kind: sessionmgr.KindSession, Name: "seshagy", Target: "seshagy", Windows: 2},
		{
			Kind:     sessionmgr.KindHost,
			Name:     "homelab",
			Target:   "seshagy-homelab",
			Location: remote.TransportSSH,
		},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo", Name: "~/code/demo"},
	}
}

// rowTitles lists the titles of the rows a list draws, in order.
func rowTitles(rows []row) []string {
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = itemTitle(r.item)
	}
	return titles
}

// sectionStart is the index of the first row in a section.
func sectionStart(rows []row, g group) int {
	for i, r := range rows {
		if r.group == g {
			return i
		}
	}
	return -1
}

// TestItemFlowWindowDrawsEachHeaderWithItsFirstRow pins the grouped scroll math:
// a header is drawn on the line above the first row of its section and never
// without it, the cursor's item is always inside the window whole, and the
// counters still count items.
func TestItemFlowWindowDrawsEachHeaderWithItsFirstRow(t *testing.T) {
	rows := filterRows(groupedItems(), scopeAll, "")
	flow := newItemFlow(rows, true)
	for height := 2; height <= 20; height++ {
		for cursor := range rows {
			win := flow.window(cursor, 0, height)
			if len(win.lines) > height {
				t.Fatalf("window(%d, %d) draws %d lines, over the %d it has",
					cursor, height, len(win.lines), height)
			}
			items := drawnItems(t, win)
			if !slices.Contains(items, cursor) {
				t.Fatalf("window(%d, %d) hides the cursor item: %v", cursor, height, items)
			}
			// The drawn items are one contiguous run, and the counters account
			// for every other item.
			for i := 1; i < len(items); i++ {
				if items[i] != items[i-1]+1 {
					t.Fatalf("window(%d, %d) draws items %v out of order", cursor, height, items)
				}
			}
			if items[0] != win.above || items[len(items)-1] != flow.items-1-win.below {
				t.Fatalf("window(%d, %d) = items %v, above %d, below %d",
					cursor, height, items, win.above, win.below)
			}
			if got := win.above + win.shown + win.below; got != flow.items {
				t.Fatalf("window(%d, %d) accounts for %d of %d items",
					cursor, height, got, flow.items)
			}
			// A header belongs to a section's first row: it is drawn directly
			// above that row, never twice, and in section order.
			before := group(-1)
			for i, line := range win.lines {
				if line.item >= 0 {
					continue
				}
				if line.group <= before {
					t.Fatalf("window(%d, %d) draws headers out of order: %v",
						cursor, height, drawnHeaders(win))
				}
				before = line.group
				if i+1 >= len(win.lines) || win.lines[i+1].item != sectionStart(rows, line.group) {
					t.Fatalf("window(%d, %d) titles section %v without its first row",
						cursor, height, line.group)
				}
			}
		}
	}
}

// TestItemFlowWindowLeavesALineBlankRatherThanStrandAHeader pins the grouped
// window's bottom edge: a header is drawn only when every line of the row it
// titles fits under it, so the leftover line stays blank instead of half an item.
func TestItemFlowWindowLeavesALineBlankRatherThanStrandAHeader(t *testing.T) {
	rows := filterRows(groupedItems(), scopeAll, "")
	flow := newItemFlow(rows, true)

	// Four lines hold the needs-you header and the blocked agent's two lines; the
	// sessions header under them would be stranded without its row.
	win := flow.window(0, 0, 4)
	if got := drawnHeaders(win); !slices.Equal(got, []group{groupNeedsYou}) {
		t.Fatalf("headers = %v, want the needs-you header alone", got)
	}
	if len(win.lines) != 3 {
		t.Fatalf("window drew %d lines, want the header and one row's 3", len(win.lines))
	}
	if win.shown != 1 || win.below != flow.items-1 {
		t.Fatalf("window shows %d items with %d below, want the first item alone",
			win.shown, win.below)
	}
}

// TestItemFlowWindowScrollsHeadersInAndOutWithTheirSections checks the header a
// scrolled window draws: the header of the section the cursor sits in comes back
// with its first row, and a header whose section scrolled past goes with it.
func TestItemFlowWindowScrollsHeadersInAndOutWithTheirSections(t *testing.T) {
	items := make([]sessionmgr.Item, 0, 9)
	for i := range 6 {
		items = append(items, agentItem("pi", fmt.Sprintf("%%%d", i), fmt.Sprintf("ws%d", i)))
	}
	for i := range 3 {
		name := fmt.Sprintf("s%d", i)
		items = append(items, sessionmgr.Item{
			Kind: sessionmgr.KindSession, Name: name, Target: name, Windows: 2,
		})
	}
	rows := filterRows(items, scopeAll, "")
	flow := newItemFlow(rows, true)

	// The top of the list opens on the agents header, above its first row.
	if got := drawnHeaders(flow.window(0, 0, 8)); !slices.Equal(got, []group{groupAgents}) {
		t.Fatalf("headers at the top of the list = %v, want the agents section", got)
	}
	// A header whose section scrolled past is not drawn.
	if got := drawnHeaders(flow.window(4, 0, 6)); len(got) != 0 {
		t.Fatalf("headers deep inside a section = %v, want none", got)
	}
	// The header comes back with the first row of its section.
	win := flow.window(6, 0, 6)
	if got := drawnHeaders(win); !slices.Contains(got, groupSessions) {
		t.Fatalf("headers around the sessions section = %v, want the sessions header", got)
	}
	if drawn := drawnItems(t, win); !slices.Contains(drawn, 6) {
		t.Fatalf("window around the sessions section draws items %v, want its first row", drawn)
	}
	// A frame with room for the cursor's item alone shows it without its header.
	if got := drawnHeaders(flow.window(6, 0, 2)); len(got) != 0 {
		t.Fatalf("headers in a two-line frame = %v, want none", got)
	}
}

// TestResultsWindowDrawsTheCursorSectionsHeaderWithItsRow pins the header the
// results window keeps on screen: the row under the cursor never loses the header
// that titles it while the frame has room for both.
func TestResultsWindowDrawsTheCursorSectionsHeaderWithItsRow(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.showPreview = false
	m = withItems(m, groupedItems())

	// Three lines hold the agents header and the agent's own two lines.
	m = m.setCursor(2)
	lines, above, below := m.resultsRows(m.rows(), 40, 3)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = strings.TrimRight(sessionmgr.StripANSI(line), " ")
	}
	if len(lines) != 3 || plain[0] != " agents  1" {
		t.Fatalf("window = %q, want the agents header above its row", plain)
	}
	if !strings.Contains(plain[1], "pi") || !strings.Contains(plain[2], "skills") {
		t.Fatalf("window = %q, want the agent's two lines under its header", plain)
	}
	if above != 2 || below != 3 {
		t.Fatalf("counters = (%d above, %d below), want (2, 3)", above, below)
	}
}

// headerOnLine reports the section a results line is the header of.
func headerOnLine(m Model, text string, counts [groupCount]int) (group, bool) {
	for g := group(0); g < groupCount; g++ {
		if counts[g] == 0 {
			continue
		}
		if text == " "+m.groupLabel(g)+"  "+strconv.Itoa(counts[g]) {
			return g, true
		}
	}
	return 0, false
}

// resultShape is the results window's content in order: a section header as
// "#label" and an item as its title, with the items' continuation lines and the
// blank padding dropped. It fails when a drawn line is neither.
func resultShape(t *testing.T, m Model) []string {
	t.Helper()
	l := m.viewLayout()
	rows := m.rows()
	counts := groupCounts(rows)
	lines, _, _ := m.resultsRows(rows, l.listWidth(), l.listHeight())
	shape := make([]string, 0, len(lines))
	next := 0
	detail := false
	for _, line := range lines {
		text := strings.TrimRight(sessionmgr.StripANSI(line), " ")
		if strings.TrimSpace(text) == "" {
			continue
		}
		if g, ok := headerOnLine(m, text, counts); ok {
			if detail {
				t.Fatalf("header %q interrupts an item:\n%s", text, strings.Join(lines, "\n"))
			}
			shape = append(shape, "#"+m.groupLabel(g))
			continue
		}
		if detail {
			detail = false
			continue
		}
		if next >= len(rows) {
			t.Fatalf("results line %q follows the last row:\n%s", text, strings.Join(lines, "\n"))
		}
		title := itemTitle(rows[next].item)
		if !strings.Contains(text, title) {
			t.Fatalf("results line %q is not the title of row %q", text, title)
		}
		shape = append(shape, title)
		detail = itemHeight(rows[next].item) > 1
		next++
	}
	if next != len(rows) {
		t.Fatalf(
			"results drew %d of the list's %d rows:\n%s",
			next,
			len(rows),
			strings.Join(lines, "\n"),
		)
	}
	return shape
}

func TestResultsWindowGroupsSectionsInOrder(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.showPreview = false
	m = withItems(m, groupedItems())

	got := resultShape(t, m)
	want := []string{
		"#needs you", "claude", "codex",
		"#agents", "pi",
		"#sessions", "seshagy",
		"#hosts", "homelab",
		"#zoxide", "~/code/demo",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("results shape = %v, want %v", got, want)
	}

	// The needs-you header is the one section styled apart, and every header
	// reports its section's item count.
	body := strings.Join(m.renderResultsWindow(m.viewLayout()), "\n")
	if !strings.Contains(body, m.styles.needsYou.Render("needs you")) {
		t.Fatalf("needs-you header is not drawn in its own style:\n%s", body)
	}
	if !strings.Contains(body, m.styles.muted.Render("agents")+"  "+
		m.styles.groupCount.Render("1")) {
		t.Fatalf("agents header does not carry its item count:\n%s", body)
	}
}

func TestResultsWindowOmitsEmptySectionsAndTheirHeaders(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.showPreview = false
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "seshagy", Target: "seshagy", Windows: 2},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo", Name: "~/code/demo"},
	})
	want := []string{"#sessions", "seshagy", "#zoxide", "~/code/demo"}
	if got := resultShape(t, m); !slices.Equal(got, want) {
		t.Fatalf("results shape = %v, want %v", got, want)
	}
	body := sessionmgr.StripANSI(strings.Join(m.renderResultsWindow(m.viewLayout()), "\n"))
	for _, label := range []string{"needs you", "agents", "hosts", "find"} {
		if strings.Contains(body, label) {
			t.Fatalf("empty section %q still draws its header:\n%s", label, body)
		}
	}
}

func TestQueryDropsTheSectionHeaders(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.showPreview = false
	m = withItems(m, groupedItems())
	if !m.grouped(m.rows()) {
		t.Fatal("an empty query is not grouped")
	}

	model, _ := m.Update(runeMsg("s"))
	m = model.(Model)
	if m.grouped(m.rows()) {
		t.Fatal("a query still groups the list")
	}
	body := sessionmgr.StripANSI(strings.Join(m.renderResultsWindow(m.viewLayout()), "\n"))
	counts := groupCounts(m.rows())
	for g := group(0); g < groupCount; g++ {
		if strings.Contains(body, " "+m.groupLabel(g)+"  "+strconv.Itoa(counts[g])) {
			t.Fatalf("query keeps the %q section header:\n%s", m.groupLabel(g), body)
		}
	}
	// What is left is one flat ranked list, not one section per kind: rows from
	// different sections interleave in the order the ranking puts them.
	got := resultShape(t, m)
	for _, title := range got {
		if strings.HasPrefix(title, "#") {
			t.Fatalf("query results keep a header: %v", got)
		}
	}
	if !slices.Equal(got, rowTitles(m.rows())) {
		t.Fatalf("query results = %v, want the ranked rows %v", got, rowTitles(m.rows()))
	}
	sections := map[group]bool{}
	for _, r := range m.rows() {
		sections[r.group] = true
	}
	if len(sections) < 2 {
		t.Fatalf("query left a single section, so the flat list proves nothing: %v", got)
	}
}

func TestLoneSectionDrawsNoHeader(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.showPreview = false
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "s0", Target: "s0", Windows: 2},
		{Kind: sessionmgr.KindSession, Name: "s1", Target: "s1", Windows: 2},
	})
	if m.grouped(m.rows()) {
		t.Fatal("a list of one section is grouped, but its header would only repeat it")
	}
	if got := resultShape(t, m); !slices.Equal(got, []string{"s0", "s1"}) {
		t.Fatalf("results = %v, want the two rows and no header", got)
	}

	// A scope narrow enough to hold one section is the same case, even when the
	// full list has several.
	m = withItems(m, groupedItems())
	m.scope = scopeZoxide
	if m.grouped(m.rows()) {
		t.Fatal("a one-section scope is grouped")
	}
	if got := resultShape(t, m); !slices.Equal(got, []string{"~/code/demo"}) {
		t.Fatalf("zoxide scope = %v, want the row and no header", got)
	}
}

func TestPreviewHardCutKeepsWidthColorsAndNoEllipsis(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.preview = "\x1b[31m" + strings.Repeat("x", 60) + "\x1b[0m"

	lines := m.renderPreviewContent(20, 2)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "…") {
		t.Fatalf("preview shows an ellipsis:\n%q", joined)
	}
	if !strings.Contains(joined, "\x1b[31m") {
		t.Fatalf("preview colors were stripped:\n%q", joined)
	}
	content := lines[len(lines)-1]
	if !strings.Contains(content, "\x1b[31m") {
		t.Fatalf("content line lost its color: %q", content)
	}
	if got := ansi.StringWidth(content); got != 20 {
		t.Fatalf("content line width = %d, want exactly 20: %q", got, content)
	}
	if !strings.HasSuffix(content, "\x1b[0m") {
		t.Fatalf("content line does not end in a reset: %q", content)
	}
}

// TestPreviewWindowTitleIsTheSelectedItem pens the preview window's top border:
// the item name is the title, it is sanitized, and a name too wide for the
// border is clamped instead of pushing the corner out.
func TestPreviewWindowTitleIsTheSelectedItem(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	l := m.viewLayout()
	title := sessionmgr.StripANSI(m.renderPreviewWindow(l)[0])
	if !strings.Contains(title, " demo ") {
		t.Fatalf("preview title row = %q, want the item name", title)
	}
	if got := ansi.StringWidth(title); got != l.preview.width {
		t.Fatalf("preview title row is %d columns, want %d: %q", got, l.preview.width, title)
	}

	long := strings.Repeat("n", 90)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: long, Target: long},
	})
	title = sessionmgr.StripANSI(m.renderPreviewWindow(m.viewLayout())[0])
	if got := ansi.StringWidth(title); got != l.preview.width {
		t.Fatalf(
			"clamped preview title row is %d columns, want %d: %q",
			got,
			l.preview.width,
			title,
		)
	}

	// A control character in a multiplexer-supplied name never reaches the
	// border.
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "ev\x1b[1;1Hil", Target: "x"},
	})
	title = m.renderPreviewWindow(m.viewLayout())[0]
	if strings.Contains(title, "\x1b[1;1H") {
		t.Fatalf("preview title kept a cursor move: %q", title)
	}
	if got := ansi.StringWidth(sessionmgr.StripANSI(title)); got != l.preview.width {
		t.Fatalf(
			"sanitized preview title row is %d columns, want %d: %q",
			got,
			l.preview.width,
			title,
		)
	}

	// An agent's title carries the workspace its row shows as a column.
	m = withItems(m, []sessionmgr.Item{agentItem("pi", "%1", "skills")})
	title = sessionmgr.StripANSI(m.renderPreviewWindow(m.viewLayout())[0])
	if !strings.Contains(title, " pi · skills ") {
		t.Fatalf("agent preview title row = %q, want the workspace beside the name", title)
	}
}

// TestPreviewContentDropsTheLineNumbers pins the rows a capture draws: every
// line is the content itself behind the window's one column of padding, with no
// line-number gutter and no digits of its own.
func TestPreviewContentDropsTheLineNumbers(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.preview = "alpha\n\ncharlie"
	m.previewKey = "session:demo"

	lines := m.renderPreviewContent(30, 5)
	if len(lines) != 5 {
		t.Fatalf("preview lines = %d, want 5", len(lines))
	}
	// A session capture is bottom-anchored, so its rows are the last content
	// lines and the padding above them is blank.
	if lines[0] != "" || lines[1] != "" {
		t.Fatalf("padding lines carry content: %q", lines[:2])
	}
	want := []string{" alpha", " ", " charlie"}
	for i, w := range want {
		line := lines[len(lines)-len(want)+i]
		if got := sessionmgr.StripANSI(line); got != w {
			t.Fatalf("row %d = %q, want %q", i+1, got, w)
		}
	}
	for i, line := range lines {
		if got := sessionmgr.StripANSI(line); strings.ContainsAny(got, "0123456789") {
			t.Fatalf("line %d still carries a line number: %q", i, got)
		}
	}
}

// TestPreviewContentKeepsTheLineExactWithANSI checks that a colored capture line
// survives the padding: the content keeps its color, is hard-cut to the columns
// the window's inner width leaves rather than ellipsized, and the line still
// measures exactly that width.
func TestPreviewContentKeepsTheLineExactWithANSI(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.preview = "\x1b[31m" + strings.Repeat("x", 60) + "\x1b[0m"
	m.previewKey = "session:demo"

	lines := m.renderPreviewContent(20, 2)
	line := lines[len(lines)-1]
	if got := ansi.StringWidth(line); got != 20 {
		t.Fatalf("preview line is %d columns, want exactly 20: %q", got, line)
	}
	if !strings.Contains(line, "\x1b[31m") {
		t.Fatalf("preview content lost its color: %q", line)
	}
	if !strings.HasSuffix(line, previewReset) {
		t.Fatalf("preview line does not close its style: %q", line)
	}
	stripped := sessionmgr.StripANSI(line)
	if strings.Contains(stripped, "…") {
		t.Fatalf("preview line is ellipsized: %q", stripped)
	}
	if want := " " + strings.Repeat("x", 19); stripped != want {
		t.Fatalf("preview line = %q, want %q", stripped, want)
	}
}

// TestPreviewFooterPerKind pins the wording of the preview window's footer: the
// pane and age of an agent, the window count of a session in the multiplexer's
// vocabulary, the entry count of a directory listing, and the transport of a
// host. A kind with nothing to report claims nothing.
func TestPreviewFooterPerKind(t *testing.T) {
	started := time.Now().Add(-3 * time.Minute)
	listing := "-rw-r--r-- 1 u g 12 Jan  1 00:00 a.txt\n\x1b[34mdrwxr-xr-x 1 u g 4k Jan  1 00:00 docs/\x1b[0m\n"
	tests := []struct {
		name         string
		item         sessionmgr.Item
		terms        sessionmgr.Terms
		preview      string
		previewError string
		want         string
	}{
		{
			name: "agent pane and age",
			item: sessionmgr.Item{Kind: sessionmgr.KindAgent, PaneID: "%7", StartedAt: started},
			want: "pane %7 · 3m",
		},
		{
			name: "agent without a pane id",
			item: sessionmgr.Item{Kind: sessionmgr.KindAgent, StartedAt: started},
			want: "3m",
		},
		{
			name: "agent without a start time",
			item: sessionmgr.Item{Kind: sessionmgr.KindAgent, PaneID: "%7"},
			want: "pane %7",
		},
		{
			name:  "herdr session tabs",
			item:  sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Windows: 2},
			terms: sessionmgr.HerdrTerms(),
			want:  "2 tabs",
		},
		{
			name:  "tmux session windows",
			item:  sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Windows: 2},
			terms: sessionmgr.TmuxTerms(),
			want:  "2 windows",
		},
		{
			name:    "zoxide listing entries",
			item:    sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
			preview: listing,
			want:    "2 entries",
		},
		{
			name:    "fd listing entries",
			item:    sessionmgr.Item{Kind: sessionmgr.KindFD, Path: "/tmp/demo"},
			preview: "one\n",
			want:    "1 entries",
		},
		{
			name: "listing before its capture lands",
			item: sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
		},
		{
			name:    "listing with nothing to list",
			item:    sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
			preview: noPreviewAvailableText,
		},
		{
			name:         "failed listing",
			item:         sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
			previewError: "capture failed",
		},
		{
			name: "host transport",
			item: sessionmgr.Item{Kind: sessionmgr.KindHost, Name: "box", Location: "ssh"},
			want: "ssh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{tt.item})
			if tt.terms != (sessionmgr.Terms{}) {
				m.terms = tt.terms
			}
			m.preview = tt.preview
			m.previewError = tt.previewError
			m.previewKey = tt.item.Key()
			if got := m.previewFooter(tt.item); got != tt.want {
				t.Fatalf("previewFooter = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPreviewWindowFooterLandsInTheBottomBorder draws the whole window: the kind's
// footer is right-aligned in the bottom border, a kind with nothing to report keeps
// a plain border, and the footer never widens the frame.
func TestPreviewWindowFooterLandsInTheBottomBorder(t *testing.T) {
	tests := []struct {
		name    string
		item    sessionmgr.Item
		preview string
		want    string
	}{
		{
			name: "agent",
			item: sessionmgr.Item{
				Kind:      sessionmgr.KindAgent,
				PaneID:    "%7",
				StartedAt: time.Now().Add(-3 * time.Minute),
			},
			preview: "working…\n",
			want:    "pane %7 · 3m",
		},
		{
			name:    "session",
			item:    sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Windows: 2},
			preview: "alpha\n",
			want:    "2 windows",
		},
		{
			name:    "directory",
			item:    sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
			preview: "a.txt\nb.txt\n",
			want:    "2 entries",
		},
		{
			name:    "host",
			item:    sessionmgr.Item{Kind: sessionmgr.KindHost, Name: "box", Location: "ssh"},
			preview: "alias seshagy-box\n",
			want:    "ssh",
		},
		{
			name: "directory without a listing",
			item: sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{tt.item})
			m.preview = tt.preview
			m.previewKey = tt.item.Key()

			l := m.viewLayout()
			block := m.renderPreviewWindow(l)
			bottom := sessionmgr.StripANSI(block[len(block)-1])
			if got := ansi.StringWidth(bottom); got != l.preview.width {
				t.Fatalf("bottom border is %d columns, want %d: %q", got, l.preview.width, bottom)
			}
			if tt.want == "" {
				if strings.Trim(bottom, "─╰╯") != "" {
					t.Fatalf("footer drawn without anything to report: %q", bottom)
				}
			} else if !strings.HasSuffix(bottom, tt.want+" ╯") {
				t.Fatalf(
					"bottom border = %q, want the footer %q flush against the corner",
					bottom,
					tt.want,
				)
			}
			frameLines(t, m)
		})
	}
}

func TestAgentTabSuffixOnlyWhenItAddsInformation(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		location string
		tab      string
		want     string
	}{
		{name: "blank label", title: "pi", location: "skills", tab: "", want: ""},
		{name: "repeats workspace", title: "pi", location: "skills", tab: "skills", want: ""},
		{
			name:     "repeats workspace case-insensitively",
			title:    "pi",
			location: "skills",
			tab:      " SKILLS ",
			want:     "",
		},
		{name: "repeats title", title: "pi", location: "skills", tab: "pi", want: ""},
		{
			name:     "repeats title case-insensitively",
			title:    "pi",
			location: "skills",
			tab:      "PI",
			want:     "",
		},
		{name: "adds information", title: "pi", location: "skills", tab: "Build", want: "Build"},
		{name: "no workspace to repeat", title: "pi", location: "", tab: "Build", want: "Build"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := agentItem("pi", "%1", tt.location)
			item.AgentDisplayName = tt.title
			item.TabLabel = tt.tab
			if got := agentTabSuffix(item); got != tt.want {
				t.Fatalf("agentTabSuffix(%q, %q, %q) = %q, want %q",
					tt.title, tt.location, tt.tab, got, tt.want)
			}
		})
	}
}

func TestAgentRowDropsTheRepeatedTabAndKindCells(t *testing.T) {
	item := agentItem("pi", "%1", "skills")
	item.AgentDisplayName = "pi"
	item.TabLabel = "skills"

	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{item})
	_, plain := itemLines(t, m, item.Key(), 60, false)
	lines := strings.Join(plain, "\n")
	if strings.Contains(lines, "— skills") {
		t.Fatalf("item repeats the workspace as a tab suffix: %q", lines)
	}
	if strings.Count(lines, "skills") != 1 {
		t.Fatalf("item shows the workspace more than once: %q", lines)
	}
}

func TestAgentKindCellFollowsTheLogo(t *testing.T) {
	m := newTestModel(t)
	item := agentItem("claude", "%1", "frontend")
	item.AgentDisplayName = "review-spec"
	m = withItems(m, []sessionmgr.Item{item})

	// The logo already names the agent, so the colored kind cell is dropped.
	t.Setenv("SESHAGY_AGENT_ICONS", "font")
	plain := sessionmgr.StripANSI(m.View())
	if !strings.ContainsRune(plain, rune(agentLogoBase)) {
		t.Fatalf("logo missing with the font enabled:\n%s", plain)
	}
	if strings.Contains(plain, "claude") {
		t.Fatalf("kind cell drawn beside the logo:\n%s", plain)
	}

	// Without the logo, the kind cell is the only place the agent type shows.
	t.Setenv("SESHAGY_AGENT_ICONS", "none")
	plain = sessionmgr.StripANSI(m.View())
	if !strings.Contains(plain, "claude") {
		t.Fatalf("kind cell missing without a logo:\n%s", plain)
	}
	if strings.ContainsRune(plain, rune(agentLogoBase)) {
		t.Fatalf("logo drawn with icons disabled:\n%s", plain)
	}
}

func TestAgentAgeFormatting(t *testing.T) {
	started := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		after time.Duration
		want  string
	}{
		{name: "seconds", after: 47 * time.Second, want: "47s"},
		{name: "just under a minute", after: 59*time.Second + 999*time.Millisecond, want: "59s"},
		{name: "minutes", after: 12 * time.Minute, want: "12m"},
		{name: "just under an hour", after: 59 * time.Minute, want: "59m"},
		{name: "hours pad the minutes", after: 2*time.Hour + 4*time.Minute, want: "2h04m"},
		{name: "hours without minutes", after: 3 * time.Hour, want: "3h00m"},
		{name: "just under a day", after: 23*time.Hour + 59*time.Minute, want: "23h59m"},
		{name: "days", after: 4*24*time.Hour + 3*time.Hour, want: "4d3h"},
		{name: "clock skew", after: -5 * time.Second, want: "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentAgeAt(started, started.Add(tt.after)); got != tt.want {
				t.Fatalf("agentAgeAt(+%v) = %q, want %q", tt.after, got, tt.want)
			}
		})
	}
}

func TestAgentRowShowsTheCachedAge(t *testing.T) {
	m := newTestModel(t)
	item := agentItem("pi", "%1", "skills")
	item.AgentPID = 4242
	item.StartedAt = time.Now().Add(-12 * time.Minute)
	m = withItems(m, []sessionmgr.Item{item})
	plain := sessionmgr.StripANSI(m.View())
	if !strings.Contains(plain, "12m") {
		t.Fatalf("age cell missing:\n%s", plain)
	}
}
