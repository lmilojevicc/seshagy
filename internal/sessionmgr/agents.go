package sessionmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const paneSep = "\x1f"

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const agentFormat = "#{pane_id}" + paneSep + "#{session_name}" + paneSep + "#{window_index}" + paneSep + "#{pane_index}" + paneSep + "#{pane_current_path}" + paneSep + "#{pane_active}" + paneSep + "#{window_active}" + paneSep + "#{session_attached}" + paneSep + "#{pane_dead}" + paneSep + "#{pane_current_command}" + paneSep + "#{pane_title}" + paneSep + "#{@agent_name}" + paneSep + "#{@agent_state}" + paneSep + "#{@agent_message}" + paneSep + "#{@agent_updated}" + paneSep + "#{@agent_source}" + paneSep + "#{@agent_session_id}" + paneSep + "#{@agent_seq}"

func ListAgents(ctx context.Context, sessionFilter string) ([]Item, error) {
	out, err := tmuxCommand(ctx, "list-panes", "-a", "-F", agentFormat).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	items := ParseAgents(out, sessionFilter)
	for i := range items {
		state, err := UpdateAgentStatusTracking(
			ctx,
			items[i].PaneID,
			items[i].AgentState,
			items[i].Visible,
		)
		if err == nil {
			items[i].AgentState = state
		}
	}
	return items, nil
}

func detectAgentName(command, title string) string {
	command = filepath.Base(command)
	command = strings.TrimSuffix(command, ".exe")
	command = strings.TrimSuffix(command, ".cmd")
	command = strings.TrimSuffix(command, ".bat")
	command = strings.TrimSuffix(command, ".ps1")
	command = strings.TrimSuffix(command, ".js")
	commandLower := strings.ToLower(command)
	titleLower := strings.ToLower(title)

	// Match bash's is_shell_command: only actual interactive shells, not runtimes.
	// This gates the π/pi title fallback — node/bun/python are NOT shells here.
	isShell := func(name string) bool {
		switch name {
		case "sh", "bash", "zsh", "fish", "tmux":
			return true
		}
		return false
	}

	// Match bash behavior: π title check is gated by !isShell, and comes before
	// command-based matching. A pane running "node" with title "π - foo" IS detected
	// as pi, but a pane running "zsh" with title "π - foo" is NOT.
	if strings.HasPrefix(titleLower, "π") || strings.HasPrefix(titleLower, "pi ") {
		if !isShell(commandLower) {
			return "pi"
		}
	}

	// isRuntime gates the other title fallbacks (claude, opencode, codex, etc.).
	// Includes interpreters/runtimes that could appear as pane_current_command
	// without being agents themselves.
	isRuntime := func(name string) bool {
		switch name {
		case "sh", "bash", "zsh", "fish", "tmux",
			"node", "bun", "python", "python3",
			"cmd", "powershell", "pwsh":
			return true
		}
		return false
	}

	switch commandLower {
	case "pi":
		return "pi"
	case "claude", "claude-code":
		return "claude"
	case "opencode", "open-code":
		return "opencode"
	case "codex":
		return "codex"
	case "droid":
		return "droid"
	case "gemini":
		return "gemini"
	case "cursor", "cursor-agent":
		return "cursor"
	case "agy", "antigravity", "antigravity-cli":
		return "agy"
	case "cline":
		return "cline"
	case "copilot", "github-copilot", "ghcs":
		return "copilot"
	case "kimi", "kimi-code":
		return "kimi"
	case "kiro", "kiro-cli":
		return "kiro"
	case "amp", "amp-local":
		return "amp"
	case "grok", "grok-build":
		return "grok"
	case "hermes", "hermes-agent":
		return "hermes"
	case "kilo", "kilo-code":
		return "kilo"
	case "qodercli", "qoderclicn", "qoder", "qodercn":
		return "qodercli"
	}

	// Wildcard matching for agents with variant binaries (e.g. codex-local, droid-agent).
	if strings.HasPrefix(commandLower, "codex-") || strings.HasPrefix(commandLower, "codex_") {
		return "codex"
	}
	if strings.HasPrefix(commandLower, "droid-") || strings.HasPrefix(commandLower, "droid_") {
		return "droid"
	}

	if isRuntime(commandLower) {
		return ""
	}

	if strings.Contains(titleLower, "claude code") {
		return "claude"
	}
	if strings.Contains(titleLower, "opencode") {
		return "opencode"
	}
	if strings.HasPrefix(titleLower, "codex") || strings.HasPrefix(titleLower, "codex -") {
		return "codex"
	}
	if strings.HasPrefix(titleLower, "droid") || strings.HasPrefix(titleLower, "droid -") {
		return "droid"
	}
	if strings.HasPrefix(titleLower, "gemini") || strings.HasPrefix(titleLower, "gemini -") {
		return "gemini"
	}
	if strings.Contains(titleLower, "cursor") {
		return "cursor"
	}

	return ""
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
		if name == "" {
			command := cleanField(parts[9])
			title := cleanField(parts[10])
			name = detectAgentName(command, title)
			if name == "" {
				continue
			}
		}
		state := NormalizeAgentState(parts[12])
		message := cleanField(parts[13])
		source := cleanField(parts[15])
		if source == "" && parts[11] == "" {
			source = "process"
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
			Visible:        parts[5] == "1" && parts[6] == "1" && parts[7] != "0",
		})
	}
	return items
}

func NormalizeAgentState(state string) AgentState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "working", "busy", "running", "thinking", "processing":
		return AgentWorking
	case "blocked",
		"permission",
		"permissions",
		"question",
		"confirm",
		"confirmation",
		"waiting",
		"wait":
		return AgentBlocked
	case "aborted", "abort", "cancelled", "canceled", "interrupted", "stopped",
		"error", "failed", "failure",
		"timeout", "timed_out", "timed-out",
		"disconnected", "offline":
		return AgentAborted
	case "done", "complete", "completed", "finished":
		return AgentDone
	case "idle", "ready":
		return AgentIdle
	default:
		return AgentUnknown
	}
}

func semanticAgentState(state AgentState) AgentState {
	if state == AgentDone {
		return AgentIdle
	}
	return state
}

func agentStateLabel(state AgentState) string {
	if state == "" {
		return string(AgentUnknown)
	}
	return string(state)
}

func UpdateAgentStatusTracking(
	ctx context.Context,
	pane string,
	detected AgentState,
	visible bool,
) (AgentState, error) {
	if pane == "" {
		return detected, nil
	}
	semantic := semanticAgentState(detected)
	previousRaw, _ := showPaneOption(ctx, pane, "@agent_last_state")
	previousStatusRaw, _ := showPaneOption(ctx, pane, "@agent_last_status")
	previousState := semanticAgentState(NormalizeAgentState(previousRaw))
	previousStatus := NormalizeAgentState(previousStatusRaw)
	var status AgentState
	switch detected {
	case AgentDone:
		if visible {
			status = AgentIdle
		} else {
			status = AgentDone
		}
	case AgentAborted:
		if visible {
			status = AgentIdle
		} else {
			status = AgentAborted
		}
	case AgentIdle:
		if visible {
			status = AgentIdle
		} else if previousStatus == AgentDone {
			status = AgentDone
		} else if previousStatus == AgentAborted {
			status = AgentAborted
		} else if previousState == AgentWorking || previousState == AgentBlocked {
			status = AgentDone
		} else {
			status = AgentIdle
		}
	case AgentWorking, AgentBlocked, AgentUnknown:
		status = detected
	default:
		status = AgentUnknown
	}
	_ = setPaneOption(ctx, pane, "@agent_last_state", string(semantic))
	_ = setPaneOption(ctx, pane, "@agent_last_status", string(status))
	if visible {
		_ = setPaneOption(ctx, pane, "@agent_last_seen", fmt.Sprintf("%d", time.Now().Unix()))
	}
	return status, nil
}

func MarkAgentSeen(ctx context.Context, pane string) {
	stateRaw, _ := showPaneOption(ctx, pane, "@agent_state")
	semantic := semanticAgentState(NormalizeAgentState(stateRaw))
	if semantic == AgentIdle || semantic == AgentAborted {
		// Use seq-safe writes to avoid overwriting concurrent hook reports.
		seqRaw, _ := showPaneOption(ctx, pane, "@agent_seq")
		seq, err := strconv.ParseInt(strings.TrimSpace(seqRaw), 10, 64)
		seqSeen := err == nil && seqRaw != ""
		setAgentPaneOptionIfCurrent(ctx, pane, "@agent_state", string(AgentIdle), seq, seqSeen)
		setAgentPaneOptionIfCurrent(
			ctx,
			pane,
			"@agent_last_status",
			string(AgentIdle),
			seq,
			seqSeen,
		)
	}
	_ = setPaneOption(ctx, pane, "@agent_last_seen", fmt.Sprintf("%d", time.Now().Unix()))
}

func FocusAgentCommand(pane string) *exec.Cmd {
	// Keep the whole focus flow in one foreground process so Bubble Tea can suspend once.
	script := `set -e
pane="$1"
session_id=$(tmux display-message -p -t "$pane" '#{session_id}')
window_id=$(tmux display-message -p -t "$pane" '#{window_id}')
tmux select-window -t "$window_id"
tmux select-pane -t "$pane"
tmux set-option -qpt "$pane" @agent_last_seen "$(date +%s)" 2>/dev/null || true
state=$(tmux show-option -qvpt "$pane" @agent_state 2>/dev/null || true)
case "${state}" in done|complete|completed|finished|idle|ready|aborted|cancelled|canceled|stopped|error|failed|timeout) tmux set-option -qpt "$pane" @agent_state idle 2>/dev/null || true; tmux set-option -qpt "$pane" @agent_last_status idle 2>/dev/null || true ;; esac
if [ -n "${TMUX:-}" ]; then
  tmux switch-client -t "$session_id"
else
  tmux attach-session -t "$session_id"
fi`
	return exec.Command("sh", "-c", script, "seshagy-focus-agent", pane)
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
	_, _ = UpdateAgentStatusTracking(ctx, pane, state, visible)
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

func showPaneOption(ctx context.Context, pane, opt string) (string, error) {
	out, err := tmuxCommand(ctx, "show-option", "-qvpt", pane, opt).Output()
	return strings.TrimSpace(string(out)), err
}

func setPaneOption(ctx context.Context, pane, opt, value string) error {
	return tmuxCommand(ctx, "set-option", "-qpt", pane, opt, value).Run()
}

func unsetPaneOption(ctx context.Context, pane, opt string) error {
	return tmuxCommand(ctx, "set-option", "-qupt", pane, opt).Run()
}

func displayPane(ctx context.Context, pane, format string) (string, error) {
	out, err := tmuxCommand(ctx, "display-message", "-p", "-t", pane, format).Output()
	return strings.TrimSpace(string(out)), err
}

func paneVisibleNow(ctx context.Context, pane string) bool {
	out, err := displayPane(ctx, pane, "#{pane_active} #{window_active} #{session_attached}")
	if err != nil {
		return false
	}
	parts := strings.Fields(out)
	return len(parts) >= 3 && parts[0] == "1" && parts[1] == "1" && parts[2] != "0"
}

func cleanField(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
