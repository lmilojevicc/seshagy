package sessionmgr

import (
	"context"
	"fmt"
	"time"

	"github.com/lmilojevicc/seshagy/internal/integrations"
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
// report (within the 60s freshness window).
func hasFreshHookState(item Item) bool {
	return !item.AgentUpdated.IsZero() && time.Since(item.AgentUpdated) < agentFreshnessWindow
}

// shouldRunManifest decides whether capture-pane screen-rule detection should
// run for this pane. It follows the herdr authority model:
//
//   - Lifecycle-authority agents (pi, opencode): suppress manifest when hooks
//     are fresh; fall back to manifest when hooks go stale.
//   - Non-lifecycle agents (codex, claude, droid, cursor, agy, grok): ALWAYS
//     run manifest, even when hooks are fresh. Their hooks miss approval-result
//     and ESC transitions, so the screen is the authoritative state source and
//     can overwrite stale hook state within one poll.
func shouldRunManifest(item Item) bool {
	if integrations.LifecycleAuthorityFor(item.AgentName) && hasFreshHookState(item) {
		return false
	}
	return true
}

// ApplyManifestFallback classifies agent panes via capture-pane screen-rule
// matching. It mutates items[i].AgentState in-memory (no tmux option writes).
// For lifecycle-authority agents with fresh hooks it is skipped entirely. For
// non-lifecycle agents it ALWAYS runs and overwrites on a positive match so the
// screen can correct stale hook state (ESC/approval-lag fix). One capture per
// pane per sweep (cache).
func ApplyManifestFallback(ctx context.Context, items []Item) {
	cache := make(manifestCaptureCache)
	for i := range items {
		item := items[i]
		if item.AgentName == "" || item.Kind != KindAgent {
			continue
		}
		if !shouldRunManifest(item) {
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
