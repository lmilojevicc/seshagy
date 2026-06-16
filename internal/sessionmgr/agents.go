package sessionmgr

import (
	"context"
	"regexp"
	"strings"
)

const paneSep = "\x1f"

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const (
	agentPaneMinFields = 18
	agentPanePIDIndex  = 18

	agentSourceUnhooked            = "unhooked"
	agentMessageInstallIntegration = "install integration"
)

const agentFormat = "#{pane_id}" + paneSep + "#{session_name}" + paneSep + "#{window_index}" + paneSep + "#{pane_index}" + paneSep + "#{pane_current_path}" + paneSep + "#{pane_active}" + paneSep + "#{window_active}" + paneSep + "#{session_attached}" + paneSep + "#{pane_dead}" + paneSep + "#{pane_current_command}" + paneSep + "#{pane_title}" + paneSep + "#{@agent_name}" + paneSep + "#{@agent_state}" + paneSep + "#{@agent_message}" + paneSep + "#{@agent_updated}" + paneSep + "#{@agent_source}" + paneSep + "#{@agent_session_id}" + paneSep + "#{@agent_seq}" + paneSep + "#{pane_pid}"

func showPaneOption(ctx context.Context, pane, opt string) (string, error) {
	out, err := tmuxOutput(ctx, "show-option", "-qvpt", pane, opt)
	return strings.TrimSpace(string(out)), err
}

// PaneAgentState reads the current @agent_state from a pane.
func PaneAgentState(ctx context.Context, pane string) (AgentState, error) {
	raw, err := showPaneOption(ctx, pane, "@agent_state")
	if err != nil {
		return "", err
	}
	return NormalizeAgentState(raw), nil
}

func setPaneOption(ctx context.Context, pane, opt, value string) error {
	return tmuxRun(ctx, "set-option", "-qpt", pane, opt, value)
}

func unsetPaneOption(ctx context.Context, pane, opt string) error {
	return tmuxRun(ctx, "set-option", "-qupt", pane, opt)
}

func displayPane(ctx context.Context, pane, format string) (string, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", pane, format)
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

func panePIDFromParts(parts []string) string {
	if len(parts) > agentPanePIDIndex {
		return cleanField(parts[agentPanePIDIndex])
	}
	return ""
}

func cleanField(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
