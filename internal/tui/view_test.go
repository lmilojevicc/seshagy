package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

func TestRowShowsAgentStateGlyphAndWord(t *testing.T) {
	withColor(t)
	m := newTestModel(t)
	item := agentItem("claude", "%1", "frontend")
	item.AgentState = sessionmgr.AgentBlocked
	m = withItems(m, []sessionmgr.Item{item})

	out := m.View()
	plain := sessionmgr.StripANSI(out)
	if !strings.Contains(plain, "◉") {
		t.Fatalf("blocked glyph missing:\n%s", plain)
	}
	if !strings.Contains(plain, "needs you") {
		t.Fatalf("blocked word missing:\n%s", plain)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("state row rendered without styles")
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
