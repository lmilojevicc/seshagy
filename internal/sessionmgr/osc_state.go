package sessionmgr

import (
	"strings"
	"time"
)

// InferStateFromTitle infers agent state from OSC pane titles when hooks are
// silent. Braille spinner and blocked-title patterns are validated for claude,
// codex, and cursor; the same heuristics apply to other session-only agents.
func InferStateFromTitle(agentName, title string) AgentState {
	title = strings.TrimSpace(StripANSI(title))
	if title == "" {
		return AgentUnknown
	}
	titleLower := strings.ToLower(title)
	_ = agentName
	return inferKnownAgentStateFromTitle(title, titleLower)
}

func inferKnownAgentStateFromTitle(title, titleLower string) AgentState {
	if titleLooksBlocked(titleLower) {
		return AgentBlocked
	}
	if titleHasWorkingSpinner(title) {
		return AgentWorking
	}
	return AgentUnknown
}

func titleLooksBlocked(titleLower string) bool {
	for _, pattern := range []string{
		"action required",
		"waiting",
		"permission",
		"approve",
	} {
		if strings.Contains(titleLower, pattern) {
			return true
		}
	}
	return false
}

var workingSpinnerRunes = map[rune]struct{}{
	'⠋': {}, '⠙': {}, '⠹': {}, '⠸': {}, '⠼': {},
	'⠴': {}, '⠦': {}, '⠧': {}, '⠇': {}, '⠏': {},
}

func titleHasWorkingSpinner(title string) bool {
	if strings.Contains(title, "⋯") {
		return true
	}
	for _, r := range title {
		if _, ok := workingSpinnerRunes[r]; ok {
			return true
		}
	}
	return false
}

func hookStateAllowsFallback(hookStateRaw string, state AgentState, stale bool) bool {
	if stale {
		return true
	}
	hookStateRaw = strings.TrimSpace(hookStateRaw)
	if hookStateRaw == "" || state == AgentUnknown {
		return true
	}
	switch state {
	case AgentWorking, AgentBlocked, AgentIdle:
		return false
	}
	return false
}

func shouldSupplementStateFromTitle(
	hookStateRaw string,
	state AgentState,
	agentName,
	source,
	agentUpdated string,
	now time.Time,
) bool {
	_ = agentName
	_ = source
	stale := isHookStateStale(state, agentUpdated, now)
	return hookStateAllowsFallback(hookStateRaw, state, stale)
}

func resolveAgentState(
	hookStateRaw, agentName, source, title, agentUpdated string,
	skipTitleInference bool,
) AgentState {
	return resolveAgentStateAt(
		hookStateRaw,
		agentName,
		source,
		title,
		agentUpdated,
		skipTitleInference,
		time.Now(),
	)
}

func resolveAgentStateAt(
	hookStateRaw, agentName, source, title, agentUpdated string,
	skipTitleInference bool,
	now time.Time,
) AgentState {
	state := NormalizeAgentState(hookStateRaw)
	effectiveState, stale := effectiveHookStateForFallback(state, agentUpdated, now)
	if !shouldSupplementStateFromTitle(
		hookStateRaw,
		state,
		agentName,
		source,
		agentUpdated,
		now,
	) {
		if stale {
			return effectiveState
		}
		return state
	}
	if skipTitleInference {
		if stale {
			return effectiveState
		}
		return state
	}
	if inferred := InferStateFromTitle(agentName, title); inferred != AgentUnknown {
		return inferred
	}
	if stale {
		return effectiveState
	}
	return state
}
