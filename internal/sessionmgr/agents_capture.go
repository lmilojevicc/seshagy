package sessionmgr

import (
	"context"
	"fmt"
	"time"
)

// CaptureAgentPane captures the bottom N lines of a tmux pane via
// capture-pane. ANSI escape sequences are preserved (-e) so manifest regexes
// can match structured output.
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

type manifestCaptureCache map[string]string

func captureAgentPaneCached(
	ctx context.Context,
	cache manifestCaptureCache,
	pane string,
	lines int,
) (string, error) {
	if cache != nil {
		if screen, ok := cache[pane]; ok {
			return screen, nil
		}
	}
	screen, err := CaptureAgentPane(ctx, pane, lines)
	if err != nil {
		return "", err
	}
	if cache != nil {
		cache[pane] = screen
	}
	return screen, nil
}

// hasFreshHookState returns true when the item has a non-stale @seshagy_agent_state
// report (within the 60s freshness window). Capture-pane only runs when this is
// false, so fresh hook-reported state is never overridden.
func hasFreshHookState(item Item) bool {
	return !item.AgentUpdated.IsZero() && time.Since(item.AgentUpdated) < agentFreshnessWindow
}

// ApplyManifestFallback classifies agent panes that lack fresh hook state via
// capture-pane screen-rule matching. It mutates items[i].AgentState in-memory
// (no tmux option writes). Only runs for panes with no fresh hook state and a
// registered manifest. One capture per pane per sweep (cache).
func ApplyManifestFallback(ctx context.Context, items []Item) {
	cache := make(manifestCaptureCache)
	for i := range items {
		item := items[i]
		if item.AgentName == "" || item.Kind != KindAgent {
			continue
		}
		if hasFreshHookState(item) {
			continue
		}
		if _, ok := manifestForAgent(item.AgentName); !ok {
			continue
		}
		screen, err := captureAgentPaneCached(ctx, cache, item.PaneID, manifestCaptureLines)
		if err != nil {
			continue
		}
		result := detectManifest(item.AgentName, manifestDetectionInput{screen: screen})
		if result.SkipStateUpdate {
			continue
		}
		if result.Matched {
			items[i].AgentState = result.State
		}
	}
}
