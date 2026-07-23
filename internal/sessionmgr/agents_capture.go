package sessionmgr

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/logging"
)

// parseOSCSequences extracts the most-recent OSC window title and progress
// payload from a capture-pane output that preserved escape sequences (-e).
//
// tmux capture-pane -e keeps SGR and OSC sequences inline. The relevant OSC
// patterns used by the bundled codex/claude manifests are:
//
//   - OSC 0 / OSC 2 (set window title): `\x1b]0;<title>\x07` or `\x1b]2;<title>\x07`
//     (also ST-terminated `\x1b\\`). The codex/claude osc_title rules match
//     the title TEXT AFTER StripANSI against patterns like `^[braille] ` or
//     `Action Required`.
//   - OSC 4 (progress, herdr uses `OSC 4;0` for idle): `\x1b]4;<payload>\x07`.
//     The claude osc_progress rule matches `^4;0`, i.e. the payload must
//     INCLUDE the leading `4;` prefix (the manifest regex starts with `4`).
//
// When multiple OSC sequences of the same kind appear, the LAST one wins
// (most-recent state). Returns empty strings when none are present.
func parseOSCSequences(screen string) (title, progress string) {
	// Find all OSC sequences: ESC ] ... (BEL | ST)
	// BEL = 0x07, ST = ESC backslash (0x1b 0x5c).
	start := 0
	for {
		idx := strings.Index(screen[start:], "\x1b]")
		if idx < 0 {
			break
		}
		idx += start
		// payload starts after "ESC ]"
		payloadStart := idx + 2
		// find terminator: BEL or ST
		end := -1
		termLen := 0
		if bell := strings.IndexByte(screen[payloadStart:], '\x07'); bell >= 0 {
			end = payloadStart + bell
			termLen = 1
		} else if st := strings.Index(screen[payloadStart:], "\x1b\\"); st >= 0 {
			end = payloadStart + st
			termLen = 2
		}
		if end < 0 {
			break
		}
		payload := screen[payloadStart:end]
		// OSC 0 or 2: window title.
		if strings.HasPrefix(payload, "0;") || strings.HasPrefix(payload, "2;") {
			// strip the "0;" / "2;" prefix, then strip ANSI SGR sequences.
			rawTitle := payload[2:]
			title = StripANSI(rawTitle)
		} else if strings.HasPrefix(payload, "4;") {
			// OSC 4; progress — keep the full payload INCLUDING the "4;" prefix
			// because the claude osc_progress rule matches `^4;0`.
			progress = StripANSI(payload)
		}
		start = end + termLen
	}
	return title, progress
}

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
	logger := logging.Default()
	enabled := logger.Enabled(ctx, slog.LevelDebug)
	started := time.Time{}
	if enabled {
		started = time.Now()
	}
	cache := make(manifestCaptureCache)
	skipped, matched, changed, warnings := 0, 0, 0, 0
	for i := range items {
		item := items[i]
		if item.AgentName == "" || item.Kind != KindAgent {
			skipped++
			continue
		}
		if !shouldRunManifest(item) {
			skipped++
			continue
		}
		if _, ok := manifestForAgent(item.AgentName); !ok {
			skipped++
			continue
		}
		// Suppress manifest for a short window after a release to prevent
		// capture-pane from visually resurrecting a just-released pane whose
		// screen may still match a working/blocked rule.
		if isRecentlyReleased(ctx, item.PaneID) {
			skipped++
			continue
		}
		screen, err := captureAgentPaneCached(ctx, cache, item.PaneID, manifestCaptureLines)
		if err != nil {
			warnings++
			continue
		}
		title, progress := parseOSCSequences(screen)
		result := detectManifest(item.AgentName, manifestDetectionInput{
			screen:      screen,
			oscTitle:    title,
			oscProgress: progress,
		})
		if result.SkipStateUpdate {
			skipped++
			continue
		}
		if result.Matched {
			matched++
			previous := items[i].AgentState
			items[i].AgentState = result.State
			if previous != result.State {
				changed++
				if !enabled {
					continue
				}
				logging.LogAttrs(
					ctx,
					logger,
					slog.LevelDebug,
					logging.EventManifestStateChange,
					logging.ComponentManifest,
					slog.String("pane_id", item.PaneID),
					slog.String("agent_type", item.AgentName),
					slog.String(
						"previous_state",
						string(previous),
					),
					slog.String("state", string(result.State)),
				)
			}
		}
	}
	if enabled {
		logging.LogAttrs(
			ctx,
			logger,
			slog.LevelDebug,
			logging.EventManifestSweep,
			logging.ComponentManifest,
			slog.Int("item_count", len(items)),
			slog.Int("skipped_count", skipped),
			slog.Int("matched_count", matched),
			slog.Int("changed_count", changed),
			slog.Int(
				"warning_count",
				warnings,
			),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		)
	}
}

// isRecentlyReleased returns true when the pane has a @seshagy_agent_released_at
// timestamp within the manifestReleaseSuppressWindow. This prevents capture-pane
// from visually resurrecting a just-released pane. On error/absence returns false
// (no suppression — current behavior).
func isRecentlyReleased(ctx context.Context, pane string) bool {
	raw, _ := showPaneOption(ctx, pane, "@seshagy_agent_released_at")
	if raw == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return false
	}
	return time.Since(t) < manifestReleaseSuppressWindow
}
