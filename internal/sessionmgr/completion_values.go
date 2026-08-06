package sessionmgr

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// CompletionPane is the minimal, read-only pane metadata used by shell completion.
type CompletionPane struct {
	ID     string
	Cwd    string
	Agent  string
	Source string
}

const (
	completionPaneFormat      = "#{pane_id}\x1f#{pane_current_path}\x1f#{pane_dead}"
	completionAgentPaneFormat = "#{pane_id}\x1f#{pane_current_path}\x1f#{pane_current_command}\x1f#{pane_pid}\x1f#{pane_dead}\x1f#{@seshagy_agent_source}\x1f#{@seshagy_agent_name}"
)

// CompletionPanes performs the lightweight pane/cwd read. It never walks the
// process tree, captures pane content, or changes agent lifecycle state.
func CompletionPanes(ctx context.Context, mux Multiplexer) ([]CompletionPane, error) {
	return completionPanes(ctx, mux, false)
}

// CompletionAgentPanes adds agent/source discovery for value kinds that need
// it. It remains read-only, but may take one bounded process snapshot to find a
// node-hosted agent that has not reported metadata yet.
func CompletionAgentPanes(ctx context.Context, mux Multiplexer) ([]CompletionPane, error) {
	return completionPanes(ctx, mux, true)
}

func completionPanes(
	ctx context.Context,
	mux Multiplexer,
	detectAgents bool,
) ([]CompletionPane, error) {
	switch mux.Kind() {
	case BackendTmux:
		format := completionPaneFormat
		if detectAgents {
			format = completionAgentPaneFormat
		}
		out, err := tmuxOutput(ctx, "list-panes", "-a", "-F", format)
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				return nil, nil
			}
			return nil, fmt.Errorf("tmux list-panes: %w", err)
		}
		if detectAgents {
			return parseTmuxCompletionAgentPanes(ctx, out), nil
		}
		return parseTmuxCompletionPanes(out), nil
	case BackendHerdr:
		out, err := herdrOutput(ctx, "pane", "list")
		if err != nil {
			return nil, fmt.Errorf("herdr pane list: %w", err)
		}
		panes, err := parseHerdrPanes(out)
		if err != nil {
			return nil, err
		}
		values := make([]CompletionPane, 0, len(panes))
		for _, pane := range panes {
			cwd := pane.ForegroundCwd
			if cwd == "" {
				cwd = pane.Cwd
			}
			value := CompletionPane{ID: pane.PaneID, Cwd: cwd}
			if detectAgents {
				value.Agent = pane.Agent
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, nil
	}
}

func parseTmuxCompletionPanes(raw []byte) []CompletionPane {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	values := make([]CompletionPane, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\x1f")
		if len(parts) != 3 || parts[0] == "" || parts[2] == "1" {
			continue
		}
		values = append(values, CompletionPane{ID: parts[0], Cwd: parts[1]})
	}
	return values
}

func parseTmuxCompletionAgentPanes(ctx context.Context, raw []byte) []CompletionPane {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	values := make([]CompletionPane, 0, len(lines))
	var processTable map[int32]procEntry
	processTableLoaded := false
	for _, line := range lines {
		parts := strings.Split(line, "\x1f")
		if len(parts) != 7 || parts[0] == "" || parts[4] == "1" {
			continue
		}
		agent := parts[6]
		if agent == "" {
			agent = detectAgentName(parts[2])
		}
		if agent == "" {
			if !processTableLoaded {
				processTable, _ = completionProcessSnapshot(ctx)
				processTableLoaded = true
			}
			if panePID, err := strconv.Atoi(parts[3]); err == nil {
				for _, child := range descendants(processTable, int32(panePID)) {
					if agent = detectAgentName(processTable[child].comm); agent != "" {
						break
					}
				}
			}
		}
		values = append(values, CompletionPane{
			ID: parts[0], Cwd: parts[1], Agent: agent, Source: parts[5],
		})
	}
	return values
}

var completionProcessSnapshot = func(ctx context.Context) (map[int32]procEntry, error) {
	args := []string{"-o", "pid=,ppid=,comm="}
	switch runtime.GOOS {
	case "darwin", "freebsd", "netbsd", "openbsd":
		args = append([]string{"-ax"}, args...)
	default:
		args = append([]string{"-e"}, args...)
	}
	out, err := exec.CommandContext(ctx, "ps", args...).Output()
	if err != nil {
		return nil, err
	}
	return parsePsSnapshot(out), nil
}

// CompletionSessions performs only the minimum backend call required for
// rendered delete candidates.
func CompletionSessions(ctx context.Context, mux Multiplexer) ([]Item, error) {
	switch mux.Kind() {
	case BackendTmux:
		return ListSessions(ctx)
	case BackendHerdr:
		out, err := herdrOutput(ctx, "workspace", "list")
		if err != nil {
			return nil, fmt.Errorf("herdr workspace list: %w", err)
		}
		workspaces, err := parseHerdrWorkspaces(out)
		if err != nil {
			return nil, err
		}
		items := make([]Item, 0, len(workspaces))
		for _, workspace := range workspaces {
			name := workspace.Label
			if name == "" {
				name = workspace.WorkspaceID
			}
			items = append(
				items,
				Item{
					Kind:     KindSession,
					Name:     name,
					Target:   workspace.WorkspaceID,
					Path:     workspace.Cwd,
					Attached: workspace.Focused,
					Panes:    workspace.PaneCount,
				},
			)
		}
		return items, nil
	default:
		return nil, nil
	}
}
