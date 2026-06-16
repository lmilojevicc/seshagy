package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
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
	m.renameInput.SetValue("renamed")
	model, cmd = m.handleKey(enterMsg())
	got = model.(Model)
	if got.inputMode != modeNormal || got.renameFrom != "" || cmd == nil {
		t.Fatalf("rename submit = mode:%v renameFrom:%q cmd:%v", got.inputMode, got.renameFrom, cmd)
	}
}

func TestDeleteSelectedSessionAndAgent(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo"},
	}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if got.status != "killing session demo" || cmd == nil {
		t.Fatalf("session delete = status:%q cmd:%v", got.status, cmd)
	}

	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, PaneID: "%3"},
	}
	model, cmd = m.deleteSelected()
	got = model.(Model)
	if got.status != "killing pane %3" || cmd == nil {
		t.Fatalf("agent delete = status:%q cmd:%v", got.status, cmd)
	}

	m.items = nil
	model, cmd = m.deleteSelected()
	got = model.(Model)
	if got.status != "nothing selected" || cmd != nil {
		t.Fatalf("empty delete = status:%q cmd:%v", got.status, cmd)
	}
}

func TestStartRenameNonSessionRejected(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, PaneID: "%1", AgentName: "pi"},
	}
	model, cmd := m.startRename()
	got := model.(Model)
	if got.status != "rename only applies to sessions" || cmd != nil {
		t.Fatalf("agent rename = status:%q cmd:%v", got.status, cmd)
	}

	m.items = nil
	model, cmd = m.startRename()
	got = model.(Model)
	if got.status != "rename only applies to sessions" || cmd != nil {
		t.Fatalf("empty rename = status:%q cmd:%v", got.status, cmd)
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
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAgents: {items: testKeyItems("agent"), fetchedAt: time.Now()},
	}

	cases := []struct {
		key    string
		source sessionmgr.SourceMode
	}{
		{"a", sessionmgr.ModeAll},
		{"t", sessionmgr.ModeSessions},
		{"g", sessionmgr.ModeAgents},
		{"o", sessionmgr.ModeCurrentAgents},
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

	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindAgent, PaneID: "%2", Location: "demo:1.1"}}
	model, cmd = m.handleActionKey(enterMsg())
	got = model.(Model)
	if got.status != "focusing demo:1.1" || cmd == nil {
		t.Fatalf("agent enter = status:%q cmd:%v", got.status, cmd)
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

func TestHandleActionKeyIntegrationAndModePrompt(t *testing.T) {
	m := newTestModel(t)
	model, cmd := m.handleActionKey(keyMsg("i"))
	got := model.(Model)
	if !got.integration.active || got.status != "scanning hook integrations" || cmd == nil {
		t.Fatalf("integration scan = active:%v status:%q cmd:%v",
			got.integration.active, got.status, cmd)
	}

	model, cmd = m.handleActionKey(keyMsg("m"))
	got = model.(Model)
	if !got.setup.active || got.status != "change input mode" || cmd != nil {
		t.Fatalf("mode prompt = active:%v status:%q cmd:%v", got.setup.active, got.status, cmd)
	}
}

func TestStartYaziBlockedInsidePopup(t *testing.T) {
	m := newTestModel(t)
	old := checkTmuxPopup
	checkTmuxPopup = func(context.Context) (bool, error) { return true, nil }
	t.Cleanup(func() { checkTmuxPopup = old })

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
	old := checkTmuxPopup
	checkTmuxPopup = func(context.Context) (bool, error) { return false, nil }
	t.Cleanup(func() { checkTmuxPopup = old })

	model, cmd := m.startYazi()
	got := model.(Model)
	if cmd == nil || got.status != "opening yazi" {
		t.Fatalf("start yazi = status:%q cmd:%v", got.status, cmd)
	}
}

func TestHandleIntegrationKeyToggleEnterRescan(t *testing.T) {
	m := newTestModel(t)
	m.integration.active = true
	m.integration.rows = []integrations.Recommendation{
		{
			Target:         integrations.TargetPi,
			AgentAvailable: true,
			Installable:    true,
			State:          integrations.StatusNotInstalled,
		},
	}
	m.integration.selected = map[integrations.Target]bool{integrations.TargetPi: true}

	model, cmd := m.handleIntegrationKey(keyMsg(" "))
	got := model.(Model)
	if got.integration.selected[integrations.TargetPi] || cmd != nil {
		t.Fatalf(
			"space toggle = selected:%v cmd:%v",
			got.integration.selected[integrations.TargetPi],
			cmd,
		)
	}

	model, _ = got.handleIntegrationKey(keyMsg(" "))
	got = model.(Model)
	if !got.integration.selected[integrations.TargetPi] {
		t.Fatal("space should reselect integration")
	}

	model, cmd = got.handleIntegrationKey(enterMsg())
	got = model.(Model)
	if got.status != "installing selected hook integrations" || cmd == nil {
		t.Fatalf("enter install = status:%q cmd:%v", got.status, cmd)
	}

	model, cmd = got.handleIntegrationKey(keyMsg("r"))
	got = model.(Model)
	if got.status != "rescanning hook integrations" || cmd == nil {
		t.Fatalf("rescan = status:%q cmd:%v", got.status, cmd)
	}

	model, cmd = got.handleIntegrationKey(keyMsg("esc"))
	got = model.(Model)
	if got.integration.active || got.status != "hook installation skipped" || cmd != nil {
		t.Fatalf("esc skip = active:%v status:%q cmd:%v", got.integration.active, got.status, cmd)
	}
}

func TestDeleteSelectedUnsupportedKind(t *testing.T) {
	m := newTestModel(t)
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"}}
	model, cmd := m.deleteSelected()
	got := model.(Model)
	if got.status != "delete only applies to sessions and agents" || cmd != nil {
		t.Fatalf("zoxide delete = status:%q cmd:%v", got.status, cmd)
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
