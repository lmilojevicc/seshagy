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
	assertRenameReset := func(t *testing.T, got Model) {
		t.Helper()
		if got.inputMode != modeNormal || got.renameFrom != "" || got.renameTarget != "" ||
			got.renameKind != "" || got.renameSession != "" || got.renameAgentType != "" {
			t.Fatalf(
				"rename state = mode:%v from:%q target:%q kind:%q session:%q agentType:%q",
				got.inputMode,
				got.renameFrom,
				got.renameTarget,
				got.renameKind,
				got.renameSession,
				got.renameAgentType,
			)
		}
	}

	m := newTestModel(t)
	m.inputMode = modeRename
	m.renameFrom = "frontend"
	m.renameTarget = "pane-1"
	m.renameKind = sessionmgr.KindAgent
	m.renameSession = "workspace-1"
	m.renameAgentType = "pi"
	m.renameInput.SetValue("frontend")
	m.renameInput.Focus()

	model, cmd := m.handleKey(keyMsg("esc"))
	got := model.(Model)
	if len(got.notifications) != 0 || cmd != nil {
		t.Fatalf("rename esc = notifications:%#v cmd:%v", got.notifications, cmd)
	}
	assertRenameReset(t, got)

	// Starting a session rename after cancel must not inherit agent metadata.
	got.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	model, _ = got.startRename()
	m = model.(Model)
	if m.renameSession != "" || m.renameAgentType != "" {
		t.Fatalf(
			"session rename inherited agent metadata: session:%q agentType:%q",
			m.renameSession,
			m.renameAgentType,
		)
	}

	m.renameInput.SetValue("   ")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if len(got.notifications) != 0 || cmd != nil {
		t.Fatalf("empty rename = notifications:%#v cmd:%v", got.notifications, cmd)
	}
	assertRenameReset(t, got)

	model, _ = got.startRename()
	m = model.(Model)
	m.renameInput.SetValue("renamed")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if cmd == nil {
		t.Fatal("rename submit returned nil command")
	}
	assertRenameReset(t, got)
}

func TestDeleteSelectedSession(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
	}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if !got.killInFlight || len(got.notifications) != 0 || cmd == nil {
		t.Fatalf(
			"session delete = inFlight:%v notifications:%#v cmd:%v",
			got.killInFlight,
			got.notifications,
			cmd,
		)
	}

	m.items = nil
	model, cmd = m.deleteSelected()
	got = model.(Model)
	if len(got.notifications) != 0 || cmd != nil {
		t.Fatalf("empty delete = notifications:%#v cmd:%v", got.notifications, cmd)
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
	if text := latestNotificationText(got); !strings.Contains(text, "focusing pi on seshagy:1.2") {
		t.Fatalf("notification = %q, want 'focusing pi on seshagy:1.2'", text)
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
	if text := latestNotificationText(got); !strings.Contains(text, "cannot focus") {
		t.Fatalf("notification = %q, want 'cannot focus'", text)
	}
}

func TestHandleActionKeyDeleteAndRename(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
	}

	model, cmd := m.handleActionKey(keyMsg("x"))
	got := model.(Model)
	if !got.killInFlight || len(got.notifications) != 0 || cmd == nil {
		t.Fatalf(
			"x = inFlight:%v notifications:%#v cmd:%v",
			got.killInFlight,
			got.notifications,
			cmd,
		)
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
	if got.query != "a" || len(got.notifications) != 0 || cmd == nil {
		t.Fatalf(
			"delete rune = query:%q notifications:%#v cmd:%v",
			got.query,
			got.notifications,
			cmd,
		)
	}

	model, cmd = got.clearFilterText()
	got = model.(Model)
	if got.query != "" || len(got.notifications) != 0 || cmd == nil {
		t.Fatalf(
			"clear filter = query:%q notifications:%#v cmd:%v",
			got.query,
			got.notifications,
			cmd,
		)
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
	if len(got.notifications) != 0 || cmd == nil {
		t.Fatalf(
			"refresh = notifications:%#v cmd:%v, want no toast and a command",
			got.notifications,
			cmd,
		)
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
	if len(got.notifications) != 0 || cmd == nil {
		t.Fatalf("session enter = notifications:%#v cmd:%v", got.notifications, cmd)
	}

	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}
	model, cmd = m.handleActionKey(enterMsg())
	got = model.(Model)
	if len(got.notifications) != 0 || cmd == nil {
		t.Fatalf("zoxide enter = notifications:%#v cmd:%v", got.notifications, cmd)
	}

	m.items = nil
	model, cmd = m.handleActionKey(enterMsg())
	got = model.(Model)
	if len(got.notifications) != 0 || cmd != nil {
		t.Fatalf("empty enter = notifications:%#v cmd:%v", got.notifications, cmd)
	}
}

func TestRemovedToastsAreGone(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*Model)
		action  func(Model) tea.Model
	}{
		{
			name: "type-first filter keystroke",
			prepare: func(m *Model) {
				m.config.TypeFirst.Enabled = true
				m.items = testKeyItems("api")
			},
			action: func(m Model) tea.Model {
				model, _ := m.handleKey(keyMsg("a"))
				return model
			},
		},
		{
			name:    "activate empty list",
			prepare: func(*Model) {},
			action: func(m Model) tea.Model {
				model, _ := m.handleActionKey(enterMsg())
				return model
			},
		},
		{
			name: "start rename",
			prepare: func(m *Model) {
				m.items = testKeyItems("demo")
			},
			action: func(m Model) tea.Model {
				model, _ := m.startRename()
				return model
			},
		},
		{
			name: "start attach",
			prepare: func(m *Model) {
				m.items = testKeyItems("demo")
			},
			action: func(m Model) tea.Model {
				model, _ := m.activateSelected()
				return model
			},
		},
		{
			name: "start kill",
			prepare: func(m *Model) {
				m.items = testKeyItems("demo")
			},
			action: func(m Model) tea.Model {
				model, _ := m.deleteSelected()
				return model
			},
		},
		{
			name: "start create",
			prepare: func(m *Model) {
				m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}
			},
			action: func(m Model) tea.Model {
				model, _ := m.activateSelected()
				return model
			},
		},
		{
			name:    "startup mode prompt",
			prepare: func(*Model) {},
			action: func(m Model) tea.Model {
				model, _ := m.Update(setupMsg{prompt: true})
				return model
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			tc.prepare(&m)
			m.notify("existing", sevWarning)
			before := len(m.notifications)

			got := tc.action(m).(Model)
			if len(got.notifications) != before || latestNotificationText(got) != "existing" {
				t.Fatalf(
					"notifications changed: before=%d after=%#v",
					before,
					got.notifications,
				)
			}
		})
	}
}

func TestHandleActionKeyModePrompt(t *testing.T) {
	m := newTestModel(t)
	model, cmd := m.handleActionKey(keyMsg("m"))
	got := model.(Model)
	if !got.setup.active || len(got.notifications) != 0 || cmd != nil {
		t.Fatalf(
			"mode prompt = active:%v notifications:%#v cmd:%v",
			got.setup.active,
			got.notifications,
			cmd,
		)
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
	if text := latestNotificationText(
		got,
	); !strings.Contains(
		text,
		"cannot open yazi inside a tmux popup",
	) ||
		latestNotificationSeverity(got) != sevError {
		t.Fatalf(
			"blocked startYazi = notification:%q severity:%v",
			text,
			latestNotificationSeverity(got),
		)
	}
}

func TestStartYaziOutsidePopup(t *testing.T) {
	m := newTestModel(t)
	m.checkPopup = func(context.Context) (bool, error) { return false, nil }

	model, cmd := m.startYazi()
	got := model.(Model)
	if text := latestNotificationText(got); cmd == nil || text != "opening yazi" {
		t.Fatalf("start yazi = notification:%q cmd:%v", text, cmd)
	}
}

func TestDeleteNonSessionItemWarns(t *testing.T) {
	for _, kind := range []sessionmgr.Kind{
		sessionmgr.KindAgent,
		sessionmgr.KindZoxide,
		sessionmgr.KindFD,
	} {
		t.Run(string(kind), func(t *testing.T) {
			m := newTestModel(t)
			m.items = []sessionmgr.Item{{Kind: kind, AgentName: "pi", Path: "/tmp/project"}}

			model, cmd := m.handleActionKey(keyMsg("x"))
			got := model.(Model)
			if text := latestNotificationText(
				got,
			); text != "delete only applies to sessions" ||
				cmd != nil {
				t.Fatalf("delete = notification:%q cmd:%v", text, cmd)
			}
			if sev := latestNotificationSeverity(got); sev != sevWarning {
				t.Fatalf("delete severity = %v, want sevWarning", sev)
			}
		})
	}
}

func TestRenameNonRenameableItemWarns(t *testing.T) {
	const want = "rename only applies to sessions and agents"
	for _, tt := range []struct {
		name string
		run  func(Model) (tea.Model, tea.Cmd)
	}{
		{name: "start", run: func(m Model) (tea.Model, tea.Cmd) {
			m.items = []sessionmgr.Item{{Kind: sessionmgr.KindFD, Path: "/tmp/project"}}
			return m.startRename()
		}},
		{name: "submit", run: func(m Model) (tea.Model, tea.Cmd) {
			m.inputMode = modeRename
			m.renameKind = sessionmgr.KindFD
			m.renameFrom = "old"
			m.renameInput.SetValue("new")
			return m.handleKey(enterMsg())
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model, cmd := tt.run(newTestModel(t))
			got := model.(Model)
			if text := latestNotificationText(got); text != want || cmd != nil {
				t.Fatalf("rename = notification:%q cmd:%v", text, cmd)
			}
			if sev := latestNotificationSeverity(got); sev != sevWarning {
				t.Fatalf("rename severity = %v, want sevWarning", sev)
			}
		})
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
	m.renameFrom = "frontend-bot"
	m.renameAgentType = "pi"
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
	if done.kind != actionAgentRename || done.err != nil ||
		!strings.Contains(done.status, "cleared agent alias") {
		t.Fatalf("action = kind:%q status:%q err:%v", done.kind, done.status, done.err)
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

func TestRenameAgentAliasToCanonicalType(t *testing.T) {
	var calls int
	var gotItem sessionmgr.Item
	var gotDisplay string
	fake := &captureRenameMux{
		onRename: func(it sessionmgr.Item, display string) error {
			calls++
			gotItem = it
			gotDisplay = display
			return nil
		},
	}

	m := newTestModel(t)
	m.mux = fake
	m.items = []sessionmgr.Item{{
		Kind:             sessionmgr.KindAgent,
		AgentName:        "pi",
		AgentDisplayName: "frontend",
		PaneID:           "w1:p3",
		Session:          "w1",
	}}

	model, _ := m.startRename()
	got := model.(Model)
	got.renameInput.SetValue("pi")
	_, cmd := got.handleKey(enterMsg())
	if cmd == nil {
		t.Fatal("expected rename command from enter")
	}
	_ = cmd()

	if calls != 1 {
		t.Fatalf("RenameAgent calls = %d, want 1", calls)
	}
	if gotItem.PaneID != "w1:p3" || gotItem.AgentName != "pi" {
		t.Fatalf("RenameAgent item = %+v, want pane w1:p3 and agent pi", gotItem)
	}
	if gotDisplay != "pi" {
		t.Fatalf("RenameAgent display = %q, want pi", gotDisplay)
	}
}

func TestRenameAgentUnchangedDisplayedAliasIsNoOp(t *testing.T) {
	calls := 0
	fake := &captureRenameMux{
		onRename: func(sessionmgr.Item, string) error {
			calls++
			return nil
		},
	}

	m := newTestModel(t)
	m.mux = fake
	m.items = []sessionmgr.Item{{
		Kind:             sessionmgr.KindAgent,
		AgentName:        "pi",
		AgentDisplayName: "frontend",
		PaneID:           "w1:p3",
		Session:          "w1",
	}}

	model, _ := m.startRename()
	got := model.(Model)
	_, cmd := got.handleKey(enterMsg())
	if cmd != nil {
		_ = cmd()
		t.Fatal("unchanged displayed alias returned a command")
	}
	if calls != 0 {
		t.Fatalf("RenameAgent calls = %d, want 0", calls)
	}
}

func TestRenameHerdrAgentWithoutCanonicalType(t *testing.T) {
	var calls int
	var gotItem sessionmgr.Item
	fake := &captureRenameMux{
		onRename: func(it sessionmgr.Item, _ string) error {
			calls++
			gotItem = it
			return nil
		},
	}

	m := newTestModel(t)
	m.mux = fake
	m.items = []sessionmgr.Item{{
		Kind:             sessionmgr.KindAgent,
		AgentDisplayName: "unclassified",
		PaneID:           "opaque-pane-id",
		Session:          "workspace-id",
	}}

	model, _ := m.startRename()
	got := model.(Model)
	got.renameInput.SetValue("frontend")
	_, cmd := got.handleKey(enterMsg())
	if cmd == nil {
		t.Fatal("expected herdr rename command without a canonical agent type")
	}
	_ = cmd()

	if calls != 1 {
		t.Fatalf("RenameAgent calls = %d, want 1", calls)
	}
	if gotItem.PaneID != "opaque-pane-id" || gotItem.AgentName != "" {
		t.Fatalf("RenameAgent item = %+v, want pane target and empty agent type", gotItem)
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

func TestAgentsScopeWarningTmuxByteIdentical(t *testing.T) {
	m := newTestModel(t)
	m.source = sessionmgr.ModeAgents
	m.agentsCurrentOnly = false
	m.currentSession = ""

	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if text := latestNotificationText(m); text != "agents: not in a tmux session" {
		t.Fatalf("notification = %q, want \"agents: not in a tmux session\"", text)
	}

	before := len(m.notifications)
	model, _ = m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if len(m.notifications) != before {
		t.Fatalf("all-sessions scope added notification: %#v", m.notifications)
	}
}

// TestAgentsScopeWarningHerdrTerms verifies herdr vocabulary in the retained warning.
func TestAgentsScopeWarningHerdrTerms(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.source = sessionmgr.ModeAgents
	m.agentsCurrentOnly = false
	m.currentSession = ""

	model, _ := m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if text := latestNotificationText(m); text != "agents: not in a herdr workspace" {
		t.Fatalf("notification = %q, want \"agents: not in a herdr workspace\"", text)
	}

	before := len(m.notifications)
	model, _ = m.handleActionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m = model.(Model)
	if len(m.notifications) != before {
		t.Fatalf("all-workspaces scope added notification: %#v", m.notifications)
	}
}

// TestCurrentSessionLabelHerdrShowsWorkspaceLabel proves the persistent agents
// scope title resolves a workspace label instead of showing an opaque id.
func TestCurrentSessionLabelHerdrShowsWorkspaceLabel(t *testing.T) {
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

	if label := m.currentSessionLabel(); label != "frontend" {
		t.Fatalf("currentSessionLabel = %q, want \"frontend\"", label)
	}

	m.items = nil
	if label := m.currentSessionLabel(); label != "wB" {
		t.Fatalf("fallback currentSessionLabel = %q, want \"wB\"", label)
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

func TestDeleteSelectedDoesNotStartOverlappingKill(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	m.killInFlight = true

	model, cmd := m.deleteSelected()
	if !model.(Model).killInFlight {
		t.Fatal("overlapping delete cleared killInFlight")
	}
	if cmd != nil {
		t.Fatal("overlapping delete started another kill command")
	}
}
