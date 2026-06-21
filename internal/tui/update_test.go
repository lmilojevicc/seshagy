package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestUpdateRefreshMsgForCurrentSource(t *testing.T) {
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
	if got.status != "loaded 1 item" {
		t.Fatalf("status = %q, want loaded message", got.status)
	}
	if cmd == nil {
		t.Fatal("expected preview command after refresh")
	}
}

func TestUpdateAttachDoneMsgRefreshesAndSetsStatus(t *testing.T) {
	m := New()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: testUpdateItems("cached"), fetchedAt: time.Now()},
	}

	model, cmd := m.Update(attachDoneMsg{})
	got := model.(Model)
	if got.status != "returned from tmux" {
		t.Fatalf("status = %q, want returned from tmux", got.status)
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
	if !errors.Is(got.err, loadErr) {
		t.Fatalf("err = %v, want %v", got.err, loadErr)
	}
	if got.status != "attach failed" {
		t.Fatalf("status = %q", got.status)
	}
	if cmd == nil {
		t.Fatal("expected refresh command after attach error")
	}
}

func TestUpdateCreateDoneMsgSuccessSchedulesAttach(t *testing.T) {
	m := New()

	model, cmd := m.Update(createDoneMsg{name: "demo", created: true})
	got := model.(Model)
	if got.status != "created session demo" {
		t.Fatalf("status = %q", got.status)
	}
	if cmd == nil {
		t.Fatal("expected attach command after create")
	}

	model, cmd = m.Update(createDoneMsg{name: "existing", created: false})
	got = model.(Model)
	if got.status != "using session existing" {
		t.Fatalf("status = %q", got.status)
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
	if !errors.Is(got.err, createErr) {
		t.Fatalf("err = %v, want %v", got.err, createErr)
	}
	if got.status != "create failed" {
		t.Fatalf("status = %q", got.status)
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

	m := New()
	model, cmd := m.Update(yaziDoneMsg{path: dir})
	got := model.(Model)
	if got.err != nil || got.status != "" {
		t.Fatalf("yazi success = status:%q err:%v", got.status, got.err)
	}
	if cmd == nil {
		t.Fatal("expected createSession command after yazi path")
	}
	createMsg := cmd().(createDoneMsg)
	if createMsg.err != nil {
		t.Fatalf("createSessionCmd() err = %v", createMsg.err)
	}
}

func TestUpdateDeleteDoneMsgRecordsKillError(t *testing.T) {
	m := New()
	killErr := errors.New("kill failed")

	model, cmd := m.Update(actionDoneMsg{status: "killed session demo", err: killErr})
	got := model.(Model)
	if !errors.Is(got.err, killErr) {
		t.Fatalf("err = %v, want %v", got.err, killErr)
	}
	if got.status != "kill failed" {
		t.Fatalf("status = %q, want kill failed", got.status)
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
	if !errors.Is(got.err, yaziErr) || got.status != "yazi failed" || cmd != nil {
		t.Fatalf("yazi error = status:%q err:%v cmd:%v", got.status, got.err, cmd)
	}

	model, cmd = m.Update(yaziDoneMsg{})
	got = model.(Model)
	if got.status != "yazi closed without a directory" || cmd != nil {
		t.Fatalf("empty yazi path = status:%q cmd:%v", got.status, cmd)
	}
}

func TestUpdateActionDoneMsgRecordsRenameError(t *testing.T) {
	m := New()
	renameErr := errors.New("rename failed")

	model, cmd := m.Update(actionDoneMsg{status: "renamed old to new", err: renameErr})
	got := model.(Model)
	if !errors.Is(got.err, renameErr) {
		t.Fatalf("err = %v, want %v", got.err, renameErr)
	}
	if got.status != "rename failed" {
		t.Fatalf("status = %q", got.status)
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

	model, cmd := m.Update(actionDoneMsg{status: "killed session demo"})
	got := model.(Model)
	if got.status != "killed session demo" {
		t.Fatalf("status = %q", got.status)
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
