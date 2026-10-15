package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestCacheTTLsPerSource(t *testing.T) {
	tests := []struct {
		source sessionmgr.SourceMode
		want   time.Duration
	}{
		{sessionmgr.ModeAgents, time.Second},
		{sessionmgr.ModeSessions, 2 * time.Second},
		{sessionmgr.ModeZoxide, 15 * time.Second},
		{sessionmgr.ModeFD, time.Minute},
	}
	for _, tt := range tests {
		if got := cacheTTL(tt.source); got != tt.want {
			t.Fatalf("cacheTTL(%v) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

func TestMergedItemsDedupesFDAgainstZoxide(t *testing.T) {
	m := newTestModel(t)
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "dev", Target: "dev"},
		}},
		sessionmgr.ModeAgents: {items: []sessionmgr.Item{agentItem("claude", "%1", "dev")}},
		sessionmgr.ModeZoxide: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindZoxide, Path: "~/code", Name: "~/code"},
		}},
		sessionmgr.ModeFD: {items: []sessionmgr.Item{
			{Kind: sessionmgr.KindFD, Path: "~/code", Name: "~/code"},
			{Kind: sessionmgr.KindFD, Path: "~/other", Name: "~/other"},
		}},
	}
	items := m.mergedItems()
	if len(items) != 4 {
		t.Fatalf("merged items = %#v, want 4 (fd duplicate dropped)", items)
	}
	deduped := 0
	for _, item := range items {
		if item.Kind == sessionmgr.KindFD {
			deduped++
			if item.Path == "~/code" {
				t.Fatal("fd duplicate of the zoxide path survived")
			}
		}
	}
	if deduped != 1 {
		t.Fatalf("fd items = %d, want 1", deduped)
	}
}

func TestTickRefreshesOnlyStaleSources(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAgents: {fetchedAt: now}, // fresh (TTL 1s)
		sessionmgr.ModeSessions: {
			fetchedAt: now.Add(-3 * time.Second), // stale (TTL 2s)
		},
		sessionmgr.ModeZoxide: {fetchedAt: now.Add(-time.Minute)}, // stale (TTL 15s)
		// fd has no entry: never loaded, so stale.
	}
	model, cmd := m.Update(tickMsg(now))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("tick returned no commands")
	}
	for _, source := range []sessionmgr.SourceMode{
		sessionmgr.ModeSessions,
		sessionmgr.ModeZoxide,
		sessionmgr.ModeFD,
	} {
		if !m.refreshInflight(source) {
			t.Fatalf("stale source %v was not refreshed", source)
		}
	}
	if m.refreshInflight(sessionmgr.ModeAgents) {
		t.Fatal("fresh agents source was refreshed")
	}
}

func TestBeginRefreshCoalescesInflightAndStartsSpinner(t *testing.T) {
	m := newTestModel(t)
	var cmd tea.Cmd
	m, cmd = m.beginRefresh(sessionmgr.ModeSessions, false)
	if cmd == nil {
		t.Fatal("first refresh returned no command")
	}
	if !m.spinnerActive {
		t.Fatal("first refresh did not start the spinner")
	}
	if _, again := m.beginRefresh(sessionmgr.ModeSessions, false); again != nil {
		t.Fatal("an in-flight refresh was started twice")
	}
	gen := m.inflightRefresh[sessionmgr.ModeSessions]
	if _, forced := m.beginRefresh(sessionmgr.ModeSessions, true); forced == nil {
		t.Fatal("forced refresh did not restart the source")
	}
	if m.inflightRefresh[sessionmgr.ModeSessions] == gen {
		t.Fatal("forced refresh reused the previous generation")
	}
}

func TestRefreshMsgDropsStaleGeneration(t *testing.T) {
	m := newTestModel(t)
	m.refreshGen[sessionmgr.ModeSessions] = 5
	m.inflightRefresh[sessionmgr.ModeSessions] = 5
	model, cmd := m.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    4,
		items:  []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "stale"}},
	})
	m = model
	if cmd != nil || len(m.items) != 0 {
		t.Fatalf("stale generation applied: items = %#v cmd = %v", m.items, cmd)
	}
	if m.loadedOnce {
		t.Fatal("a stale result marked the picker as loaded")
	}
}

func TestServerWarningsAreToastedOnce(t *testing.T) {
	m := newTestModel(t)
	m.inflightRefresh[sessionmgr.ModeZoxide] = 1
	m.refreshGen[sessionmgr.ModeZoxide] = 1
	msg := refreshMsg{
		source:  sessionmgr.ModeZoxide,
		gen:     1,
		warning: "zoxide unavailable",
	}
	model, _ := m.Update(msg)
	m = model.(Model)
	if latestNotificationText(m) != "zoxide unavailable" ||
		latestNotificationSeverity(m) != sevWarning {
		t.Fatalf("notifications = %#v", m.notifications)
	}
	before := len(m.notifications)

	m.inflightRefresh[sessionmgr.ModeZoxide] = 2
	m.refreshGen[sessionmgr.ModeZoxide] = 2
	model, _ = m.Update(refreshMsg{
		source:  sessionmgr.ModeZoxide,
		gen:     2,
		warning: "zoxide unavailable",
	})
	m = model.(Model)
	if len(m.notifications) != before {
		t.Fatalf("repeated warning toasted again: %#v", m.notifications)
	}
}

func TestServerErrorIsToastedAsError(t *testing.T) {
	m := newTestModel(t)
	m.inflightRefresh[sessionmgr.ModeSessions] = 1
	m.refreshGen[sessionmgr.ModeSessions] = 1
	model, _ := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		err:    errSessionsUnavailable,
	})
	m = model.(Model)
	if latestNotificationSeverity(m) != sevError ||
		latestNotificationText(m) != errSessionsUnavailable.Error() {
		t.Fatalf("notifications = %#v", m.notifications)
	}
}

type staticError string

func (e staticError) Error() string { return string(e) }

var errSessionsUnavailable = staticError("no server running")

func TestInvalidateAllCachesDropsInflightRefreshes(t *testing.T) {
	m := newTestModel(t)
	m.inflightRefresh[sessionmgr.ModeSessions] = 1
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: []sessionmgr.Item{{Kind: sessionmgr.KindSession}}},
	}
	m = m.invalidateAllCaches()
	if len(m.cache) != 0 {
		t.Fatalf("cache not cleared: %#v", m.cache)
	}
	if m.anyRefreshInflight() {
		t.Fatal("in-flight refreshes survived invalidation")
	}
	if m.refreshGen[sessionmgr.ModeSessions] != 1 {
		t.Fatalf("generation not advanced: %d", m.refreshGen[sessionmgr.ModeSessions])
	}
}

func TestStoreCacheKeepsCursorOnRefresh(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "alpha", Target: "alpha"},
		{Kind: sessionmgr.KindSession, Name: "bravo", Target: "bravo"},
	})
	m = m.setCursor(1)
	model, _ := m.handleRefreshMsg(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    m.inflightRefresh[sessionmgr.ModeSessions],
		items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "bravo", Target: "bravo"},
			{Kind: sessionmgr.KindSession, Name: "alpha", Target: "alpha"},
		},
	})
	m = model
	if m.cursor != 0 || m.cursorKey != "session:bravo" {
		t.Fatalf("cursor = %d key = %q, want the reordered selection", m.cursor, m.cursorKey)
	}
}
