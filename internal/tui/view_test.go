package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Tests mock tmux via SetTmuxHooksForTest; force TMUX + the tmux backend so
	// those seams are exercised and InMultiplexer() is true regardless of the
	// real environment.
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	t.Setenv("HERDR_ENV", "")
	m := New()
	// Tests mock tmux via SetTmuxHooksForTest; force the tmux backend so those
	// seams are exercised even when tests run outside tmux.
	m.mux = sessionmgr.NewTmuxBackend()
	m.terms = m.mux.Terms()
	m.checkPopup = m.mux.InMultiplexerPopup
	return m
}

func TestViewRendersDashboardChromeAndRows(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{
			Kind:     sessionmgr.KindSession,
			Name:     "demo",
			Path:     "/tmp/demo",
			Windows:  1,
			Activity: time.Now(),
			Created:  time.Now(),
		},
		{
			Name:     "pi",
			PaneID:   "%1",
			Location: "demo:1.1",
			Path:     "~/demo",
		},
		{Kind: sessionmgr.KindZoxide, Name: "~/code/demo", Path: "~/code/demo"},
	}
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"1 All", "All (3", "demo", "pi", "Preview"} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() missing %q\n%s", want, out)
		}
	}
}

func TestFilterVisibleItems(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "web"},
	}
	m.query = "api"
	got := m.visibleItems()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(got), got)
	}
}

func TestSearchModeArrowNavigationKeepsInputActive(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "app"},
		{Kind: sessionmgr.KindSession, Name: "web"},
	}
	m.inputMode = modeSearch
	m.query = "ap"
	m.searchInput.SetValue("ap")
	m.searchInput.Focus()

	model, _ := m.handleKey(downMsg())
	m = model.(Model)
	if m.cursor != 1 || m.inputMode != modeSearch || m.query != "ap" ||
		m.searchInput.Value() != "ap" {
		t.Fatalf(
			"down in search mode = cursor:%d mode:%v query:%q input:%q",
			m.cursor,
			m.inputMode,
			m.query,
			m.searchInput.Value(),
		)
	}
	model, _ = m.handleKey(upMsg())
	m = model.(Model)
	if m.cursor != 0 || m.inputMode != modeSearch || m.query != "ap" ||
		m.searchInput.Value() != "ap" {
		t.Fatalf(
			"up in search mode = cursor:%d mode:%v query:%q input:%q",
			m.cursor,
			m.inputMode,
			m.query,
			m.searchInput.Value(),
		)
	}
}

func TestWrapCursorHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		up    bool
		cur   int
		count int
		want  int
	}{
		{name: "up from first wraps to last", up: true, cur: 0, count: 3, want: 2},
		{name: "down from last wraps to first", up: false, cur: 2, count: 3, want: 0},
		{name: "up in middle stays", up: true, cur: 1, count: 3, want: 0},
		{name: "down in middle stays", up: false, cur: 1, count: 3, want: 2},
		{name: "single item up stays", up: true, cur: 0, count: 1, want: 0},
		{name: "single item down stays", up: false, cur: 0, count: 1, want: 0},
		{name: "empty list stays", up: true, cur: 0, count: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got int
			if tt.up {
				got = wrapCursorUp(tt.cur, tt.count)
			} else {
				got = wrapCursorDown(tt.cur, tt.count)
			}
			if got != tt.want {
				t.Fatalf("cursor=%d count=%d: got %d, want %d", tt.cur, tt.count, got, tt.want)
			}
		})
	}
}

func TestListSelectionWrapsAtEdges(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "first"},
		{Kind: sessionmgr.KindSession, Name: "middle"},
		{Kind: sessionmgr.KindSession, Name: "last"},
	}
	m.cursor = 0

	for _, key := range []tea.KeyMsg{upMsg(), keyMsg("k")} {
		m.cursor = 0
		model, _ := m.handleActionKey(key)
		m = model.(Model)
		if m.cursor != 2 {
			t.Fatalf("%q from first item: cursor=%d, want 2", key.String(), m.cursor)
		}
	}

	for _, key := range []tea.KeyMsg{downMsg(), keyMsg("j")} {
		m.cursor = 2
		model, _ := m.handleActionKey(key)
		m = model.(Model)
		if m.cursor != 0 {
			t.Fatalf("%q from last item: cursor=%d, want 0", key.String(), m.cursor)
		}
	}
}

func TestListSelectionDoesNotWrapWithSingleItem(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "only"}}
	m.cursor = 0

	for _, key := range []tea.KeyMsg{upMsg(), downMsg(), keyMsg("k"), keyMsg("j")} {
		model, _ := m.handleActionKey(key)
		m = model.(Model)
		if m.cursor != 0 {
			t.Fatalf("%q with one item: cursor=%d, want 0", key.String(), m.cursor)
		}
	}
}

func TestSearchModeWrapsAtEdges(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "app"},
		{Kind: sessionmgr.KindSession, Name: "web"},
	}
	m.inputMode = modeSearch
	m.query = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.cursor = 0

	model, _ := m.handleKey(upMsg())
	m = model.(Model)
	if m.cursor != 2 {
		t.Fatalf("up in search mode from first: cursor=%d, want 2", m.cursor)
	}

	m.cursor = 2
	model, _ = m.handleKey(downMsg())
	m = model.(Model)
	if m.cursor != 0 {
		t.Fatalf("down in search mode from last: cursor=%d, want 0", m.cursor)
	}
}

func TestRenderTabsFitsTerminalWidth(t *testing.T) {
	for width := 20; width <= 120; width++ {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
		})
	}
}

func TestTabBarSurvivesRefreshAtWidth51(t *testing.T) {
	const width = 51
	maxW := safeWidth(width)

	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	m = model.(Model)
	m.loading = false
	m.showHelp = true
	m.status = "loaded 1212 items"
	m.items = make([]sessionmgr.Item, 1212)
	for i := range m.items {
		m.items[i] = sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/x"}
	}

	check := func(label string) {
		t.Helper()
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) < 2 {
			t.Fatalf("%s: view too short:\n%s", label, sessionmgr.StripANSI(view))
		}
		line0 := sessionmgr.StripANSI(lines[0])
		// The tab bar (containing "All") may sit below the overview hero band,
		// so find it anywhere in the top lines rather than assuming line 0.
		foundTabs := false
		for _, line := range lines {
			if strings.Contains(sessionmgr.StripANSI(line), "All") {
				foundTabs = true
				break
			}
		}
		if !foundTabs {
			t.Fatalf("%s: tab bar not found in view:\nline0=%q", label, line0)
		}
		for i, line := range lines {
			if lipgloss.Width(line) > maxW {
				t.Fatalf(
					"%s: line %d too wide (%d > %d): %q",
					label,
					i,
					lipgloss.Width(line),
					maxW,
					sessionmgr.StripANSI(line),
				)
			}
		}
	}

	check("after load")
	before := lipgloss.Width(strings.Split(m.View(), "\n")[0])

	model, _ = m.Update(refreshMsg{
		source: m.source,
		gen:    m.inflightRefresh[m.source],
		items: []sessionmgr.Item{
			{
				Kind:     sessionmgr.KindSession,
				Name:     strings.Repeat("wide-session-", 20),
				Attached: true,
			},
			{Kind: sessionmgr.KindZoxide, Path: "/very/long/path/" + strings.Repeat("x", 80)},
		},
	})
	m = model.(Model)
	check("after refresh")
	after := lipgloss.Width(strings.Split(m.View(), "\n")[0])
	if after > before+2 {
		t.Fatalf("tab bar widened from %d to %d after refresh", before, after)
	}

	model, _ = m.Update(setupMsg{})
	m = model.(Model)
	check("after setup")
}

func TestDefaultSourceNumberKeys(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.handleKey(keyMsg("3"))
	m = model.(Model)
	if m.source != sessionmgr.ModeZoxide {
		t.Fatalf("3 source = %v, want zoxide", m.source)
	}
	model, _ = m.handleKey(keyMsg("4"))
	m = model.(Model)
	if m.source != sessionmgr.ModeFD {
		t.Fatalf("4 source = %v, want fd", m.source)
	}
}

func TestConfiguredSourceOrderAndDefault(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	t.Setenv("HERDR_ENV", "")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := appconfig.Default()
	cfg.Sources.Default = "zoxide"
	cfg.Sources.Order = []string{"sessions", "zoxide", "fd", "all"}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	m := New()
	if m.source != sessionmgr.ModeZoxide {
		t.Fatalf("New() source = %v, want configured zoxide", m.source)
	}
	out := sessionmgr.StripANSI(m.renderTabs())
	for _, want := range []string{"1 Sessions", "2 Zoxide", "3 fd", "4 All"} {
		if !strings.Contains(out, want) {
			t.Fatalf("configured tabs missing %q\n%s", want, out)
		}
	}
	model, _ := m.handleKey(keyMsg("1"))
	m = model.(Model)
	if m.source != sessionmgr.ModeSessions {
		t.Fatalf("configured key 1 source = %v, want sessions", m.source)
	}
	model, _ = m.handleKey(keyMsg("4"))
	m = model.(Model)
	if m.source != sessionmgr.ModeAll {
		t.Fatalf("configured key 4 source = %v, want all", m.source)
	}
}

// TestRenderTabsChipStyle verifies the finder-style chip rendering: active tab
// is the active_tab color (chipActive, bold + padded), others are muted chips
// (chipIdle), joined by a muted middot separator, with a right-aligned count
// badge.
func TestRenderTabsChipStyle(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "a"},
		{Kind: sessionmgr.KindSession, Name: "b"},
		{Kind: sessionmgr.KindSession, Name: "c"},
	}

	// Style properties: active chip uses the active_tab color (not a reverse pill).
	if m.styles.chipActive.GetReverse() {
		t.Fatal("chipActive must NOT have Reverse (active_tab foreground, not a reversed block)")
	}
	if !m.styles.chipActive.GetBold() {
		t.Fatal("chipActive must be Bold")
	}
	if m.styles.chipIdle.GetReverse() {
		t.Fatal("chipIdle must NOT have Reverse")
	}

	clean := sessionmgr.StripANSI(m.renderTabs())

	// Middot separator present.
	if !strings.Contains(clean, "·") {
		t.Fatalf("chip row missing ' · ' separator\n%s", clean)
	}

	// Active tab label present (default source = All, key 1).
	if !strings.Contains(clean, "1 All") {
		t.Fatalf("chip row missing active '1 All'\n%s", clean)
	}

	// Count badge: 3 visible items.
	if !strings.Contains(clean, "3") {
		t.Fatalf("chip row missing count badge '3'\n%s", clean)
	}
}

// TestRenderTabsChipQueryCount verifies the filtered count badge (vis/total)
// appears when a query is active.
func TestRenderTabsChipQueryCount(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "web"},
		{Kind: sessionmgr.KindSession, Name: "app"},
	}
	m.query = "ap"
	clean := sessionmgr.StripANSI(m.renderTabs())
	// 2 matches (api, app) out of 3 total.
	if !strings.Contains(clean, "2/3") {
		t.Fatalf("chip row missing filtered count '2/3'\n%s", clean)
	}
}

// TestRenderTabsChipFitsNarrowWidth verifies the chip row never exceeds the
// terminal width at narrow sizes, falling back through label formats.
func TestRenderTabsChipFitsNarrowWidth(t *testing.T) {
	for _, width := range []int{20, 25, 30, 40, 51} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			m := newTestModel(t)
			model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			m = model.(Model)
			m.items = make([]sessionmgr.Item, 100)
			line := m.renderTabs()
			clean := sessionmgr.StripANSI(line)
			if w := lipgloss.Width(clean); w > safeWidth(width) {
				t.Fatalf(
					"width %d: chip row too wide (%d > %d): %q",
					width,
					w,
					safeWidth(width),
					clean,
				)
			}
		})
	}
}

func TestRefreshUsesConfiguredFDCommand(t *testing.T) {
	m := newTestModel(t)
	m.config.Directories.FDCommand = `printf '%s\n' /tmp/seshagy-tui-fd`
	msg, ok := refreshCmd(sessionmgr.NewTmuxBackend(), sessionmgr.ModeFD, 1, m.config.LoadOptions())().(refreshMsg)
	if !ok || msg.err != nil {
		t.Fatalf("refreshCmd = %#v, ok=%v", msg, ok)
	}
	if len(msg.items) != 1 || msg.items[0].Kind != sessionmgr.KindFD ||
		msg.items[0].Path != "/tmp/seshagy-tui-fd" {
		t.Fatalf("configured fd refresh items = %#v", msg.items)
	}
}

func TestConfiguredASCIIIconsRenderInTUI(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeText
	cfg.Icons.Session.Label = "S"
	cfg.Icons.Zoxide.Label = "Z"
	cfg.Icons.FD.Label = "F"
	m.config = cfg
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Activity: time.Now(), Created: time.Now()},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo"},
		{Kind: sessionmgr.KindFD, Path: "~/src/demo"},
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = model.(Model)
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"S [detached] demo", "Z ~/code/demo", "F ~/src/demo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("configured ascii icon output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, sessionmgr.IconSession) ||
		strings.Contains(out, sessionmgr.IconZoxide) {
		t.Fatalf("nerd font icons should not render in ascii mode\n%s", out)
	}
}

func TestDefaultIconsRenderWithConfiguredDisplaySpacing(t *testing.T) {
	m := newTestModel(t)
	sessionPrimary, _ := m.rowParts(sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"})
	if got := sessionmgr.StripANSI(sessionPrimary); got != sessionmgr.IconSession+" ◌ demo" {
		t.Fatalf("default session icon spacing = %q, want one space after icon", got)
	}
}

func TestCustomTmuxStateIconInList(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeIcons
	cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeIcons
	cfg.Icons.TmuxState.Attached.Icon = "★"
	m.config = cfg

	item := sessionmgr.Item{
		Kind:     sessionmgr.KindSession,
		Name:     "demo",
		Attached: true,
	}
	primary, _ := m.rowParts(item)
	if got := sessionmgr.StripANSI(primary); !strings.Contains(got, "★ demo") {
		t.Fatalf("custom attached icon list row = %q, want ★ demo", got)
	}
}

func TestTmuxStateModeNoneHidesListPrefix(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeNone
	m.config = cfg

	item := sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Attached: true}
	primary, _ := m.rowParts(item)
	if got := sessionmgr.StripANSI(primary); got != sessionmgr.IconSession+" demo" {
		t.Fatalf("tmux_state=none list row = %q, want session icon + name only", got)
	}

	detail := sessionmgr.StripANSI(strings.Join(m.detailLines(item), "\n"))
	if !strings.Contains(detail, "attached  yes") {
		t.Fatalf("tmux_state=none detail should show plain yes\n%s", detail)
	}
	if strings.Contains(detail, "●") || strings.Contains(detail, "[attached]") {
		t.Fatalf("tmux_state=none detail should not show glyph or bracket label\n%s", detail)
	}
}

func TestCustomTmuxStateLabelInTextMode(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeIcons
	cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeText
	cfg.Icons.TmuxState.Attached.Label = "live"
	m.config = cfg

	item := sessionmgr.Item{
		Kind:     sessionmgr.KindSession,
		Name:     "demo",
		Attached: true,
	}
	primary, _ := m.rowParts(item)
	if got := sessionmgr.StripANSI(primary); !strings.Contains(got, "[live] demo") {
		t.Fatalf("custom attached label list row = %q, want [live] demo", got)
	}
}

func TestTypeFirstTypingFiltersAndPrefixRunsActions(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "web"},
	}

	model, _ := m.handleKey(keyMsg("a"))
	m = model.(Model)
	if m.query != "a" || m.status != "filter: a" {
		t.Fatalf("typing should filter immediately, query/status = %q/%q", m.query, m.status)
	}
	if got := m.visibleItems(); len(got) != 1 || got[0].Name != "api" {
		t.Fatalf("visibleItems after typing = %#v", got)
	}

	model, _ = m.handleKey(ctrlXMsg())
	m = model.(Model)
	if !m.prefixArmed {
		t.Fatal("prefix key should arm next action")
	}
}

func TestTypeFirstPrefixIsConfigurableAndUnprefixedActionsWarn(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = "p"

	model, _ := m.handleKey(ctrlRMsg())
	m = model.(Model)
	if m.status != "press p before actions" || !isWarningStatus(m.status) {
		t.Fatalf("unprefixed non-navigation action status = %q", m.status)
	}

	model, _ = m.handleKey(keyMsg("p"))
	m = model.(Model)
	if !m.prefixArmed {
		t.Fatal("configured prefix should arm actions")
	}
}

func TestTypeFirstAllowsEnterWithoutPrefix(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}

	model, _ := m.handleKey(enterMsg())
	m = model.(Model)
	if m.status != "creating session from /tmp/demo" || m.prefixArmed || m.query != "" {
		t.Fatalf(
			"enter should dispatch action without prefix, status=%q armed=%v query=%q",
			m.status,
			m.prefixArmed,
			m.query,
		)
	}
}

func TestTypeFirstAllowsArrowNavigationWithoutPrefix(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "web"},
	}
	model, _ := m.handleKey(downMsg())
	m = model.(Model)
	if m.cursor != 1 || m.prefixArmed || m.query != "" {
		t.Fatalf(
			"down arrow should navigate without prefix, cursor=%d armed=%v query=%q",
			m.cursor,
			m.prefixArmed,
			m.query,
		)
	}
}

func TestYaziBlockedInsideTmuxPopup(t *testing.T) {
	m := newTestModel(t)
	m.checkPopup = func(context.Context) (bool, error) { return true, nil }

	model, cmd := m.startYazi()
	m = model.(Model)
	if cmd != nil {
		t.Fatal("yazi should not launch inside tmux popup")
	}
	if m.err == nil || m.status != "cannot open yazi inside a tmux popup" {
		t.Fatalf("popup yazi status/err = %q/%v", m.status, m.err)
	}
}

func TestStartupSetupPromptSavesTypeFirstChoice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	msg, ok := startupSetupCmd(appconfig.Default())().(setupMsg)
	if !ok || !msg.prompt || msg.err != nil {
		t.Fatalf("startupSetupCmd = %#v, %v", msg, ok)
	}
	m := New()
	model, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("showing startup setup prompt should not run a command yet")
	}
	m = model.(Model)
	if !m.setup.active || m.setup.manual || m.setup.cursor != 1 {
		t.Fatalf(
			"startup prompt state = prompt:%v manual:%v cursor:%d",
			m.setup.active,
			m.setup.manual,
			m.setup.cursor,
		)
	}
	m.setup.cursor = 0
	m.width = 100
	if out := sessionmgr.StripANSI(
		m.renderSetupPrompt(28),
	); !strings.Contains(
		out,
		"Choose startup input mode",
	) {
		t.Fatalf("startup setup prompt should use startup title\n%s", out)
	}
	model, _ = m.handleSetupKey(keyMsg("enter"))
	m = model.(Model)
	if m.setup.active || !m.config.TypeFirst.Enabled || !m.config.Setup.TypeFirstPromptSeen {
		t.Fatalf(
			"setup did not enable/save type-first: prompt=%v config=%#v",
			m.setup.active,
			m.config,
		)
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after setup: %v", err)
	}
	if !loaded.TypeFirst.Enabled || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("saved setup config = %#v", loaded)
	}
	afterMsg, ok := startupSetupCmd(loaded)().(setupMsg)
	if !ok || afterMsg.prompt || afterMsg.err != nil {
		t.Fatalf("startupSetupCmd after saved choice = %#v, %v", afterMsg, ok)
	}
	m.width = 100
	out := sessionmgr.StripANSI(m.renderFooter())
	if !strings.Contains(out, "type-first") {
		t.Fatalf("footer should show type-first after setup\n%s", out)
	}
}

func TestManualModePromptInClassicSavesWithoutHookScan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := New()
	m.width = 100

	model, cmd := m.handleKey(keyMsg("m"))
	if cmd != nil {
		t.Fatal("opening manual input-mode prompt should not run a command")
	}
	m = model.(Model)
	if !m.setup.active || !m.setup.manual || m.setup.cursor != 1 ||
		m.status != "change input mode" {
		t.Fatalf(
			"manual prompt state = prompt:%v manual:%v cursor:%d status:%q",
			m.setup.active,
			m.setup.manual,
			m.setup.cursor,
			m.status,
		)
	}
	if out := sessionmgr.StripANSI(
		m.renderSetupPrompt(28),
	); !strings.Contains(
		out,
		"Change input mode",
	) {
		t.Fatalf("manual setup prompt should use manual title\n%s", out)
	}
	if out := sessionmgr.StripANSI(m.renderSetupPrompt(28)); !strings.Contains(out, "esc cancel") {
		t.Fatalf("manual setup prompt should show cancel key\n%s", out)
	}

	model, cmd = m.handleSetupKey(keyMsg("y"))
	if cmd != nil {
		t.Fatal("manual input-mode save should not trigger hook integration startup scan")
	}
	m = model.(Model)
	if m.setup.active || m.setup.manual || !m.config.TypeFirst.Enabled ||
		!m.config.Setup.TypeFirstPromptSeen {
		t.Fatalf(
			"manual mode save state = prompt:%v manual:%v config:%#v",
			m.setup.active,
			m.setup.manual,
			m.config,
		)
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after manual mode save: %v", err)
	}
	if !loaded.TypeFirst.Enabled || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("saved manual mode config = %#v", loaded)
	}
}

func TestManualModePromptEscCancelsWithoutSaving(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := New()

	model, _ := m.handleKey(keyMsg("m"))
	m = model.(Model)
	model, cmd := m.handleSetupKey(keyMsg("esc"))
	if cmd != nil {
		t.Fatal("manual input-mode cancel should not run a command")
	}
	m = model.(Model)
	if m.setup.active || m.setup.manual || m.config.TypeFirst.Enabled ||
		m.config.Setup.TypeFirstPromptSeen {
		t.Fatalf(
			"manual cancel state = prompt:%v manual:%v config:%#v",
			m.setup.active,
			m.setup.manual,
			m.config,
		)
	}
	if m.status != "input mode change cancelled" || !isWarningStatus(m.status) {
		t.Fatalf("manual cancel status = %q", m.status)
	}
	if appconfig.Exists() {
		t.Fatal("manual cancel should not write config")
	}
}

func TestTypeFirstManualModePromptEscDoesNotDisable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := appconfig.Default()
	cfg.TypeFirst.Enabled = true
	cfg.Setup.TypeFirstPromptSeen = true
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	m := New()

	model, _ := m.handleKey(ctrlXMsg())
	m = model.(Model)
	model, _ = m.handleKey(keyMsg("m"))
	m = model.(Model)
	if !m.setup.active || !m.setup.manual || m.setup.cursor != 0 {
		t.Fatalf(
			"manual type-first prompt state = prompt:%v manual:%v cursor:%d",
			m.setup.active,
			m.setup.manual,
			m.setup.cursor,
		)
	}
	model, cmd := m.handleSetupKey(keyMsg("esc"))
	if cmd != nil {
		t.Fatal("manual type-first cancel should not run a command")
	}
	m = model.(Model)
	if m.setup.active || m.setup.manual || !m.config.TypeFirst.Enabled {
		t.Fatalf(
			"manual type-first cancel state = prompt:%v manual:%v config:%#v",
			m.setup.active,
			m.setup.manual,
			m.config,
		)
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after manual cancel: %v", err)
	}
	if !loaded.TypeFirst.Enabled || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("manual cancel should preserve saved type-first config = %#v", loaded)
	}
}

func TestTypeFirstManualModePromptRequiresPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := New()
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix

	model, _ := m.handleKey(keyMsg("m"))
	m = model.(Model)
	if m.setup.active || m.query != "m" {
		t.Fatalf(
			"unprefixed m should filter in type-first mode, prompt=%v query=%q",
			m.setup.active,
			m.query,
		)
	}

	model, _ = m.handleKey(ctrlXMsg())
	m = model.(Model)
	if !m.prefixArmed {
		t.Fatal("prefix should arm actions")
	}
	model, cmd := m.handleKey(keyMsg("m"))
	if cmd != nil {
		t.Fatal("opening prefixed manual input-mode prompt should not run a command")
	}
	m = model.(Model)
	if !m.setup.active || !m.setup.manual || m.setup.cursor != 0 || m.prefixArmed {
		t.Fatalf(
			"prefixed m prompt state = prompt:%v manual:%v cursor:%d prefix:%v",
			m.setup.active,
			m.setup.manual,
			m.setup.cursor,
			m.prefixArmed,
		)
	}

	model, cmd = m.handleSetupKey(keyMsg("n"))
	if cmd != nil {
		t.Fatal("manual type-first mode save should not trigger hook integration startup scan")
	}
	m = model.(Model)
	if m.config.TypeFirst.Enabled || !m.config.Setup.TypeFirstPromptSeen {
		t.Fatalf("manual mode disable config = %#v", m.config)
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after manual disable: %v", err)
	}
	if loaded.TypeFirst.Enabled || !loaded.Setup.TypeFirstPromptSeen {
		t.Fatalf("saved manual disable config = %#v", loaded)
	}
}

func TestFooterKeepsStatusOnOneLine(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.source = sessionmgr.ModeAll
	m.status = "loaded 1171 items"
	m.showHelp = false

	footer := m.renderFooter()
	if height := lipgloss.Height(footer); height != 2 {
		t.Fatalf("footer height = %d, want 2\n%s", height, sessionmgr.StripANSI(footer))
	}
	clean := sessionmgr.StripANSI(footer)
	lines := strings.Split(clean, "\n")
	if len(lines) != 2 {
		t.Fatalf("footer lines = %d, want 2\n%s", len(lines), clean)
	}
	if !strings.Contains(lines[0], "loaded 1171 items") {
		t.Fatalf("status wrapped or disappeared from first line:\n%s", clean)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > safeWidth(m.width) {
			t.Fatalf(
				"footer line %d width = %d, want at most safe width %d",
				i,
				width,
				safeWidth(m.width),
			)
		}
	}
}

func TestFooterHelpShowsSourceAndModeKeys(t *testing.T) {
	m := newTestModel(t)
	// Wide enough that the full footer help renders without clampText
	// truncation, so end-of-list keys like "x kill" stay visible.
	m.width = 180

	// Default All tab: universal keys plus the sessions/all-only keys; the
	// agents-only keys are omitted.
	out := sessionmgr.StripANSI(m.renderFooter())
	for _, want := range []string{"m mode", "r refresh", "R rename", "x kill", "y yazi", "p preview"} {
		if !strings.Contains(out, want) {
			t.Fatalf("All-tab footer should mention %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"o this session", "s filter state"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("All-tab footer should not mention %q\n%s", unwanted, out)
		}
	}

	// Agents tab swaps in the agents-only keys and drops the sessions-only ones.
	m.source = sessionmgr.ModeAgents
	out = sessionmgr.StripANSI(m.renderFooter())
	for _, want := range []string{"o this session", "s filter state", "R rename", "m mode", "r refresh"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Agents-tab footer should mention %q\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"x kill", "y yazi"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("Agents-tab footer should not mention %q\n%s", unwanted, out)
		}
	}

	// Type-first help is independent of the active tab.
	m.source = sessionmgr.ModeAll
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	out = sessionmgr.StripANSI(m.renderFooter())
	if !strings.Contains(out, "ctrl+x m mode") {
		t.Fatalf("type-first footer should mention prefixed mode key\n%s", out)
	}

	m.prefixArmed = true
	out = sessionmgr.StripANSI(m.renderFooter())
	for _, want := range []string{"m mode", "r refresh", "x kill"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prefix-armed footer should mention %q\n%s", want, out)
		}
	}
}

func TestFooterWarningStatusesUseWarningStyle(t *testing.T) {
	s := defaultStyles()
	warnings := []string{
		"no integrations selected",
		"hook installation skipped",
		"input mode change cancelled",
		"rename cancelled",
		"yazi closed without a directory",
		"nothing selected",
		"delete only applies to sessions",
		"rename only applies to sessions and agents",
	}
	for _, status := range warnings {
		style := footerStatusStyle(s, status, false)
		if style.GetForeground() != s.warning.GetForeground() || !style.GetBold() {
			t.Fatalf(
				"footerStatusStyle(%q) = foreground %v bold %v, want warning foreground %v bold true",
				status,
				style.GetForeground(),
				style.GetBold(),
				s.warning.GetForeground(),
			)
		}
		m := newTestModel(t)
		m.width = 80
		m.status = status
		m.showHelp = false
		if clean := sessionmgr.StripANSI(
			m.renderFooter(),
		); !strings.Contains(
			strings.Split(clean, "\n")[0],
			status,
		) {
			t.Fatalf("footer did not render warning status %q on first line:\n%s", status, clean)
		}
	}
}

func TestRenderPreviewPaneBottomAnchoredForTmuxCaptureKinds(t *testing.T) {
	// Build content with more lines than fit so overflow clipping is exercised.
	var contentLines []string
	for i := 0; i < 10; i++ {
		contentLines = append(contentLines, fmt.Sprintf("line%02d", i))
	}
	content := strings.Join(contentLines, "\n")

	for _, tt := range []struct {
		name      string
		kind      sessionmgr.Kind
		wantFirst bool
		wantLast  bool
	}{
		{name: "agent bottom-anchored", kind: sessionmgr.KindAgent, wantFirst: false, wantLast: true},
		{name: "session bottom-anchored", kind: sessionmgr.KindSession, wantFirst: false, wantLast: true},
		{name: "fd top-down", kind: sessionmgr.KindFD, wantFirst: true, wantLast: false},
		{name: "zoxide top-down", kind: sessionmgr.KindZoxide, wantFirst: true, wantLast: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.items = []sessionmgr.Item{{
				Kind:     tt.kind,
				Name:     "demo",
				PaneID:   "%1",
				Path:     "/tmp/demo",
				Activity: time.Now(),
				Created:  time.Now(),
			}}
			m.preview = content
			out := sessionmgr.StripANSI(m.renderPreviewPane(60, 8))
			if tt.wantLast && !strings.Contains(out, "line09") {
				t.Fatalf("%s: preview should contain last line\n%s", tt.name, out)
			}
			if tt.wantLast && strings.Contains(out, "line00") {
				t.Fatalf("%s: preview should NOT contain first (clipped) line\n%s", tt.name, out)
			}
			if tt.wantFirst && !strings.Contains(out, "line00") {
				t.Fatalf("%s: preview should contain first line\n%s", tt.name, out)
			}
			if tt.wantFirst && strings.Contains(out, "line09") {
				t.Fatalf("%s: preview should NOT contain last (clipped) line\n%s", tt.name, out)
			}
		})
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func ctrlXMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlX}
}

func downMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyDown}
}

func upMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyUp}
}

func enterMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

func ctrlRMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlR}
}

func TestFooterStatusStylesKeepErrorsRedAndNormalMuted(t *testing.T) {
	s := defaultStyles()
	if style := footerStatusStyle(
		s,
		"loaded 1171 items",
		false,
	); style.GetForeground() != s.muted.GetForeground() ||
		style.GetBold() != s.muted.GetBold() {
		t.Fatalf(
			"normal status style = foreground %v bold %v, want muted foreground %v bold %v",
			style.GetForeground(),
			style.GetBold(),
			s.muted.GetForeground(),
			s.muted.GetBold(),
		)
	}
	if style := footerStatusStyle(
		s,
		"nothing selected",
		true,
	); style.GetForeground() != s.danger.GetForeground() ||
		style.GetBold() != s.danger.GetBold() {
		t.Fatalf(
			"error status style = foreground %v bold %v, want danger foreground %v bold %v",
			style.GetForeground(),
			style.GetBold(),
			s.danger.GetForeground(),
			s.danger.GetBold(),
		)
	}
}

func TestDefaultStylesUseTerminalPalette(t *testing.T) {
	s := defaultStyles()
	if _, ok := s.app.GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf("app foreground should use terminal default, got %T", s.app.GetForeground())
	}
	if _, ok := s.app.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("app background should use terminal default, got %T", s.app.GetBackground())
	}
	if _, ok := s.status.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("status background should use terminal default, got %T", s.status.GetBackground())
	}
	if !s.selectedBG.GetReverse() {
		t.Fatal("selected rows should use reverse video so selection follows terminal colors")
	}

	for name, color := range map[string]lipgloss.TerminalColor{
		"session": s.p.green,
		"zoxide":  s.p.sky,
		"fd":      s.p.peach,
		"agent":   s.p.mauve,
	} {
		value, ok := color.(lipgloss.Color)
		if !ok {
			t.Fatalf("%s icon color should come from ANSI terminal palette, got %T", name, color)
		}
		if strings.HasPrefix(string(value), "#") {
			t.Fatalf("%s icon color should not be fixed truecolor: %s", name, value)
		}
	}
}

func TestConfiguredThemeColorsApply(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Theme.Colors.PopupBorder = "#ff79c6"
	cfg.Theme.Colors.ActiveTab = "#f5c2e7"
	cfg.Theme.Colors.Border = "#313244"
	cfg.Theme.Colors.InactiveTab = "#6c7086"
	cfg.Theme.Colors.PopupTitle = "#b4befe"
	cfg.Theme.Colors.Accent = "#cba6f7"
	cfg.Theme.Colors.Key = "#f9e2af"
	cfg.Theme.Colors.Muted = "#7f849c"
	cfg.Theme.Colors.Success = "#a6e3a1"
	cfg.Theme.Colors.Info = "#89dceb"
	cfg.Theme.Colors.Warning = "#f9e2af"
	cfg.Theme.Colors.Danger = "#f38ba8"
	cfg.Theme.Colors.ListBorder = "#111111"
	cfg.Theme.Colors.MetadataBorder = "#222222"
	cfg.Theme.Colors.PreviewBorder = "#333333"
	cfg.Theme.Colors.ListBorderTitle = "#444444"
	cfg.Theme.Colors.MetadataBorderTitle = "#555555"
	cfg.Theme.Colors.PreviewBorderTitle = "#666666"
	s := stylesFromConfig(cfg)

	wantBorder := lipgloss.Color("#ff79c6")
	for side, got := range map[string]lipgloss.TerminalColor{
		"top":    s.panePopup.GetBorderTopForeground(),
		"right":  s.panePopup.GetBorderRightForeground(),
		"bottom": s.panePopup.GetBorderBottomForeground(),
		"left":   s.panePopup.GetBorderLeftForeground(),
	} {
		if got != wantBorder {
			t.Fatalf("%s focused border color = %v, want %v", side, got, wantBorder)
		}
	}
	if got := s.tabActive.GetForeground(); got != lipgloss.Color("#f5c2e7") {
		t.Fatalf("active tab color = %v, want #f5c2e7", got)
	}
	if got := s.pane.GetBorderTopForeground(); got != lipgloss.Color("#313244") {
		t.Fatalf("border color = %v, want #313244", got)
	}
	if got := s.tabInactive.GetForeground(); got != lipgloss.Color("#6c7086") {
		t.Fatalf("inactive tab color = %v, want #6c7086", got)
	}
	if got := s.title.GetForeground(); got != lipgloss.Color("#b4befe") {
		t.Fatalf("popup title color = %v, want #b4befe", got)
	}
	if got := s.emphasis.GetForeground(); got != lipgloss.Color("#cba6f7") {
		t.Fatalf("accent emphasis color = %v, want #cba6f7", got)
	}
	if got := s.bar.GetForeground(); got != lipgloss.Color("#cba6f7") {
		t.Fatalf("accent bar color = %v, want #cba6f7", got)
	}
	if got := s.key.GetForeground(); got != lipgloss.Color("#f9e2af") {
		t.Fatalf("key color = %v, want #f9e2af", got)
	}
	if got := s.muted.GetForeground(); got != lipgloss.Color("#7f849c") {
		t.Fatalf("muted color = %v, want #7f849c", got)
	}
	if got := s.success.GetForeground(); got != lipgloss.Color("#a6e3a1") {
		t.Fatalf("success color = %v, want #a6e3a1", got)
	}
	if got := s.info.GetForeground(); got != lipgloss.Color("#89dceb") {
		t.Fatalf("info color = %v, want #89dceb", got)
	}
	if got := s.warning.GetForeground(); got != lipgloss.Color("#f9e2af") {
		t.Fatalf("warning color = %v, want #f9e2af", got)
	}
	if got := s.danger.GetForeground(); got != lipgloss.Color("#f38ba8") {
		t.Fatalf("danger color = %v, want #f38ba8", got)
	}
	// Per-pane borders and border titles resolve to their configured colors.
	if got := s.paneList.GetBorderTopForeground(); got != lipgloss.Color("#111111") {
		t.Fatalf("list border = %v, want #111111", got)
	}
	if got := s.paneDetail.GetBorderTopForeground(); got != lipgloss.Color("#222222") {
		t.Fatalf("metadata border = %v, want #222222", got)
	}
	if got := s.panePreview.GetBorderTopForeground(); got != lipgloss.Color("#333333") {
		t.Fatalf("preview border = %v, want #333333", got)
	}
	if s.listTitle != lipgloss.Color("#444444") {
		t.Fatalf("list title = %v, want #444444", s.listTitle)
	}
	if s.metadataTitle != lipgloss.Color("#555555") {
		t.Fatalf("metadata title = %v, want #555555", s.metadataTitle)
	}
	if s.previewTitle != lipgloss.Color("#666666") {
		t.Fatalf("preview title = %v, want #666666", s.previewTitle)
	}

	cfg.Theme.Colors.ActiveTab = "default"
	s = stylesFromConfig(cfg)
	if _, ok := s.tabActive.GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf(
			"default active tab should use terminal foreground, got %T",
			s.tabActive.GetForeground(),
		)
	}
}

func TestItemNameStyleIndependentOfActiveTab(t *testing.T) {
	cases := []struct {
		name      string
		activeTab string
		wantTab   lipgloss.TerminalColor
	}{
		{
			name:      "real active_tab color does not leak into item names",
			activeTab: "#f5c2e7",
			wantTab:   lipgloss.Color("#f5c2e7"),
		},
		{
			name:      "default active_tab keeps terminal-default item names",
			activeTab: "default",
			wantTab:   lipgloss.NoColor{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.Theme.Colors.ActiveTab = tc.activeTab
			s := stylesFromConfig(cfg)

			// Item names must be decoupled from active_tab: always terminal default.
			if _, ok := s.itemName.GetForeground().(lipgloss.NoColor); !ok {
				t.Fatalf(
					"itemName foreground with active_tab=%q = %T, want lipgloss.NoColor (terminal default)",
					tc.activeTab,
					s.itemName.GetForeground(),
				)
			}
			// The active tab itself must still follow active_tab.
			if got := s.tabActive.GetForeground(); got != tc.wantTab {
				t.Fatalf(
					"tabActive foreground with active_tab=%q = %v, want %v",
					tc.activeTab,
					got,
					tc.wantTab,
				)
			}
		})
	}
}

// TestRowPartsItemNameIgnoresActiveTabColor is the behavioral guard for issue
// #30: a rendered session/agent row must not inherit the active_tab color.
// Unlike TestItemNameStyleIndependentOfActiveTab (which checks the style
// field), this asserts on rowParts() rendered output, so it catches a revert
// of rowParts() back to s.tabActive.
//
// lipgloss strips styling when no color terminal is detected (as in tests),
// so the TrueColor profile is forced for the duration of this test and
// restored afterward. Under TrueColor a real active_tab color emits a
// truecolor SGR escape that NoColor (the itemName foreground) does not, so
// the row would byte-differ if rowParts used s.tabActive.
//
// newTestModel sets m.styles from the default config, so changing m.config
// alone does not propagate active_tab into styles; render() reassigns
// m.styles via stylesFromConfig(cfg) to make the active_tab value take effect.
func TestRowPartsItemNameIgnoresActiveTabColor(t *testing.T) {
	origProfile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(origProfile)
	lipgloss.SetColorProfile(termenv.TrueColor)

	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
		{Kind: sessionmgr.KindAgent, AgentName: "pi"},
	}

	render := func(activeTab string, item sessionmgr.Item) string {
		cfg := appconfig.Default()
		cfg.Theme.Colors.ActiveTab = activeTab
		m := newTestModel(t)
		m.config = cfg
		m.styles = stylesFromConfig(cfg) // propagate active_tab into styles
		primary, _ := m.rowParts(item)
		return primary
	}

	for _, item := range items {
		pink := render("#f5c2e7", item)
		def := render("default", item)
		if pink != def {
			t.Fatalf("%s row must not inherit active_tab color:\npink:   %q\ndefault: %q",
				item.Kind, pink, def)
		}
	}
}

// TestItemNameIsRegularWeight guards the row-weight unification: list-row
// item names (session/agent/zoxide/fd) render at regular weight via
// s.itemName, which uses terminal-default foreground with no bold. Fails if
// .Bold(true) is re-added to s.itemName. TrueColor is forced because lipgloss
// strips styling under the default no-TTY profile.
func TestItemNameIsRegularWeight(t *testing.T) {
	origProfile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(origProfile)
	lipgloss.SetColorProfile(termenv.TrueColor)

	s := stylesFromConfig(appconfig.Default())
	got := s.itemName.Render("demo")
	if strings.Contains(got, "\x1b[1m") || strings.Contains(got, "\x1b[1;") {
		t.Fatalf("itemName must be regular weight (no bold SGR): got %q", got)
	}
}

// TestTmuxTermsByteIdenticalStrings is the regression guard: under tmux terms,
// every parameterized string must match the pre-Phase-5 literal exactly.
func TestTmuxTermsByteIdenticalStrings(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 32

	// Footer: ✓ tmux
	footer := m.renderFooter()
	if !strings.Contains(sessionmgr.StripANSI(footer), "✓ tmux") {
		t.Fatalf("footer missing '✓ tmux' under tmux terms\n%s", footer)
	}

	// Session detail: "tmux session" and "windows" key
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindSession,
		Name:    "demo",
		Path:    "/tmp/demo",
		Windows: 3,
	}}
	m.cursor = 0
	detail := m.renderDetailPane(60, 10)
	clean := sessionmgr.StripANSI(detail)
	if !strings.Contains(clean, "tmux session") {
		t.Fatalf("session detail missing 'tmux session'\n%s", clean)
	}
	if !strings.Contains(clean, "windows") {
		t.Fatalf("session detail missing 'windows' key\n%s", clean)
	}

	// Directory detail: "create/switch tmux session"
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindFD, Path: "/tmp/foo"}}
	m.cursor = 0
	detail = m.renderDetailPane(60, 10)
	clean = sessionmgr.StripANSI(detail)
	if !strings.Contains(clean, "create/switch tmux session") {
		t.Fatalf("dir detail missing 'create/switch tmux session'\n%s", clean)
	}

	// Agent detail: "session" key
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, Name: "pi", PaneID: "%1", Session: "demo"},
	}
	m.cursor = 0
	detail = m.renderDetailPane(60, 10)
	clean = sessionmgr.StripANSI(detail)
	if !strings.Contains(clean, "session") {
		t.Fatalf("agent detail missing 'session' key\n%s", clean)
	}
}

// TestHerdrTermsRenderedStrings verifies the herdr vocabulary appears when the
// model uses herdr terms.
func TestHerdrTermsRenderedStrings(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.width = 120
	m.height = 32

	// Footer: ✓ herdr
	footer := m.renderFooter()
	if !strings.Contains(sessionmgr.StripANSI(footer), "✓ herdr") {
		t.Fatalf("footer missing '✓ herdr'\n%s", footer)
	}

	// Session detail: "herdr workspace", "tabs", and "panes" keys; path shown;
	// activity/created omitted (herdr exposes no timestamps).
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindSession,
		Name:    "proj",
		Target:  "w1",
		Path:    "/tmp/proj",
		Windows: 2,
		Panes:   5,
	}}
	m.cursor = 0
	detail := m.renderDetailPane(60, 10)
	clean := sessionmgr.StripANSI(detail)
	if !strings.Contains(clean, "herdr workspace") {
		t.Fatalf("session detail missing 'herdr workspace'\n%s", clean)
	}
	if !strings.Contains(clean, "tabs") {
		t.Fatalf("session detail missing 'tabs' key\n%s", clean)
	}
	if !strings.Contains(clean, "panes") {
		t.Fatalf("session detail missing 'panes' key\n%s", clean)
	}
	if !strings.Contains(clean, "/tmp/proj") {
		t.Fatalf("session detail missing path value\n%s", clean)
	}
	if strings.Contains(clean, "activity") || strings.Contains(clean, "created") {
		t.Fatalf("herdr session detail must omit activity/created (no timestamps)\n%s", clean)
	}
}

// TestHerdrAgentDetailShowsTabLabel verifies the agent detail panel shows the
// resolved tab label (not the opaque tab id) under herdr.
func TestHerdrAgentDetailShowsTabLabel(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.items = []sessionmgr.Item{{
		Kind:      sessionmgr.KindAgent,
		Name:      "pi",
		AgentName: "pi",
		Session:   "w1",
		Window:    "w1:t2",
		PaneID:    "w1:p1",
		Location:  "proj",
		TabLabel:  "logs",
	}}
	m.cursor = 0
	detail := m.renderDetailPane(60, 12)
	clean := sessionmgr.StripANSI(detail)
	if !strings.Contains(clean, "tab") || !strings.Contains(clean, "logs") {
		t.Fatalf("agent detail missing tab label 'logs'\n%s", clean)
	}
	// The opaque tab id must NOT leak.
	if strings.Contains(clean, "w1:t2") {
		t.Fatalf("agent detail leaks opaque tab id\n%s", clean)
	}
}

// TestSessionDetailShowsPanesAndTimestampsWhenSet pins the positive branch: when
// a backend reports a pane count and real created/activity timestamps, all
// three rows render. Guards against a regression that always-hides them.
func TestSessionDetailShowsPanesAndTimestampsWhenSet(t *testing.T) {
	m := newTestModel(t) // default tmux terms
	m.width = 120
	m.height = 32
	m.items = []sessionmgr.Item{{
		Kind:     sessionmgr.KindSession,
		Name:     "demo",
		Path:     "/tmp/demo",
		Windows:  2,
		Panes:    4,
		Created:  time.Now().Add(-24 * time.Hour),
		Activity: time.Now().Add(-time.Hour),
	}}
	m.cursor = 0
	detail := m.renderDetailPane(60, 14)
	clean := sessionmgr.StripANSI(detail)
	for _, want := range []string{"panes", "activity", "created"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("session detail missing %q\n%s", want, clean)
		}
	}
}

// TestSessionDetailHidesPanesAndTimestampsWhenZero pins the negative branch:
// when pane count and timestamps are absent, the rows are omitted rather than
// showing a misleading "unknown".
func TestSessionDetailHidesPanesAndTimestampsWhenZero(t *testing.T) {
	m := newTestModel(t) // default tmux terms
	m.width = 120
	m.height = 32
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindSession,
		Name:    "demo",
		Path:    "/tmp/demo",
		Windows: 2,
		// Panes, Activity, Created intentionally zero.
	}}
	m.cursor = 0
	detail := m.renderDetailPane(60, 14)
	clean := sessionmgr.StripANSI(detail)
	for _, unwanted := range []string{"panes", "activity", "created"} {
		if strings.Contains(clean, unwanted) {
			t.Fatalf("session detail must omit %q when absent\n%s", unwanted, clean)
		}
	}
}

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

// sgrPrefix returns the leading foreground SGR escape sequence lipgloss emits
// for c, so tests can assert a specific color sequence is present in output.
func sgrPrefix(c lipgloss.TerminalColor) string {
	rendered := lipgloss.NewStyle().Foreground(c).Render("X")
	if i := strings.Index(rendered, "X"); i >= 0 {
		return rendered[:i]
	}
	return rendered
}

// TestListPreviewDetailTitlesOnBorder verifies the section titles moved from
// the pane body onto the top border (fieldset style), and the old in-body
// title/kind lines are gone.
func TestListPreviewDetailTitlesOnBorder(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.width = 120
	m.height = 32
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindSession,
		Name:    "proj",
		Target:  "w1",
		Path:    "/tmp/proj",
		Windows: 2,
		Panes:   5,
	}}
	m.cursor = 0
	m.source = sessionmgr.ModeAll

	// List: ModeAll summary lives on the top border line.
	listOut := sessionmgr.StripANSI(m.renderListPane(60, 16))
	listTop := strings.Split(listOut, "\n")[0]
	if !strings.HasPrefix(listTop, "╭") || !strings.HasSuffix(listTop, "╮") {
		t.Fatalf("list top line is not a border edge: %q", listTop)
	}
	if !strings.Contains(listTop, "All (") {
		t.Fatalf("list border missing ModeAll summary: %q", listTop)
	}

	// Detail: name · kind on the border; body's first non-blank line is a kv.
	detailOut := sessionmgr.StripANSI(m.renderDetailPane(60, 14))
	detailTop := strings.Split(detailOut, "\n")[0]
	if !strings.HasPrefix(detailTop, "╭") {
		t.Fatalf("detail top line is not a border edge: %q", detailTop)
	}
	if !strings.Contains(detailTop, "proj · herdr workspace") {
		t.Fatalf("detail border missing 'proj · herdr workspace': %q", detailTop)
	}
	// Old in-body title/kind lines must be gone from the body.
	body := strings.TrimSpace(strings.Join(strings.Split(detailOut, "\n")[1:], "\n"))
	if strings.HasPrefix(body, "proj") {
		t.Fatalf("detail body still starts with the name line:\n%s", detailOut)
	}
	if !strings.Contains(detailOut, "path") {
		t.Fatalf("detail body missing 'path' kv:\n%s", detailOut)
	}

	// Preview: 'Preview · name' on the border.
	m.preview = "hello world"
	previewOut := sessionmgr.StripANSI(m.renderPreviewPane(60, 12))
	previewTop := strings.Split(previewOut, "\n")[0]
	if !strings.HasPrefix(previewTop, "╭") {
		t.Fatalf("preview top line is not a border edge: %q", previewTop)
	}
	if !strings.Contains(previewTop, "Preview · proj") {
		t.Fatalf("preview border missing 'Preview · proj': %q", previewTop)
	}
}

// TestPaneTitlesUsePerPaneColors verifies each pane renders its top border in
// its configured border color and the title text in its configured title color.
func TestPaneTitlesUsePerPaneColors(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	cfg := appconfig.Default()
	cfg.Theme.Colors.ListBorder = "#100000"
	cfg.Theme.Colors.MetadataBorder = "#001000"
	cfg.Theme.Colors.PreviewBorder = "#000010"
	cfg.Theme.Colors.ListBorderTitle = "#110000"
	cfg.Theme.Colors.MetadataBorderTitle = "#001100"
	cfg.Theme.Colors.PreviewBorderTitle = "#000011"

	m := newTestModel(t)
	m.config = cfg
	m.styles = stylesFromConfig(cfg)
	m.terms = sessionmgr.HerdrTerms()
	m.width = 120
	m.height = 32
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindSession,
		Name:    "proj",
		Target:  "w1",
		Path:    "/tmp/proj",
		Windows: 2,
		Panes:   5,
	}}
	m.cursor = 0
	m.source = sessionmgr.ModeAll
	m.preview = "hello"

	s := m.styles
	border := map[string]string{
		"list":     sgrPrefix(s.paneList.GetBorderTopForeground()),
		"metadata": sgrPrefix(s.paneDetail.GetBorderTopForeground()),
		"preview":  sgrPrefix(s.panePreview.GetBorderTopForeground()),
	}
	title := map[string]string{
		"list":     sgrPrefix(s.listTitle),
		"metadata": sgrPrefix(s.metadataTitle),
		"preview":  sgrPrefix(s.previewTitle),
	}
	outs := map[string]string{
		"list":     m.renderListPane(60, 16),
		"metadata": m.renderDetailPane(60, 14),
		"preview":  m.renderPreviewPane(60, 12),
	}
	for _, pane := range []string{"list", "metadata", "preview"} {
		top := strings.Split(outs[pane], "\n")[0]
		if border[pane] == title[pane] {
			t.Fatalf("%s pane border and title colors collide", pane)
		}
		if !strings.Contains(top, border[pane]) {
			t.Fatalf("%s pane top border missing its border color sequence", pane)
		}
		if !strings.Contains(top, title[pane]) {
			t.Fatalf("%s pane top border missing its title color sequence", pane)
		}
	}
}

// TestRenderOverview verifies the hero band renders the workspaces + agents
// tiles from the warmed ModeAll cache, and hides when no data or short height.
func TestRenderOverview(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.source = sessionmgr.ModeSessions
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Attached: true},
			{Kind: sessionmgr.KindSession, Name: "proj", Attached: false},
			{Kind: sessionmgr.KindSession, Name: "api", Attached: true},
			{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking},
			{Kind: sessionmgr.KindAgent, AgentName: "claude", AgentState: sessionmgr.AgentBlocked},
		}, fetchedAt: time.Now()},
	}

	out := sessionmgr.StripANSI(m.renderOverview())
	// Workspaces tile title + count + attached.
	if !strings.Contains(out, "SESSIONS") {
		t.Fatalf("overview missing SESSIONS tile title\n%s", out)
	}
	if !strings.Contains(out, "3") {
		t.Fatalf("overview missing session count\n%s", out)
	}
	if !strings.Contains(out, "(2 attached)") {
		t.Fatalf("overview missing attached count\n%s", out)
	}
	// Agents tile title.
	if !strings.Contains(out, "AGENTS") {
		t.Fatalf("overview missing AGENTS tile title\n%s", out)
	}
	// Both tiles are bordered (fieldset top edges on line 0 and 2).
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("overview too short: %d lines\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("line 0 not a tile top edge: %q", lines[0])
	}
}

// TestRenderOverviewHidesWhenNoDataOrShort verifies the hero band returns ""
// before the ModeAll cache loads and on short terminals.
func TestRenderOverviewHidesWhenNoDataOrShort(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.source = sessionmgr.ModeSessions
	// No ModeAll cache yet.
	if got := m.renderOverview(); got != "" {
		t.Fatalf("overview should hide before data loads, got %q", got)
	}

	// Prime cache, but make terminal short.
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo"},
		}, fetchedAt: time.Now()},
	}
	m.height = 10
	if got := m.renderOverview(); got != "" {
		t.Fatalf("overview should hide on short terminals, got %q", got)
	}
}

// TestRenderOverviewNoAgents verifies the agents tile shows the full state
// legend (all five states, with 0 counts) even when there are zero agents.
func TestRenderOverviewNoAgents(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.source = sessionmgr.ModeSessions
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo"},
		}, fetchedAt: time.Now()},
	}
	out := sessionmgr.StripANSI(m.renderOverview())
	if !strings.Contains(out, "AGENTS") {
		t.Fatalf("overview missing AGENTS tile title\n%s", out)
	}
	// The legend renders every state with a 0 count, not a placeholder.
	zeros := strings.Count(out, "0")
	if zeros < 5 {
		t.Fatalf(
			"overview agents tile should show five 0-count states, found %d zeros\n%s",
			zeros,
			out,
		)
	}
}
