package sessionmgr

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// agentPaneFormat is the tmux list-panes format used for agent discovery. Fields
// are separated by the unit separator (\x1f), mirroring sessionFormat.
const agentPaneFormat = "#{pane_id}\x1f#{session_name}\x1f#{window_index}\x1f#{pane_index}" +
	"\x1f#{pane_current_path}\x1f#{pane_current_command}\x1f#{pane_pid}\x1f#{pane_dead}" +
	"\x1f#{@seshagy_agent_state}\x1f#{@seshagy_agent_updated}\x1f#{@seshagy_agent_seq}"

// agentProcessNames maps a pane_current_command basename to the canonical agent
// name. The canonical names (pi, opencode, codex, claude, cursor, antigravity,
// droid, grok, copilot) are used throughout the UI and alias store.
var agentProcessNames = map[string]string{
	"pi":           "pi",
	"opencode":     "opencode",
	"codex":        "codex",
	"claude":       "claude",
	"cursor-agent": "cursor",
	"cursor":       "cursor",
	"agy":          "antigravity",
	"antigravity":  "antigravity",
	"droid":        "droid",
	"factory":      "droid",
	"grok":         "grok",
	"copilot":      "copilot",
}

const agentFreshnessWindow = 60 * time.Second

// NormalizeAgentState maps hook-reported state synonyms to the 4-state enum.
// Unknown/aborted/error states all fall back to idle — the strict rule is:
// never guess working or blocked from an unrecognised value. An agent that
// crashed is gone (pane dies); an ESC interrupt returns to idle.
func NormalizeAgentState(state string) AgentState {
	s := strings.ToLower(strings.TrimSpace(state))
	switch s {
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
	case "done", "complete", "completed", "finished":
		return AgentDone
	case "idle", "ready":
		return AgentIdle
	default:
		return AgentIdle
	}
}

// isStateFresh returns true when the @seshagy_agent_updated timestamp is within the
// freshness window. Stale reports (older than 60s) fall back to idle so a
// crashed agent doesn't show a stale working state.
func isStateFresh(updated, seqStr string) bool {
	if updated == "" || seqStr == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return false
	}
	return time.Since(t) < agentFreshnessWindow
}

// detectAgentName maps a process name to the canonical agent name, returning
// "" when the process is not a known agent. It handles architecture-suffixed
// binary names (e.g. codex-aarch64-a) via prefix matching, and basenames the
// input via filepath.Base so full-path comm values (e.g.
// /Users/x/.local/bin/cursor-agent) match correctly.
func detectAgentName(command string) string {
	name := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	if agent := agentProcessNames[name]; agent != "" {
		return agent
	}
	// Match architecture-suffixed binary names (e.g. codex-aarch64-a). The
	// trailing "-" guard prevents substring false positives (pi vs pihole).
	for prefix, agent := range agentProcessNames {
		if strings.HasPrefix(name, prefix+"-") {
			return agent
		}
	}
	return ""
}

// ListAgents returns agent panes across all sessions (or, when sessionFilter is
// non-empty, only those in the given session). It mirrors ListSessions: the
// tmux exit-1 case (no server) is treated as "no agents".
func ListAgents(ctx context.Context, sessionFilter string) ([]Item, error) {
	out, err := tmuxOutput(ctx, "list-panes", "-a", "-F", agentPaneFormat)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	return ParseAgents(out, sessionFilter), nil
}

// ParseAgents parses raw list-panes output into agent items. Dead panes and
// non-agent processes are skipped; sessionFilter limits results to a session.
// A per-call snapshotCache ensures the process table is read at most once for
// the descendant-walk fallback across all panes.
func ParseAgents(raw []byte, sessionFilter string) []Item {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	items := make([]Item, 0, len(lines))
	cache := &snapshotCache{}
	for _, line := range lines {
		parts := strings.Split(line, "\x1f")
		if len(parts) < 8 {
			continue
		}
		if parts[7] == "1" { // pane_dead
			continue
		}
		if sessionFilter != "" && parts[1] != sessionFilter {
			continue
		}
		agentName := detectAgent(parts[5], parts[6], cache)
		if agentName == "" {
			continue
		}

		// Resolve state from @seshagy_agent_* metadata. Falls back to idle when
		// hooks are absent or the report is stale.
		agentState := AgentIdle
		if len(parts) > 10 {
			rawState := parts[8] // @seshagy_agent_state
			updated := parts[9]  // @seshagy_agent_updated
			seqStr := parts[10]  // @seshagy_agent_seq
			if rawState != "" && isStateFresh(updated, seqStr) {
				agentState = NormalizeAgentState(rawState)
			}
		}

		items = append(items, Item{
			Kind:       KindAgent,
			Name:       agentName,
			AgentName:  agentName,
			AgentState: agentState,
			PaneID:     parts[0],
			Session:    parts[1],
			Window:     parts[2],
			Pane:       parts[3],
			Path:       parts[4],
			Location:   fmt.Sprintf("%s:%s.%s", parts[1], parts[2], parts[3]),
		})
	}
	return ApplyAgentLabels(items)
}
