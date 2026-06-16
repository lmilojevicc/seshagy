package tui

import (
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestTUIFirstRefreshSmoke(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := New()
	if m.Init() == nil {
		t.Fatal("Init() returned nil")
	}

	m.source = sessionmgr.ModeSessions
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}
	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "dev"},
		},
	})
	m = model.(Model)
	if len(m.items) == 0 {
		t.Fatal("expected items after refresh")
	}
	if m.status == "" {
		t.Fatal("expected non-empty status after refresh")
	}
	if m.View() == "" {
		t.Fatal("expected non-empty view after refresh")
	}
	if cmd == nil {
		t.Fatal("expected preview command after refresh")
	}
}

func TestSelectedKeySortItemsAndPlural(t *testing.T) {
	m := New()
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindFD, Path: "/tmp/b"},
		{Kind: sessionmgr.KindSession, Name: "alpha"},
		{Kind: sessionmgr.KindAgent, PaneID: "%1", AgentName: "claude"},
	}
	SortItems(m.items)
	if m.items[0].Kind != sessionmgr.KindSession || m.items[1].Kind != sessionmgr.KindAgent {
		t.Fatalf("sort order = %#v", m.items)
	}

	m.cursor = 0
	if key := m.selectedKey(); key != "session:alpha" {
		t.Fatalf("selectedKey = %q, want session:alpha", key)
	}

	m.cursor = 99
	if m.selectedKey() != "" {
		t.Fatal("expected empty selectedKey for invalid cursor")
	}

	if plural(1) != "" || plural(2) != "s" {
		t.Fatalf("plural(1)=%q plural(2)=%q", plural(1), plural(2))
	}
}

func TestNextAgentStateFilterCyclesAndResets(t *testing.T) {
	state := sessionmgr.AgentState("")
	for _, want := range []sessionmgr.AgentState{
		sessionmgr.AgentWorking,
		sessionmgr.AgentBlocked,
		sessionmgr.AgentAborted,
		sessionmgr.AgentDone,
		sessionmgr.AgentIdle,
		sessionmgr.AgentUnknown,
	} {
		got := nextAgentStateFilter(state)
		if got != want {
			t.Fatalf("nextAgentStateFilter(%q) = %q, want %q", state, got, want)
		}
		state = got
	}
	if nextAgentStateFilter(state) != "" {
		t.Fatal("expected filter reset after unknown")
	}
	if agentStateFilterLabel("") != "all" ||
		agentStateFilterLabel(sessionmgr.AgentWorking) != "working" {
		t.Fatalf(
			"labels = %q / %q",
			agentStateFilterLabel(""),
			agentStateFilterLabel(sessionmgr.AgentWorking),
		)
	}
}

func TestSortedCountsGroupsItemsByKind(t *testing.T) {
	counts := sortedCounts([]sessionmgr.Item{
		{Kind: sessionmgr.KindSession},
		{Kind: sessionmgr.KindAgent},
		{Kind: sessionmgr.KindSession},
	})
	if counts[sessionmgr.KindSession] != 2 || counts[sessionmgr.KindAgent] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}
