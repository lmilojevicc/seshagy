package sessionmgr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type AgentReport struct {
	Pane          string
	Name          string
	State         AgentState
	Message       string
	MessageSeen   bool
	Source        string
	SourceSeen    bool
	SessionID     string
	SessionIDSeen bool
	Seq           int64
	SeqSeen       bool
}

type AgentRelease struct {
	Pane       string
	Source     string
	SourceSeen bool
	Seq        int64
	SeqSeen    bool
}

func ResolvePane(ctx context.Context, pane string) (string, error) {
	if pane == "" {
		pane = os.Getenv("TMUX_PANE")
	}
	if pane == "" && InTmux() {
		out, err := tmuxOutput(ctx, "display-message", "-p", "#{pane_id}")
		if err == nil {
			pane = strings.TrimSpace(string(out))
		}
	}
	if pane == "" {
		return "", fmt.Errorf("--pane is required outside tmux")
	}
	out, err := tmuxOutput(ctx, "display-message", "-p", "-t", pane, "#{pane_id}")
	if err != nil {
		return "", fmt.Errorf("pane not found: %s", pane)
	}
	return strings.TrimSpace(string(out)), nil
}

func ReportAgent(ctx context.Context, opts AgentReport) (bool, error) {
	pane, err := ResolvePane(ctx, opts.Pane)
	if err != nil {
		return false, err
	}
	var applied bool
	err = withAgentPaneLock(pane, func() error {
		var lockErr error
		applied, lockErr = reportAgentLocked(ctx, pane, opts)
		return lockErr
	})
	return applied, err
}

func reportAgentLocked(ctx context.Context, pane string, opts AgentReport) (bool, error) {
	name := opts.Name
	if name == "" {
		name, _ = showPaneOption(ctx, pane, "@agent_name")
	}
	if name == "" {
		return false, fmt.Errorf("--agent/--name is required for hook-based agent reporting")
	}
	state := opts.State
	if state == "" {
		stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
		state = NormalizeAgentState(stateRaw)
	} else {
		state = NormalizeAgentState(string(state))
	}
	if !agentSeqStillCurrent(ctx, pane, opts.Seq, opts.SeqSeen) {
		return false, nil
	}
	if !opts.SeqSeen {
		existingSeq, _ := showPaneOption(ctx, pane, "@agent_seq")
		if strings.TrimSpace(existingSeq) != "" {
			return false, nil
		}
	}
	// Write seq FIRST so concurrent reports with higher seq can't have
	// their options overwritten by a lower-seq write that passed the
	// pre-check but arrived after them.
	if opts.SeqSeen {
		if !setAgentPaneOptionIfCurrent(
			ctx,
			pane,
			"@agent_seq",
			strconv.FormatInt(opts.Seq, 10),
			opts.Seq,
			opts.SeqSeen,
		) {
			return false, nil
		}
	}
	visible := paneVisibleNow(ctx, pane)
	lifecycle := HasLifecycleAuthority(name, opts.Source)
	_, _ = UpdateAgentStatusTracking(ctx, pane, state, visible, lifecycle)
	updated := fmt.Sprintf("%d", time.Now().Unix())
	_ = setPaneOption(ctx, pane, "@agent_name", name)
	_ = setPaneOption(ctx, pane, "@agent_state", string(semanticAgentState(state)))
	_ = setPaneOption(ctx, pane, "@agent_updated", updated)
	if opts.MessageSeen {
		if opts.Message != "" {
			_ = setPaneOption(ctx, pane, "@agent_message", cleanField(opts.Message))
		} else {
			_ = unsetPaneOption(ctx, pane, "@agent_message")
		}
	}
	if opts.SourceSeen {
		if opts.Source != "" {
			_ = setPaneOption(ctx, pane, "@agent_source", cleanField(opts.Source))
		} else {
			_ = unsetPaneOption(ctx, pane, "@agent_source")
		}
	}
	if opts.SessionIDSeen {
		if opts.SessionID != "" {
			_ = setPaneOption(ctx, pane, "@agent_session_id", cleanField(opts.SessionID))
		} else {
			_ = unsetPaneOption(ctx, pane, "@agent_session_id")
		}
	}
	return true, nil
}

func ReleaseAgent(ctx context.Context, opts AgentRelease) (bool, error) {
	resolved, err := ResolvePane(ctx, opts.Pane)
	if err != nil {
		return false, err
	}
	var released bool
	err = withAgentPaneLock(resolved, func() error {
		released = releaseAgentLocked(ctx, resolved, opts)
		return nil
	})
	return released, err
}

func releaseAgentLocked(ctx context.Context, resolved string, opts AgentRelease) bool {
	if opts.SourceSeen {
		source := cleanField(opts.Source)
		existing, _ := showPaneOption(ctx, resolved, "@agent_source")
		if existing != "" && existing != source {
			return false
		}
		if existing == "" {
			name, _ := showPaneOption(ctx, resolved, "@agent_name")
			state, _ := showPaneOption(ctx, resolved, "@agent_state")
			sessionID, _ := showPaneOption(ctx, resolved, "@agent_session_id")
			if name != "" || state != "" || sessionID != "" {
				return false
			}
		}
	}
	if !agentSeqStillCurrent(ctx, resolved, opts.Seq, opts.SeqSeen) {
		return false
	}
	if !opts.SeqSeen {
		existingSeq, _ := showPaneOption(ctx, resolved, "@agent_seq")
		if strings.TrimSpace(existingSeq) != "" {
			return false
		}
	}
	// Write seq first to claim the epoch with strict > comparison.
	if opts.SeqSeen {
		if !setAgentPaneOptionIfCurrent(
			ctx,
			resolved,
			"@agent_seq",
			strconv.FormatInt(opts.Seq, 10),
			opts.Seq,
			true,
		) {
			return false
		}
	}
	// Clear metadata unconditionally — seq ownership was established above.
	for _, opt := range agentPaneOptions() {
		if opts.SeqSeen && opt == "@agent_seq" {
			continue
		}
		_ = unsetPaneOption(ctx, resolved, opt)
	}
	return true
}

// agentPaneLockHook is overridden in tests to simulate lock acquisition failures.
var agentPaneLockHook func(pane string, fn func() error) error

func withAgentPaneLock(pane string, fn func() error) error {
	if agentPaneLockHook != nil {
		return agentPaneLockHook(pane, fn)
	}
	path := agentLockPath(pane)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func agentLockPath(pane string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", ".", "_", "%", "p")
	name := replacer.Replace(cleanField(pane))
	if name == "" {
		name = "unknown"
	}
	return filepath.Join(os.TempDir(), "seshagy-agent-"+name+".lock")
}

func agentPaneOptions() []string {
	return []string{
		"@agent_name",
		"@agent_state",
		"@agent_message",
		"@agent_updated",
		"@agent_source",
		"@agent_session_id",
		"@agent_seq",
		"@agent_last_state",
		"@agent_last_status",
		"@agent_last_seen",
		"@agent_startup_grace",
		"@agent_pending_idle_since",
		"@agent_pending_idle_count",
	}
}

func agentSeqStillCurrent(ctx context.Context, pane string, seq int64, seqSeen bool) bool {
	existingSeq, _ := showPaneOption(ctx, pane, "@agent_seq")
	return shouldApplyAgentSeq(existingSeq, seq, seqSeen)
}

func setAgentPaneOptionIfCurrent(
	ctx context.Context,
	pane, opt, value string,
	seq int64,
	seqSeen bool,
) bool {
	if !agentSeqStillCurrent(ctx, pane, seq, seqSeen) {
		return false
	}
	_ = setPaneOption(ctx, pane, opt, value)
	return true
}

func shouldApplyAgentSeq(existing string, incoming int64, incomingSeen bool) bool {
	if !incomingSeen {
		return true
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return true
	}
	existingSeq, err := strconv.ParseInt(existing, 10, 64)
	if err != nil {
		return false
	}
	return incoming > existingSeq
}
