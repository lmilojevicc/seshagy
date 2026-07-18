package tui

import (
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// overviewStats holds the counts rendered in the top overview hero band.
// Only sessions and agents are summarized here (directories are excluded).
type overviewStats struct {
	sessions int
	agents   map[sessionmgr.AgentState]int
}

// agentStateOrder is the display order for the agent state chips.
var agentStateOrder = []sessionmgr.AgentState{
	sessionmgr.AgentWorking,
	sessionmgr.AgentBlocked,
	sessionmgr.AgentDone,
	sessionmgr.AgentIdle,
	sessionmgr.AgentUnknown,
}

// aggregateOverviewStats reduces a ModeAll item list into the hero counts.
// Sessions are tallied; agents are bucketed by their
// normalized state (raw states are mapped through NormalizeAgentState so the
// zero value and any wire variants land in a known bucket).
func aggregateOverviewStats(items []sessionmgr.Item) overviewStats {
	st := overviewStats{
		agents: make(map[sessionmgr.AgentState]int, len(agentStateOrder)),
	}
	for _, s := range agentStateOrder {
		st.agents[s] = 0
	}
	for _, item := range items {
		switch item.Kind {
		case sessionmgr.KindSession:
			st.sessions++
		case sessionmgr.KindAgent:
			st.agents[sessionmgr.NormalizeAgentState(string(item.AgentState))]++
		}
	}
	return st
}
