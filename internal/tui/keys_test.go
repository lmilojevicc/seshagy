package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func testKeyItems(names ...string) []sessionmgr.Item {
	items := make([]sessionmgr.Item, len(names))
	for i, name := range names {
		items[i] = sessionmgr.Item{Kind: sessionmgr.KindSession, Name: name}
	}
	return items
}

func TestHandleKeySearchModeEscEnter(t *testing.T) {
	m := newTestModel(t)
	m.items = testKeyItems("api", "app", "web")
	m.inputMode = modeSearch
	m.query = "ap"
	m.searchInput.SetValue("ap")
	m.searchInput.Focus()

	model, cmd := m.handleKey(keyMsg("esc"))
	got := model.(Model)
	if got.inputMode != modeNormal || got.query != "ap" || cmd != nil {
		t.Fatalf("esc = mode:%v query:%q cmd:%v", got.inputMode, got.query, cmd)
	}

	m.inputMode = modeSearch
	m.searchInput.SetValue("api")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if got.inputMode != modeNormal || got.query != "api" || cmd == nil {
		t.Fatalf("enter = mode:%v query:%q cmd:%v", got.inputMode, got.query, cmd)
	}
}

func TestHandleKeyRenameModeCancelAndSubmit(t *testing.T) {
	m := newTestModel(t)
	m.inputMode = modeRename
	m.renameFrom = "demo"
	m.renameInput.SetValue("demo")
	m.renameInput.Focus()

	model, cmd := m.handleKey(keyMsg("esc"))
	got := model.(Model)
	if got.inputMode != modeNormal || got.status != "rename cancelled" || cmd != nil {
		t.Fatalf("rename esc = mode:%v status:%q cmd:%v", got.inputMode, got.status, cmd)
	}

	m.inputMode = modeRename
	m.renameFrom = "demo"
	m.renameInput.SetValue("   ")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if got.inputMode != modeNormal || got.status != "rename cancelled" || cmd != nil {
		t.Fatalf("empty rename = mode:%v status:%q cmd:%v", got.inputMode, got.status, cmd)
	}

	m.inputMode = modeRename
	m.renameFrom = "demo"
	m.renameKind = sessionmgr.KindSession
	m.renameInput.SetValue("renamed")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if got.inputMode != modeNormal || got.renameFrom != "" || cmd == nil {
		t.Fatalf("rename submit = mode:%v renameFrom:%q cmd:%v", got.inputMode, got.renameFrom, cmd)
	}
}

func TestDeleteSelectedSession(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
	}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if got.status != "killing session demo" || cmd == nil {
		t.Fatalf("session delete = status:%q cmd:%v", got.status, cmd)
	}

	m.items = nil
	model, cmd = m.deleteSelected()
	got = model.(Model)
	if got.status != "nothing selected" || cmd != nil {
		t.Fatalf("empty delete = status:%q cmd:%v", got.status, cmd)
	}
}

func TestHandleActionKeyPgUpDownHomeEnd(t *testing.T) {
	m := newTestModel(t)
	m.items = testKeyItems("00", "01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11")
	m.cursor = 0
	m.showPreview = true

	model, cmd := m.handleActionKey(tea.KeyMsg{Type: tea.KeyPgDown})
	got := model.(Model)
	if got.cursor != 10 || cmd == nil {
		t.Fatalf("pgdown = cursor:%d cmd:%v", got.cursor, cmd)
	}

	model, cmd = got.handleActionKey(tea.KeyMsg{Type: tea.KeyPgUp})
	got = model.(Model)
	if got.cursor != 0 || cmd == nil {
		t.Fatalf("pgup = cursor:%d cmd:%v", got.cursor, cmd)
	}

	model, cmd = got.handleActionKey(tea.KeyMsg{Type: tea.KeyEnd})
	got = model.(Model)
	if got.cursor != 11 || cmd == nil {
		t.Fatalf("end = cursor:%d cmd:%v", got.cursor, cmd)
	}

	model, cmd = got.handleActionKey(tea.KeyMsg{Type: tea.KeyHome})
	got = model.(Model)
	if got.cursor != 0 || cmd == nil {
		t.Fatalf("home = cursor:%d cmd:%v", got.cursor, cmd)
	}
}

func TestActivateSelectedAgentFocusesPane(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{{
		Kind:      sessionmgr.KindAgent,
		Name:      "pi",
		AgentName: "pi",
		Session:   "seshagy",
		Window:    "1",
		Pane:      "2",
		PaneID:    "%5",
		Location:  "seshagy:1.2",
	}}
	m.cursor = 0

	model, cmd := m.activateSelected()
	got := model.(Model)
	if cmd == nil {
		t.Fatal("activateSelected on agent returned nil cmd")
	}
	if !strings.Contains(got.status, "focusing pi on seshagy:1.2") {
		t.Fatalf("status = %q, want 'focusing pi on seshagy:1.2'", got.status)
	}
}

func TestActivateSelectedAgentMissingPaneInfoNoOps(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{{
		Kind:    sessionmgr.KindAgent,
		Name:    "pi",
		PaneID:  "%5",
		Session: "seshagy",
		// Window + PaneID pane empty: Window missing
	}}
	m.cursor = 0

	model, cmd := m.activateSelected()
	got := model.(Model)
	if cmd != nil {
		t.Fatal("activateSelected on malformed agent should return nil cmd")
	}
	if !strings.Contains(got.status, "cannot focus") {
		t.Fatalf("status = %q, want 'cannot focus'", got.status)
	}
}

func TestHandleActionKeyDeleteAndRename(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
	}

	model, cmd := m.handleActionKey(keyMsg("x"))
	got := model.(Model)
	if got.status != "killing session demo" || cmd == nil {
		t.Fatalf("x = status:%q cmd:%v", got.status, cmd)
	}

	model, cmd = m.handleActionKey(keyMsg("R"))
	got = model.(Model)
	if got.inputMode != modeRename || got.renameFrom != "demo" || cmd == nil {
		t.Fatalf("R = mode:%v renameFrom:%q cmd:%v", got.inputMode, got.renameFrom, cmd)
	}
}

func TestClearFilterTextAndDeleteFilterRune(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	m.items = testKeyItems("alpha", "beta")
	m.query = "al"
	m.searchInput.SetValue("al")

	model, cmd := m.deleteFilterRune()
	got := model.(Model)
	if got.query != "a" || got.status != "filter: a" || cmd == nil {
		t.Fatalf("delete rune = query:%q status:%q cmd:%v", got.query, got.status, cmd)
	}

	model, cmd = got.clearFilterText()
	got = model.(Model)
	if got.query != "" || got.status != "filter cleared" || cmd == nil {
		t.Fatalf("clear filter = query:%q status:%q cmd:%v", got.query, got.status, cmd)
	}

	model, cmd = got.deleteFilterRune()
	got = model.(Model)
	if got.query != "" || cmd != nil {
		t.Fatalf("delete on empty = query:%q cmd:%v", got.query, cmd)
	}
}

func TestHandleActionKeyRefreshToggleHelpAndPreview(t *testing.T) {
	m := newTestModel(t)
	m.items = testKeyItems("demo")
	m.showPreview = false

	model, cmd := m.handleActionKey(keyMsg("r"))
	got := model.(Model)
	if got.status != "refreshing" || cmd == nil {
		t.Fatalf("refresh = status:%q cmd:%v", got.status, cmd)
	}

	model, cmd = got.handleActionKey(keyMsg("p"))
	got = model.(Model)
	if !got.showPreview || cmd == nil {
		t.Fatalf("preview toggle = show:%v cmd:%v", got.showPreview, cmd)
	}

	got.showHelp = false
	model, cmd = got.handleActionKey(keyMsg("?"))
	got = model.(Model)
	if !got.showHelp || cmd != nil {
		t.Fatalf("help toggle = show:%v cmd:%v", got.showHelp, cmd)
	}

	model, cmd = got.handleActionKey(keyMsg("backspace"))
	got = model.(Model)
	if got.query != "" || cmd != nil {
		t.Fatalf("backspace on empty query = query:%q cmd:%v", got.query, cmd)
	}

	got.query = "demo"
	got.searchInput.SetValue("demo")
	model, cmd = got.handleActionKey(keyMsg("backspace"))
	got = model.(Model)
	if got.query != "" || got.cursor != 0 || cmd == nil {
		t.Fatalf("backspace clear = query:%q cursor:%d cmd:%v", got.query, got.cursor, cmd)
	}
}

func TestHandleActionKeySourceSwitchesAndNumberKeys(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeSessions
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.cache = map[sessionmgr.SourceMode]modeCache{}

	cases := []struct {
		key    string
		source sessionmgr.SourceMode
	}{
		{"a", sessionmgr.ModeAll},
		{"t", sessionmgr.ModeSessions},
		{"z", sessionmgr.ModeZoxide},
		{"f", sessionmgr.ModeFD},
		{"1", sessionmgr.ModeAll},
	}
	for _, tc := range cases {
		m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
		model, _ := m.handleActionKey(keyMsg(tc.key))
		got := model.(Model)
		if got.source != tc.source {
			t.Fatalf("%q switch = source:%v, want %v", tc.key, got.source, tc.source)
		}
		m = got
	}
}

func TestHandleActionKeyActivateSelectedKinds(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	model, cmd := m.handleActionKey(enterMsg())
	got := model.(Model)
	if got.status != "attaching demo" || cmd == nil {
		t.Fatalf("session enter = status:%q cmd:%v", got.status, cmd)
	}

	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}
	model, cmd = m.handleActionKey(enterMsg())
	got = model.(Model)
	if got.status != "creating session from /tmp/demo" || cmd == nil {
		t.Fatalf("zoxide enter = status:%q cmd:%v", got.status, cmd)
	}

	m.items = nil
	model, cmd = m.handleActionKey(enterMsg())
	got = model.(Model)
	if got.status != "nothing selected" || cmd != nil {
		t.Fatalf("empty enter = status:%q cmd:%v", got.status, cmd)
	}
}

func TestHandleActionKeyModePrompt(t *testing.T) {
	m := newTestModel(t)
	model, cmd := m.handleActionKey(keyMsg("m"))
	got := model.(Model)
	if !got.setup.active || got.status != "change input mode" || cmd != nil {
		t.Fatalf("mode prompt = active:%v status:%q cmd:%v", got.setup.active, got.status, cmd)
	}
}

func TestStartYaziBlockedInsidePopup(t *testing.T) {
	m := newTestModel(t)
	m.checkPopup = func(context.Context) (bool, error) { return true, nil }

	model, cmd := m.startYazi()
	got := model.(Model)
	if cmd != nil {
		t.Fatalf("blocked startYazi should not exec, cmd=%v", cmd)
	}
	if got.err == nil || !strings.Contains(got.status, "cannot open yazi inside a tmux popup") {
		t.Fatalf("blocked startYazi = status:%q err:%v", got.status, got.err)
	}
}

func TestStartYaziOutsidePopup(t *testing.T) {
	m := newTestModel(t)
	m.checkPopup = func(context.Context) (bool, error) { return false, nil }

	model, cmd := m.startYazi()
	got := model.(Model)
	if cmd == nil || got.status != "opening yazi" {
		t.Fatalf("start yazi = status:%q cmd:%v", got.status, cmd)
	}
}

func TestDeleteSelectedUnsupportedKind(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if got.status != "delete only applies to sessions" || cmd != nil {
		t.Fatalf("zoxide delete = status:%q cmd:%v", got.status, cmd)
	}
}

func TestRenameAgentEmptyInputClearsLabel(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Pre-populate an alias so the clear path has something to remove.
	if err := sessionmgr.SaveAgentLabel("pi", "seshagy", "frontend-bot"); err != nil {
		t.Fatalf("seed label: %v", err)
	}

	m := newTestModel(t)
	m.inputMode = modeRename
	m.renameKind = sessionmgr.KindAgent
	m.renameFrom = "pi"
	m.renameSession = "seshagy"
	m.renameInput.SetValue("")
	m.renameInput.Focus()

	model, cmd := m.handleKey(enterMsg())
	got := model.(Model)
	if got.inputMode != modeNormal || got.renameKind != "" || cmd == nil {
		t.Fatalf("enter = mode:%v renameKind:%q cmd:%v", got.inputMode, got.renameKind, cmd)
	}

	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("cmd msg = %T, want actionDoneMsg", msg)
	}
	if done.err != nil || !strings.Contains(done.status, "cleared agent alias") {
		t.Fatalf("action = status:%q err:%v", done.status, done.err)
	}

	store := sessionmgr.LoadAgentLabels()
	if _, ok := store.Labels["pi:seshagy"]; ok {
		t.Fatalf("label not cleared; store=%+v", store.Labels)
	}
}

func TestHandleKeySearchModeTypingAndQuit(t *testing.T) {
	m := newTestModel(t)
	m.items = testKeyItems("api", "app")
	m.inputMode = modeSearch
	m.searchInput.Focus()

	model, cmd := m.handleKey(keyMsg("a"))
	got := model.(Model)
	if got.query != "a" || cmd == nil {
		t.Fatalf("search typing = query:%q cmd:%v", got.query, cmd)
	}

	_, cmd = got.handleKey(keyMsg("ctrl+c"))
	if cmd == nil {
		t.Fatal("expected quit command from search mode")
	}
}

func TestRenameSessionFlowSetsAndUsesTarget(t *testing.T) {
	var renamedTarget, renamedNew string
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		if len(args) >= 4 && args[0] == "rename-session" {
			renamedTarget = strings.TrimPrefix(args[2], "=")
			renamedNew = args[3]
		}
		return nil
	})

	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "myproj", Target: "myproj"},
	}
	m.cursor = 0

	// startRename must populate renameTarget from the selected item's ActionTarget.
	model, _ := m.startRename()
	got := model.(Model)
	if got.renameTarget != "myproj" {
		t.Fatalf("startRename() renameTarget = %q, want myproj", got.renameTarget)
	}
	if got.inputMode != modeRename {
		t.Fatalf("startRename() inputMode = %v, want modeRename", got.inputMode)
	}

	// Enter the new name and commit; the target must reach the backend.
	got.renameInput.SetValue("newname")
	_, cmd := got.handleKey(enterMsg())
	if cmd == nil {
		t.Fatal("expected rename command from enter")
	}
	_ = cmd() // drive the tmux rename-session call

	if renamedTarget != "myproj" {
		t.Fatalf("rename-session target = %q, want myproj", renamedTarget)
	}
	if renamedNew != "newname" {
		t.Fatalf("rename-session newName = %q, want newname", renamedNew)
	}
}

// TestAgentsScopeStatusTmuxByteIdentical is the regression guard for the 'o'
// toggle agents-scope status under tmux terms.
// TestRenameAgentFlowThreadsPaneID proves the agent-rename path threads the
// pane id from startRename through the enter-commit into RenameAgent. Without
// this, herdr (which renames by pane id) gets an empty target and fails.
func TestRenameAgentFlowThreadsPaneID(t *testing.T) {
	var gotItem sessionmgr.Item
	var gotDisplay string
	fake := &captureRenameMux{
		onRename: func(it sessionmgr.Item, display string) error {
			gotItem = it
			gotDisplay = display
			return nil
		},
	}

	m := newTestModel(t)
	m.mux = fake
	m.items = []sessionmgr.Item{{
		Kind:      sessionmgr.KindAgent,
		Name:      "pi",
		AgentName: "pi",
		PaneID:    "w1:p3",
		Session:   "w1",
	}}
	m.cursor = 0

	model, _ := m.startRename()
	got := model.(Model)
	if got.renameTarget != "w1:p3" {
		t.Fatalf("startRename() renameTarget = %q, want w1:p3", got.renameTarget)
	}

	got.renameInput.SetValue("frontend")
	_, cmd := got.handleKey(enterMsg())
	if cmd != nil {
		_ = cmd() // drive RenameAgent on the fake mux
	}

	if gotItem.PaneID != "w1:p3" {
		t.Fatalf("RenameAgent PaneID = %q, want w1:p3", gotItem.PaneID)
	}
	if gotDisplay != "frontend" {
		t.Fatalf("RenameAgent display = %q, want frontend", gotDisplay)
	}
}

type captureRenameMux struct {
	onRename func(sessionmgr.Item, string) error
}

func (c *captureRenameMux) Kind() sessionmgr.BackendKind { return sessionmgr.BackendHerdr }

func (c *captureRenameMux) Terms() sessionmgr.Terms                          { return sessionmgr.HerdrTerms() }
func (c *captureRenameMux) InMultiplexer() bool                              { return true }
func (c *captureRenameMux) InMultiplexerPopup(context.Context) (bool, error) { return false, nil }
func (c *captureRenameMux) CurrentSession(context.Context) (string, error)   { return "", nil }
func (c *captureRenameMux) ListSessions(context.Context) ([]sessionmgr.Item, error) {
	return nil, nil
}

func (c *captureRenameMux) HasSession(context.Context, string) (bool, error) {
	return false, nil
}

func (c *captureRenameMux) CreateSessionFromDir(
	context.Context,
	string,
) (sessionmgr.Item, bool, error) {
	return sessionmgr.Item{}, false, nil
}
func (c *captureRenameMux) KillSession(context.Context, string) error           { return nil }
func (c *captureRenameMux) RenameSession(context.Context, string, string) error { return nil }
func (c *captureRenameMux) CaptureSession(context.Context, string, int) (string, error) {
	return "", nil
}
func (c *captureRenameMux) AttachOrSwitchCommand(sessionmgr.Item) *exec.Cmd { return nil }
func (c *captureRenameMux) ListAgents(context.Context, string) ([]sessionmgr.Item, error) {
	return nil, nil
}

func (c *captureRenameMux) CaptureAgentPane(context.Context, string, int) (string, error) {
	return "", nil
}
func (c *captureRenameMux) FocusAgentCommand(sessionmgr.Item) *exec.Cmd { return nil }

func (c *captureRenameMux) RenameAgent(
	_ context.Context,
	it sessionmgr.Item,
	display string,
) error {
	return c.onRename(it, display)
}

func (c *captureRenameMux) ResolvePane(_ context.Context, pane string) (string, error) {
	return pane, nil
}

func (c *captureRenameMux) ResolvePaneByCwd(context.Context, string) (string, error) {
	return "", nil
}

func (c *captureRenameMux) ReportAgent(context.Context, sessionmgr.AgentReport) (bool, error) {
	return false, nil
}

func (c *captureRenameMux) ReleaseAgent(context.Context, sessionmgr.AgentRelease) (bool, error) {
	return false, nil
}

func (c *captureRenameMux) MarkAgentVisited(context.Context, string) (bool, error) {
	return false, nil
}
func (c *captureRenameMux) MarkActiveDoneAgentsIdle(context.Context, []sessionmgr.Item) {}

func TestAgentsScopeStatusTmuxByteIdentical(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents

	// Not in a session → toggle to current-only → "agents: not in a tmux session"
	m.agentsCurrentOnly = false
	m.currentSession = ""
	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if m.status != "agents: not in a tmux session" {
		t.Fatalf("status = %q, want \"agents: not in a tmux session\"", m.status)
	}

	// Toggle to all → "agents: all sessions"
	model, _ = m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if m.status != "agents: all sessions" {
		t.Fatalf("status = %q, want \"agents: all sessions\"", m.status)
	}
}

// TestAgentsScopeStatusHerdrTerms verifies herdr vocabulary in agents scope.
func TestAgentsScopeStatusHerdrTerms(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.source = sessionmgr.ModeAgents

	m.agentsCurrentOnly = false
	m.currentSession = ""
	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if m.status != "agents: not in a herdr workspace" {
		t.Fatalf("status = %q, want \"agents: not in a herdr workspace\"", m.status)
	}

	model, _ = m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if m.status != "agents: all workspaces" {
		t.Fatalf("status = %q, want \"agents: all workspaces\"", m.status)
	}
}

// TestAgentsScopeStatusHerdrShowsWorkspaceLabel proves the 'o' status shows
// the resolved workspace label (from agent item Location), not the raw opaque
// workspace id like "wB".
func TestAgentsScopeStatusHerdrShowsWorkspaceLabel(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{{
		Kind:      sessionmgr.KindAgent,
		Name:      "pi",
		AgentName: "pi",
		Session:   "wB",       // opaque herdr workspace id
		Location:  "frontend", // resolved workspace label
	}}
	m.currentSession = "wB"
	m.agentsCurrentOnly = false

	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if m.status != "agents: frontend" {
		t.Fatalf("status = %q, want \"agents: frontend\" (label, not id)", m.status)
	}
	// Falls back to the raw id when no matching agent item is loaded.
	m2 := newTestModel(t)
	m2.terms = sessionmgr.HerdrTerms()
	m2.source = sessionmgr.ModeAgents
	m2.currentSession = "wB"
	m2.agentsCurrentOnly = false
	model2, _ := m2.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m2 = model2.(Model)
	if m2.status != "agents: wB" {
		t.Fatalf("fallback status = %q, want \"agents: wB\"", m2.status)
	}
}

func TestTabCyclesSourceSections(t *testing.T) {
	order := []sessionmgr.SourceMode{
		sessionmgr.ModeAll,
		sessionmgr.ModeSessions,
		sessionmgr.ModeZoxide,
		sessionmgr.ModeFD,
		sessionmgr.ModeAgents,
	}

	m := newTestModel(t)
	if got := m.config.SourceOrder(); len(got) != len(order) {
		t.Fatalf("SourceOrder len = %d, want %d", len(got), len(order))
	}
	m.source = sessionmgr.ModeAll
	m.cursor = 3 // non-zero so the cursor-reset assertion below is load-bearing

	// Tab walks the full order forward and wraps back to the first tab.
	for i := 1; i <= len(order); i++ {
		want := order[i%len(order)]
		model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyTab})
		m = model.(Model)
		if m.source != want {
			t.Fatalf("tab #%d: source = %v, want %v", i, m.source, want)
		}
		if m.cursor != 0 {
			t.Fatalf("tab #%d: cursor = %d, want 0 (reset on switch)", i, m.cursor)
		}
	}

	// Shift+Tab from the first tab wraps to the last tab.
	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = model.(Model)
	if m.source != order[len(order)-1] {
		t.Fatalf("shift+tab from first: source = %v, want %v", m.source, order[len(order)-1])
	}

	// Shift+Tab walks the order backward one more step.
	model, _ = m.handleActionKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = model.(Model)
	if m.source != order[len(order)-2] {
		t.Fatalf("shift+tab backward: source = %v, want %v", m.source, order[len(order)-2])
	}
}

func TestTabCyclesSectionsInTypeFirstMode(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = appconfig.DefaultPrefix
	m.source = sessionmgr.ModeAll

	// Tab is navigation, so it routes through handleActionKey even in type-first
	// mode without the prefix armed.
	model, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(Model)
	if m.source != sessionmgr.ModeSessions {
		t.Fatalf("type-first tab: source = %v, want %v", m.source, sessionmgr.ModeSessions)
	}
	if m.prefixArmed || m.query != "" {
		t.Fatalf(
			"type-first tab: prefixArmed=%v query=%q, want no filter side effects",
			m.prefixArmed,
			m.query,
		)
	}
}

// TestDeleteSelectedSetsKillInFlight verifies that pressing x on a session
// arms killInFlight so the ephemeral focus-loss poll is suppressed while
// KillSession (and its focus-restore) runs. Non-session deletes must not arm.
func TestDeleteSelectedSetsKillInFlight(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if !got.killInFlight {
		t.Fatal("killInFlight not set after session delete")
	}
	if cmd == nil {
		t.Fatal("expected deleteSessionCmd after session delete")
	}

	// Unsupported kind must not arm the flag.
	m2 := newTestModel(t)
	m2.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Name: "z"}}
	model2, _ := m2.deleteSelected()
	if model2.(Model).killInFlight {
		t.Fatal("killInFlight should not be set for non-session delete")
	}
}
