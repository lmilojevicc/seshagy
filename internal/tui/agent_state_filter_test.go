package tui

import (
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// agentStateItems builds KindAgent items in distinct sessions, each tagged
// with the given agent states so the state filter has something to narrow.
func agentStateItems(states ...sessionmgr.AgentState) []sessionmgr.Item {
	items := make([]sessionmgr.Item, len(states))
	for i, st := range states {
		items[i] = sessionmgr.Item{
			Kind:       sessionmgr.KindAgent,
			Name:       "pi",
			AgentName:  "pi",
			Session:    "s" + string(rune('a'+i)),
			Location:   "s" + string(rune('a'+i)) + ":1.1",
			PaneID:     "%" + string(rune('a'+i)),
			AgentState: st,
		}
	}
	return items
}

// TestAgentsStateKeyOnlyActsInAgentsTab drives the 's' key and asserts the
// state filter advances in the Agents tab (no source switch) and is a no-op
// elsewhere.
func TestAgentsStateKeyOnlyActsInAgentsTab(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.items = agentStateItems(
		sessionmgr.AgentWorking,
		sessionmgr.AgentIdle,
		sessionmgr.AgentDone,
	)

	model, _ := m.handleKey(keyMsg("s"))
	got := model.(Model)
	if got.agentsStateFilter != sessionmgr.AgentWorking {
		t.Fatalf("agentsStateFilter = %q after first 's', want %q",
			got.agentsStateFilter, sessionmgr.AgentWorking)
	}
	if got.source != sessionmgr.ModeAgents {
		t.Fatalf("source = %v, want ModeAgents (no tab switch)", got.source)
	}

	// 's' is a no-op outside the Agents tab.
	got.source = sessionmgr.ModeSessions
	before := got.agentsStateFilter
	model, _ = got.handleKey(keyMsg("s"))
	got = model.(Model)
	if got.agentsStateFilter != before {
		t.Fatalf("'s' outside Agents tab mutated filter: %q -> %q",
			before, got.agentsStateFilter)
	}
}

// TestAgentsStateFilterNarrowsVisibleItems proves the state filter hides
// non-matching agents in visibleItems() and lifts cleanly when cleared.
func TestAgentsStateFilterNarrowsVisibleItems(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.items = agentStateItems(
		sessionmgr.AgentWorking,
		sessionmgr.AgentIdle,
		sessionmgr.AgentDone,
		sessionmgr.AgentBlocked,
	)
	m.agentsStateFilter = sessionmgr.AgentWorking

	vis := m.visibleItems()
	if len(vis) != 1 {
		t.Fatalf("visible with working filter = %d, want 1", len(vis))
	}
	for _, it := range vis {
		if it.AgentState != sessionmgr.AgentWorking {
			t.Fatalf("visible item state = %q, want working", it.AgentState)
		}
	}

	// No filter restores all.
	m.agentsStateFilter = ""
	if got := len(m.visibleItems()); got != 4 {
		t.Fatalf("visible after clearing filter = %d, want 4", got)
	}
}

// TestAgentsStateFilterCycle drives 's' a full cycle and asserts the field
// returns to "" (all states) after visiting every state.
func TestAgentsStateFilterCycle(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.items = agentStateItems(sessionmgr.AgentWorking)

	want := []sessionmgr.AgentState{
		sessionmgr.AgentWorking,
		sessionmgr.AgentBlocked,
		sessionmgr.AgentIdle,
		sessionmgr.AgentDone,
		sessionmgr.AgentUnknown,
		"", // back to all states
	}
	got := m
	for i, w := range want {
		model, _ := got.handleKey(keyMsg("s"))
		got = model.(Model)
		if got.agentsStateFilter != w {
			t.Fatalf("press %d: agentsStateFilter = %q, want %q", i+1, got.agentsStateFilter, w)
		}
	}
}
