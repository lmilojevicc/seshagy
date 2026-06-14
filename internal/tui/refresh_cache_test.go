package tui

import (
	"errors"
	"testing"
	"time"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func testItems(names ...string) []sessionmgr.Item {
	items := make([]sessionmgr.Item, len(names))
	for i, name := range names {
		items[i] = sessionmgr.Item{Kind: sessionmgr.KindSession, Name: name}
	}
	return items
}

func TestSwitchSourceUsesFreshCacheWithoutBlockingLoad(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	m.items = testItems("stale-session")
	m.loading = false
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAgents: {
			items:     testItems("cached-agent"),
			fetchedAt: time.Now(),
		},
	}

	model, cmd := m.switchSource(sessionmgr.ModeAgents)
	got := model.(Model)
	if got.loading {
		t.Fatal("loading = true, want false for fresh cache hit")
	}
	if len(got.items) != 1 || got.items[0].Name != "cached-agent" {
		t.Fatalf("items = %#v, want cached agent", got.items)
	}
	if cmd == nil {
		t.Fatal("expected background refresh command")
	}
}

func TestSwitchSourceClearsItemsWhenCacheMissing(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	m.items = testItems("session-a")
	m.loading = false

	model, _ := m.switchSource(sessionmgr.ModeAgents)
	got := model.(Model)
	if !got.loading {
		t.Fatalf("loading = %v, want true", got.loading)
	}
	if len(got.items) != 0 {
		t.Fatalf("items = %#v, want empty while loading uncached mode", got.items)
	}
}

func TestRefreshMsgDropsStaleGeneration(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	m.items = testItems("keep-me")
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 2,
	}

	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		items:  testItems("stale"),
	})
	got := model.(Model)
	if len(got.items) != 1 || got.items[0].Name != "keep-me" {
		t.Fatalf("stale refresh updated items = %#v", got.items)
	}
	if cmd != nil {
		t.Fatal("expected no follow-up command for dropped refresh")
	}
	if _, ok := got.cache[sessionmgr.ModeSessions]; ok {
		t.Fatal("stale refresh should not update cache")
	}
}

func TestRefreshMsgUpdatesCacheForBackgroundMode(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	m.items = testItems("session-a")
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeAgents: 1,
	}

	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeAgents,
		gen:    1,
		items:  testItems("agent-a"),
	})
	got := model.(Model)
	if cmd != nil {
		t.Fatal("background refresh for other mode should not trigger preview")
	}
	if len(got.items) != 1 || got.items[0].Name != "session-a" {
		t.Fatalf("visible items changed = %#v", got.items)
	}
	entry, ok := got.cache[sessionmgr.ModeAgents]
	if !ok || len(entry.items) != 1 || entry.items[0].Name != "agent-a" {
		t.Fatalf("agents cache = %#v, ok=%v", entry, ok)
	}
}

func TestTickSkipsRefreshWhenCacheFresh(t *testing.T) {
	m := New()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		m.source: {
			items:     testItems("fresh"),
			fetchedAt: time.Now(),
		},
	}
	before := m.inflightRefresh[m.source]

	model, cmd := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if cmd == nil {
		t.Fatal("expected tick reschedule command")
	}
	if got.inflightRefresh[got.source] != before {
		t.Fatal("tick started refresh despite fresh cache")
	}
}

func TestTickSkipsRefreshWhenOverlayActive(t *testing.T) {
	m := New()
	m.integration.active = true
	beforeGen := m.refreshGen[m.source]

	model, cmd := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if cmd == nil {
		t.Fatal("expected tick reschedule command")
	}
	if got.refreshGen[got.source] != beforeGen {
		t.Fatal("tick started refresh during integration overlay")
	}
}

func TestInvalidateCachesOnAttachDone(t *testing.T) {
	m := New()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: testItems("cached"), fetchedAt: time.Now()},
	}

	model, cmd := m.Update(attachDoneMsg{})
	got := model.(Model)
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}
	if cmd == nil {
		t.Fatal("expected refresh after attach")
	}
}

func TestBeginRefreshCoalescesInflight(t *testing.T) {
	m := New()
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeAgents: 1,
	}

	model, cmd := m.beginRefresh(sessionmgr.ModeAgents, false)
	if cmd != nil {
		t.Fatal("expected nil cmd for inflight coalesce")
	}
	if model.inflightRefresh[sessionmgr.ModeAgents] != 1 {
		t.Fatalf("inflight gen = %d, want 1", model.inflightRefresh[sessionmgr.ModeAgents])
	}
}

func TestBackgroundRefreshingFooterState(t *testing.T) {
	m := New()
	m.loading = false
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		m.source: 1,
	}
	if !m.backgroundRefreshing() {
		t.Fatal("backgroundRefreshing = false, want true")
	}
}

func TestInvalidateAllCachesDropsInflightRefresh(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	m.refreshGen = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeAgents: 1,
	}
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeAgents: 1,
	}
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAgents: {items: testItems("old-agent"), fetchedAt: time.Now()},
	}

	got := m.invalidateAllCaches()
	if got.inflightRefresh[sessionmgr.ModeAgents] != 0 {
		t.Fatalf(
			"inflight gen = %d, want 0 after invalidation",
			got.inflightRefresh[sessionmgr.ModeAgents],
		)
	}
	if got.refreshGen[sessionmgr.ModeAgents] != 2 {
		t.Fatalf("refreshGen = %d, want 2 after bump", got.refreshGen[sessionmgr.ModeAgents])
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}

	model, cmd := got.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeAgents,
		gen:    1,
		items:  testItems("stale-agent"),
	})
	if cmd != nil {
		t.Fatal("expected no follow-up command for dropped refresh")
	}
	if _, ok := model.cache[sessionmgr.ModeAgents]; ok {
		t.Fatal("stale refresh repopulated cache after invalidation")
	}
}

func TestHandleRefreshMsgStoresErrorInCache(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeAgents
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeAgents: 1,
	}
	loadErr := errors.New("load failed")

	got, _ := m.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeAgents,
		gen:    1,
		err:    loadErr,
	})
	entry, ok := got.cache[sessionmgr.ModeAgents]
	if !ok || !errors.Is(entry.err, loadErr) {
		t.Fatalf("cache err = %v, ok=%v", entry.err, ok)
	}
	if got.err == nil || got.err.Error() != "load failed" {
		t.Fatalf("model err = %v", got.err)
	}
}
