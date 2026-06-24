package sessionmgr

import (
	"context"
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

// agentPaneOptions returns all @agent_* option names that ReportAgent writes.
// ReleaseAgent unsets the state-bearing options (everything except @agent_seq)
// and writes @agent_seq as a tombstone high-water mark.
func agentPaneOptions() []string {
	return []string{
		"@agent_name", "@agent_state", "@agent_message",
		"@agent_updated", "@agent_source", "@agent_session_id",
		"@agent_seq",
	}
}

// ResolvePane resolves a pane target to a canonical pane id (%NN).
func ResolvePane(ctx context.Context, pane string) (string, error) {
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", pane, "#{pane_id}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ReportAgent writes a state update to the pane's @agent_* options. It enforces
// the AGENTS.md-mandated sequence invariant: a report with seq <= the existing
// @agent_seq is silently ignored so stale updates cannot resurrect cleared
// state. Seq is written FIRST so a crash mid-write leaves the highest seq.
func ReportAgent(ctx context.Context, opts AgentReport) (bool, error) {
	applied := false
	err := withAgentPaneLock(opts.Pane, func() error {
		existing, _ := showPaneOption(ctx, opts.Pane, "@agent_seq")
		existingSeq, parseErr := strconv.ParseInt(existing, 10, 64)
		if parseErr == nil && opts.Seq <= existingSeq {
			return nil // stale — strict > guard
		}
		// Write seq FIRST so a crash leaves the highest seq, not a stale lower one.
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@agent_seq",
			strconv.FormatInt(opts.Seq, 10),
		); err != nil {
			return err
		}
		if err := setPaneOption(ctx, opts.Pane, "@agent_state", string(opts.State)); err != nil {
			return err
		}
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@agent_updated",
			time.Now().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		if opts.Name != "" {
			if err := setPaneOption(ctx, opts.Pane, "@agent_name", opts.Name); err != nil {
				return err
			}
		}
		if opts.Source != "" {
			if err := setPaneOption(ctx, opts.Pane, "@agent_source", opts.Source); err != nil {
				return err
			}
		}
		if opts.Message != "" {
			if err := setPaneOption(ctx, opts.Pane, "@agent_message", opts.Message); err != nil {
				return err
			}
		}
		if opts.SessionID != "" {
			if err := setPaneOption(
				ctx,
				opts.Pane,
				"@agent_session_id",
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

// ReleaseAgent clears all state-bearing @agent_* options for a pane (tombstone
// semantics). A release with seq < the existing @agent_seq is ignored (stale).
// A release at the same seq as the last report is valid (session-end at the
// same epoch).
//
// Unlike a full unset, @agent_seq is WRITTEN to the release seq (not unset)
// as a high-water tombstone. This ensures that a late stale report with a
// lower seq is rejected by ReportAgent's strict-> guard — preventing the
// post-release resurrection window where @agent_seq="" would let a stale
// report bypass the guard. Visible state (@agent_state etc.) is still cleared,
// so ParseAgents reads an empty state and falls back to idle. The tombstone
// seq is written LAST so a crash mid-release leaves the highest seq recorded.
func ReleaseAgent(ctx context.Context, opts AgentRelease) (bool, error) {
	applied := false
	err := withAgentPaneLock(opts.Pane, func() error {
		existing, _ := showPaneOption(ctx, opts.Pane, "@agent_seq")
		existingSeq, parseErr := strconv.ParseInt(existing, 10, 64)
		if parseErr == nil && opts.Seq < existingSeq {
			return nil // stale — strict < guard (equal seq is valid for release)
		}
		// Tombstone: unset all state-bearing options except @agent_seq.
		for _, opt := range agentPaneOptions() {
			if opt == "@agent_seq" {
				continue
			}
			if err := unsetPaneOption(ctx, opts.Pane, opt); err != nil {
				return err
			}
		}
		// Write @agent_seq LAST as the tombstone high-water mark so a crash
		// mid-release leaves the highest seq recorded, and any subsequent
		// stale report (seq <= this) is rejected by ReportAgent's guard.
		if err := setPaneOption(
			ctx,
			opts.Pane,
			"@agent_seq",
			strconv.FormatInt(opts.Seq, 10),
		); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}
