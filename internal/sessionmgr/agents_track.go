package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	agentStartupGraceWindow       = 3 * time.Second
	agentPendingIdleDebounce      = 700 * time.Millisecond
	agentPendingIdleConfirmations = 3
)

var agentTrackNow = time.Now

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

func AgentStateLabel(state AgentState) string {
	if state == "" {
		return string(AgentUnknown)
	}
	return string(state)
}

func ensureAgentStartupGrace(ctx context.Context, pane string, now time.Time) {
	name, _ := showPaneOption(ctx, pane, "@agent_name")
	if name == "" {
		return
	}
	grace, _ := showPaneOption(ctx, pane, "@agent_startup_grace")
	if grace == "" {
		_ = setPaneOption(ctx, pane, "@agent_startup_grace", fmt.Sprintf("%d", now.Unix()))
	}
}

func inAgentStartupGrace(ctx context.Context, pane string, now time.Time) bool {
	graceRaw, _ := showPaneOption(ctx, pane, "@agent_startup_grace")
	if graceRaw == "" {
		return false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(graceRaw), 10, 64)
	if err != nil {
		return false
	}
	return now.Sub(time.Unix(start, 0)) < agentStartupGraceWindow
}

func clearAgentPendingIdle(ctx context.Context, pane string) {
	_ = unsetPaneOption(ctx, pane, "@agent_pending_idle_since")
	_ = unsetPaneOption(ctx, pane, "@agent_pending_idle_count")
}

func tickAgentPendingIdle(ctx context.Context, pane string, now time.Time) bool {
	sinceRaw, _ := showPaneOption(ctx, pane, "@agent_pending_idle_since")
	countRaw, _ := showPaneOption(ctx, pane, "@agent_pending_idle_count")
	count := 0
	if countRaw != "" {
		count, _ = strconv.Atoi(strings.TrimSpace(countRaw))
	}
	if sinceRaw == "" {
		_ = setPaneOption(ctx, pane, "@agent_pending_idle_since", fmt.Sprintf("%d", now.Unix()))
	} else if _, err := strconv.ParseInt(strings.TrimSpace(sinceRaw), 10, 64); err != nil {
		_ = setPaneOption(ctx, pane, "@agent_pending_idle_since", fmt.Sprintf("%d", now.Unix()))
	}
	count++
	_ = setPaneOption(ctx, pane, "@agent_pending_idle_count", strconv.Itoa(count))

	var since time.Time
	if ts, err := strconv.ParseInt(
		strings.TrimSpace(sinceRaw),
		10,
		64,
	); err == nil &&
		sinceRaw != "" {
		since = time.Unix(ts, 0)
	} else {
		since = now
	}
	return count >= agentPendingIdleConfirmations && now.Sub(since) >= agentPendingIdleDebounce
}

func holdLifecycleWorkingStatus(previousStatus AgentState) AgentState {
	switch previousStatus {
	case AgentWorking, AgentBlocked:
		return previousStatus
	default:
		return AgentWorking
	}
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
	now := agentTrackNow()
	ensureAgentStartupGrace(ctx, pane, now)
	var status AgentState
	switch detected {
	case AgentDone:
		clearAgentPendingIdle(ctx, pane)
		if visible {
			status = AgentIdle
		} else {
			status = AgentDone
		}
	case AgentAborted:
		clearAgentPendingIdle(ctx, pane)
		if visible {
			status = AgentIdle
		} else {
			status = AgentAborted
		}
	case AgentIdle:
		if visible {
			clearAgentPendingIdle(ctx, pane)
			status = AgentIdle
		} else if previousStatus == AgentDone {
			status = AgentDone
		} else if previousStatus == AgentAborted {
			status = AgentAborted
		} else if lifecycleAuthority &&
			(previousState == AgentWorking || previousState == AgentBlocked) {
			if tickAgentPendingIdle(ctx, pane, now) {
				clearAgentPendingIdle(ctx, pane)
				if inAgentStartupGrace(ctx, pane, now) {
					status = AgentIdle
				} else {
					status = AgentDone
				}
			} else {
				status = holdLifecycleWorkingStatus(previousStatus)
				semantic = previousState
			}
		} else {
			status = AgentIdle
		}
	case AgentWorking, AgentBlocked, AgentUnknown:
		clearAgentPendingIdle(ctx, pane)
		status = detected
	default:
		clearAgentPendingIdle(ctx, pane)
		status = AgentUnknown
	}
	if string(semantic) == strings.TrimSpace(previousRaw) &&
		string(status) == strings.TrimSpace(previousStatusRaw) {
		return status, nil
	}
	_ = setPaneOption(ctx, pane, "@agent_last_state", string(semantic))
	_ = setPaneOption(ctx, pane, "@agent_last_status", string(status))
	if visible {
		_ = setPaneOption(ctx, pane, "@agent_last_seen", fmt.Sprintf("%d", now.Unix()))
	}
	return status, nil
}

func MarkAgentSeen(ctx context.Context, pane string) (bool, error) {
	pane, err := ResolvePane(ctx, pane)
	if err != nil {
		return false, err
	}
	var seen bool
	err = withAgentPaneLock(pane, func() error {
		var lockErr error
		seen, lockErr = markAgentSeenLocked(ctx, pane)
		return lockErr
	})
	return seen, err
}

func markAgentSeenLocked(ctx context.Context, pane string) (bool, error) {
	name, _ := showPaneOption(ctx, pane, "@agent_name")
	stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
	if strings.TrimSpace(name) == "" && strings.TrimSpace(stateRaw) == "" {
		return false, nil
	}
	seqRaw, _ := showPaneOption(ctx, pane, "@agent_seq")
	seqRaw = strings.TrimSpace(seqRaw)
	if seqRaw != "" {
		if _, parseErr := strconv.ParseInt(seqRaw, 10, 64); parseErr != nil {
			return false, nil //nolint:nilerr // corrupt @agent_seq is a no-op, not a caller error
		}
	}
	semantic := semanticAgentState(NormalizeAgentState(stateRaw))
	if semantic == AgentIdle || semantic == AgentAborted {
		// Use seq-safe writes to avoid overwriting concurrent hook reports.
		seq, _ := strconv.ParseInt(seqRaw, 10, 64)
		seqSeen := seqRaw != ""
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
	if err := setPaneOption(
		ctx,
		pane,
		"@agent_last_seen",
		fmt.Sprintf("%d", agentTrackNow().Unix()),
	); err != nil {
		return false, err
	}
	return true, nil
}

func FocusAgentCommand(pane string) *exec.Cmd {
	// Keep the whole focus flow in one foreground process so Bubble Tea can suspend once.
	script := `set -e
pane="$1"
session_id=$(tmux display-message -p -t "$pane" '#{session_id}')
window_id=$(tmux display-message -p -t "$pane" '#{window_id}')
tmux select-window -t "$window_id"
tmux select-pane -t "$pane"
name=$(tmux show-option -qvpt "$pane" @agent_name 2>/dev/null || true)
seq=$(tmux show-option -qvpt "$pane" @agent_seq 2>/dev/null || true)
state=$(tmux show-option -qvpt "$pane" @agent_state 2>/dev/null || true)
seq_ok=1
if [ -n "${seq}" ]; then
  case "${seq}" in
    *[!0-9]*) seq_ok=0 ;;
  esac
fi
if [ "${seq_ok}" -eq 1 ]; then
  tmux set-option -qpt "$pane" @agent_last_seen "$(date +%s)" 2>/dev/null || true
fi
if [ "${seq_ok}" -eq 1 ] && [ -n "${name}" ] && [ -z "${seq}" ]; then
  case "${state}" in
    working|busy|running|thinking|processing|blocked|permission|permissions|question|confirm|confirmation|waiting|wait) ;;
    done|complete|completed|finished|idle|ready|aborted|cancelled|canceled|stopped|error|failed|timeout)
      tmux set-option -qpt "$pane" @agent_state idle 2>/dev/null || true
      tmux set-option -qpt "$pane" @agent_last_status idle 2>/dev/null || true
    ;;
  esac
fi
if [ -n "${TMUX:-}" ]; then
  tmux switch-client -t "$session_id"
else
  tmux attach-session -t "$session_id"
fi`
	return exec.Command("sh", "-c", script, "seshagy-focus-agent", pane)
}
