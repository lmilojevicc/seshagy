package sessionmgr

import (
	"context"
	"errors"
	"os/exec"
)

// errNoBackend is returned by noop backend action methods.
var errNoBackend = errors.New("not inside tmux or herdr")

// noopBackend is the Multiplexer used when neither tmux nor herdr is detected.
// Discovery returns empty results; action methods fail clearly; preview methods
// return empty strings so the UI degrades gracefully.
type noopBackend struct{}

func (noopBackend) Kind() BackendKind { return BackendNone }
func (noopBackend) Terms() Terms      { return NoneTerms() }

func (noopBackend) InMultiplexer() bool                              { return false }
func (noopBackend) InMultiplexerPopup(context.Context) (bool, error) { return false, nil }
func (noopBackend) CurrentSession(context.Context) (string, error)   { return "", nil }

func (noopBackend) ListSessions(context.Context) ([]Item, error)       { return nil, nil }
func (noopBackend) ListAgents(context.Context, string) ([]Item, error) { return nil, nil }

func (noopBackend) HasSession(context.Context, string) (bool, error) { return false, nil }
func (noopBackend) CreateSessionFromDir(context.Context, string) (Item, bool, error) {
	return Item{}, false, errNoBackend
}

func (noopBackend) KillSession(ctx context.Context, _ string) error {
	started := sessionKillStart(ctx)
	logSessionKill(ctx, BackendNone, started, errNoBackend)
	return errNoBackend
}
func (noopBackend) RenameSession(context.Context, string, string) error { return errNoBackend }
func (noopBackend) CaptureSession(context.Context, string, int) (string, error) {
	return "", nil
}
func (noopBackend) AttachOrSwitchCommand(Item) *exec.Cmd { return nil }

func (noopBackend) CaptureAgentPane(context.Context, string, int) (string, error) {
	return "", nil
}
func (noopBackend) FocusAgentCommand(Item) *exec.Cmd                { return nil }
func (noopBackend) RenameAgent(context.Context, Item, string) error { return errNoBackend }

func (noopBackend) ResolvePane(context.Context, string) (string, error) {
	return "", errNoBackend
}

func (noopBackend) ResolvePaneByCwd(context.Context, string) (string, error) {
	return "", errNoBackend
}

func (noopBackend) ReportAgent(context.Context, AgentReport) (bool, error) {
	return false, nil
}

func (noopBackend) ReleaseAgent(context.Context, AgentRelease) (bool, error) {
	return false, nil
}

func (noopBackend) MarkAgentVisited(context.Context, string) (bool, error) {
	return false, nil
}
func (noopBackend) MarkActiveDoneAgentsIdle(context.Context, []Item) {}
