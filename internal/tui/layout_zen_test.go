package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func zenTestTheme() layoutRenderTheme {
	cfg := appconfig.Default()
	return layoutRenderTheme{styles: stylesFromConfig(cfg), icons: cfg.IconSet()}
}

func zenTestView() layoutView {
	return layoutView{
		Frame: frameView{Width: 120, Height: 24, ContentHeight: 23},
		Sources: sourcesView{Entries: []sourceEntryView{
			{ID: "all", Label: "All"},
			{ID: "agents", Label: "Agents", Selected: true},
			{ID: "sessions", Label: "Sessions"},
		}},
		Collection: collectionView{
			Source:           "agents",
			Title:            "Agents",
			SessionPlural:    "Sessions",
			State:            collectionReady,
			ScopeLabel:       "demo",
			AgentStateFilter: "blocked",
			StateFilterLabel: "state: blocked",
			Rows: []rowView{
				{
					Kind: rowKind(sessionmgr.KindAgent), Selected: true,
					Agent: agentRowView{
						DisplayName: "reviewer", State: "blocked", Location: "demo:1",
						Indicator: indicatorView{Mode: displayIcon, Icon: "◐", Color: "11"},
					},
				},
				{
					Kind: rowKind(sessionmgr.KindSession), Label: "workspace",
					Session: sessionRowView{Activity: "2m"},
				},
			},
		},
		Search:  searchView{Query: "review", Editing: false},
		Actions: actionsView{Expanded: true},
		Overview: &overviewView{
			State: overviewReady, Items: 9, Sessions: 3,
			Agents: overviewAgentCountsView{
				Working: 2, Blocked: 1, Done: 1, Idle: 1, Unknown: 1,
			},
		},
		Preview: &previewView{
			State:   previewReady,
			Title:   "PREVIEW_SENTINEL",
			Content: "RAW_PREVIEW_SENTINEL",
		},
		Details: &detailsView{State: detailsReady, Title: "DETAILS_SENTINEL"},
	}
}

func TestZenContentWidth(t *testing.T) {
	for _, tt := range []struct {
		frame int
		want  int
	}{
		{frame: 0, want: 72},
		{frame: 30, want: 29},
		{frame: 40, want: 39},
		{frame: 45, want: 40},
		{frame: 77, want: 72},
		{frame: 93, want: 88},
		{frame: 140, want: 88},
	} {
		t.Run(fmt.Sprint(tt.frame), func(t *testing.T) {
			if got := zenContentWidth(tt.frame); got != tt.want {
				t.Fatalf("zenContentWidth(%d) = %d, want %d", tt.frame, got, tt.want)
			}
		})
	}
}

func TestZenHeaderUsesProjectedNavigationOverviewAndFilters(t *testing.T) {
	view := zenTestView()
	theme := zenTestTheme()
	header := sessionmgr.StripANSI(renderZenHeader(view, theme, 88))
	for _, want := range []string{
		"seshagy", "agents", "○ ● ○", "3 Sessions", "2 working", "1 blocked",
		"1 done", "1 idle", "1 unknown", "6 agents", "· demo · state: blocked", "/review",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("Zen header missing %q\n%s", want, header)
		}
	}

	active := renderZenAgentStateChip(
		theme.styles,
		theme.icons,
		sessionmgr.AgentBlocked,
		1,
		true,
	)
	wantActive := theme.styles.emphasis.Render("◐ 1 blocked")
	if active != wantActive {
		t.Fatalf("active blocked chip = %q, want %q", active, wantActive)
	}

	view.Collection.SessionPlural = "Workspaces"
	if got := sessionmgr.StripANSI(
		renderZenOverview(view, theme, 88),
	); !strings.Contains(
		got,
		"3 Workspaces",
	) {
		t.Fatalf("Zen overview does not use backend terminology\n%s", got)
	}
}

func TestZenOverviewPreservesAllFactsWhenWrapped(t *testing.T) {
	view := zenTestView()
	view.Overview.Sessions = 12
	view.Overview.Agents = overviewAgentCountsView{
		Working: 12,
		Blocked: 23,
		Done:    34,
		Idle:    45,
		Unknown: 56,
	}
	for _, width := range []int{40, 72, 88} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			raw := renderZenOverview(view, zenTestTheme(), width)
			clean := sessionmgr.StripANSI(raw)
			for _, fact := range []string{
				"12 Sessions",
				"12 working",
				"23 blocked",
				"34 done",
				"45 idle",
				"56 unknown",
				"170 agents",
			} {
				if !strings.Contains(clean, fact) {
					t.Errorf("Zen overview width %d dropped %q\n%s", width, fact, clean)
				}
			}
			for _, line := range strings.Split(raw, "\n") {
				if lipgloss.Width(line) > width {
					t.Fatalf(
						"Zen overview width = %d, want <= %d\n%q",
						lipgloss.Width(line),
						width,
						line,
					)
				}
			}
		})
	}
}

func TestZenOverviewLifecycleAndCachedCounts(t *testing.T) {
	view := zenTestView()
	theme := zenTestTheme()
	view.Overview.State = overviewLoading
	if got := renderZenOverview(view, theme, 88); got != "" {
		t.Fatalf("loading overview = %q, want empty", got)
	}
	view.Overview.State = overviewError
	view.Overview.Warning = "cached"
	view.Overview.Error = "refresh failed"
	if got := sessionmgr.StripANSI(
		renderZenOverview(view, theme, 88),
	); !strings.Contains(got, "3 Sessions") ||
		!strings.Contains(got, "6 agents") {
		t.Fatalf("cached overview counts disappeared on error\n%s", got)
	}
	view.Overview = &overviewView{State: overviewEmpty}
	if got := strings.TrimSpace(
		sessionmgr.StripANSI(renderZenOverview(view, theme, 88)),
	); got != "0 Sessions" {
		t.Fatalf("known empty overview = %q, want %q", got, "0 Sessions")
	}
}

func TestZenCollectionRendersLifecycleStates(t *testing.T) {
	theme := zenTestTheme()
	for _, tt := range []struct {
		name string
		view collectionView
		want string
	}{
		{name: "loading", view: collectionView{State: collectionLoading}, want: "refreshing…"},
		{name: "empty", view: collectionView{State: collectionEmpty, EmptyMessage: "nothing here"}, want: "nothing here"},
		{name: "filtered", view: collectionView{State: collectionFilteredEmpty, EmptyMessage: "no matches for x"}, want: "no matches for x"},
		{name: "error", view: collectionView{State: collectionError, Error: "refresh failed"}, want: "refresh failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionmgr.StripANSI(renderZenCollection(tt.view, theme, 48, 5))
			if !strings.Contains(got, tt.want) {
				t.Fatalf("Zen collection missing %q\n%s", tt.want, got)
			}
			for _, line := range strings.Split(got, "\n") {
				if lipgloss.Width(line) > 48 {
					t.Fatalf(
						"Zen collection line width = %d, want <= 48\n%q",
						lipgloss.Width(line),
						line,
					)
				}
			}
		})
	}
}

func TestZenRowsCoverKindsAndUseQuietSelection(t *testing.T) {
	theme := zenTestTheme()
	rows := []struct {
		name string
		row  rowView
		want []string
	}{
		{
			name: "session",
			row: rowView{
				Kind: rowKind(sessionmgr.KindSession), Label: "alpha", Selected: true,
				Session: sessionRowView{Activity: "2m"},
			},
			want: []string{"›", "alpha", "2m"},
		},
		{
			name: "agent",
			row: rowView{
				Kind:  rowKind(sessionmgr.KindAgent),
				Agent: agentRowView{DisplayName: "pi", State: "working", Location: "alpha:1"},
			},
			want: []string{"pi", "alpha:1"},
		},
		{
			name: "zoxide",
			row: rowView{
				Kind:      rowKind(sessionmgr.KindZoxide),
				Directory: directoryRowView{Path: "/src/zoxide"},
			},
			want: []string{"/src/zoxide", "zoxide"},
		},
		{
			name: "fd",
			row: rowView{
				Kind:      rowKind(sessionmgr.KindFD),
				Directory: directoryRowView{Path: "/src/fd"},
			},
			want: []string{"/src/fd", "fd"},
		},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			raw := renderZenRow(tt.row, 48, theme)
			clean := sessionmgr.StripANSI(raw)
			for _, want := range tt.want {
				if !strings.Contains(clean, want) {
					t.Errorf("Zen row missing %q: %q", want, clean)
				}
			}
			if strings.Contains(raw, "\x1b[7m") || strings.Contains(raw, "\x1b[4m") {
				t.Fatalf("Zen row uses reverse or underline styling: %q", raw)
			}
			if lipgloss.Width(raw) > 48 {
				t.Fatalf("Zen row width = %d, want <= 48: %q", lipgloss.Width(raw), raw)
			}
		})
	}
}

func TestZenCollectionKeepsSelectedRowAndOverflowVisible(t *testing.T) {
	view := collectionView{State: collectionReady}
	for index := range 8 {
		view.Rows = append(view.Rows, rowView{
			Kind: rowKind(sessionmgr.KindSession), Label: fmt.Sprintf("row-%d", index),
			Selected: index == 5,
		})
	}
	got := sessionmgr.StripANSI(renderZenCollection(view, zenTestTheme(), 48, 5))
	for _, want := range []string{"↑", "↓", "row-5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Zen overflow collection missing %q\n%s", want, got)
		}
	}
	if lipgloss.Height(got) != 5 {
		t.Fatalf("Zen collection height = %d, want 5\n%s", lipgloss.Height(got), got)
	}
	short := sessionmgr.StripANSI(renderZenCollection(view, zenTestTheme(), 48, 1))
	if !strings.Contains(short, "row-5") {
		t.Fatalf("one-line Zen collection hid the selected row: %q", short)
	}
}

func TestZenCompositionBudgetsHeaderAndPreservesBody(t *testing.T) {
	for contentHeight := 1; contentHeight <= 8; contentHeight++ {
		t.Run(fmt.Sprint(contentHeight), func(t *testing.T) {
			view := zenTestView()
			view.Frame.ContentHeight = contentHeight
			dashboard := renderZen(view, zenTestTheme())
			height := 0
			if dashboard.Header != "" {
				height += lipgloss.Height(dashboard.Header)
			}
			if dashboard.Body != "" {
				height += lipgloss.Height(dashboard.Body)
			}
			if height > contentHeight {
				t.Fatalf(
					"Zen content height = %d, allocated %d\nheader:\n%s\nbody:\n%s",
					height,
					contentHeight,
					dashboard.Header,
					dashboard.Body,
				)
			}
			if !strings.Contains(sessionmgr.StripANSI(dashboard.Body), "reviewer") {
				t.Fatalf(
					"Zen body disappeared at content height %d\n%s",
					contentHeight,
					dashboard.Body,
				)
			}
		})
	}
}

func TestZenShortSurfaceKeepsCollectionAndActionsReachable(t *testing.T) {
	m := newTestModel(t)
	m.layout = zenLayout
	m.width, m.height = 80, 2
	m.loading = false
	m.spinnerActive = false
	m.source = sessionmgr.ModeSessions
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "body-sentinel"}}
	m.showHelp = false

	raw := m.View()
	clean := sessionmgr.StripANSI(raw)
	if lipgloss.Height(raw) != 2 {
		t.Fatalf("short Zen surface height = %d, want 2\n%s", lipgloss.Height(raw), raw)
	}
	for _, want := range []string{"body-sentinel", "? help"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("short Zen surface hid %q\n%s", want, clean)
		}
	}
}

func TestZenCompositionIsWidthSafeAndNeverRendersOptionalPanes(t *testing.T) {
	for _, width := range []int{30, 45, 77, 93, 140} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			view := zenTestView()
			view.Frame.Width = width
			dashboard := renderZen(view, zenTestTheme())
			raw := dashboard.Header + "\n" + dashboard.Body
			clean := sessionmgr.StripANSI(raw)
			for _, forbidden := range []string{
				"PREVIEW_SENTINEL", "RAW_PREVIEW_SENTINEL", "DETAILS_SENTINEL",
			} {
				if strings.Contains(clean, forbidden) {
					t.Fatalf("Zen composition rendered %q\n%s", forbidden, clean)
				}
			}
			for _, line := range strings.Split(raw, "\n") {
				if lipgloss.Width(line) > safeWidth(width) {
					t.Fatalf(
						"Zen frame line width = %d, want <= %d at terminal width %d\n%q",
						lipgloss.Width(line),
						safeWidth(width),
						width,
						line,
					)
				}
			}
		})
	}
}

func TestClampStyledTextPreservesANSIAndUnicode(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff00ff")).
		Render("東京👩‍💻abcdef")
	const openLink = "\x1b]8;;https://example.com\x1b\\"
	const closeLink = "\x1b]8;;\x1b\\"
	got := clampStyledText(openLink+styled+closeLink, 7)
	if !utf8.ValidString(got) {
		t.Fatalf("ANSI-aware truncation emitted invalid UTF-8: %q", got)
	}
	if ansi.StringWidth(got) > 7 {
		t.Fatalf("ANSI-aware truncation width = %d, want <= 7: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, openLink) || !strings.Contains(got, closeLink) {
		t.Fatalf("ANSI-aware truncation split OSC hyperlink sequences: %q", got)
	}
	if !strings.Contains(got, "\x1b[38;2;255;0;255m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("ANSI-aware truncation lost SGR open/reset sequences: %q", got)
	}
	clean := ansi.Strip(got)
	if strings.ContainsRune(clean, utf8.RuneError) || !strings.Contains(clean, "東京") {
		t.Fatalf("ANSI-aware truncation damaged Unicode text: %q", clean)
	}

	unicode := clampStyledText("東京👩‍💻abc", 7)
	if !utf8.ValidString(unicode) || ansi.StringWidth(unicode) > 7 ||
		!strings.Contains(unicode, "👩‍💻") {
		t.Fatalf("grapheme-aware truncation damaged CJK/emoji text: %q", unicode)
	}
}

func TestZenActionsUseCenteredContentColumn(t *testing.T) {
	m := newTestModel(t)
	m.showHelp = true
	actions := m.projectActions()
	for _, frameWidth := range []int{51, 140} {
		t.Run(fmt.Sprint(frameWidth), func(t *testing.T) {
			usable := safeWidth(frameWidth)
			contentWidth := zenContentWidth(frameWidth)
			raw := renderZenActionsLine(actions, m.styles, usable)
			clean := sessionmgr.StripANSI(raw)
			if !strings.Contains(clean, "? help") {
				t.Fatalf("Zen Actions lost Help reachability at width %d: %q", frameWidth, clean)
			}
			if lipgloss.Width(raw) > usable {
				t.Fatalf(
					"Zen Actions width = %d, want <= %d: %q",
					lipgloss.Width(raw),
					usable,
					raw,
				)
			}
			leading := len(clean) - len(strings.TrimLeft(clean, " "))
			outerLeft := (usable - contentWidth) / 2
			if leading < outerLeft || lipgloss.Width(strings.TrimLeft(raw, " ")) > contentWidth {
				t.Fatalf(
					"Zen Actions escaped centered %d-column content at frame width %d: %q",
					contentWidth,
					frameWidth,
					raw,
				)
			}
		})
	}
}

func TestDefaultActionsRendererPreservesTileBytes(t *testing.T) {
	m := newTestModel(t)
	m.showHelp = true
	view := m.projectActions()
	width := 80
	contentWidth := width - 4
	want := paneWithTitle(
		m.styles.tileHelp,
		m.styles.helpTileTitle,
		clampText(renderActionsHelp(view, m.styles), contentWidth),
		"HELP",
		width,
		0,
	)
	got := defaultLayout.renderActions(view, m.styles, width)
	if got != want {
		t.Fatalf("Default Actions bytes changed\nwant: %q\n got: %q", want, got)
	}
}

func TestZenActionsAndSharedInputPresentation(t *testing.T) {
	actions := actionsView{
		Expanded: true,
		Hints: []actionHintView{
			{Key: "?", Label: "help", Available: true},
			{Key: "tab", Label: "source", Available: true},
		},
	}
	raw := renderZenActionsLine(actions, defaultStyles(), 40)
	clean := sessionmgr.StripANSI(raw)
	if !strings.Contains(clean, "? help") || !strings.Contains(clean, "tab source") {
		t.Fatalf("Zen Actions line missing hints: %q", clean)
	}
	if strings.ContainsAny(clean, "╭╮╰╯") || lipgloss.Width(raw) > 40 {
		t.Fatalf("Zen Actions line is bordered or too wide: %q", clean)
	}

	m := newTestModel(t)
	m.layout = zenLayout
	m.width, m.height = 80, 20
	m.inputMode = modeSearch
	m.searchInput.SetValue("needle")
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	if !m.inputPopupActive() {
		t.Fatal("Zen unexpectedly disabled shared popup input")
	}
	popupFooter := sessionmgr.StripANSI(m.renderShellFooter(m.projectActions()))
	if strings.Contains(popupFooter, "SEARCH") || strings.Contains(popupFooter, "needle") {
		t.Fatalf("Zen popup input leaked into footer\n%s", popupFooter)
	}
	if lipgloss.Height(popupFooter) != 1 {
		t.Fatalf(
			"Zen Actions footer height = %d, want 1\n%s",
			lipgloss.Height(popupFooter),
			popupFooter,
		)
	}

	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	if m.inputPopupActive() {
		t.Fatal("Zen cmdline input unexpectedly became a popup")
	}
	cmdlineFooter := sessionmgr.StripANSI(m.renderShellFooter(m.projectActions()))
	if strings.Contains(cmdlineFooter, "SEARCH") || !strings.Contains(cmdlineFooter, "/ needle") ||
		!strings.Contains(cmdlineFooter, "enter to filter · esc to cancel") {
		t.Fatalf("Zen cmdline input presentation changed\n%s", cmdlineFooter)
	}
	if strings.ContainsAny(cmdlineFooter, "╭╮╰╯") || lipgloss.Height(cmdlineFooter) != 3 {
		t.Fatalf("Zen cmdline footer is bordered or has wrong height\n%s", cmdlineFooter)
	}
}

func TestZenInlineInputUsesCenteredContentColumn(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, width := range []int{51, 120} {
		for _, tt := range []struct {
			name string
			mode inputMode
			line string
			help string
		}{
			{
				name: "search",
				mode: modeSearch,
				line: "/ needle",
				help: "enter to filter · esc to cancel",
			},
			{
				name: "rename",
				mode: modeRename,
				line: "old-name -> new-name",
				help: "enter to rename · esc to cancel",
			},
		} {
			t.Run(fmt.Sprintf("%s/%d", tt.name, width), func(t *testing.T) {
				m := newTestModel(t)
				m.layout = zenLayout
				m.width, m.height = width, 20
				m.inputMode = tt.mode
				m.config.TUI.InputStyle = appconfig.InputStyleCmdline
				m.renameFrom = "old-name"
				m.searchInput.SetValue("needle")
				m.renameInput.SetValue("new-name")

				raw := m.renderInlineInputTile()
				clean := sessionmgr.StripANSI(raw)
				lines := strings.Split(clean, "\n")
				if len(lines) != 2 {
					t.Fatalf("Zen inline input lines = %d, want 2\n%s", len(lines), clean)
				}
				if strings.ContainsAny(clean, "╭╮╰╯") || strings.Contains(clean, "SEARCH") ||
					strings.Contains(clean, "RENAME") {
					t.Fatalf("Zen inline input gained Default chrome\n%s", clean)
				}
				if !strings.Contains(lines[0], tt.line) || !strings.Contains(lines[1], tt.help) {
					t.Fatalf("Zen inline input content changed\n%s", clean)
				}

				usable := safeWidth(width)
				contentWidth := zenContentWidthForUsable(usable)
				outerLeft := (usable - contentWidth) / 2
				for index, line := range lines {
					leading := len(line) - len(strings.TrimLeft(line, " "))
					if leading != outerLeft {
						t.Fatalf(
							"Zen inline line %d left offset = %d, want %d: %q",
							index,
							leading,
							outerLeft,
							line,
						)
					}
					if got := lipgloss.Width(strings.TrimLeft(line, " ")); got > contentWidth {
						t.Fatalf(
							"Zen inline line %d width = %d, want <= %d: %q",
							index,
							got,
							contentWidth,
							line,
						)
					}
				}
				if !strings.Contains(raw, m.styles.muted.Render(tt.help)) {
					t.Fatalf("Zen inline help is not muted: %q", raw)
				}
			})
		}
	}
}

func TestZenInlineInputSizesWidgetBeforeRendering(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	m := newTestModel(t)
	m.layout = zenLayout
	m.width, m.height = 51, 20
	m.inputMode = modeSearch
	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	m.searchInput.Width = 20
	m.searchInput.SetValue("前置-" + strings.Repeat("界", 30) + "-tail")
	m.searchInput.CursorEnd()
	m.searchInput.Focus()
	m.searchInput.Cursor.Blink = false

	usable := safeWidth(m.width)
	contentWidth := zenInputContentWidth(usable)
	chrome := m.projectInputChrome(contentWidth)
	raw := m.renderInlineInputTile()
	clean := sessionmgr.StripANSI(raw)
	firstLine := strings.Split(clean, "\n")[0]
	if !strings.Contains(raw, chrome.Line) {
		t.Fatalf("Zen inline input was not rendered from the width-specific widget: %q", raw)
	}
	if !strings.Contains(firstLine, "-tail") || strings.Contains(firstLine, "前置-") {
		t.Fatalf("Zen inline input lost the active scrolled viewport: %q", firstLine)
	}
	if !strings.Contains(raw, "\x1b[7m") {
		t.Fatalf("Zen inline input lost visible cursor state: %q", raw)
	}
	if !utf8.ValidString(raw) {
		t.Fatalf("Zen inline input produced invalid UTF-8: %q", raw)
	}
	if got := lipgloss.Width(strings.TrimLeft(firstLine, " ")); got > contentWidth {
		t.Fatalf("Zen inline input width = %d, want <= %d: %q", got, contentWidth, firstLine)
	}
}

func TestZenInlineInputRoutesCmdlineAndConstrainedPopup(t *testing.T) {
	for _, tt := range []struct {
		name   string
		style  string
		width  int
		height int
		mode   inputMode
		value  string
		help   string
	}{
		{
			name: "cmdline-search", style: appconfig.InputStyleCmdline,
			width: 120, height: 20, mode: modeSearch, value: "needle",
			help: "enter to filter · esc to cancel",
		},
		{
			name: "cmdline-rename", style: appconfig.InputStyleCmdline,
			width: 51, height: 20, mode: modeRename, value: "new-name",
			help: "enter to rename · esc to cancel",
		},
		{
			name: "narrow-popup-fallback", style: appconfig.InputStylePopup,
			width: 24, height: 12, mode: modeSearch, value: "needle",
			help: "enter to filter · esc to cancel",
		},
		{
			name: "short-popup-fallback", style: appconfig.InputStylePopup,
			width: 80, height: 4, mode: modeRename, value: "new-name",
			help: "enter to rename · esc to cancel",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.layout = zenLayout
			m.width, m.height = tt.width, tt.height
			m.inputMode = tt.mode
			m.config.TUI.InputStyle = tt.style
			m.renameFrom = "old-name"
			m.searchInput.SetValue(tt.value)
			m.renameInput.SetValue(tt.value)

			if !m.inlineInputActive() || m.inputPopupActive() {
				t.Fatal("expected Zen input to use the shared inline path")
			}
			clean := sessionmgr.StripANSI(m.View())
			helpWant := tt.help
			if zenInputContentWidth(safeWidth(tt.width)) < lipgloss.Width(helpWant) {
				helpWant = strings.Split(helpWant, " · ")[0]
			}
			if !strings.Contains(clean, tt.value) || !strings.Contains(clean, helpWant) {
				t.Fatalf("Zen inline route lost input/help\n%s", clean)
			}
			if strings.Contains(clean, "SEARCH") || strings.Contains(clean, "RENAME") ||
				strings.ContainsAny(clean, "╭╮╰╯") {
				t.Fatalf("Zen inline route gained Default chrome\n%s", clean)
			}
		})
	}
}

func TestZenNormalPopupPreservesSharedPresentation(t *testing.T) {
	m := newTestModel(t)
	m.layout = zenLayout
	m.width, m.height = 80, 20
	m.inputMode = modeSearch
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.searchInput.SetValue("popup-query")

	if !m.inputPopupActive() || m.inlineInputActive() {
		t.Fatal("Zen changed the shared normal popup decision")
	}
	boxWidth := min(60, m.width-4)
	contentWidth := max(1, boxWidth-4)
	want := renderPopupInput(m.projectInputChrome(contentWidth), m.styles, boxWidth)
	if got := m.renderInputPopup(); got != want {
		t.Fatalf("Zen changed shared popup bytes\nwant: %q\n got: %q", want, got)
	}
	footer := sessionmgr.StripANSI(m.renderShellFooter(m.projectActions()))
	if strings.Contains(footer, "popup-query") || strings.Contains(footer, "SEARCH") {
		t.Fatalf("Zen normal popup leaked into inline footer\n%s", footer)
	}
}

func TestZenInlineInputHeightBoundaries(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			m := newTestModel(t)
			m.layout = zenLayout
			m.width, m.height = 80, height
			m.inputMode = modeSearch
			m.config.TUI.InputStyle = appconfig.InputStyleCmdline
			m.searchInput.SetValue("typed-value")

			view := m.View()
			clean := sessionmgr.StripANSI(view)
			if height == 1 {
				want := renderInputOnly(m.projectInputChrome(safeWidth(m.width)))
				if view != want {
					t.Fatalf("height-one Zen input changed: got=%q want=%q", view, want)
				}
			} else if !strings.Contains(clean, "enter to filter · esc to cancel") {
				t.Fatalf("height %d lost Zen inline help\n%s", height, clean)
			}
			if !strings.Contains(clean, "typed-value") {
				t.Fatalf("height %d lost typed input\n%s", height, clean)
			}
			if strings.Contains(clean, "SEARCH") || strings.ContainsAny(clean, "╭╮╰╯") {
				t.Fatalf("height %d gained Default input chrome\n%s", height, clean)
			}
			if height < 5 && strings.Contains(clean, "? help") {
				t.Fatalf("height %d should prioritize input over Actions\n%s", height, clean)
			}
			if height == 5 && !strings.Contains(clean, "? help") {
				t.Fatalf("height five should retain Zen Actions\n%s", clean)
			}
			if got := lipgloss.Height(view); got != height {
				t.Fatalf("Zen inline view height = %d, want %d\n%s", got, height, clean)
			}
		})
	}
}

func TestDefaultInlineInputRendererPreservesBytes(t *testing.T) {
	m := newTestModel(t)
	m.layout = defaultLayout
	m.width, m.height = 80, 20
	m.inputMode = modeSearch
	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	m.searchInput.SetValue("needle")

	footerWidth := safeWidth(m.width)
	contentWidth := max(1, footerWidth-4)
	want := renderCmdlineInput(m.projectInputChrome(contentWidth), m.styles, footerWidth)
	if got := m.renderInlineInputTile(); got != want {
		t.Fatalf("Default inline input bytes changed\nwant: %q\n got: %q", want, got)
	}
}
