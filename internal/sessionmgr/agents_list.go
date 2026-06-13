package sessionmgr

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

func ListAgents(ctx context.Context, sessionFilter string, opts LoadOptions) ([]Item, error) {
	out, err := tmuxCommand(ctx, "list-panes", "-a", "-F", agentFormat).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	items := ParseAgents(out, sessionFilter)
	if opts.ManifestFallback {
		applyManifestFallback(ctx, items)
	}
	for i := range items {
		pane := items[i].PaneID
		detected := items[i].AgentState
		visible := items[i].Visible
		lifecycle := HasLifecycleAuthority(items[i].AgentName, items[i].AgentSource)
		var state AgentState
		// Hold the per-pane lock so a concurrent hook report/release cannot
		// interleave with the tracking-option writes below.
		err := withAgentPaneLock(pane, func() error {
			s, trackErr := UpdateAgentStatusTracking(
				ctx,
				pane,
				detected,
				visible,
				lifecycle,
			)
			state = s
			return trackErr
		})
		if err == nil {
			items[i].AgentState = state
		}
	}
	return items, nil
}

func applyManifestFallback(ctx context.Context, items []Item) {
	for i := range items {
		if !shouldApplyManifestFallback(
			items[i].AgentState,
			items[i].AgentName,
			items[i].AgentSource,
		) {
			continue
		}
		screen, err := CaptureAgentPane(ctx, items[i].PaneID, manifestCaptureLines)
		if err != nil {
			continue
		}
		match, ok := detectStateFromManifest(items[i].AgentName, screen)
		if !ok {
			continue
		}
		items[i].AgentState = match.State
	}
}

func ParseAgents(raw []byte, sessionFilter string) []Item {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	var items []Item
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Split(line, paneSep)
		if len(parts) < 18 {
			continue
		}
		if parts[8] == "1" {
			continue
		}
		if sessionFilter != "" && parts[1] != sessionFilter {
			continue
		}
		name := parts[11]
		hookReported := name != ""
		title := cleanField(parts[10])
		if name == "" {
			command := cleanField(parts[9])
			name = detectAgentName(command, title)
			if name == "" {
				continue
			}
			if integrations.HookCapableAgent(name) {
				continue
			}
		}
		source := cleanField(parts[15])
		if source == "" && !hookReported {
			source = "process"
		}
		state := resolveAgentState(parts[12], name, source, title)
		message := cleanField(parts[13])
		sessionID := cleanField(parts[16])
		seq := cleanField(parts[17])
		path := ContractHome(parts[4])
		location := fmt.Sprintf("%s:%s.%s", parts[1], parts[2], parts[3])
		items = append(items, Item{
			Kind:           KindAgent,
			Name:           name,
			Target:         parts[0],
			PaneID:         parts[0],
			Session:        parts[1],
			Window:         parts[2],
			Pane:           parts[3],
			Path:           path,
			Location:       location,
			AgentName:      name,
			AgentState:     state,
			AgentMessage:   message,
			AgentUpdated:   cleanField(parts[14]),
			AgentSource:    source,
			AgentSessionID: sessionID,
			AgentSeq:       seq,
			Visible:        parts[5] == "1" && parts[6] == "1" && parts[7] != "0",
		})
	}
	return items
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
