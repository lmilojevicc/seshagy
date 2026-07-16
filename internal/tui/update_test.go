package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestUpdateRefreshMsgForCurrentSource(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := New()
	m.source = sessionmgr.ModeSessions
	m.loading = true
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}

	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		items:  testUpdateItems("alpha"),
	})
	got := model.(Model)
	if got.loading {
		t.Fatal("loading still true after refresh")
	}
	if len(got.items) != 1 || got.items[0].Name != "alpha" {
		t.Fatalf("items = %#v", got.items)
	}
	if len(got.notifications) != 0 {
		t.Fatalf("notifications = %#v, want none for successful refresh", got.notifications)
	}
	if cmd == nil {
		t.Fatal("expected preview command after refresh")
	}
}

func TestSpinnerTickStopsWhenIdle(t *testing.T) {
	m := New()
	m.loading = false
	m.spinnerActive = true
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}

	model, cmd := m.Update(spinnerTickMsg{})
	got := model.(Model)
	if cmd != nil {
		t.Fatalf("idle spinner tick rescheduled command: %v", cmd)
	}
	if got.spinnerActive {
		t.Fatal("spinnerActive remained true after idle tick")
	}
	if got.spinnerFrame != m.spinnerFrame {
		t.Fatalf("idle spinner frame advanced from %d to %d", m.spinnerFrame, got.spinnerFrame)
	}
}

func TestNotificationTickDismissesAfterTTL(t *testing.T) {
	now := time.Now()
	m := Model{notifications: []notification{
		{text: "expired", sev: sevInfo, at: now.Add(-notificationTTL - time.Millisecond)},
		{text: "fresh", sev: sevInfo, at: now},
	}}

	model, cmd := m.Update(notificationTickMsg(now))
	got := model.(Model)
	if len(got.notifications) != 1 || got.notifications[0].text != "fresh" {
		t.Fatalf("notifications = %#v, want only fresh", got.notifications)
	}
	if cmd == nil {
		t.Fatal("notification tick did not reschedule")
	}
}

func TestUpdateAttachDoneMsgRefreshesAndSetsStatus(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	t.Setenv("HERDR_ENV", "")
	m := New()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: testUpdateItems("cached"), fetchedAt: time.Now()},
	}

	model, cmd := m.Update(attachDoneMsg{})
	got := model.(Model)
	if text := latestNotificationText(got); text != "returned from tmux" {
		t.Fatalf("notification = %q, want returned from tmux", text)
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after attach")
	}
}

func TestUpdateAttachDoneMsgRecordsError(t *testing.T) {
	m := New()
	loadErr := errors.New("attach failed")

	model, cmd := m.Update(attachDoneMsg{err: loadErr})
	got := model.(Model)
	if sev := latestNotificationSeverity(got); sev != sevError {
		t.Fatalf("severity = %v, want sevError", sev)
	}
	if text := latestNotificationText(got); text != "attach failed" {
		t.Fatalf("notification = %q", text)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after attach error")
	}
}

func TestUpdateCreateDoneMsgSuccessSchedulesAttach(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	t.Setenv("HERDR_ENV", "")
	m := New()

	model, cmd := m.Update(
		createDoneMsg{
			item:    sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
			created: true,
		},
	)
	got := model.(Model)
	if text := latestNotificationText(got); text != "created session demo" {
		t.Fatalf("notification = %q", text)
	}
	if cmd == nil {
		t.Fatal("expected attach command after create")
	}

	model, cmd = m.Update(
		createDoneMsg{
			item: sessionmgr.Item{
				Kind:   sessionmgr.KindSession,
				Name:   "existing",
				Target: "existing",
			},
			created: false,
		},
	)
	got = model.(Model)
	if text := latestNotificationText(got); text != "using session existing" {
		t.Fatalf("notification = %q", text)
	}
	if cmd == nil {
		t.Fatal("expected attach command after reuse")
	}
}

func TestUpdateCreateDoneMsgRecordsError(t *testing.T) {
	m := New()
	createErr := errors.New("create failed")

	model, cmd := m.Update(createDoneMsg{err: createErr})
	got := model.(Model)
	if sev := latestNotificationSeverity(got); sev != sevError {
		t.Fatalf("severity = %v, want sevError", sev)
	}
	if text := latestNotificationText(got); text != "create failed" {
		t.Fatalf("notification = %q", text)
	}
	if cmd != nil {
		t.Fatal("expected no command after create error")
	}
}

func TestUpdatePreviewMsgIgnoresStaleSelection(t *testing.T) {
	m := New()
	m.items = testUpdateItems("alpha")
	m.cursor = 0

	model, cmd := m.Update(previewMsg{key: "session:other", preview: "stale"})
	got := model.(Model)
	if got.preview != "" || cmd != nil {
		t.Fatalf("stale preview should be ignored: preview=%q cmd=%v", got.preview, cmd)
	}
}

func TestUpdatePreviewMsgErrorRendersDanger(t *testing.T) {
	m := New()
	m.items = testUpdateItems("alpha")
	m.cursor = 0
	previewErr := errors.New("preview failed")

	model, cmd := m.Update(previewMsg{
		key: "session:alpha",
		err: previewErr,
	})
	got := model.(Model)
	if !strings.Contains(got.preview, "preview failed") {
		t.Fatalf("preview = %q, want danger-rendered error", got.preview)
	}
	if got.previewKey != "session:alpha" || cmd != nil {
		t.Fatalf("previewKey = %q cmd = %v", got.previewKey, cmd)
	}
}

func TestUpdateYaziDoneMsgSchedulesCreateSession(t *testing.T) {
	dir := t.TempDir()
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return nil, nil
		}
		return nil, nil
	}, func(_ context.Context, args ...string) error {
		if sessionmgr.MatchNewSession(args) {
			return nil
		}
		return nil
	})

	m := newTestModel(t)
	model, cmd := m.Update(yaziDoneMsg{path: dir})
	got := model.(Model)
	if text := latestNotificationText(got); text != "" {
		t.Fatalf("yazi success notification = %q", text)
	}
	if cmd == nil {
		t.Fatal("expected createSession command after yazi path")
	}
	createMsg := cmd().(createDoneMsg)
	if createMsg.err != nil {
		t.Fatalf("createSessionCmd() err = %v", createMsg.err)
	}
}

func TestErrorNotifiedAsError(t *testing.T) {
	m := New()
	killErr := errors.New("kill failed")

	model, cmd := m.Update(actionDoneMsg{
		kind:   actionKill,
		status: "killed session demo",
		err:    killErr,
	})
	got := model.(Model)
	if sev := latestNotificationSeverity(got); sev != sevError {
		t.Fatalf("severity = %v, want sevError", sev)
	}
	if text := latestNotificationText(got); text != "kill failed" {
		t.Fatalf("notification = %q, want kill failed", text)
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after delete error")
	}
}

func TestUpdateYaziDoneMsgErrorAndEmptyPath(t *testing.T) {
	m := New()
	yaziErr := errors.New("yazi failed")

	model, cmd := m.Update(yaziDoneMsg{err: yaziErr})
	got := model.(Model)
	if text := latestNotificationText(got); text != "yazi failed" || cmd != nil {
		t.Fatalf("yazi error notification = %q cmd:%v", text, cmd)
	}

	model, cmd = m.Update(yaziDoneMsg{})
	got = model.(Model)
	if text := latestNotificationText(
		got,
	); text != "yazi closed without a directory" ||
		cmd != nil {
		t.Fatalf("empty yazi path notification = %q cmd:%v", text, cmd)
	}
}

func TestUpdateActionDoneMsgRecordsRenameError(t *testing.T) {
	m := New()
	renameErr := errors.New("rename failed")

	model, cmd := m.Update(actionDoneMsg{
		kind:   actionRename,
		status: "renamed old to new",
		err:    renameErr,
	})
	got := model.(Model)
	if sev := latestNotificationSeverity(got); sev != sevError {
		t.Fatalf("severity = %v, want sevError", sev)
	}
	if text := latestNotificationText(got); text != "rename failed" {
		t.Fatalf("notification = %q", text)
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after rename error")
	}
}

func TestUpdateDeleteDoneMsgSetsStatusAndRefreshes(t *testing.T) {
	m := New()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: testUpdateItems("cached"), fetchedAt: time.Now()},
	}

	model, cmd := m.Update(actionDoneMsg{kind: actionKill, status: "killed session demo"})
	got := model.(Model)
	if text := latestNotificationText(got); text != "killed session demo" {
		t.Fatalf("notification = %q", text)
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after delete")
	}
}

func testUpdateItems(names ...string) []sessionmgr.Item {
	items := make([]sessionmgr.Item, len(names))
	for i, name := range names {
		items[i] = sessionmgr.Item{Kind: sessionmgr.KindSession, Name: name}
	}
	return items
}

// TestActionDoneMsgClearsKillInFlight verifies that completing a kill (success
// or error) clears the in-flight flag so the ephemeral focus-loss poll resumes.
func TestActionDoneMsgClearsKillInFlight(t *testing.T) {
	m := New()
	m.killInFlight = true
	model, _ := m.Update(actionDoneMsg{kind: actionKill, status: "killed session demo"})
	if model.(Model).killInFlight {
		t.Fatal("killInFlight not cleared on success")
	}

	m2 := New()
	m2.killInFlight = true
	model2, _ := m2.Update(actionDoneMsg{
		kind: actionKill,
		err:  errors.New("boom"),
	})
	if model2.(Model).killInFlight {
		t.Fatal("killInFlight not cleared on error")
	}
}

func TestActionDoneMsgRenameKindsDoNotClearKillInFlight(t *testing.T) {
	for _, kind := range []actionKind{actionRename, actionAgentRename} {
		for _, err := range []error{nil, errors.New("boom")} {
			name := string(kind) + "/success"
			if err != nil {
				name = string(kind) + "/error"
			}
			t.Run(name, func(t *testing.T) {
				m := New()
				m.killInFlight = true

				model, _ := m.Update(actionDoneMsg{
					kind:   kind,
					status: "rename complete",
					err:    err,
				})
				if !model.(Model).killInFlight {
					t.Fatalf("%s completion cleared killInFlight", kind)
				}
			})
		}
	}

	// Positive control: only a completed kill owns and clears the flag.
	m := New()
	m.killInFlight = true
	model, _ := m.Update(actionDoneMsg{kind: actionKill, status: "killed session demo"})
	if model.(Model).killInFlight {
		t.Fatal("kill completion did not clear killInFlight")
	}
}

// TestEphemeralTickSkipsQuitWhileKillInFlight verifies that while a kill is in
// flight, the ephemeral focus-loss poll does NOT quit seshagy (the close's
// refocus would otherwise dismiss the dashboard before the focus-restore
// lands). When no kill is in flight, focus-loss still quits as before.
func TestEphemeralTickSkipsQuitWhileKillInFlight(t *testing.T) {
	m := New()
	m.mux = sessionmgr.NewHerdrBackend()
	m.ephemeral = true
	m.herdrPaneID = "pSelf"
	m.herdrWorkspaceID = "wSelf"

	// In flight: must re-schedule the tick, not quit.
	m.killInFlight = true
	_, cmd := m.Update(ephemeralTickMsg{})
	if cmd == nil {
		t.Fatal("expected re-tick cmd while killInFlight")
	}
	if _, ok := cmd().(tea.QuitMsg); ok {
		t.Fatal("ephemeral tick quit while killInFlight")
	}

	// Not in flight: the pane query fails (no such pane) → focus lost → quit.
	m.killInFlight = false
	_, cmd = m.Update(ephemeralTickMsg{})
	if cmd == nil {
		t.Fatal("expected a cmd when not in flight")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.Quit on focus-loss when not in flight; got %T", cmd())
	}
}
