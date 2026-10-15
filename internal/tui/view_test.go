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

func TestViewSnapshotSanityAcrossWidths(t *testing.T) {
	for _, width := range []int{40, 80, 140} {
		m := newTestModel(t)
		m.width, m.height = width, 30
		m.loadedOnce = true
		m = withItems(m, pickerItems())

		rendered := m.View()
		out := sessionmgr.StripANSI(rendered)
		lines := strings.Split(out, "\n")
		if len(lines) != m.height {
			t.Fatalf("width %d: view has %d lines, want %d", width, len(lines), m.height)
		}
		for i, line := range lines {
			if got := ansi.StringWidth(line); got >= width {
				t.Fatalf(
					"width %d: line %d is %d columns, want < %d: %q",
					width,
					i,
					got,
					width,
					line,
				)
			}
		}
		// The chips share the prompt row with the query; there is no separate
		// filter strip row, so the line under the prompt is the top rule.
		prompt := lines[0]
		for _, text := range []string{"❯", placeholderText, "all"} {
			if !strings.Contains(prompt, text) {
				t.Fatalf("width %d: prompt row missing %q: %q", width, text, prompt)
			}
		}
		if stripped := strings.Trim(lines[1], "─"); stripped != "" {
			t.Fatalf("width %d: line 1 is not a rule: %q", width, lines[1])
		}

		want := []string{
			"❯",
			"all",
			"@ agents",
			"seshagy",
			"↑↓",
			"4",
		}
		if width >= 60 {
			want = append(want,
				"! needs you",
				"$ sessions",
				"* dirs",
				"~ zoxide",
				"needs you",
			)
			// Narrow frames ellipsize titles and the placeholder; keep the
			// full-text assertions to widths where nothing has to be clipped.
			want = append(want, "claude-refactor", "review-spec", "~/code/demo")
		}
		if width >= 100 {
			// The chips share the prompt row with the query, so the last ones
			// only fit on a wide frame.
			want = append(want, "/ find")
		}
		for _, text := range want {
			if !strings.Contains(out, text) {
				t.Fatalf("width %d: view missing %q\n%s", width, text, out)
			}
		}
		if strings.Contains(out, "# hosts") {
			t.Fatalf("width %d: hosts chip shown without host items\n%s", width, out)
		}
	}
}

func TestViewHidesAndShowsHostsChip(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindHost, Name: "box", Target: "box"}})
	if out := sessionmgr.StripANSI(m.View()); !strings.Contains(out, "# hosts") {
		t.Fatalf("hosts chip missing with a host item\n%s", out)
	}
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "dev", Target: "dev"}})
	if out := sessionmgr.StripANSI(m.View()); strings.Contains(out, "# hosts") {
		t.Fatalf("hosts chip still shown without host items\n%s", out)
	}
}

func TestViewHighlightsActiveScopeChip(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents
	out := m.View()
	plain := sessionmgr.StripANSI(out)
	if !strings.Contains(plain, "@ agents") {
		t.Fatalf("active chip missing:\n%s", plain)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("chips are rendered without styles")
	}
	if !strings.Contains(strings.Split(plain, "\n")[0], "@ agents") {
		t.Fatal("chips are not on the prompt row")
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

func TestPromptRowShowsShortPlaceholderOnly(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	plain := sessionmgr.StripANSI(m.View())
	if !strings.Contains(plain, placeholderText) {
		t.Fatalf("placeholder missing\n%s", plain)
	}
	if strings.Contains(placeholderText, "@ agents") {
		t.Fatal("placeholder lists sigils; the chips are the only place they appear")
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

	empty := m.renderPromptRow(80)
	if got := cursorColumn(t, empty); got != 2 {
		t.Fatalf("empty query: cursor column = %d, want 2: %q", got, empty)
	}
	if strings.Index(empty, "\x1b[7m") > strings.Index(empty, placeholderText) {
		t.Fatalf("empty query: placeholder precedes the cursor: %q", empty)
	}

	m.query = "api"
	typed := m.renderPromptRow(80)
	if got := cursorColumn(t, typed); got != 5 {
		t.Fatalf("typed query: cursor column = %d, want 5: %q", got, typed)
	}
}

func TestPromptRowDropsInactiveChipsFromTheEnd(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents

	row := sessionmgr.StripANSI(m.renderPromptRow(30))
	if !strings.Contains(row, "@ agents") {
		t.Fatalf("active chip dropped: %q", row)
	}
	for _, dropped := range []string{"/ find", "~ zoxide", "* dirs", "$ sessions"} {
		if strings.Contains(row, dropped) {
			t.Fatalf("narrow row kept %q: %q", dropped, row)
		}
	}

	// The active chip is the last one: it survives while the others go.
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindHost, Name: "box", Target: "box"}})
	m.scope = scopeHosts
	hosts := sessionmgr.StripANSI(m.renderPromptRow(28))
	if !strings.Contains(hosts, "# hosts") {
		t.Fatalf("trailing active chip dropped: %q", hosts)
	}
	if strings.Contains(hosts, "all") {
		t.Fatalf("narrow row kept an inactive chip: %q", hosts)
	}

	// The query wins: a long query leaves no room for any chip.
	m.query = strings.Repeat("q", 40)
	if row := sessionmgr.StripANSI(m.renderPromptRow(30)); strings.Contains(row, "all") {
		t.Fatalf("chips overlap a long query: %q", row)
	}
}

func TestFooterCounterAndHintDropping(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, pickerItems())
	m.scope = scopeAgents
	m.query = "claude"
	wide := sessionmgr.StripANSI(m.renderFooter(m.width))
	if !strings.Contains(wide, "1/4") {
		t.Fatalf("wide footer = %q, want the 1/4 counter", wide)
	}
	if !strings.Contains(wide, "yazi") {
		t.Fatalf("wide footer dropped hints: %q", wide)
	}

	narrow := sessionmgr.StripANSI(m.renderFooter(40))
	if !strings.Contains(narrow, "1/4") {
		t.Fatalf("narrow footer = %q, want the counter kept", narrow)
	}
	if strings.Contains(narrow, "yazi") {
		t.Fatalf("narrow footer kept a hint that must drop from the end: %q", narrow)
	}
}

func TestFooterKillConfirmationReplacesHints(t *testing.T) {
	m := newTestModel(t)
	m = withItems(
		m,
		[]sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"}},
	)
	m, _ = press(t, m, "ctrl+x")
	footer := sessionmgr.StripANSI(m.renderFooter(m.width))
	if !strings.Contains(footer, "kill session demo?") {
		t.Fatalf("footer = %q, want the confirmation question", footer)
	}
	if !strings.Contains(footer, "⏎/y confirm · any other key cancels") {
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
	prompt := sessionmgr.StripANSI(m.renderPromptRow(m.width))
	if !strings.Contains(prompt, "rename") || !strings.Contains(prompt, "demo") {
		t.Fatalf("rename prompt row = %q", prompt)
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

			out := strings.Join(m.renderPreview(40, 6), "\n")
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

	lines := m.renderPreview(20, 4)
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

func TestPreviewColumnOnlyWhenWideEnough(t *testing.T) {
	for _, tt := range []struct {
		width     int
		wantSplit bool
	}{
		{80, false},
		{99, false},
		{100, true},
		{160, true},
	} {
		m := newTestModel(t)
		m.width, m.height = tt.width, 30
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		l := m.viewLayout()
		if tt.wantSplit != (l.previewWidth > 0) {
			t.Fatalf(
				"width %d: previewWidth = %d, want split = %v",
				tt.width,
				l.previewWidth,
				tt.wantSplit,
			)
		}
		if l.previewWidth > 0 && l.previewWidth < previewMinCols {
			t.Fatalf(
				"width %d: previewWidth = %d, below the %d column floor",
				tt.width,
				l.previewWidth,
				previewMinCols,
			)
		}
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

func TestMetaGridShrinksLeadingColumnsBeforeSqueezingTheTitle(t *testing.T) {
	m := newTestModel(t)
	item := agentItem("claude", "%1", "a-very-long-workspace-name")
	m = withItems(m, []sessionmgr.Item{item})
	metas := [][]metaCell{m.rowMeta(m.rows()[0])}
	grid := buildMetaGrid(metas, 30)

	if grid.total() > 30 {
		t.Fatalf("grid total = %d, want <= 30", grid.total())
	}
	if len(grid.widths) != 3 {
		t.Fatalf("columns = %d, want 3 (no column dropped)", len(grid.widths))
	}
	if grid.widths[0] >= metaColumnMax {
		t.Fatalf("leading column not shrunk: %d", grid.widths[0])
	}
	if grid.widths[2] < len("idle") {
		t.Fatalf("state column squeezed to %d", grid.widths[2])
	}
}

func TestRowMetaColumnsAlignFlushRight(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		agentItem("claude", "%1", "frontend"),
		{Kind: sessionmgr.KindSession, Name: "dev", Target: "dev", Attached: true},
	})
	rows := m.rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	metas := [][]metaCell{m.rowMeta(rows[0]), m.rowMeta(rows[1])}
	grid := buildMetaGrid(metas, 60-m.rowPrefixWidth()-titleMinWidth-2)
	agentLine := sessionmgr.StripANSI(m.renderRow(rows[0], metas[0], grid, 60, false))
	sessionLine := sessionmgr.StripANSI(m.renderRow(rows[1], metas[1], grid, 60, false))

	if got := ansi.StringWidth(agentLine); got != 60 {
		t.Fatalf("agent row width = %d, want 60: %q", got, agentLine)
	}
	if got := ansi.StringWidth(sessionLine); got != 60 {
		t.Fatalf("session row width = %d, want 60: %q", got, sessionLine)
	}
	// The session row carries one meta cell fewer, so its "attached" state must
	// start in the same column as the agent's state word rather than shift left.
	column := func(line, word string) int {
		index := strings.LastIndex(line, word)
		if index < 0 {
			t.Fatalf("%q does not contain %q", line, word)
		}
		return ansi.StringWidth(line[:index])
	}
	agentColumn := column(agentLine, "idle")
	sessionColumn := column(sessionLine, "attached")
	if agentColumn != sessionColumn {
		t.Fatalf("meta columns are not flush right (agent %d, session %d):\n%q\n%q",
			agentColumn, sessionColumn, agentLine, sessionLine)
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

func TestSessionRowMetaPutsAttachedFirstAndCountLast(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{
			Kind:     sessionmgr.KindSession,
			Name:     "attached3",
			Target:   "attached3",
			Attached: true,
			Windows:  3,
		},
		{Kind: sessionmgr.KindSession, Name: "detached5", Target: "detached5", Windows: 5},
	})
	rows := m.rows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	attached := m.rowMeta(rows[0])
	detached := m.rowMeta(rows[1])

	if attached[0].plain != "attached" {
		t.Fatalf("attached session's first cell = %q, want attached", attached[0].plain)
	}
	if want := windowCountLabel(m.terms, 3); attached[len(attached)-1].plain != want {
		t.Fatalf("attached session's last cell = %q, want %q",
			attached[len(attached)-1].plain, want)
	}
	// The unattached session keeps an empty placeholder in the attached column
	// so its count still lands in the last column.
	if detached[0].plain != "" {
		t.Fatalf("unattached session's first cell = %q, want empty", detached[0].plain)
	}
	if want := windowCountLabel(m.terms, 5); detached[len(detached)-1].plain != want {
		t.Fatalf("unattached session's last cell = %q, want %q",
			detached[len(detached)-1].plain, want)
	}
	for _, cells := range [][]metaCell{attached, detached} {
		for _, cell := range cells {
			if cell.plain == "current" {
				t.Fatal("the session row still shows a current cell")
			}
		}
	}

	// Both counts end in the same column, whatever the attached flag says.
	grid := buildMetaGrid([][]metaCell{attached, detached}, 40)
	attachedLine := sessionmgr.StripANSI(m.renderRow(rows[0], attached, grid, 60, false))
	detachedLine := sessionmgr.StripANSI(m.renderRow(rows[1], detached, grid, 60, false))
	countColumn := func(line, count string) int {
		t.Helper()
		index := strings.LastIndex(line, count)
		if index < 0 {
			t.Fatalf("%q does not contain %q", line, count)
		}
		return ansi.StringWidth(line[:index])
	}
	attachedColumn := countColumn(attachedLine, windowCountLabel(m.terms, 3))
	detachedColumn := countColumn(detachedLine, windowCountLabel(m.terms, 5))
	if attachedColumn != detachedColumn {
		t.Fatalf("window counts are not in the same column (%d, %d):\n%q\n%q",
			attachedColumn, detachedColumn, attachedLine, detachedLine)
	}
}

func TestListWindowReservesCounterLines(t *testing.T) {
	tests := []struct {
		name                             string
		total, cursor, height            int
		wantStart, wantEnd, wantCounters int
	}{
		{
			name:         "top hides below",
			total:        40,
			cursor:       0,
			height:       6,
			wantStart:    0,
			wantEnd:      5,
			wantCounters: 1,
		},
		{
			name:         "bottom hides above",
			total:        40,
			cursor:       39,
			height:       6,
			wantStart:    35,
			wantEnd:      40,
			wantCounters: 1,
		},
		{
			name:         "middle hides both",
			total:        40,
			cursor:       20,
			height:       6,
			wantStart:    18,
			wantEnd:      22,
			wantCounters: 2,
		},
		{
			name:         "everything fits",
			total:        4,
			cursor:       0,
			height:       6,
			wantStart:    0,
			wantEnd:      4,
			wantCounters: 2,
		},
		{
			name:         "exact fit",
			total:        6,
			cursor:       3,
			height:       6,
			wantStart:    0,
			wantEnd:      6,
			wantCounters: 0,
		},
		{
			name:         "single row list",
			total:        40,
			cursor:       20,
			height:       1,
			wantStart:    20,
			wantEnd:      21,
			wantCounters: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, counters := listWindow(tt.total, tt.cursor, tt.height)
			if start != tt.wantStart || end != tt.wantEnd || counters != tt.wantCounters {
				t.Fatalf("listWindow(%d, %d, %d) = (%d, %d, %d), want (%d, %d, %d)",
					tt.total, tt.cursor, tt.height,
					start, end, counters,
					tt.wantStart, tt.wantEnd, tt.wantCounters)
			}
		})
	}
}

func TestRenderBodyShowsScrollCounters(t *testing.T) {
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
		wantFirst    string
		wantLast     string
		wantSelected string
	}{
		{name: "top only", cursor: 0, wantFirst: "s00", wantLast: "↓ 35 more", wantSelected: "s00"},
		{
			name:         "both",
			cursor:       20,
			wantFirst:    "↑ 18 more",
			wantLast:     "↓ 18 more",
			wantSelected: "s20",
		},
		{
			name:         "bottom only",
			cursor:       39,
			wantFirst:    "↑ 35 more",
			wantLast:     "s39",
			wantSelected: "s39",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width, m.height = 80, 10
			m = withItems(m, items)
			m = m.setCursor(tt.cursor)

			l := m.viewLayout()
			body := m.renderBody(l)
			if len(body) != l.bodyHeight {
				t.Fatalf("body lines = %d, want %d", len(body), l.bodyHeight)
			}
			first := sessionmgr.StripANSI(body[0])
			last := sessionmgr.StripANSI(body[len(body)-1])
			if !strings.Contains(first, tt.wantFirst) {
				t.Fatalf("first list line = %q, want %q", first, tt.wantFirst)
			}
			if !strings.Contains(last, tt.wantLast) {
				t.Fatalf("last list line = %q, want %q", last, tt.wantLast)
			}
			selected := ""
			for _, line := range body {
				if strings.Contains(line, "\x1b[7m") || strings.Contains(line, "▌") {
					selected = sessionmgr.StripANSI(line)
				}
			}
			if !strings.Contains(selected, tt.wantSelected) {
				t.Fatalf("selected line = %q, want %q", selected, tt.wantSelected)
			}
		})
	}

	// No counters when every row fits.
	m := newTestModel(t)
	m.width, m.height = 80, 20
	m = withItems(m, items[:4])
	for _, line := range m.renderBody(m.viewLayout()) {
		plain := sessionmgr.StripANSI(line)
		if strings.Contains(plain, "more") {
			t.Fatalf("counter drawn without hidden rows: %q", plain)
		}
	}
}

func TestPreviewHardCutKeepsWidthColorsAndNoEllipsis(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.preview = "\x1b[31m" + strings.Repeat("x", 60) + "\x1b[0m"

	lines := m.renderPreview(20, 2)
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

func TestPreviewHeaderIsHardCutWithoutEllipsis(t *testing.T) {
	m := newTestModel(t)
	long := strings.Repeat("n", 40)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: long, Target: long},
	})
	header := sessionmgr.StripANSI(m.renderPreview(20, 2)[0])
	if strings.Contains(header, "…") {
		t.Fatalf("preview header shows an ellipsis: %q", header)
	}
	if got := ansi.StringWidth(header); got != 20 {
		t.Fatalf("preview header width = %d, want exactly 20: %q", got, header)
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
