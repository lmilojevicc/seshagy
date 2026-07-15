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
		sessionmgr.ModeZoxide: {
			items:     testItems("cached-zoxide"),
			fetchedAt: time.Now(),
		},
	}

	model, cmd := m.switchSource(sessionmgr.ModeZoxide)
	got := model.(Model)
	if got.loading {
		t.Fatal("loading = true, want false for fresh cache hit")
	}
	if len(got.items) != 1 || got.items[0].Name != "cached-zoxide" {
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

	model, _ := m.switchSource(sessionmgr.ModeZoxide)
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
		sessionmgr.ModeZoxide: 1,
	}

	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeZoxide,
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
	entry, ok := got.cache[sessionmgr.ModeZoxide]
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
	m.setup.active = true
	beforeGen := m.refreshGen[m.source]

	model, cmd := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if cmd == nil {
		t.Fatal("expected tick reschedule command")
	}
	if got.refreshGen[got.source] != beforeGen {
		t.Fatal("tick started refresh during setup overlay")
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
		sessionmgr.ModeZoxide: 1,
	}

	model, cmd := m.beginRefresh(sessionmgr.ModeZoxide, false)
	if cmd != nil {
		t.Fatal("expected nil cmd for inflight coalesce")
	}
	if model.inflightRefresh[sessionmgr.ModeZoxide] != 1 {
		t.Fatalf("inflight gen = %d, want 1", model.inflightRefresh[sessionmgr.ModeZoxide])
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
		sessionmgr.ModeZoxide: 1,
	}
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeZoxide: 1,
	}
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeZoxide: {items: testItems("old-agent"), fetchedAt: time.Now()},
	}

	got := m.invalidateAllCaches()
	if got.inflightRefresh[sessionmgr.ModeZoxide] != 0 {
		t.Fatalf(
			"inflight gen = %d, want 0 after invalidation",
			got.inflightRefresh[sessionmgr.ModeZoxide],
		)
	}
	if got.refreshGen[sessionmgr.ModeZoxide] != 2 {
		t.Fatalf("refreshGen = %d, want 2 after bump", got.refreshGen[sessionmgr.ModeZoxide])
	}
	if len(got.cache) != 0 {
		t.Fatalf("cache not cleared = %#v", got.cache)
	}

	model, cmd := got.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeZoxide,
		gen:    1,
		items:  testItems("stale-agent"),
	})
	if cmd != nil {
		t.Fatal("expected no follow-up command for dropped refresh")
	}
	if _, ok := model.cache[sessionmgr.ModeZoxide]; ok {
		t.Fatal("stale refresh repopulated cache after invalidation")
	}
}

func TestHandleRefreshMsgStoresErrorInCache(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeZoxide
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeZoxide: 1,
	}
	loadErr := errors.New("load failed")

	got, _ := m.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeZoxide,
		gen:    1,
		err:    loadErr,
	})
	entry, ok := got.cache[sessionmgr.ModeZoxide]
	if !ok || !errors.Is(entry.err, loadErr) {
		t.Fatalf("cache err = %v, ok=%v", entry.err, ok)
	}
	if text := latestNotificationText(got); text != "load failed" {
		t.Fatalf("notification = %q, want load failed", text)
	}
}

func TestHandleRefreshMsgSetsStatusFromWarning(t *testing.T) {
	const warn = "manifest fallback used"
	m := New()
	m.source = sessionmgr.ModeSessions
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}

	got, _ := m.handleRefreshMsg(refreshMsg{
		source:  sessionmgr.ModeSessions,
		gen:     1,
		items:   testItems("session-a", "session-b"),
		warning: warn,
	})
	if text := latestNotificationText(got); text != warn {
		t.Fatalf("notification = %q, want warning %q", text, warn)
	}
	if sev := latestNotificationSeverity(got); sev != sevWarning {
		t.Fatalf("severity = %v, want sevWarning", sev)
	}
	entry, ok := got.cache[sessionmgr.ModeSessions]
	if !ok || entry.warning != warn {
		t.Fatalf("cache warning = %q, ok=%v", entry.warning, ok)
	}
}

func TestSwitchSourceAppliesCachedWarningStatus(t *testing.T) {
	const warn = "partial agent list"
	m := New()
	m.source = sessionmgr.ModeSessions
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeZoxide: {
			items:     testItems("agent-a"),
			fetchedAt: time.Now(),
			warning:   warn,
		},
	}

	model, _ := m.switchSource(sessionmgr.ModeZoxide)
	got := model.(Model)
	if text := latestNotificationText(got); text != warn {
		t.Fatalf("notification = %q, want cached warning %q", text, warn)
	}
	if sev := latestNotificationSeverity(got); sev != sevWarning {
		t.Fatalf("severity = %v, want sevWarning", sev)
	}
}

// TestTickWarmsModeAllForOverview verifies the tick handler kicks off a
// background ModeAll refresh (for the overview hero counts) even when the
// active source tab is something else and already fresh.
func TestTickWarmsModeAllForOverview(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	// Active source is fresh; ModeAll cache is absent (stale).
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: testItems("s1"), fetchedAt: time.Now()},
	}

	model, _ := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if got.inflightRefresh[sessionmgr.ModeAll] == 0 {
		t.Fatal("tick did not start a background ModeAll refresh for the overview")
	}
}

// TestOverviewItemsReadsModeAllCache verifies overviewItems returns the warmed
// ModeAll cache when another tab is active, m.items when ModeAll is active,
// and nil before any ModeAll load.
func TestOverviewItemsReadsModeAllCache(t *testing.T) {
	m := New()
	m.source = sessionmgr.ModeSessions
	if got := m.overviewItems(); got != nil {
		t.Fatalf("overviewItems before cache = %v, want nil", got)
	}

	cached := testItems("ws")
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {items: cached, fetchedAt: time.Now()},
	}
	if got := m.overviewItems(); len(got) != len(cached) {
		t.Fatalf("overviewItems = %d items, want %d (ModeAll cache)", len(got), len(cached))
	}

	// When ModeAll is the active tab, m.items is used directly.
	m.source = sessionmgr.ModeAll
	m.items = testItems("active")
	got := m.overviewItems()
	if len(got) != len(m.items) {
		t.Fatalf("overviewItems in ModeAll = %d, want m.items %d", len(got), len(m.items))
	}
}
