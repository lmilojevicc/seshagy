package sessionmgr

import (
	"cmp"
	"context"
	"strings"
)

type (
	CompletionPane     struct{ ID, Cwd, Agent, Source, Session string }
	CompletionSession  struct{ Target, Label string }
	CompletionSnapshot struct {
		Panes    []CompletionPane
		Sessions []CompletionSession
	}
)

const completionPaneFormat = "#{pane_id}\x1f#{session_name}\x1f#{pane_current_path}\x1f#{pane_current_command}\x1f#{@seshagy_agent_name}\x1f#{@seshagy_agent_source}"

func ReadCompletionSnapshot(ctx context.Context, mux Multiplexer) (CompletionSnapshot, error) {
	switch mux.Kind() {
	case BackendTmux:
		out, err := tmuxOutput(ctx, "list-panes", "-a", "-F", completionPaneFormat)
		if err != nil {
			return CompletionSnapshot{}, err
		}
		return parseTmuxCompletionSnapshot(out), nil
	case BackendHerdr:
		out, err := herdrOutput(ctx, "pane", "list")
		if err != nil {
			return CompletionSnapshot{}, err
		}
		panes, err := parseHerdrPanes(out)
		return herdrCompletionSnapshot(panes), err
	default:
		return CompletionSnapshot{}, nil
	}
}

func ReadCompletionSessions(ctx context.Context, mux Multiplexer) ([]CompletionSession, error) {
	if mux.Kind() != BackendHerdr {
		snapshot, err := ReadCompletionSnapshot(ctx, mux)
		return snapshot.Sessions, err
	}
	out, err := herdrOutput(ctx, "workspace", "list")
	if err != nil {
		return nil, err
	}
	workspaces, err := parseHerdrWorkspaces(out)
	if err != nil {
		return nil, err
	}
	sessions := make([]CompletionSession, 0, len(workspaces))
	for _, workspace := range workspaces {
		sessions = append(sessions, CompletionSession{workspace.WorkspaceID, workspace.Label})
	}
	return sessions, nil
}

func parseTmuxCompletionSnapshot(raw []byte) CompletionSnapshot {
	result := CompletionSnapshot{}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		row := strings.Split(line, "\x1f")
		if len(row) < 6 {
			continue
		}
		agent := cmp.Or(row[4], detectAgentName(row[3]))
		result.Panes = append(result.Panes, CompletionPane{row[0], row[2], agent, row[5], row[1]})
		if row[1] != "" && !seen[row[1]] {
			seen[row[1]] = true
			result.Sessions = append(result.Sessions, CompletionSession{row[1], row[1]})
		}
	}
	return result
}

func herdrCompletionSnapshot(panes []paneInfo) CompletionSnapshot {
	result := CompletionSnapshot{}
	for _, pane := range panes {
		result.Panes = append(result.Panes, CompletionPane{
			ID: pane.PaneID, Cwd: cmp.Or(pane.ForegroundCwd, pane.Cwd),
			Agent: cmp.Or(pane.Agent, pane.DisplayAgent), Session: pane.WorkspaceID,
		})
	}
	return result
}
