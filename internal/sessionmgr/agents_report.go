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
		out, err := tmuxCommand(ctx, "display-message", "-p", "#{pane_id}").Output()
		if err == nil {
			pane = strings.TrimSpace(string(out))
		}
	}
	if pane == "" {
		return "", fmt.Errorf("--pane is required outside tmux")
	}
	out, err := tmuxCommand(ctx, "display-message", "-p", "-t", pane, "#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("pane not found: %s", pane)
	}
	return strings.TrimSpace(string(out)), nil
}

func ReportAgent(ctx context.Context, opts AgentReport) error {
	pane, err := ResolvePane(ctx, opts.Pane)
	if err != nil {
		return err
	}
	return withAgentPaneLock(pane, func() error {
		return reportAgentLocked(ctx, pane, opts)
	})
}

func reportAgentLocked(ctx context.Context, pane string, opts AgentReport) error {
	name := opts.Name
	if name == "" {
		name, _ = showPaneOption(ctx, pane, "@agent_name")
	}
	if name == "" {
		return fmt.Errorf("--agent/--name is required for hook-based agent reporting")
	}
	state := opts.State
	if state == "" {
		stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
		state = NormalizeAgentState(stateRaw)
	} else {
		state = NormalizeAgentState(string(state))
	}
	if !agentSeqStillCurrent(ctx, pane, opts.Seq, opts.SeqSeen) {
		return nil
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
			return nil
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
	return nil
}

func ReleaseAgent(ctx context.Context, opts AgentRelease) error {
	resolved, err := ResolvePane(ctx, opts.Pane)
	if err != nil {
		return err
	}
	return withAgentPaneLock(resolved, func() error {
		return releaseAgentLocked(ctx, resolved, opts)
	})
}

func releaseAgentLocked(ctx context.Context, resolved string, opts AgentRelease) error {
	if opts.SourceSeen {
		source := cleanField(opts.Source)
		existing, _ := showPaneOption(ctx, resolved, "@agent_source")
		if existing != "" && existing != source {
			return nil
		}
		if existing == "" {
			name, _ := showPaneOption(ctx, resolved, "@agent_name")
			state, _ := showPaneOption(ctx, resolved, "@agent_state")
			sessionID, _ := showPaneOption(ctx, resolved, "@agent_session_id")
			if name != "" || state != "" || sessionID != "" {
				return nil
			}
		}
	}
	if !agentSeqStillCurrent(ctx, resolved, opts.Seq, opts.SeqSeen) {
		return nil
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
			return nil
		}
	}
	// Clear metadata unconditionally — seq ownership was established above.
	for _, opt := range agentPaneOptions() {
		if opts.SeqSeen && opt == "@agent_seq" {
			continue
		}
		_ = unsetPaneOption(ctx, resolved, opt)
	}
	return nil
}

func withAgentPaneLock(pane string, fn func() error) error {
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
	if !seqSeen {
		return true
	}
	existingSeq, _ := showPaneOption(ctx, pane, "@agent_seq")
	return shouldApplyAgentSeq(existingSeq, seq, true)
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
		return true
	}
	return incoming > existingSeq
}
