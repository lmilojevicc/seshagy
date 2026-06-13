package tui

import (
	"context"
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

func TestAgentDetailAndPreviewShowSessionID(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{
			Kind:           sessionmgr.KindAgent,
			AgentName:      "pi",
			AgentState:     sessionmgr.AgentWorking,
			PaneID:         "%1",
			Location:       "demo:1.1",
			Path:           "~/demo",
			AgentSessionID: "session-1234567890abcdef",
		},
	}
	m.preview = "agent pane output"

	detail := sessionmgr.StripANSI(strings.Join(m.detailLines(m.items[0], 40), "\n"))
	if !strings.Contains(detail, "session") || !strings.Contains(detail, "session-12345678…") {
		t.Fatalf("detail should show truncated session id\n%s", detail)
	}

	preview := sessionmgr.StripANSI(m.renderPreviewPane(50, 12))
	if !strings.Contains(preview, "session-12345678…") || !strings.Contains(preview, "V full") {
		t.Fatalf("preview footer should show truncated session id and expand hint\n%s", preview)
	}

	model, _ = m.handleKey(keyMsg("V"))
	m = model.(Model)
	detail = sessionmgr.StripANSI(strings.Join(m.detailLines(m.items[0], 60), "\n"))
	if !strings.Contains(detail, "session-1234567890abcdef") {
		t.Fatalf("detail should show full session id after V\n%s", detail)
	}
	preview = sessionmgr.StripANSI(m.renderPreviewPane(50, 12))
	if !strings.Contains(preview, "session-1234567890abcdef") {
		t.Fatalf("preview footer should show full session id after V\n%s", preview)
	}
	if m.status != "session id: session-1234567890abcdef" {
		t.Fatalf("status after expand = %q", m.status)
	}
}

func TestAgentSessionIDHiddenWhenAbsent(t *testing.T) {
	m := newTestModel(t)
	item := sessionmgr.Item{
		Kind:       sessionmgr.KindAgent,
		AgentName:  "pi",
		AgentState: sessionmgr.AgentWorking,
		PaneID:     "%1",
	}
	detail := sessionmgr.StripANSI(strings.Join(m.detailLines(item, 40), "\n"))
	if strings.Contains(detail, "session") {
		t.Fatalf("detail should omit session id when absent\n%s", detail)
	}
	m.items = []sessionmgr.Item{item}
	m.preview = "output"
	preview := sessionmgr.StripANSI(m.renderPreviewPane(50, 10))
	if strings.Contains(preview, "V full") || strings.Contains(preview, "session") {
		t.Fatalf("preview should omit session footer when absent\n%s", preview)
	}
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
			Kind:       sessionmgr.KindAgent,
			Name:       "pi",
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%1",
			Location:   "demo:1.1",
			Path:       "~/demo",
		},
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

func TestIntegrationPromptWrapsAtEdges(t *testing.T) {
	m := newTestModel(t)
	m.integration.active = true
	m.integration.rows = []integrations.Recommendation{
		{Target: integrations.TargetPi},
		{Target: integrations.TargetClaude},
		{Target: integrations.TargetCodex},
	}
	m.integration.cursor = 0

	model, _ := m.handleIntegrationKey(upMsg())
	m = model.(Model)
	if m.integration.cursor != 2 {
		t.Fatalf("up from first integration: cursor=%d, want 2", m.integration.cursor)
	}

	m.integration.cursor = 2
	model, _ = m.handleIntegrationKey(downMsg())
	m = model.(Model)
	if m.integration.cursor != 0 {
		t.Fatalf("down from last integration: cursor=%d, want 0", m.integration.cursor)
	}
}

func TestTabsUseCurrentAgentsFourthAndNoTrailingCWD(t *testing.T) {
	m := newTestModel(t)
	out := sessionmgr.StripANSI(m.renderTabs())
	for _, want := range []string{"[1] All", "[2] Sessions", "[3] Agents", "[4] Current agents", "[5] Zoxide", "[6] fd"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tabs missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "·") {
		t.Fatalf("tabs should not render trailing cwd label\n%s", out)
	}
}

func TestDefaultSourceNumberKeys(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.handleKey(keyMsg("4"))
	m = model.(Model)
	if m.source != sessionmgr.ModeCurrentAgents {
		t.Fatalf("4 source = %v, want current agents", m.source)
	}
	model, _ = m.handleKey(keyMsg("5"))
	m = model.(Model)
	if m.source != sessionmgr.ModeZoxide {
		t.Fatalf("5 source = %v, want zoxide", m.source)
	}
	model, _ = m.handleKey(keyMsg("6"))
	m = model.(Model)
	if m.source != sessionmgr.ModeFD {
		t.Fatalf("6 source = %v, want fd", m.source)
	}
}

func TestConfiguredSourceOrderAndDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := appconfig.Default()
	cfg.Sources.Default = "current-agents"
	cfg.Sources.Order = []string{"sessions", "agents", "current-agents", "zoxide", "fd", "all"}
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	m := New()
	if m.source != sessionmgr.ModeCurrentAgents {
		t.Fatalf("New() source = %v, want configured current agents", m.source)
	}
	out := sessionmgr.StripANSI(m.renderTabs())
	for _, want := range []string{"[1] Sessions", "[2] Agents", "[3] Current agents", "[4] Zoxide", "[5] fd", "[6] All"} {
		if !strings.Contains(out, want) {
			t.Fatalf("configured tabs missing %q\n%s", want, out)
		}
	}
	model, _ := m.handleKey(keyMsg("1"))
	m = model.(Model)
	if m.source != sessionmgr.ModeSessions {
		t.Fatalf("configured key 1 source = %v, want sessions", m.source)
	}
	model, _ = m.handleKey(keyMsg("6"))
	m = model.(Model)
	if m.source != sessionmgr.ModeAll {
		t.Fatalf("configured key 6 source = %v, want all", m.source)
	}
}

func TestRefreshUsesConfiguredFDCommand(t *testing.T) {
	m := newTestModel(t)
	m.config.Directories.FDCommand = `printf '%s\n' /tmp/seshagy-tui-fd`
	msg, ok := refreshCmd(sessionmgr.ModeFD, m.config.LoadOptions())().(refreshMsg)
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
	cfg.Icons.Agent.Label = "A"
	m.config = cfg
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Activity: time.Now(), Created: time.Now()},
		{Kind: sessionmgr.KindZoxide, Path: "~/code/demo"},
		{Kind: sessionmgr.KindFD, Path: "~/src/demo"},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%1",
		},
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 28})
	m = model.(Model)
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"S ◌ demo", "Z ~/code/demo", "F ~/src/demo", "A ▶ pi"} {
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
	agentPrimary, _ := m.rowParts(
		sessionmgr.Item{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
		},
	)
	if got := sessionmgr.StripANSI(agentPrimary); got != sessionmgr.IconAgent+"  ▶ pi" {
		t.Fatalf("default agent icon spacing = %q, want two spaces after icon", got)
	}
}

func TestNoIconsAgentRowsRenderStateLabel(t *testing.T) {
	m := newTestModel(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeNone
	m.config = cfg

	sessionPrimary, _ := m.rowParts(sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"})
	if got := sessionmgr.StripANSI(sessionPrimary); got != "demo" {
		t.Fatalf("no-icons session primary = %q, want no source or state prefix", got)
	}
	attachedPrimary, _ := m.rowParts(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "attached", Attached: true},
	)
	if got := sessionmgr.StripANSI(attachedPrimary); got != "attached" {
		t.Fatalf("no-icons attached session primary = %q, want no source or state prefix", got)
	}
	zoxidePrimary, _ := m.rowParts(
		sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "~/code/demo"},
	)
	if got := sessionmgr.StripANSI(zoxidePrimary); got != "~/code/demo" {
		t.Fatalf("no-icons zoxide primary = %q, want no source prefix", got)
	}
	agentPrimary, _ := m.rowParts(
		sessionmgr.Item{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
		},
	)
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
		t.Fatalf(
			"unprefixed action key should type into filter, source/query = %v/%q",
			m.source,
			m.query,
		)
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

func TestTypeFirstVTogglesSessionIDExpand(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	m.items = []sessionmgr.Item{
		{
			Kind:           sessionmgr.KindAgent,
			Name:           "pi",
			AgentName:      "pi",
			AgentState:     sessionmgr.AgentWorking,
			PaneID:         "%1",
			AgentSessionID: "session-1234567890abcdef",
		},
	}
	m.cursor = 0

	model, _ := m.handleKey(keyMsg("V"))
	m = model.(Model)
	if m.query != "" {
		t.Fatalf("V should toggle session id, not filter; query = %q", m.query)
	}
	if m.expandedAgentSessionKey != m.items[0].Key() {
		t.Fatalf("expected expanded session id, key = %q", m.expandedAgentSessionKey)
	}
	if m.status != "session id: session-1234567890abcdef" {
		t.Fatalf("status after V = %q", m.status)
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
	old := checkTmuxPopup
	checkTmuxPopup = func(context.Context) (bool, error) { return true, nil }
	t.Cleanup(func() { checkTmuxPopup = old })

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
	model, cmd = m.handleSetupKey(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("startup setup should continue to startup integration checks")
	}
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

func TestAgentStateFilterOnlyAppliesInAgentSources(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%1",
		},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "claude",
			AgentState: sessionmgr.AgentBlocked,
			PaneID:     "%2",
		},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "codex",
			AgentState: sessionmgr.AgentIdle,
			PaneID:     "%3",
		},
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
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			Location:   "api:1.1",
			PaneID:     "%1",
		},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "claude",
			AgentState: sessionmgr.AgentWorking,
			Location:   "web:1.1",
			PaneID:     "%2",
		},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "codex",
			AgentState: sessionmgr.AgentBlocked,
			Location:   "api:1.2",
			PaneID:     "%3",
		},
	}
	got := m.visibleItems()
	if len(got) != 1 || got[0].AgentName != "pi" {
		t.Fatalf("combined filtered items = %#v, want only working api agent", got)
	}
}

func TestAgentStateFilterKeyCyclesAndClears(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%1",
		},
	}

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
	m.items = []sessionmgr.Item{
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%1",
		},
	}
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
			t.Fatalf(
				"footer line %d width = %d, want less than terminal width %d",
				i,
				width,
				m.width,
			)
		}
	}
}

func TestFooterHelpShowsSourceAndModeKeys(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	out := sessionmgr.StripANSI(m.renderFooter())
	if !strings.Contains(out, "m mode") {
		t.Fatalf("footer should mention mode key\n%s", out)
	}
	for _, want := range []string{"g agents", "o current agents"} {
		if !strings.Contains(out, want) {
			t.Fatalf("footer should mention source key %q\n%s", want, out)
		}
	}

	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	out = sessionmgr.StripANSI(m.renderFooter())
	if !strings.Contains(out, "ctrl+x m mode") {
		t.Fatalf("type-first footer should mention prefixed mode key\n%s", out)
	}

	m.prefixArmed = true
	out = sessionmgr.StripANSI(m.renderFooter())
	for _, want := range []string{"g agents", "o current agents", "m mode"} {
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
		"delete only applies to sessions and agents",
		"rename only applies to sessions",
		"state filter only applies to agent panes",
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
	cfg.Theme.Colors.FocusedBorder = "#ff79c6"
	cfg.Theme.Colors.ActiveTab = "#f5c2e7"
	cfg.Theme.Colors.Border = "#313244"
	cfg.Theme.Colors.InactiveTab = "#6c7086"
	cfg.Theme.Colors.Title = "#b4befe"
	cfg.Theme.Colors.Accent = "#cba6f7"
	cfg.Theme.Colors.Key = "#f9e2af"
	cfg.Theme.Colors.Muted = "#7f849c"
	cfg.Theme.Colors.Success = "#a6e3a1"
	cfg.Theme.Colors.Info = "#89dceb"
	cfg.Theme.Colors.Warning = "#f9e2af"
	cfg.Theme.Colors.Danger = "#f38ba8"
	s := stylesFromConfig(cfg)

	wantBorder := lipgloss.Color("#ff79c6")
	for side, got := range map[string]lipgloss.TerminalColor{
		"top":    s.paneFocus.GetBorderTopForeground(),
		"right":  s.paneFocus.GetBorderRightForeground(),
		"bottom": s.paneFocus.GetBorderBottomForeground(),
		"left":   s.paneFocus.GetBorderLeftForeground(),
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
		t.Fatalf("title color = %v, want #b4befe", got)
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
	if got := s.subtitle.GetForeground(); got != lipgloss.Color("#7f849c") {
		t.Fatalf("subtitle color = %v, want #7f849c", got)
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

	cfg.Theme.Colors.ActiveTab = "default"
	s = stylesFromConfig(cfg)
	if _, ok := s.tabActive.GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf(
			"default active tab should use terminal foreground, got %T",
			s.tabActive.GetForeground(),
		)
	}
}

func TestIntegrationPromptRendersToggleRows(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = model.(Model)
	m.integration.active = true
	m.integration.rows = []integrations.Recommendation{
		{
			Target:         integrations.TargetPi,
			Label:          "Pi",
			AgentAvailable: true,
			Installable:    true,
			State:          integrations.StatusNotInstalled,
		},
	}
	m.integration.selected[integrations.TargetPi] = true
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"Install agent state hooks?", "[x] Pi", "space toggle", "pane text or process", "inspection"} {
		if !strings.Contains(out, want) {
			t.Fatalf("integration prompt missing %q\n%s", want, out)
		}
	}
}
