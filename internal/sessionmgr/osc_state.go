package sessionmgr

import "strings"

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

func titleHasWorkingSpinner(title string) bool {
	if strings.Contains(title, "⋯") {
		return true
	}
	for _, r := range title {
		if r >= 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}

func hookStateAllowsFallback(hookStateRaw string, state AgentState) bool {
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
	source string,
) bool {
	_ = agentName
	_ = source
	return hookStateAllowsFallback(hookStateRaw, state)
}

func resolveAgentState(
	hookStateRaw, agentName, source, title string,
	skipTitleInference bool,
) AgentState {
	state := NormalizeAgentState(hookStateRaw)
	if !shouldSupplementStateFromTitle(hookStateRaw, state, agentName, source) {
		return state
	}
	if skipTitleInference {
		return state
	}
	if inferred := InferStateFromTitle(agentName, title); inferred != AgentUnknown {
		return inferred
	}
	return state
}
