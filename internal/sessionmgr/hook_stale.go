package sessionmgr

import (
	"strconv"
	"strings"
	"time"
)

var agentResolveNow = time.Now

const agentHookStaleTTL = 5 * time.Minute

// Ignore implausibly small @agent_updated values from legacy fixtures or unset data.
const agentHookUpdatedMinUnix = 1_600_000_000

func isHookStateStale(state AgentState, agentUpdated string, now time.Time) bool {
	if state != AgentWorking && state != AgentBlocked {
		return false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(agentUpdated), 10, 64)
	if err != nil || ts < agentHookUpdatedMinUnix {
		return false
	}
	return now.Sub(time.Unix(ts, 0)) > agentHookStaleTTL
}

func effectiveHookStateForFallback(
	state AgentState,
	agentUpdated string,
	now time.Time,
) (AgentState, bool) {
	if !isHookStateStale(state, agentUpdated, now) {
		return state, false
	}
	return AgentUnknown, true
}
