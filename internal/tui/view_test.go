package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return New()
}

func TestViewRendersDashboardChromeAndRows(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Path: "/tmp/demo", Windows: 1, Activity: time.Now(), Created: time.Now()},
		{Kind: sessionmgr.KindAgent, Name: "pi", AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1", Location: "demo:1.1", Path: "~/demo"},
		{Kind: sessionmgr.KindZoxide, Name: "~/code/demo", Path: "~/code/demo"},
	}
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"seshagy", "[1] All", "All (3", "demo", "pi", "Preview"} {
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
		{Kind: sessionmgr.KindAgent, AgentName: "pi", Location: "api:1.1"},
	}
	m.query = "api"
	got := m.visibleItems()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
}

func TestConfiguredASCIIIconsRenderInTUI(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.ASCII = true
	cfg.Icons.Session.ASCII = "S"
	cfg.Icons.Zoxide.ASCII = "Z"
	cfg.Icons.FD.ASCII = "F"
	cfg.Icons.Agent.ASCII = "A"
	m.config = cfg
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Activity: time.Now(), Created: time.Now()},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo"},
		{Kind: sessionmgr.KindFD, Path: "~/src/demo"},
		{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1"},
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = model.(Model)
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"S ◌ demo", "Z ~/code/demo", "F ~/src/demo", "A ▶ pi"} {
		if !strings.Contains(out, want) {
			t.Fatalf("configured ascii icon output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, sessionmgr.IconSession) || strings.Contains(out, sessionmgr.IconZoxide) {
		t.Fatalf("nerd font icons should not render in ascii mode\n%s", out)
	}
}

func TestNoIconsAgentRowsRenderStateLabel(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Enabled = false
	cfg.Icons.ASCII = true
	m.config = cfg

	sessionPrimary, _ := m.rowParts(sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"})
	if got := sessionmgr.StripANSI(sessionPrimary); got != "◌ demo" {
		t.Fatalf("no-icons session primary = %q, want no source prefix", got)
	}
	zoxidePrimary, _ := m.rowParts(sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "~/code/demo"})
	if got := sessionmgr.StripANSI(zoxidePrimary); got != "~/code/demo" {
		t.Fatalf("no-icons zoxide primary = %q, want no source prefix", got)
	}
	agentPrimary, _ := m.rowParts(sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking})
	if got := sessionmgr.StripANSI(agentPrimary); got != "[working] pi" {
		t.Fatalf("no-icons agent primary = %q, want [working] pi", got)
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

	model, _ = m.handleKey(keyMsg("g"))
	m = model.(Model)
	if m.source != sessionmgr.ModeAll || m.query != "ag" {
		t.Fatalf("unprefixed action key should type into filter, source/query = %v/%q", m.source, m.query)
	}

	model, _ = m.handleKey(ctrlXMsg())
	m = model.(Model)
	if !m.prefixArmed {
		t.Fatal("prefix key should arm next action")
	}
	model, _ = m.handleKey(keyMsg("g"))
	m = model.(Model)
	if m.source != sessionmgr.ModeAgents || m.prefixArmed {
		t.Fatalf("prefixed g should switch to agents, source=%v armed=%v", m.source, m.prefixArmed)
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
	model, _ = m.handleKey(keyMsg("g"))
	m = model.(Model)
	if m.source != sessionmgr.ModeAgents {
		t.Fatalf("custom-prefixed g source = %v, want agents", m.source)
	}
}

func TestTypeFirstAllowsEnterWithoutPrefix(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}

	model, _ := m.handleKey(enterMsg())
	m = model.(Model)
	if m.status != "creating session from /tmp/demo" || m.prefixArmed || m.query != "" {
		t.Fatalf("enter should dispatch action without prefix, status=%q armed=%v query=%q", m.status, m.prefixArmed, m.query)
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
		t.Fatalf("down arrow should navigate without prefix, cursor=%d armed=%v query=%q", m.cursor, m.prefixArmed, m.query)
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
	m.setupPrompt = true
	m.setupCursor = 0
	model, _ := m.handleSetupKey(keyMsg("enter"))
	m = model.(Model)
	if m.setupPrompt || !m.config.TypeFirst.Enabled || !m.config.Setup.TypeFirstPromptSeen {
		t.Fatalf("setup did not enable/save type-first: prompt=%v config=%#v", m.setupPrompt, m.config)
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

func TestAgentStateFilterOnlyAppliesInAgentSources(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1"},
		{Kind: sessionmgr.KindAgent, AgentName: "claude", AgentState: sessionmgr.AgentBlocked, PaneID: "%2"},
		{Kind: sessionmgr.KindAgent, AgentName: "codex", AgentState: sessionmgr.AgentIdle, PaneID: "%3"},
		{Kind: sessionmgr.KindSession, Name: "api"},
	}
	m.source = sessionmgr.ModeAgents
	m.agentStateFilter = sessionmgr.AgentWorking
	got := m.visibleItems()
	if len(got) != 1 || got[0].AgentName != "pi" {
		t.Fatalf("agent state filtered items = %#v, want only pi", got)
	}

	m.source = sessionmgr.ModeCurrentAgents
	m.agentStateFilter = sessionmgr.AgentBlocked
	got = m.visibleItems()
	if len(got) != 1 || got[0].AgentName != "claude" {
		t.Fatalf("current-agent state filtered items = %#v, want only claude", got)
	}

	m.source = sessionmgr.ModeAll
	m.agentStateFilter = sessionmgr.AgentWorking
	got = m.visibleItems()
	if len(got) != 4 {
		t.Fatalf("all mode should ignore state filter, got %#v", got)
	}
}

func TestAgentStateFilterCombinesWithTextQuery(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.agentStateFilter = sessionmgr.AgentWorking
	m.query = "api"
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking, Location: "api:1.1", PaneID: "%1"},
		{Kind: sessionmgr.KindAgent, AgentName: "claude", AgentState: sessionmgr.AgentWorking, Location: "web:1.1", PaneID: "%2"},
		{Kind: sessionmgr.KindAgent, AgentName: "codex", AgentState: sessionmgr.AgentBlocked, Location: "api:1.2", PaneID: "%3"},
	}
	got := m.visibleItems()
	if len(got) != 1 || got[0].AgentName != "pi" {
		t.Fatalf("combined filtered items = %#v, want only working api agent", got)
	}
}

func TestAgentStateFilterKeyCyclesAndClears(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1"}}

	model, _ := m.handleKey(keyMsg("s"))
	m = model.(Model)
	if m.agentStateFilter != sessionmgr.AgentWorking || m.status != "agent state filter: working" {
		t.Fatalf("after s filter/status = %q/%q, want working", m.agentStateFilter, m.status)
	}
	model, _ = m.handleKey(keyMsg("s"))
	m = model.(Model)
	if m.agentStateFilter != sessionmgr.AgentBlocked || m.status != "agent state filter: blocked" {
		t.Fatalf("after second s filter/status = %q/%q, want blocked", m.agentStateFilter, m.status)
	}
	model, _ = m.handleKey(keyMsg("S"))
	m = model.(Model)
	if m.agentStateFilter != "" || m.status != "agent state filter: all" {
		t.Fatalf("after S filter/status = %q/%q, want all", m.agentStateFilter, m.status)
	}
}

func TestAgentStateFilterKeyWarnsOutsideAgentPane(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeSessions
	model, _ := m.handleKey(keyMsg("s"))
	m = model.(Model)
	if m.agentStateFilter != "" {
		t.Fatalf("filter changed outside agent source: %q", m.agentStateFilter)
	}
	if m.status != "state filter only applies to agent panes" {
		t.Fatalf("status = %q, want state filter warning", m.status)
	}
	if !isWarningStatus(m.status) {
		t.Fatalf("state filter warning should render as warning")
	}
}

func TestAgentStateFilterRendersTitleHelpAndEmptyState(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = model.(Model)
	m.source = sessionmgr.ModeAgents
	m.agentStateFilter = sessionmgr.AgentBlocked
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1"}}
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"Agents · blocked", "no agent panes with state blocked", "state:blocked", "s state", "S all"} {
		if !strings.Contains(out, want) {
			t.Fatalf("filtered agent view missing %q\n%s", want, out)
		}
	}

	m.source = sessionmgr.ModeSessions
	out = sessionmgr.StripANSI(m.renderFooter())
	if strings.Contains(out, "s state") || strings.Contains(out, "S all") {
		t.Fatalf("agent state filter help should not render outside agent panes\n%s", out)
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
		if width := lipgloss.Width(line); width >= m.width {
			t.Fatalf("footer line %d width = %d, want less than terminal width %d", i, width, m.width)
		}
	}
}

func TestFooterWarningStatusesUseWarningStyle(t *testing.T) {
	s := defaultStyles()
	warnings := []string{
		"no integrations selected",
		"hook installation skipped",
		"rename cancelled",
		"yazi closed without a directory",
		"nothing selected",
		"delete only applies to sessions and agents",
		"rename only applies to sessions",
		"state filter only applies to agent panes",
	}
	for _, status := range warnings {
		style := footerStatusStyle(s, status, false)
		if style.GetForeground() != s.warning.GetForeground() || !style.GetBold() {
			t.Fatalf("footerStatusStyle(%q) = foreground %v bold %v, want warning foreground %v bold true", status, style.GetForeground(), style.GetBold(), s.warning.GetForeground())
		}
		m := newTestModel(t)
		m.width = 80
		m.status = status
		m.showHelp = false
		if clean := sessionmgr.StripANSI(m.renderFooter()); !strings.Contains(strings.Split(clean, "\n")[0], status) {
			t.Fatalf("footer did not render warning status %q on first line:\n%s", status, clean)
		}
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

func enterMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

func ctrlRMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlR}
}

func TestFooterStatusStylesKeepErrorsRedAndNormalMuted(t *testing.T) {
	s := defaultStyles()
	if style := footerStatusStyle(s, "loaded 1171 items", false); style.GetForeground() != s.muted.GetForeground() || style.GetBold() != s.muted.GetBold() {
		t.Fatalf("normal status style = foreground %v bold %v, want muted foreground %v bold %v", style.GetForeground(), style.GetBold(), s.muted.GetForeground(), s.muted.GetBold())
	}
	if style := footerStatusStyle(s, "nothing selected", true); style.GetForeground() != s.danger.GetForeground() || style.GetBold() != s.danger.GetBold() {
		t.Fatalf("error status style = foreground %v bold %v, want danger foreground %v bold %v", style.GetForeground(), style.GetBold(), s.danger.GetForeground(), s.danger.GetBold())
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

func TestIntegrationPromptRendersToggleRows(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = model.(Model)
	m.integrationPrompt = true
	m.integrationRows = []integrations.Recommendation{{Target: integrations.TargetPi, Label: "Pi", AgentAvailable: true, Installable: true, State: integrations.StatusNotInstalled}}
	m.integrationSelected[integrations.TargetPi] = true
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"Install agent state hooks?", "[x] Pi", "space toggle", "pane text or process", "inspection"} {
		if !strings.Contains(out, want) {
			t.Fatalf("integration prompt missing %q\n%s", want, out)
		}
	}
}
