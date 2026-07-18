package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// textModeModel builds a dashboard model in icon text mode at the given size
// with a distinct multi-digit count for every agent state.
func textModeModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = model.(Model)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	for state, count := range distinctAgentCounts() {
		for i := range count {
			m.items = append(m.items, sessionmgr.Item{
				Kind:       sessionmgr.KindAgent,
				AgentName:  fmt.Sprintf("%s-%d", state, i),
				AgentState: state,
			})
		}
	}
	return m
}

func distinctAgentCounts() map[sessionmgr.AgentState]int {
	return map[sessionmgr.AgentState]int{
		sessionmgr.AgentWorking: 11,
		sessionmgr.AgentBlocked: 12,
		sessionmgr.AgentDone:    13,
		sessionmgr.AgentIdle:    14,
		sessionmgr.AgentUnknown: 15,
	}
}

func statsWithDistinctCounts() overviewStats {
	return overviewStats{agents: distinctAgentCounts()}
}

func displaySegment(line string, start, width int) string {
	return ansi.Truncate(ansi.TruncateLeft(line, start, ""), width, "")
}

// TestAgentChipsTextModeFitsTileWidthAndKeepsAllStates verifies the legend never
// overflows its tile inner width (which would make lipgloss wrap it to a second
// line and break the top row) while still representing all five states.
func TestAgentChipsTextModeFitsTileWidthAndKeepsAllStates(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	icons := m.config.IconSet()
	stats := statsWithDistinctCounts()

	// innerW values span the real AGENTS tile content widths the layout
	// produces (compact glyph width ~22 up to the full-label width ~47).
	for _, innerW := range []int{22, 25, 30, 36, 43, 47, 60} {
		t.Run(fmt.Sprintf("inner%d", innerW), func(t *testing.T) {
			row := m.agentChips(icons, stats, innerW)
			if w := lipgloss.Width(row); w > innerW {
				t.Fatalf(
					"innerW=%d: agent chips width %d overflows tile (%q)",
					innerW,
					w,
					sessionmgr.StripANSI(row),
				)
			}
			clean := sessionmgr.StripANSI(row)
			if fields := strings.Fields(clean); len(fields) != 10 {
				t.Fatalf("innerW=%d: want five label/count chip pairs, got %q", innerW, clean)
			}
			if innerW >= 24 {
				for _, want := range []string{"11", "12", "13", "14", "15"} {
					if !strings.Contains(clean, want) {
						t.Fatalf(
							"innerW=%d: legend dropped state count %q (%q)",
							innerW,
							want,
							clean,
						)
					}
				}
			}
		})
	}
}

func TestAgentChipsCompactsWideCountsWithoutDroppingChips(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	stats := overviewStats{agents: map[sessionmgr.AgentState]int{
		sessionmgr.AgentWorking: 1111,
		sessionmgr.AgentBlocked: 2222,
		sessionmgr.AgentDone:    3333,
		sessionmgr.AgentIdle:    4444,
		sessionmgr.AgentUnknown: 5555,
	}}

	_, agentW, _, ok := m.topRowWidths(safeWidth(90), stats, m.config.IconSet())
	if !ok {
		t.Fatal("w=90: expected three-tile allocation")
	}
	innerW := agentW - 4
	row := m.agentChips(m.config.IconSet(), stats, innerW)
	if width := lipgloss.Width(row); width > innerW {
		t.Fatalf(
			"compacted row width %d > allocated inner width %d (%q)",
			width,
			innerW,
			sessionmgr.StripANSI(row),
		)
	}
	clean := sessionmgr.StripANSI(row)
	if fields := strings.Fields(clean); len(fields) != 10 {
		t.Fatalf("wide counts dropped a chip: want five label/count pairs, got %q", clean)
	}
	for _, want := range []string{"1k", "2k", "3k", "4k", "5k"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("compacted row missing count %q (%q)", want, clean)
		}
	}
}

// TestAgentChipsTextModeFullLabelsAtWideWidth verifies that when there is room,
// text mode renders the full state labels rather than always truncating.
func TestAgentChipsTextModeFullLabelsAtWideWidth(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	icons := m.config.IconSet()
	stats := overviewStats{agents: map[sessionmgr.AgentState]int{}}

	row := sessionmgr.StripANSI(m.agentChips(icons, stats, 80))
	for _, label := range []string{"idle", "working", "blocked", "done", "unknown"} {
		if !strings.Contains(row, label) {
			t.Fatalf("wide text-mode legend should show full label %q\n%s", label, row)
		}
	}
}

// TestAgentOverviewTileTextModeNotCollapsed renders the full top row in text
// mode at normal and moderately narrow widths and asserts the AGENTS tile is
// present, fits on a single content line (no wrap → exactly three border lines),
// keeps all five states, and no line exceeds the terminal width.
func TestAgentOverviewTileTextModeNotCollapsed(t *testing.T) {
	for _, width := range []int{120, 100, 90} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			m := textModeModel(t, width, 32)
			top := sessionmgr.StripANSI(m.renderTopRow())
			lines := strings.Split(top, "\n")

			if !strings.Contains(top, "AGENTS") {
				t.Fatalf("w=%d: AGENTS tile collapsed away in text mode\n%s", width, top)
			}
			// A single-line legend renders as exactly three lines (top border,
			// content, bottom border). A wrapped legend makes the tile taller.
			if len(lines) != 3 {
				t.Fatalf(
					"w=%d: top row has %d lines, want 3 (legend wrapped)\n%s",
					width,
					len(lines),
					top,
				)
			}
			safe := safeWidth(width)
			for i, ln := range lines {
				if w := lipgloss.Width(ln); w > safe {
					t.Fatalf("w=%d: line %d width %d > safe %d (%q)", width, i, w, safe, ln)
				}
			}
			stats := aggregateOverviewStats(m.items)
			sourcesW, agentW, _, ok := m.topRowWidths(safe, stats, m.config.IconSet())
			if !ok {
				t.Fatalf("w=%d: expected three-tile allocation", width)
			}
			agentSegment := displaySegment(lines[1], sourcesW+1, agentW)
			for _, want := range []string{"11", "12", "13", "14", "15"} {
				if !strings.Contains(agentSegment, want) {
					t.Fatalf(
						"w=%d: AGENTS tile dropped state count %q\nsegment: %s\nrow: %s",
						width,
						want,
						agentSegment,
						lines[1],
					)
				}
			}
		})
	}
}

// TestAgentOverviewCollapseIsModeIndependent verifies the top-row fallback to a
// SOURCES-only tile triggers at the same width regardless of icon mode: text
// labels must not collapse the AGENTS tile on their own, only genuine width
// starvation does. Width 85 is below the three-tile viability threshold for the
// default source set in both modes.
func TestAgentOverviewCollapseIsModeIndependent(t *testing.T) {
	for _, width := range []int{85, 80} {
		for _, mode := range []string{appconfig.IconModeText, appconfig.IconModeIcons} {
			t.Run(mode+fmt.Sprintf("-w%d", width), func(t *testing.T) {
				m := newTestModel(t)
				cfg := appconfig.Default()
				cfg.Icons.Mode = mode
				m.config = cfg
				model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
				m = model.(Model)
				m.items = []sessionmgr.Item{
					{Kind: sessionmgr.KindSession, Name: "demo"},
					{
						Kind:       sessionmgr.KindAgent,
						AgentName:  "pi",
						AgentState: sessionmgr.AgentWorking,
					},
				}
				top := sessionmgr.StripANSI(m.renderTopRow())
				if strings.Contains(top, "AGENTS") {
					t.Fatalf(
						"%s w=%d: expected SOURCES-only collapse, but AGENTS tile present\n%s",
						mode,
						width,
						top,
					)
				}
				if !strings.Contains(top, "SOURCES") {
					t.Fatalf(
						"%s w=%d: SOURCES-only fallback is missing SOURCES\n%s",
						mode,
						width,
						top,
					)
				}
				lines := strings.Split(top, "\n")
				if len(lines) != 3 {
					t.Fatalf(
						"%s w=%d: fallback has %d lines, want 3\n%s",
						mode,
						width,
						len(lines),
						top,
					)
				}
				for i, line := range lines {
					if got, limit := lipgloss.Width(line), safeWidth(width); got > limit {
						t.Fatalf(
							"%s w=%d: fallback line %d width %d > safe %d (%q)",
							mode,
							width,
							i,
							got,
							limit,
							line,
						)
					}
				}
			})
		}
	}
}

// TestTopRowWidthsRebalance verifies the per-tile width allocation gives AGENTS
// a larger share and WORKSPACES only its title+count width, while keeping the
// SOURCES allocation and the mode-independent collapse threshold unchanged.
func TestTopRowWidthsRebalance(t *testing.T) {
	for _, width := range []int{120, 100, 90} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			m := newTestModel(t)
			cfg := appconfig.Default()
			cfg.Icons.Mode = appconfig.IconModeIcons
			m.config = cfg
			model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
			m = model.(Model)
			usableW := safeWidth(width)
			stats := aggregateOverviewStats([]sessionmgr.Item{
				{Kind: sessionmgr.KindSession, Name: "demo"},
				{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
			})
			icons := m.config.IconSet()
			sourcesW, agentW, wsW, ok := m.topRowWidths(usableW, stats, icons)
			if !ok {
				t.Fatalf("w=%d: expected three-tile layout, got collapse", width)
			}
			// Pre-rebalance (compact) allocation, for comparison.
			oldAgent := clampVal(34, 26, usableW/3)
			oldWs := clampVal(22, 16, usableW/6)
			oldSources := usableW - oldWs - oldAgent - 2
			if agentW <= oldAgent {
				t.Fatalf("w=%d: AGENTS not wider (%d <= old %d)", width, agentW, oldAgent)
			}
			if wsW >= oldWs {
				t.Fatalf("w=%d: WORKSPACES not narrower (%d >= old %d)", width, wsW, oldWs)
			}
			if wsW != 16 {
				t.Fatalf("w=%d: WORKSPACES width %d, want minimal 16", width, wsW)
			}
			// Icon mode pins SOURCES to its pre-rebalance allocation.
			if sourcesW != oldSources {
				t.Fatalf("w=%d: SOURCES changed (%d != old %d)", width, sourcesW, oldSources)
			}
			if sourcesW+agentW+wsW+2 != usableW {
				t.Fatalf(
					"w=%d: tiles %d+%d+%d+2 != usableW %d",
					width,
					sourcesW,
					agentW,
					wsW,
					usableW,
				)
			}
		})
	}
}

// TestTopRowWidthsTextModeFullLabelsAtWideWidth verifies that in text mode at a
// wide width the AGENTS tile is grown enough to fit the full legend (labels are
// not truncated) while WORKSPACES stays minimal and all three tiles fit.
func TestTopRowWidthsTextModeFullLabelsAtWideWidth(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = model.(Model)
	usableW := safeWidth(120)
	stats := aggregateOverviewStats([]sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
	})
	icons := m.config.IconSet()
	sourcesW, agentW, wsW, ok := m.topRowWidths(usableW, stats, icons)
	if !ok {
		t.Fatalf("w=120 text: expected three-tile layout, got collapse")
	}
	// The AGENTS inner width holds the full single-space legend without ellipsis.
	fullLegend := lipgloss.Width(m.agentChipRow(icons, stats, 0, " "))
	if agentW-4 < fullLegend {
		t.Fatalf(
			"w=120 text: AGENTS inner %d < full legend %d (labels truncated)",
			agentW-4,
			fullLegend,
		)
	}
	if wsW != 16 {
		t.Fatalf("w=120 text: WORKSPACES width %d, want minimal 16", wsW)
	}
	if sourcesW+agentW+wsW+2 != usableW {
		t.Fatalf("w=120 text: tiles %d+%d+%d+2 != usableW %d", sourcesW, agentW, wsW, usableW)
	}
}

// TestTopRowWidthsCollapseThresholdUnchanged verifies the three-tile layout
// collapses at the same narrow width in both icon and text mode (genuine
// SOURCES starvation only — the rebalance never collapses due to label width).
func TestTopRowWidthsCollapseThresholdUnchanged(t *testing.T) {
	for _, width := range []int{85, 80} {
		for _, mode := range []string{appconfig.IconModeIcons, appconfig.IconModeText} {
			t.Run(mode+fmt.Sprintf("-w%d", width), func(t *testing.T) {
				m := newTestModel(t)
				cfg := appconfig.Default()
				cfg.Icons.Mode = mode
				m.config = cfg
				model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
				m = model.(Model)
				stats := aggregateOverviewStats([]sessionmgr.Item{
					{Kind: sessionmgr.KindSession, Name: "demo"},
					{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
				})
				icons := m.config.IconSet()
				if _, _, _, ok := m.topRowWidths(safeWidth(width), stats, icons); ok {
					t.Fatalf("%s w=%d: expected collapse, got three-tile layout", mode, width)
				}
			})
		}
	}
}

// TestAgentOverviewIconModeRendersGlyphs confirms icon mode still shows the
// AGENTS tile with glyphs (not text labels) and a single-line legend. The tile
// widths are wider now (AGENTS grew, WORKSPACES shrank), but the glyph legend
// content itself is byte-identical to before the rebalance.
func TestAgentOverviewIconModeRendersGlyphs(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	for _, width := range []int{120, 100, 90} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			m := newTestModel(t)
			cfg := appconfig.Default()
			cfg.Icons.Mode = appconfig.IconModeIcons
			m.config = cfg
			model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
			m = model.(Model)
			m.items = []sessionmgr.Item{
				{Kind: sessionmgr.KindSession, Name: "demo"},
				{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking},
			}
			stats := overviewStats{agents: map[sessionmgr.AgentState]int{
				sessionmgr.AgentWorking: 1,
				sessionmgr.AgentBlocked: 2,
				sessionmgr.AgentDone:    3,
				sessionmgr.AgentIdle:    4,
				sessionmgr.AgentUnknown: 5,
			}}
			_, agentW, _, ok := m.topRowWidths(safeWidth(width), stats, m.config.IconSet())
			if !ok {
				t.Fatalf("w=%d: expected icon-mode three-tile allocation", width)
			}
			want := m.agentChipRow(m.config.IconSet(), stats, 0, "  ")
			got := m.agentChips(m.config.IconSet(), stats, agentW-4)
			if got != want {
				t.Fatalf(
					"w=%d: fitted icon row differs from canonical row\nwant raw: %q\n got raw: %q",
					width,
					want,
					got,
				)
			}

			top := sessionmgr.StripANSI(m.renderTopRow())
			lines := strings.Split(top, "\n")
			if !strings.Contains(top, "AGENTS") {
				t.Fatalf("w=%d: icon mode AGENTS tile missing\n%s", width, top)
			}
			if len(lines) != 3 {
				t.Fatalf("w=%d: icon mode top row has %d lines, want 3\n%s", width, len(lines), top)
			}
			if !strings.Contains(lines[1], "\u25cf") { // working glyph ●
				t.Fatalf("w=%d: icon mode legend missing working glyph\n%s", width, lines[1])
			}
			if strings.Contains(lines[1], "working") {
				t.Fatalf("w=%d: icon mode legend leaked a text label\n%s", width, lines[1])
			}
			safe := safeWidth(width)
			for i, ln := range lines {
				if w := lipgloss.Width(ln); w > safe {
					t.Fatalf("w=%d: icon line %d width %d > safe %d (%q)", width, i, w, safe, ln)
				}
			}
		})
	}
}

// TestAgentStateTextModeListAndDetailNoTruncation asserts the agents list rows
// and detail pane render the full state label (no truncation) at a normal width
// in text mode.
func TestAgentStateTextModeListAndDetailNoTruncation(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	m.config = cfg
	m.width = 90
	item := sessionmgr.Item{
		Kind:       sessionmgr.KindAgent,
		AgentName:  "pi",
		AgentState: sessionmgr.AgentWorking,
		Location:   "demo:1.1",
	}

	row := sessionmgr.StripANSI(m.renderRow(item, false, 60))
	if !strings.Contains(row, "[working]") {
		t.Fatalf("text-mode agent list row missing full [working] label\n%s", row)
	}

	detail := sessionmgr.StripANSI(strings.Join(m.detailLines(item), "\n"))
	if !strings.Contains(detail, "working") {
		t.Fatalf("text-mode agent detail missing full working label\n%s", detail)
	}
}
