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
	out, err := tmuxOutput(ctx, "list-panes", "-a", "-F", agentFormat)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	items := ParseAgents(out, sessionFilter, opts)
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
			name, _ := showPaneOption(ctx, pane, "@agent_name")
			stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
			if strings.TrimSpace(name) == "" && strings.TrimSpace(stateRaw) == "" {
				state = detected
				return nil
			}
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

func TrackAgentPane(ctx context.Context, pane string, opts LoadOptions) (AgentState, error) {
	resolved, err := ResolvePane(ctx, pane)
	if err != nil {
		return "", err
	}
	line, err := displayPane(ctx, resolved, agentFormat)
	if err != nil {
		return "", fmt.Errorf("pane metadata: %w", err)
	}
	items := ParseAgents([]byte(line), "", opts)
	if len(items) == 0 {
		return AgentUnknown, fmt.Errorf("pane %s is not a listed agent pane", resolved)
	}
	item := items[0]
	detected := item.AgentState
	if opts.ManifestFallback {
		applyManifestFallback(ctx, items)
		detected = items[0].AgentState
	}
	lifecycle := HasLifecycleAuthority(item.AgentName, item.AgentSource)
	var state AgentState
	err = withAgentPaneLock(resolved, func() error {
		name, _ := showPaneOption(ctx, resolved, "@agent_name")
		stateRaw, _ := showPaneOption(ctx, resolved, "@agent_state")
		if strings.TrimSpace(name) == "" && strings.TrimSpace(stateRaw) == "" {
			state = detected
			return nil
		}
		s, trackErr := UpdateAgentStatusTracking(
			ctx,
			resolved,
			detected,
			item.Visible,
			lifecycle,
		)
		state = s
		return trackErr
	})
	return state, err
}

func applyManifestFallback(ctx context.Context, items []Item) {
	cache := make(manifestCaptureCache)
	for i := range items {
		if !shouldApplyManifestFallback(
			items[i].AgentState,
			items[i].AgentName,
			items[i].AgentSource,
		) {
			continue
		}
		screen, err := captureAgentPaneCached(ctx, cache, items[i].PaneID, manifestCaptureLines)
		if err != nil {
			continue
		}
		result := detectManifest(items[i].AgentName, manifestDetectionInput{
			screen:      screen,
			oscTitle:    strings.TrimSpace(StripANSI(items[i].PaneTitle)),
			oscProgress: "",
		})
		if result.SkipStateUpdate {
			continue
		}
		if result.Matched && result.State != AgentUnknown {
			items[i].AgentState = result.State
		}
	}
}

func ParseAgents(raw []byte, sessionFilter string, opts LoadOptions) []Item {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	var items []Item
	for _, line := range strings.Split(text, "\n") {
		parts := strings.Split(line, paneSep)
		if len(parts) < agentPaneMinFields {
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
		panePID := panePIDFromParts(parts)
		unhooked := false
		if name == "" {
			command := cleanField(parts[9])
			name = detectAgentName(command, title, panePID)
			if name == "" {
				continue
			}
			if integrations.HookCapableAgent(name) {
				unhooked = true
			}
		}
		source := cleanField(parts[15])
		switch {
		case unhooked:
			source = agentSourceUnhooked
		case source == "" && !hookReported:
			source = "process"
		}
		agentUpdated := cleanField(parts[14])
		state := resolveAgentStateAt(
			parts[12],
			name,
			source,
			title,
			agentUpdated,
			opts.ManifestFallback,
			agentResolveNow(),
		)
		message := cleanField(parts[13])
		if unhooked && message == "" {
			message = agentMessageInstallIntegration
		}
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
			PaneTitle:      title,
			Visible:        parts[5] == "1" && parts[6] == "1" && parts[7] != "0",
		})
	}
	return items
}

func KillAgentPane(ctx context.Context, pane string) error {
	if err := tmuxRun(ctx, "kill-pane", "-t", pane); err != nil {
		return fmt.Errorf("tmux kill-pane: %w", err)
	}
	return nil
}

func CaptureAgentPane(ctx context.Context, pane string, lines int) (string, error) {
	args := []string{"capture-pane", "-ep", "-t", pane}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	out, err := tmuxOutput(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return string(out), nil
}
