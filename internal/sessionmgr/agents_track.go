package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func NormalizeAgentState(state string) AgentState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "working", "busy", "running", "thinking", "processing":
		return AgentWorking
	case "blocked",
		"permission",
		"permissions",
		"question",
		"confirm",
		"confirmation",
		"waiting",
		"wait":
		return AgentBlocked
	case "aborted", "abort", "cancelled", "canceled", "interrupted", "stopped",
		"error", "failed", "failure",
		"timeout", "timed_out", "timed-out",
		"disconnected", "offline":
		return AgentAborted
	case "done", "complete", "completed", "finished":
		return AgentDone
	case "idle", "ready":
		return AgentIdle
	default:
		return AgentUnknown
	}
}

func semanticAgentState(state AgentState) AgentState {
	if state == AgentDone {
		return AgentIdle
	}
	return state
}

func agentStateLabel(state AgentState) string {
	if state == "" {
		return string(AgentUnknown)
	}
	return string(state)
}

func UpdateAgentStatusTracking(
	ctx context.Context,
	pane string,
	detected AgentState,
	visible bool,
	lifecycleAuthority bool,
) (AgentState, error) {
	if pane == "" {
		return detected, nil
	}
	semantic := semanticAgentState(detected)
	previousRaw, _ := showPaneOption(ctx, pane, "@agent_last_state")
	previousStatusRaw, _ := showPaneOption(ctx, pane, "@agent_last_status")
	previousState := semanticAgentState(NormalizeAgentState(previousRaw))
	previousStatus := NormalizeAgentState(previousStatusRaw)
	var status AgentState
	switch detected {
	case AgentDone:
		if visible {
			status = AgentIdle
		} else {
			status = AgentDone
		}
	case AgentAborted:
		if visible {
			status = AgentIdle
		} else {
			status = AgentAborted
		}
	case AgentIdle:
		if visible {
			status = AgentIdle
		} else if previousStatus == AgentDone {
			status = AgentDone
		} else if previousStatus == AgentAborted {
			status = AgentAborted
		} else if lifecycleAuthority &&
			(previousState == AgentWorking || previousState == AgentBlocked) {
			status = AgentDone
		} else {
			status = AgentIdle
		}
	case AgentWorking, AgentBlocked, AgentUnknown:
		status = detected
	default:
		status = AgentUnknown
	}
	_ = setPaneOption(ctx, pane, "@agent_last_state", string(semantic))
	_ = setPaneOption(ctx, pane, "@agent_last_status", string(status))
	if visible {
		_ = setPaneOption(ctx, pane, "@agent_last_seen", fmt.Sprintf("%d", time.Now().Unix()))
	}
	return status, nil
}

func MarkAgentSeen(ctx context.Context, pane string) {
	stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
	semantic := semanticAgentState(NormalizeAgentState(stateRaw))
	if semantic == AgentIdle || semantic == AgentAborted {
		// Use seq-safe writes to avoid overwriting concurrent hook reports.
		seqRaw, _ := showPaneOption(ctx, pane, "@agent_seq")
		seq, err := strconv.ParseInt(strings.TrimSpace(seqRaw), 10, 64)
		seqSeen := err == nil && seqRaw != ""
		setAgentPaneOptionIfCurrent(ctx, pane, "@agent_state", string(AgentIdle), seq, seqSeen)
		setAgentPaneOptionIfCurrent(
			ctx,
			pane,
			"@agent_last_status",
			string(AgentIdle),
			seq,
			seqSeen,
		)
	}
	_ = setPaneOption(ctx, pane, "@agent_last_seen", fmt.Sprintf("%d", time.Now().Unix()))
}

func FocusAgentCommand(pane string) *exec.Cmd {
	// Keep the whole focus flow in one foreground process so Bubble Tea can suspend once.
	script := `set -e
pane="$1"
session_id=$(tmux display-message -p -t "$pane" '#{session_id}')
window_id=$(tmux display-message -p -t "$pane" '#{window_id}')
tmux select-window -t "$window_id"
tmux select-pane -t "$pane"
tmux set-option -qpt "$pane" @agent_last_seen "$(date +%s)" 2>/dev/null || true
state=$(tmux show-option -qvpt "$pane" @agent_state 2>/dev/null || true)
case "${state}" in done|complete|completed|finished|idle|ready|aborted|cancelled|canceled|stopped|error|failed|timeout) tmux set-option -qpt "$pane" @agent_state idle 2>/dev/null || true; tmux set-option -qpt "$pane" @agent_last_status idle 2>/dev/null || true ;; esac
if [ -n "${TMUX:-}" ]; then
  tmux switch-client -t "$session_id"
else
  tmux attach-session -t "$session_id"
fi`
	return exec.Command("sh", "-c", script, "seshagy-focus-agent", pane)
}
