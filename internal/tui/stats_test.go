package tui

import (
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestAggregateOverviewStats(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "a", Attached: true},
		{Kind: sessionmgr.KindSession, Name: "b", Attached: false},
		{Kind: sessionmgr.KindSession, Name: "c", Attached: true},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentBlocked},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentDone},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentIdle},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentUnknown},
		// Directories are not summarized.
		{Kind: sessionmgr.KindZoxide, Path: "/x"},
		{Kind: sessionmgr.KindFD, Path: "/y"},
	}
	stats := aggregateOverviewStats(items)

	if stats.sessions != 3 {
		t.Fatalf("sessions = %d, want 3", stats.sessions)
	}
	if stats.attached != 2 {
		t.Fatalf("attached = %d, want 2", stats.attached)
	}
	wantAgents := map[sessionmgr.AgentState]int{
		sessionmgr.AgentWorking: 2,
		sessionmgr.AgentBlocked: 1,
		sessionmgr.AgentDone:    1,
		sessionmgr.AgentIdle:    1,
		sessionmgr.AgentUnknown: 1,
	}
	for state, want := range wantAgents {
		if got := stats.agents[state]; got != want {
			t.Fatalf("agents[%s] = %d, want %d", state, got, want)
		}
	}
	if got := stats.agentTotal(); got != 6 {
		t.Fatalf("agentTotal = %d, want 6", got)
	}
}

// TestAggregateOverviewStatsNormalizesRawStates verifies that a raw/empty agent
// state is normalized into a known bucket rather than producing a missing key.
func TestAggregateOverviewStatsNormalizesRawStates(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, AgentState: ""},                               // -> idle
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentState("WORKING")}, // -> working
	}
	stats := aggregateOverviewStats(items)
	if stats.agents[sessionmgr.AgentIdle] != 1 {
		t.Fatalf("idle = %d, want 1 (empty normalized)", stats.agents[sessionmgr.AgentIdle])
	}
	if stats.agents[sessionmgr.AgentWorking] != 1 {
		t.Fatalf("working = %d, want 1 (raw normalized)", stats.agents[sessionmgr.AgentWorking])
	}
}

func TestAggregateOverviewStatsEmpty(t *testing.T) {
	stats := aggregateOverviewStats(nil)
	if stats.sessions != 0 || stats.attached != 0 || stats.agentTotal() != 0 {
		t.Fatalf("empty stats = %#v, want zeros", stats)
	}
	// All five state buckets are initialized to 0.
	if len(stats.agents) != len(agentStateOrder) {
		t.Fatalf("agent buckets = %d, want %d", len(stats.agents), len(agentStateOrder))
	}
}
