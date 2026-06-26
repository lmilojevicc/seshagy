package sessionmgr

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AgentReport carries a state update from a hook/extension to seshagy.
type AgentReport struct {
	Pane      string
	Name      string
	State     AgentState
	Source    string
	Seq       int64
	Message   string
	SessionID string
}

// AgentRelease clears all agent state for a pane (session-end tombstone).
type AgentRelease struct {
	Pane   string
	Source string
	Seq    int64
}

// agentPaneOptions returns all @seshagy_agent_* option names that ReportAgent writes.
// ReleaseAgent unsets the state-bearing options (everything except @seshagy_agent_seq)
// and writes @seshagy_agent_seq as a tombstone high-water mark.
func agentPaneOptions() []string {
	return []string{
		"@seshagy_agent_name", "@seshagy_agent_state", "@seshagy_agent_message",
		"@seshagy_agent_updated", "@seshagy_agent_source", "@seshagy_agent_session_id",
		"@seshagy_agent_last_seen", "@seshagy_agent_released_at",
		"@seshagy_agent_seq",
	}
}

// manifestReleaseSuppressWindow is how long after a release the manifest
// classifier is suppressed for that pane. This prevents capture-pane from
// visually resurrecting a just-released pane (whose screen may still match a
// working/blocked rule). After the window the process has likely changed
// screens and manifest can run normally.
const manifestReleaseSuppressWindow = 10 * time.Second

// ResolvePane resolves a pane target to a canonical pane id (%NN).
func ResolvePane(ctx context.Context, pane string) (string, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", pane, "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolvePaneByCwd resolves a working directory to a unique tmux pane id by
// matching against pane_current_path across all panes. Exact match wins; else a
// parent/child prefix match. Refuses (returns "", nil) on 0 or >1 matches so the
// caller can no-op silently — mirroring the opensessions unique-match model.
func ResolvePaneByCwd(ctx context.Context, cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	// Clean + evaluate symlinks on both sides to avoid mismatches on
	// trailing slashes, relative components ("./"), and symlinks.
	cwd = filepath.Clean(cwd)
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	out, err := tmuxOutput(ctx, "list-panes", "-a", "-F", "#{pane_id}\x1f#{pane_current_path}")
	if err != nil {
		return "", err
	}
	var exact, prefix []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		paneID, rawPath, ok := strings.Cut(line, "\x1f")
		if !ok {
			continue
		}
		paneID = strings.TrimSpace(paneID)
		if paneID == "" {
			continue
		}
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "" || path == "." {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		switch {
		case path == cwd:
			exact = append(exact, paneID)
		case strings.HasPrefix(path, cwd+string(filepath.Separator)) || strings.HasPrefix(cwd, path+string(filepath.Separator)):
			prefix = append(prefix, paneID)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) == 0 && len(prefix) == 1 {
		return prefix[0], nil
	}
	return "", nil // 0 or ambiguous — refuse to guess
}

// ReportAgent writes a state update to the pane's @seshagy_agent_* options. It enforces
// the AGENTS.md-mandated sequence invariant: a report with seq <= the existing
// @seshagy_agent_seq is silently ignored so stale updates cannot resurrect cleared
// state. Seq is written FIRST so a crash mid-write leaves the highest seq.
func ReportAgent(ctx context.Context, opts AgentReport) (bool, error) {
	applied := false
	err := withAgentPaneLock(opts.Pane, func() error {
		existing, _ := showPaneOption(ctx, opts.Pane, "@seshagy_agent_seq")
		existingSeq, parseErr := strconv.ParseInt(existing, 10, 64)
		if parseErr == nil && opts.Seq <= existingSeq {
			return nil // stale — strict > guard
		}
		// Clear any prior release tombstone so a fresh report in a reused pane
		// is not manifest-suppressed by the previous release's 10s window.
		_ = unsetPaneOption(ctx, opts.Pane, "@seshagy_agent_released_at")
		// Write seq FIRST so a crash leaves the highest seq, not a stale lower one.
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@seshagy_agent_seq",
			strconv.FormatInt(opts.Seq, 10),
		); err != nil {
			return err
		}
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@seshagy_agent_state",
			string(opts.State),
		); err != nil {
			return err
		}
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@seshagy_agent_updated",
			time.Now().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		if opts.Name != "" {
			if err := setPaneOption(ctx, opts.Pane, "@seshagy_agent_name", opts.Name); err != nil {
				return err
			}
		}
		if opts.Source != "" {
			if err := setPaneOption(
				ctx,
				opts.Pane,
				"@seshagy_agent_source",
				opts.Source,
			); err != nil {
				return err
			}
		}
		if opts.Message != "" {
			if err := setPaneOption(
				ctx,
				opts.Pane,
				"@seshagy_agent_message",
				opts.Message,
			); err != nil {
				return err
			}
		}
		if opts.SessionID != "" {
			if err := setPaneOption(
				ctx,
				opts.Pane,
				"@seshagy_agent_session_id",
				opts.SessionID,
			); err != nil {
				return err
			}
		}
		applied = true
		return nil
	})
	return applied, err
}

// ReleaseAgent clears all state-bearing @seshagy_agent_* options for a pane (tombstone
// semantics). A release with seq < the existing @seshagy_agent_seq is ignored (stale).
// A release at the same seq as the last report is valid (session-end at the
// same epoch).
//
// Unlike a full unset, @seshagy_agent_seq is WRITTEN to the release seq (not unset)
// as a high-water tombstone. This ensures that a late stale report with a
// lower seq is rejected by ReportAgent's strict-> guard — preventing the
// post-release resurrection window where @seshagy_agent_seq="" would let a stale
// report bypass the guard. Visible state (@seshagy_agent_state etc.) is still cleared,
// so ParseAgents reads an empty state and falls back to idle. The tombstone
// seq is written LAST so a crash mid-release leaves the highest seq recorded.
func ReleaseAgent(ctx context.Context, opts AgentRelease) (bool, error) {
	applied := false
	err := withAgentPaneLock(opts.Pane, func() error {
		existing, _ := showPaneOption(ctx, opts.Pane, "@seshagy_agent_seq")
		existingSeq, parseErr := strconv.ParseInt(existing, 10, 64)
		if parseErr == nil && opts.Seq < existingSeq {
			return nil // stale — strict < guard (equal seq is valid for release)
		}
		// Cross-source guard: reject if the releasing source differs from the
		// pane's recorded source. Prevents a delayed release from agent A
		// (e.g. opencode) from clearing agent B's current state in a reused
		// pane. An unidentified releaser (Source="") is also rejected when
		// the pane has a recorded source. When the existing source is empty
		// (never set) or matches the releaser, proceed as normal.
		existingSource, _ := showPaneOption(ctx, opts.Pane, "@seshagy_agent_source")
		if existingSource != "" && existingSource != opts.Source {
			return nil // cross-source / unidentified release — refuse to clear
		}
		// Tombstone: unset all state-bearing options except @seshagy_agent_seq.
		for _, opt := range agentPaneOptions() {
			if opt == "@seshagy_agent_seq" {
				continue
			}
			if err := unsetPaneOption(ctx, opts.Pane, opt); err != nil {
				return err
			}
		}
		// Write @seshagy_agent_released_at as a release timestamp so the
		// manifest classifier can suppress immediate resurrection.
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@seshagy_agent_released_at",
			time.Now().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		// Write @seshagy_agent_seq LAST as the tombstone high-water mark so a crash
		// mid-release leaves the highest seq recorded, and any subsequent
		// stale report (seq <= this) is rejected by ReportAgent's guard.
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@seshagy_agent_seq",
			strconv.FormatInt(opts.Seq, 10),
		); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

// MarkAgentVisited flips a pane from "done" to "idle" when the user visits it,
// recording @seshagy_agent_last_seen. The write is seq-safe: it re-reads
// @seshagy_agent_seq immediately before writing and bails if a newer report
// landed in the meantime, so a visit can never clobber a fresher hook state
// (the AGENTS.md stale-resurrection invariant). The seq itself is left
// unchanged — the idle flip happens at the current epoch, so a subsequent
// higher-seq report still applies normally.
func MarkAgentVisited(ctx context.Context, pane string) (bool, error) {
	flipped := false
	err := withAgentPaneLock(pane, func() error {
		stateRaw, _ := showPaneOption(ctx, pane, "@seshagy_agent_state")
		if NormalizeAgentState(stateRaw) != AgentDone {
			return nil
		}
		// Seq-safe guard: re-read seq before writing. Under the per-pane flock
		// no concurrent writer can interleave, but this defends against external
		// writers that bypass the lock (best-effort) and preserves the invariant
		// that a visit never overwrites a newer report.
		seqBefore, _ := showPaneOption(ctx, pane, "@seshagy_agent_seq")
		seqNow, _ := showPaneOption(ctx, pane, "@seshagy_agent_seq")
		if seqNow != seqBefore {
			return nil
		}
		now := time.Now().Format(time.RFC3339Nano)
		if err := setPaneOption(ctx, pane, "@seshagy_agent_state", string(AgentIdle)); err != nil {
			return err
		}
		if err := setPaneOption(ctx, pane, "@seshagy_agent_updated", now); err != nil {
			return err
		}
		if err := setPaneOption(ctx, pane, "@seshagy_agent_last_seen", now); err != nil {
			return err
		}
		flipped = true
		return nil
	})
	return flipped, err
}

// MarkActiveDoneAgentsIdle is the refresh-loop backstop for done→idle-on-visit.
// For each agent pane currently in the "done" state that is also the active
// pane of its session, it calls MarkAgentVisited to flip it to idle. This
// covers direct tmux navigation that bypasses seshagy's Enter-focus path.
// Only one detection tmux call (list-panes) is issued when a done agent exists;
// per-pane flips then issue a bounded number of option writes.
func MarkActiveDoneAgentsIdle(ctx context.Context, items []Item) {
	var done []Item
	for i := range items {
		if items[i].Kind == KindAgent && items[i].AgentState == AgentDone && items[i].PaneID != "" {
			done = append(done, items[i])
		}
	}
	if len(done) == 0 {
		return
	}
	out, err := tmuxOutput(ctx, "list-panes", "-a", "-f", "#{pane_active}", "-F", "#{pane_id}")
	if err != nil {
		return
	}
	active := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			active[line] = true
		}
	}
	for _, it := range done {
		if active[it.PaneID] {
			_, _ = MarkAgentVisited(ctx, it.PaneID)
		}
	}
}
