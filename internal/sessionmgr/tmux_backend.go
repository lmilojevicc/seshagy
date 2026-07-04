package sessionmgr

import (
	"context"
	"os/exec"
)

// tmuxBackend adapts the existing tmux free functions to the Multiplexer
// interface. It contains no logic of its own — every method delegates to the
// package-level tmux helpers. This keeps the refactor behaviour-preserving:
// the tmux code paths are unchanged, only routed through an interface.
type tmuxBackend struct{}

func (tmuxBackend) Kind() BackendKind { return BackendTmux }
func (tmuxBackend) Terms() Terms      { return TmuxTerms() }

func (tmuxBackend) InMultiplexer() bool { return InTmux() }
func (tmuxBackend) InMultiplexerPopup(ctx context.Context) (bool, error) {
	return InTmuxPopup(ctx)
}

func (tmuxBackend) CurrentSession(ctx context.Context) (string, error) {
	return CurrentTmuxSession(ctx)
}

func (tmuxBackend) ListSessions(ctx context.Context) ([]Item, error) {
	return ListSessions(ctx)
}

func (tmuxBackend) HasSession(ctx context.Context, target string) (bool, error) {
	return HasSession(ctx, target)
}

func (tmuxBackend) CreateSessionFromDir(ctx context.Context, dir string) (Item, bool, error) {
	name, reused, err := CreateSessionFromDir(ctx, dir)
	if err != nil {
		return Item{}, reused, err
	}
	return Item{Kind: KindSession, Name: name, Target: name}, reused, nil
}

func (tmuxBackend) KillSession(ctx context.Context, target string) error {
	return KillSession(ctx, target)
}

func (tmuxBackend) RenameSession(ctx context.Context, target, newName string) error {
	return RenameSession(ctx, target, newName)
}

func (tmuxBackend) CaptureSession(ctx context.Context, target string, lines int) (string, error) {
	return CaptureSession(ctx, target, lines)
}

func (tmuxBackend) AttachOrSwitchCommand(item Item) *exec.Cmd {
	return AttachOrSwitchCommand(item.ActionTarget())
}

func (tmuxBackend) ListAgents(ctx context.Context, sessionFilter string) ([]Item, error) {
	return ListAgents(ctx, sessionFilter)
}

func (tmuxBackend) CaptureAgentPane(ctx context.Context, paneID string, lines int) (string, error) {
	return CaptureAgentPane(ctx, paneID, lines)
}

func (tmuxBackend) FocusAgentCommand(item Item) *exec.Cmd {
	return FocusAgentCommand(item.Session, item.Window, item.PaneID)
}

func (tmuxBackend) RenameAgent(_ context.Context, item Item, displayName string) error {
	return SaveAgentLabel(item.AgentName, item.Session, displayName)
}

func (tmuxBackend) ResolvePane(ctx context.Context, pane string) (string, error) {
	return ResolvePane(ctx, pane)
}

func (tmuxBackend) ResolvePaneByCwd(ctx context.Context, cwd string) (string, error) {
	return ResolvePaneByCwd(ctx, cwd)
}

func (tmuxBackend) ReportAgent(ctx context.Context, report AgentReport) (bool, error) {
	return ReportAgent(ctx, report)
}

func (tmuxBackend) ReleaseAgent(ctx context.Context, release AgentRelease) (bool, error) {
	return ReleaseAgent(ctx, release)
}

func (tmuxBackend) MarkAgentVisited(ctx context.Context, paneID string) (bool, error) {
	return MarkAgentVisited(ctx, paneID)
}

func (tmuxBackend) MarkActiveDoneAgentsIdle(ctx context.Context, items []Item) {
	MarkActiveDoneAgentsIdle(ctx, items)
}
