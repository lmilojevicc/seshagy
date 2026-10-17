package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
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

	empty := m.renderPromptContent(80)
	if got := cursorColumn(t, empty); got != 2 {
		t.Fatalf("empty query: cursor column = %d, want 2: %q", got, empty)
	}
	if strings.Index(empty, "\x1b[7m") > strings.Index(empty, placeholderText) {
		t.Fatalf("empty query: placeholder precedes the cursor: %q", empty)
	}

	m.query = "api"
	typed := m.renderPromptContent(80)
	if got := cursorColumn(t, typed); got != 5 {
		t.Fatalf("typed query: cursor column = %d, want 5: %q", got, typed)
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
		color string
	}{
		{sessionmgr.AgentBlocked, "∅", "blocked", "\x1b[91m"},
		{sessionmgr.AgentWorking, "●", "working", "\x1b[92m"},
		{sessionmgr.AgentDone, "◉", "done", "\x1b[96m"},
		{sessionmgr.AgentIdle, "○", "idle", "\x1b[90m"},
		{sessionmgr.AgentUnknown, "?", "unknown", "\x1b[90m"},
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
			if !strings.Contains(out, tt.color+tt.glyph) {
				t.Fatalf("%s glyph is not drawn in the configured color %q:\n%q",
					tt.state, tt.color, out)
			}
			if !strings.Contains(out, tt.color+tt.word) {
				t.Fatalf("%s word is not drawn in the configured color %q:\n%q",
					tt.state, tt.color, out)
			}
		})
	}
}

func TestAgentStateStyleOverrideFromConfig(t *testing.T) {
	withColor(t)
	cfg := defaultTestConfig()
	cfg.Icons.AgentState.Blocked = appconfig.IconConfig{
		Icon: "!", Label: "waiting", Color: "#ff0000",
	}
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32
	item := agentItem("claude", "%1", "frontend")
	item.AgentState = sessionmgr.AgentBlocked
	m = withItems(m, []sessionmgr.Item{item})

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	out := m.View()
	plain := sessionmgr.StripANSI(out)
	if !strings.Contains(plain, "!") || !strings.Contains(plain, "waiting") {
		t.Fatalf("configured blocked glyph/label missing:\n%s", plain)
	}
	if strings.Contains(plain, "blocked") || strings.Contains(plain, "∅") {
		t.Fatalf("built-in blocked vocabulary still drawn:\n%s", plain)
	}
	if !strings.Contains(out, style.Render("!")) {
		t.Fatalf("configured blocked color not applied:\n%q", out)
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

func TestMetaGridShrinksLeadingColumnsBeforeSqueezingTheStateColumn(t *testing.T) {
	m := newTestModel(t)
	item := agentItem("claude", "%1", "a-very-long-workspace-name")
	m = withItems(m, []sessionmgr.Item{item})
	metas := [][]metaCell{m.rowMeta(m.rows()[0])}
	grid := buildMetaGrid(metas, 30)

	if grid.total() > 30 {
		t.Fatalf("grid total = %d, want <= 30", grid.total())
	}
	if len(grid.widths) != 4 {
		t.Fatalf("columns = %d, want 4 (no column dropped)", len(grid.widths))
	}
	if grid.widths[1] >= metaColumnMax {
		t.Fatalf("workspace column not shrunk: %d", grid.widths[1])
	}
	if grid.widths[3] < metaColumnMin {
		t.Fatalf("state column squeezed to %d", grid.widths[3])
	}
}

// TestResultRowsAlignTheirColumnsPerKind pins the row table: rows of one kind
// share the title and meta columns, so their titles, context, and state line up
// down the window, and the selected row is the full-width selection bar.
func TestResultRowsAlignTheirColumnsPerKind(t *testing.T) {
	withColor(t)
	blocked := agentItem("claude", "%1", "frontend")
	blocked.AgentState = sessionmgr.AgentBlocked
	blocked.AgentDisplayName = "claude"
	idle := agentItem("pi", "%2", "a-very-long-workspace")
	idle.AgentDisplayName = "pi"
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{blocked, idle})
	rows := m.rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	metas := [][]metaCell{m.rowMeta(rows[0]), m.rowMeta(rows[1])}
	grid := buildRowGrid(rows, metas, 60, m.rowPrefixWidth())
	cols := grid[sessionmgr.KindAgent]
	blockedLine := sessionmgr.StripANSI(m.renderRow(rows[0], metas[0], cols, 60, false))
	idleLine := sessionmgr.StripANSI(m.renderRow(rows[1], metas[1], cols, 60, false))

	for _, line := range []string{blockedLine, idleLine} {
		if got := ansi.StringWidth(line); got != 60 {
			t.Fatalf("row width = %d, want 60: %q", got, line)
		}
	}
	// Both state cells start in the same column, and the block is left aligned:
	// the state ends before the row does instead of being pushed to the edge.
	column := func(line, word string) int {
		t.Helper()
		index := strings.Index(line, word)
		if index < 0 {
			t.Fatalf("%q does not contain %q", line, word)
		}
		return ansi.StringWidth(line[:index])
	}
	blockedColumn := column(blockedLine, "∅ blocked")
	idleColumn := column(idleLine, "○ idle")
	if blockedColumn != idleColumn {
		t.Fatalf("state columns do not line up (%d, %d):\n%q\n%q",
			blockedColumn, idleColumn, blockedLine, idleLine)
	}
	if end := blockedColumn + ansi.StringWidth("∅ blocked"); end >= 60 {
		t.Fatalf("state cell is flush right (%d of 60): %q", end, blockedLine)
	}

	// The selection bar covers the whole row: no cell's reset is left without
	// the bar re-opened behind it.
	selected := m.renderRow(rows[0], metas[0], cols, 60, true)
	bar := strings.TrimSuffix(m.styles.selectedBG.Render("x"), "x"+sgrReset)
	if !strings.HasPrefix(selected, bar) {
		t.Fatalf("selected row does not open with the selection bar: %q", selected)
	}
	if !strings.HasSuffix(selected, sgrReset) {
		t.Fatalf("selected row does not close its style: %q", selected)
	}
	for i, part := range strings.Split(strings.TrimSuffix(selected, sgrReset), sgrReset) {
		if i > 0 && !strings.HasPrefix(part, bar) {
			t.Fatalf("selection bar stops at cell %d: %q", i, selected)
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

// TestSessionRowColumns pins the session row: the name, the dim window count,
// the green attached flag (kept as a blank column when the session is detached),
// and the agents inside the session counted per state.
func TestSessionRowColumns(t *testing.T) {
	withColor(t)
	working := agentItem("pi", "%1", "seshagy")
	working.AgentState = sessionmgr.AgentWorking
	working.Session = "seshagy"
	blocked := agentItem("claude", "%2", "seshagy")
	blocked.AgentState = sessionmgr.AgentBlocked
	blocked.Session = "seshagy"
	idle := agentItem("codex", "%3", "fff-cli")
	idle.Session = "fff-cli"

	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		working,
		blocked,
		idle,
		{
			Kind:     sessionmgr.KindSession,
			Name:     "seshagy",
			Target:   "seshagy",
			Attached: true,
			Windows:  3,
		},
		{Kind: sessionmgr.KindSession, Name: "fff-cli", Target: "fff-cli", Windows: 5},
	})
	rows := m.rows()
	var sessions []row
	for _, r := range rows {
		if r.item.Kind == sessionmgr.KindSession {
			sessions = append(sessions, r)
		}
	}
	if len(sessions) != 2 {
		t.Fatalf("session rows = %d, want 2", len(sessions))
	}
	attached := m.rowMeta(sessions[0])
	detached := m.rowMeta(sessions[1])

	if want := windowCountLabel(m.terms, 3); attached[0].plain != want {
		t.Fatalf("attached session's first cell = %q, want %q", attached[0].plain, want)
	}
	if attached[1].plain != "attached" {
		t.Fatalf("attached session's second cell = %q, want attached", attached[1].plain)
	}
	if want := "●1 ∅1"; attached[len(attached)-1].plain != want {
		t.Fatalf("attached session's last cell = %q, want %q",
			attached[len(attached)-1].plain, want)
	}
	if !strings.Contains(attached[1].text, m.styles.success.Render("attached")) {
		t.Fatalf("attached is not drawn in the success accent: %q", attached[1].text)
	}
	// A detached session keeps the attached column blank so the counts that
	// follow it still line up, and it counts only its own agents.
	if detached[0].plain != windowCountLabel(m.terms, 5) {
		t.Fatalf("detached session's first cell = %q", detached[0].plain)
	}
	if detached[1].plain != "" {
		t.Fatalf("detached session's attached cell = %q, want empty", detached[1].plain)
	}
	if want := "○1"; detached[len(detached)-1].plain != want {
		t.Fatalf("detached session's last cell = %q, want %q",
			detached[len(detached)-1].plain, want)
	}

	// Both rows draw their counts in the same column, whatever the attached flag
	// says, and no row shows the removed "current" cell.
	grid := buildRowGrid([]row{sessions[0], sessions[1]},
		[][]metaCell{attached, detached}, 60, m.rowPrefixWidth())
	cols := grid[sessionmgr.KindSession]
	attachedLine := sessionmgr.StripANSI(m.renderRow(sessions[0], attached, cols, 60, false))
	detachedLine := sessionmgr.StripANSI(m.renderRow(sessions[1], detached, cols, 60, false))
	countColumn := func(line, count string) int {
		t.Helper()
		index := strings.Index(line, count)
		if index < 0 {
			t.Fatalf("%q does not contain %q", line, count)
		}
		return ansi.StringWidth(line[:index])
	}
	attachedColumn := countColumn(attachedLine, "●1")
	detachedColumn := countColumn(detachedLine, "○1")
	if attachedColumn != detachedColumn {
		t.Fatalf("agent counts are not in the same column (%d, %d):\n%q\n%q",
			attachedColumn, detachedColumn, attachedLine, detachedLine)
	}
	for _, line := range []string{attachedLine, detachedLine} {
		if strings.Contains(line, "current") {
			t.Fatalf("the session row still shows a current cell: %q", line)
		}
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
			got := m.renderTitle(row{item: item, matches: tt.matches}, 80, false)
			if got != want.String() {
				t.Fatalf("title = %q, want %q", got, want.String())
			}
		})
	}
}

// TestAgentRowShowsNameContextAndState pins the agent row: the bright display
// name, the dim `workspace · age` context, and the configured state glyph and
// word in the state color. A blocked or done agent renders its name bold, and a
// visited, working, or idle one does not.
func TestAgentRowShowsNameContextAndState(t *testing.T) {
	withColor(t)
	const name = "review-spec"
	tests := []struct {
		state sessionmgr.AgentState
		glyph string
		word  string
		bold  bool
	}{
		{sessionmgr.AgentBlocked, "∅", "blocked", true},
		{sessionmgr.AgentDone, "◉", "done", true},
		{sessionmgr.AgentWorking, "●", "working", false},
		{sessionmgr.AgentIdle, "○", "idle", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			m := newTestModel(t)
			item := agentItem("claude", "%1", "seshagy")
			item.AgentDisplayName = name
			item.AgentState = tt.state
			item.StartedAt = time.Now().Add(-3 * time.Minute)
			m = withItems(m, []sessionmgr.Item{
				item,
				{Kind: sessionmgr.KindSession, Name: "other", Target: "other"},
			})
			// The cursor starts on the first row and bolds it; keep the agent row
			// out of the selection so only the state decides the weight.
			m.cursor = 1

			row := resultRowLine(t, m.View(), name)
			plain := sessionmgr.StripANSI(row)

			// Name, workspace, age, state: one single-line row in that order.
			order := []string{name, "seshagy", "·", "3m", tt.glyph, tt.word}
			previous := -1
			for _, part := range order {
				index := strings.Index(plain, part)
				if index <= previous {
					t.Fatalf("agent row is out of order at %q: %q", part, plain)
				}
				previous = index
			}

			bold := m.styles.itemName.Bold(true)
			if tt.bold && !strings.Contains(row, bold.Render(name)) {
				t.Fatalf("an agent that needs you is not bold: %q", row)
			}
			if !tt.bold && strings.Contains(row, bold.Render(name)) {
				t.Fatalf("a settled agent is bold: %q", row)
			}
			if !strings.Contains(row, m.styles.muted.Render("seshagy")) {
				t.Fatalf("workspace is not dim: %q", row)
			}
			if !strings.Contains(row, m.styles.muted.Render("3m")) {
				t.Fatalf("age is not dim: %q", row)
			}
			style := m.agentStateStyle(tt.state)
			if !strings.Contains(row, style.Render(tt.glyph)) {
				t.Fatalf("state glyph is not drawn in the state color: %q", row)
			}
			if !strings.Contains(row, style.Render(tt.word)) {
				t.Fatalf("state word is not drawn in the state color: %q", row)
			}
		})
	}
}

// TestResultsWindowCountersLiveInTheBottomBorder pins the results window frame:
// every inner row is an item and the scroll counters are drawn in the bottom
// border instead of taking rows away from the list.
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
		{name: "top only", cursor: 0, wantBelow: "↓ 26 more", wantSelected: "s00"},
		{
			name:         "both",
			cursor:       20,
			wantAbove:    "↑ 13 more · ↓ 13 more",
			wantSelected: "s20",
		},
		{name: "bottom only", cursor: 39, wantAbove: "↑ 26 more", wantSelected: "s39"},
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
			// No inner row is spent on a counter, and the selection is visible.
			selected := ""
			for _, line := range block[1 : len(block)-1] {
				plain := sessionmgr.StripANSI(line)
				if strings.Contains(plain, "more") {
					t.Fatalf("counter drawn inside the results window: %q", plain)
				}
				if strings.Contains(line, "\x1b[7m") || strings.Contains(plain, "▌") {
					selected = plain
				}
			}
			if !strings.Contains(selected, tt.wantSelected) {
				t.Fatalf("selected row = %q, want %q", selected, tt.wantSelected)
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
}

// TestPreviewGutterNumbersEveryContentLine pins the preview window's line-number
// gutter: every content line is numbered from 1, right-aligned in the three-column
// dim gutter, and followed by the two-column gap before the content. The blank
// lines that pad the window to its height carry no number.
func TestPreviewGutterNumbersEveryContentLine(t *testing.T) {
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
		t.Fatalf("padding lines carry a gutter: %q", lines[:2])
	}
	want := []string{"  1  alpha", "  2  ", "  3  charlie"}
	for i, w := range want {
		line := lines[len(lines)-len(want)+i]
		prefix := m.styles.gutter.Render(fmt.Sprintf("%*d", previewGutterWidth, i+1)) +
			strings.Repeat(" ", previewGutterGap)
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("row %d does not start with the numbered gutter %q: %q", i+1, prefix, line)
		}
		if got := sessionmgr.StripANSI(line); got != w {
			t.Fatalf("row %d = %q, want %q", i+1, got, w)
		}
	}
}

// TestPreviewGutterKeepsTheLineExactWithANSI checks that a colored capture line
// survives the gutter: the content keeps its color, is hard-cut to the columns the
// gutter leaves rather than ellipsized, and the line still measures exactly the
// window's inner width.
func TestPreviewGutterKeepsTheLineExactWithANSI(t *testing.T) {
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
	want := "  1  " + strings.Repeat("x", 20-previewGutterWidth-previewGutterGap)
	if stripped != want {
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
	plain := sessionmgr.StripANSI(m.View())
	if strings.Contains(plain, "— skills") {
		t.Fatalf("row repeats the workspace as a tab suffix:\n%s", plain)
	}
	if strings.Count(plain, "skills") != 1 {
		t.Fatalf("row shows the workspace more than once:\n%s", plain)
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
