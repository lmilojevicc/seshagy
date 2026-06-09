package sessionmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const paneSep = "\x1f"

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const agentFormat = "#{pane_id}" + paneSep + "#{session_name}" + paneSep + "#{window_index}" + paneSep + "#{pane_index}" + paneSep + "#{pane_current_path}" + paneSep + "#{pane_active}" + paneSep + "#{window_active}" + paneSep + "#{session_attached}" + paneSep + "#{pane_dead}" + paneSep + "#{@agent_name}" + paneSep + "#{@agent_state}" + paneSep + "#{@agent_message}" + paneSep + "#{@agent_updated}" + paneSep + "#{@agent_source}"

func ListAgents(ctx context.Context, sessionFilter string) ([]Item, error) {
	out, err := tmuxCommand(ctx, "list-panes", "-a", "-F", agentFormat).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	items := ParseAgents(out, sessionFilter)
	for i := range items {
		state, err := UpdateAgentStatusTracking(ctx, items[i].PaneID, items[i].AgentState, items[i].Visible)
		if err == nil {
			items[i].AgentState = state
		}
	}
	return items, nil
}

func ParseAgents(raw []byte, sessionFilter string) []Item {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	var items []Item
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Split(line, paneSep)
		if len(parts) < 14 {
			continue
		}
		if parts[8] == "1" {
			continue
		}
		if sessionFilter != "" && parts[1] != sessionFilter {
			continue
		}
		name := parts[9]
		if name == "" {
			continue
		}
		state := NormalizeAgentState(parts[10])
		message := cleanField(parts[11])
		source := cleanField(parts[13])
		path := ContractHome(parts[4])
		location := fmt.Sprintf("%s:%s.%s", parts[1], parts[2], parts[3])
		items = append(items, Item{
			Kind:         KindAgent,
			Name:         name,
			Target:       parts[0],
			PaneID:       parts[0],
			Session:      parts[1],
			Window:       parts[2],
			Pane:         parts[3],
			Path:         path,
			Location:     location,
			AgentName:    name,
			AgentState:   state,
			AgentMessage: message,
			AgentUpdated: cleanField(parts[12]),
			AgentSource:  source,
			Visible:      parts[5] == "1" && parts[6] == "1" && parts[7] != "0",
		})
	}
	return items
}

func NormalizeAgentState(state string) AgentState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "working", "busy", "running", "thinking", "processing":
		return AgentWorking
	case "blocked", "permission", "permissions", "question", "confirm", "confirmation", "waiting", "wait":
		return AgentBlocked
	case "aborted", "abort", "cancelled", "canceled", "interrupted", "stopped":
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

func UpdateAgentStatusTracking(ctx context.Context, pane string, detected AgentState, visible bool) (AgentState, error) {
	if pane == "" {
		return detected, nil
	}
	semantic := semanticAgentState(detected)
	previousRaw, _ := showPaneOption(ctx, pane, "@agent_last_state")
	previousStatusRaw, _ := showPaneOption(ctx, pane, "@agent_last_status")
	previousState := semanticAgentState(NormalizeAgentState(previousRaw))
	previousStatus := NormalizeAgentState(previousStatusRaw)
	status := detected
	switch detected {
	case AgentDone:
		if visible {
			status = AgentIdle
		} else {
			status = AgentDone
		}
	case AgentIdle:
		if visible {
			status = AgentIdle
		} else if previousStatus == AgentDone {
			status = AgentDone
		} else if previousState == AgentWorking || previousState == AgentBlocked {
			status = AgentDone
		} else {
			status = AgentIdle
		}
	case AgentWorking, AgentBlocked, AgentAborted, AgentUnknown:
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
	if semantic == AgentIdle {
		_ = setPaneOption(ctx, pane, "@agent_state", string(AgentIdle))
		_ = setPaneOption(ctx, pane, "@agent_last_status", string(AgentIdle))
	}
	_ = setPaneOption(ctx, pane, "@agent_last_state", string(semantic))
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
case "${state}" in done|complete|completed|finished|idle|ready) tmux set-option -qpt "$pane" @agent_state idle 2>/dev/null || true; tmux set-option -qpt "$pane" @agent_last_status idle 2>/dev/null || true ;; esac
if [ -n "${TMUX:-}" ]; then
  tmux switch-client -t "$session_id"
else
  tmux attach-session -t "$session_id"
fi`
	return exec.Command("sh", "-c", script, "seshagy-focus-agent", pane)
}

func KillAgentPane(ctx context.Context, pane string) error {
	if out, err := tmuxCommand(ctx, "kill-pane", "-t", pane).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-pane: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func CaptureAgentPane(ctx context.Context, pane string, lines int) (string, error) {
	args := []string{"capture-pane", "-ep", "-t", pane}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	out, err := tmuxCommand(ctx, args...).Output()
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return string(out), nil
}

func ResolvePane(ctx context.Context, pane string) (string, error) {
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	if pane == "" && InTmux() {
		out, err := tmuxCommand(ctx, "display-message", "-p", "#{pane_id}").Output()
		if err == nil {
			pane = strings.TrimSpace(string(out))
		}
	}
	if pane == "" {
		return "", fmt.Errorf("--pane is required outside tmux")
	}
	out, err := tmuxCommand(ctx, "display-message", "-p", "-t", pane, "#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("pane not found: %s", pane)
	}
	return strings.TrimSpace(string(out)), nil
}

func ReportAgent(ctx context.Context, opts AgentReport) error {
	pane, err := ResolvePane(ctx, opts.Pane)
	if err != nil {
		return err
	}
	name := opts.Name
	if name == "" {
		name, _ = showPaneOption(ctx, pane, "@agent_name")
	}
	if name == "" {
		return fmt.Errorf("--agent/--name is required for hook-based agent reporting")
	}
	state := opts.State
	if state == "" {
		stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
		state = NormalizeAgentState(stateRaw)
	} else {
		state = NormalizeAgentState(string(state))
	}
	visible := paneVisibleNow(ctx, pane)
	_, _ = UpdateAgentStatusTracking(ctx, pane, state, visible)
	updated := fmt.Sprintf("%d", time.Now().Unix())
	_ = setPaneOption(ctx, pane, "@agent_name", name)
	_ = setPaneOption(ctx, pane, "@agent_state", string(semanticAgentState(state)))
	_ = setPaneOption(ctx, pane, "@agent_updated", updated)
	if opts.MessageSeen {
		if opts.Message != "" {
			_ = setPaneOption(ctx, pane, "@agent_message", cleanField(opts.Message))
		} else {
			_ = unsetPaneOption(ctx, pane, "@agent_message")
		}
	}
	if opts.SourceSeen {
		if opts.Source != "" {
			_ = setPaneOption(ctx, pane, "@agent_source", cleanField(opts.Source))
		} else {
			_ = unsetPaneOption(ctx, pane, "@agent_source")
		}
	}
	return nil
}

type AgentReport struct {
	Pane        string
	Name        string
	State       AgentState
	Message     string
	MessageSeen bool
	Source      string
	SourceSeen  bool
}

func ReleaseAgent(ctx context.Context, pane, source string, sourceSeen bool) error {
	resolved, err := ResolvePane(ctx, pane)
	if err != nil {
		return err
	}
	if sourceSeen {
		source = cleanField(source)
		existing, _ := showPaneOption(ctx, resolved, "@agent_source")
		if existing != source {
			return nil
		}
	}
	for _, opt := range []string{"@agent_name", "@agent_state", "@agent_message", "@agent_updated", "@agent_source", "@agent_last_state", "@agent_last_status", "@agent_last_seen"} {
		_ = unsetPaneOption(ctx, resolved, opt)
	}
	return nil
}

func showPaneOption(ctx context.Context, pane, opt string) (string, error) {
	out, err := tmuxCommand(ctx, "show-option", "-qvpt", pane, opt).Output()
	return strings.TrimSpace(string(out)), err
}

func setPaneOption(ctx context.Context, pane, opt, value string) error {
	return tmuxCommand(ctx, "set-option", "-qpt", pane, opt, value).Run()
}

func unsetPaneOption(ctx context.Context, pane, opt string) error {
	return tmuxCommand(ctx, "set-option", "-qupt", pane, opt).Run()
}

func displayPane(ctx context.Context, pane, format string) (string, error) {
	out, err := tmuxCommand(ctx, "display-message", "-p", "-t", pane, format).Output()
	return strings.TrimSpace(string(out)), err
}

func paneVisibleNow(ctx context.Context, pane string) bool {
	out, err := displayPane(ctx, pane, "#{pane_active} #{window_active} #{session_attached}")
	if err != nil {
		return false
	}
	parts := strings.Fields(out)
	return len(parts) >= 3 && parts[0] == "1" && parts[1] == "1" && parts[2] != "0"
}

func cleanField(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
