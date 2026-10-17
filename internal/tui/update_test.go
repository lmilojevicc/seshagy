package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestUpdateRefreshMsgMergesSourcesAndSchedulesAgentStartTimes(t *testing.T) {
	m := newTestModel(t)
	m.inflightRefresh[sessionmgr.ModeSessions] = 1
	m.refreshGen[sessionmgr.ModeSessions] = 1

	model, _ := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "dev", Target: "dev"},
		},
	})
	m = model.(Model)
	if !m.loadedOnce {
		t.Fatal("loadedOnce not set after the first result")
	}
	if len(m.items) != 1 || m.items[0].Name != "dev" {
		t.Fatalf("items = %#v", m.items)
	}
	if m.refreshInflight(sessionmgr.ModeSessions) {
		t.Fatal("refresh still marked in flight after its result arrived")
	}

	m.inflightRefresh[sessionmgr.ModeAgents] = 1
	m.refreshGen[sessionmgr.ModeAgents] = 1
	pane := agentItem("claude", "%1", "frontend")
	pane.AgentPID = 4242
	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeAgents,
		gen:    1,
		items:  []sessionmgr.Item{pane},
	})
	m = model.(Model)
	if len(m.items) != 2 {
		t.Fatalf("merged items = %#v, want sessions and agents", m.items)
	}
	if got := sessionmgr.StripANSI(m.promptCount()); got != "2" {
		t.Fatalf("prompt count = %q, want 2", got)
	}
	// The new agent pane has no cached process start time, so an agent refresh
	// queues the background resolution the age column renders from.
	if !m.agentStartPending || cmd == nil {
		t.Fatalf("agent refresh scheduled no start-time resolution (pending=%v)",
			m.agentStartPending)
	}
}

func TestSpinnerTickStopsWhenIdle(t *testing.T) {
	m := newTestModel(t)
	m.spinnerActive = true
	model, cmd := m.Update(spinnerTickMsg(time.Now()))
	m = model.(Model)
	if cmd != nil {
		t.Fatalf("idle spinner kept ticking: %v", cmd)
	}
	if m.spinnerActive {
		t.Fatal("spinner still active with nothing in flight")
	}
}

func TestNotificationTickDismissesAfterTTL(t *testing.T) {
	m := newTestModel(t)
	m.notify("hello", sevInfo)
	model, cmd := m.Update(notificationTickMsg(time.Now().Add(2 * notificationTTL)))
	m = model.(Model)
	if len(m.notifications) != 0 {
		t.Fatalf("notifications = %#v, want none", m.notifications)
	}
	if cmd == nil {
		t.Fatal("notification tick stopped re-arming")
	}
}

func TestUpdatePreviewMsgIgnoresStaleSelection(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	model, _ := m.Update(previewMsg{key: "session:other", preview: "stale"})
	m = model.(Model)
	if m.preview != "" {
		t.Fatalf("stale preview stored: %q", m.preview)
	}
	model, _ = m.Update(previewMsg{key: "session:demo", preview: "fresh"})
	m = model.(Model)
	if m.preview != "fresh" || m.previewKey != "session:demo" {
		t.Fatalf("preview = %q key = %q", m.preview, m.previewKey)
	}
	model, _ = m.Update(previewMsg{key: "session:demo", err: errors.New("capture failed")})
	m = model.(Model)
	if m.previewError != "capture failed" || m.preview != "" {
		t.Fatalf("preview error = %q preview = %q", m.previewError, m.preview)
	}
}

func TestUpdateCreateDoneMsgSchedulesAttach(t *testing.T) {
	m := newTestModel(t)
	model, cmd := m.Update(createDoneMsg{
		item:    sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		created: true,
	})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("create did not schedule an attach")
	}
	if got := latestNotificationText(m); got != "created session demo" {
		t.Fatalf("notification = %q", got)
	}

	errMsg := errors.New("boom")
	model, cmd = m.Update(createDoneMsg{err: errMsg})
	m = model.(Model)
	if cmd != nil || latestNotificationSeverity(m) != sevError {
		t.Fatalf("error create: cmd = %v notifications = %#v", cmd, m.notifications)
	}
}

func TestUpdateAttachDoneMsgRefreshesSources(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	model, cmd := m.Update(attachDoneMsg{})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("returning from the multiplexer did not reload")
	}
	if len(m.cache) != 0 {
		t.Fatalf("cache not invalidated: %#v", m.cache)
	}
	if got := latestNotificationText(m); got != "returned from tmux" {
		t.Fatalf("notification = %q", got)
	}
}

func TestActionDoneMsgClearsKillInFlightForKillsOnly(t *testing.T) {
	m := newTestModel(t)
	m.killInFlight = true
	model, _ := m.Update(actionDoneMsg{kind: actionKill, status: "killed session demo"})
	if model.(Model).killInFlight {
		t.Fatal("kill completion did not clear killInFlight")
	}

	m = newTestModel(t)
	m.killInFlight = true
	model, _ = m.Update(actionDoneMsg{kind: actionRename, status: "renamed session demo"})
	if !model.(Model).killInFlight {
		t.Fatal("rename completion cleared killInFlight")
	}
}

func TestActionDoneMsgErrorIsNotifiedAsError(t *testing.T) {
	m := newTestModel(t)
	model, cmd := m.Update(actionDoneMsg{kind: actionKill, err: errors.New("kill failed")})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("failed action did not reload")
	}
	if latestNotificationSeverity(m) != sevError || latestNotificationText(m) != "kill failed" {
		t.Fatalf("notifications = %#v", m.notifications)
	}
}

func TestUpdateYaziDoneMsgSchedulesCreateSession(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(yaziDoneMsg{path: "/tmp/picked"})
	if cmd == nil {
		t.Fatal("yazi result did not schedule session creation")
	}

	m = newTestModel(t)
	model, cmd := m.Update(yaziDoneMsg{path: ""})
	m = model.(Model)
	if cmd != nil {
		t.Fatalf("empty yazi path scheduled a command: %v", cmd)
	}
	if latestNotificationSeverity(m) != sevWarning {
		t.Fatalf("notifications = %#v, want a warning", m.notifications)
	}
}

func TestUpdateEphemeralTickSkipsQuitWhileKillInFlight(t *testing.T) {
	m := newTestModel(t)
	m.ephemeral = true
	m.killInFlight = true
	_, cmd := m.Update(ephemeralTickMsg(time.Now()))
	if isQuitCmd(cmd) {
		t.Fatal("focus-loss poll quit seshagy while a kill was in flight")
	}
	if cmd == nil {
		t.Fatal("focus-loss poll stopped re-arming")
	}
}

func TestCatalogRefreshMsgReloadsCaches(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	model, cmd := m.Update(catalogRefreshMsg{
		result: sessionmgr.ManifestFetchResult{Fetched: []string{"claude"}},
	})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("manifest update did not reload sources")
	}
	if len(m.cache) != 0 {
		t.Fatalf("cache not invalidated: %#v", m.cache)
	}
	if got := latestNotificationText(m); got != "agent rules updated (1)" {
		t.Fatalf("notification = %q", got)
	}

	m = newTestModel(t)
	model, cmd = m.Update(catalogRefreshMsg{err: errors.New("offline")})
	m = model.(Model)
	if cmd != nil || latestNotificationSeverity(m) != sevWarning {
		t.Fatalf("failed catalog refresh: cmd = %v notifications = %#v", cmd, m.notifications)
	}
}

func TestWindowSizeUpdateKeepsModelSized(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = model.(Model)
	if m.width != 140 || m.height != 40 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
}
