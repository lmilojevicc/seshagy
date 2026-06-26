package sessionmgr

import (
	"context"
	"os"
	"os/exec"
)

// BackendKind identifies which terminal multiplexer seshagy is operating on.
type BackendKind string

const (
	BackendNone  BackendKind = "none"
	BackendTmux  BackendKind = "tmux"
	BackendHerdr BackendKind = "herdr"
)

// Terms is the multiplexer-specific user-facing vocabulary. The UI adapts its
// labels (session vs workspace, window vs tab) from this.
type Terms struct {
	BackendName   string
	SessionNoun   string
	SessionPlural string
	SessionTitle  string
	WindowNoun    string
	WindowPlural  string
	WindowTitle   string
	PaneNoun      string
	PanePlural    string
}

func TmuxTerms() Terms {
	return Terms{
		BackendName:   "tmux",
		SessionNoun:   "session",
		SessionPlural: "sessions",
		SessionTitle:  "Session",
		WindowNoun:    "window",
		WindowPlural:  "windows",
		WindowTitle:   "Window",
		PaneNoun:      "pane",
		PanePlural:    "panes",
	}
}

func HerdrTerms() Terms {
	return Terms{
		BackendName:   "herdr",
		SessionNoun:   "workspace",
		SessionPlural: "workspaces",
		SessionTitle:  "Workspace",
		WindowNoun:    "tab",
		WindowPlural:  "tabs",
		WindowTitle:   "Tab",
		PaneNoun:      "pane",
		PanePlural:    "panes",
	}
}

// NoneTerms is the neutral vocabulary used when no multiplexer is detected.
func NoneTerms() Terms {
	return TmuxTerms() // neutral fallback; Phase 5 will differentiate "outside tmux/herdr"
}

// Multiplexer abstracts the terminal multiplexer seshagy drives. Both the tmux
// and herdr backends implement it so the TUI/CLI layers never reference tmux
// directly. The session/agent discovery, lifecycle actions, and pane-preview
// capture all flow through this interface.
type Multiplexer interface {
	Kind() BackendKind
	Terms() Terms

	InMultiplexer() bool
	InMultiplexerPopup(ctx context.Context) (bool, error)
	CurrentSession(ctx context.Context) (string, error)

	ListSessions(ctx context.Context) ([]Item, error)
	HasSession(ctx context.Context, target string) (bool, error)
	CreateSessionFromDir(ctx context.Context, dir string) (Item, bool, error)
	KillSession(ctx context.Context, target string) error
	RenameSession(ctx context.Context, target, newName string) error
	CaptureSession(ctx context.Context, target string, lines int) (string, error)
	AttachOrSwitchCommand(item Item) *exec.Cmd

	ListAgents(ctx context.Context, sessionFilter string) ([]Item, error)
	CaptureAgentPane(ctx context.Context, paneID string, lines int) (string, error)
	FocusAgentCommand(item Item) *exec.Cmd
	RenameAgent(ctx context.Context, item Item, displayName string) error

	ResolvePane(ctx context.Context, pane string) (string, error)
	ResolvePaneByCwd(ctx context.Context, cwd string) (string, error)
	ReportAgent(ctx context.Context, report AgentReport) (bool, error)
	ReleaseAgent(ctx context.Context, release AgentRelease) (bool, error)
	MarkAgentVisited(ctx context.Context, paneID string) (bool, error)
	MarkActiveDoneAgentsIdle(ctx context.Context, items []Item)
}

// Detect selects the active multiplexer from the process environment.
func Detect() Multiplexer { return DetectFromEnv(os.Getenv) }

// DetectFromEnv is the env-driven, testable form of Detect. Detection priority:
//
//	HERDR_ENV=1 → herdr backend (Phase 4 wires this up; until then detection
//	              falls through to tmux/noop so the branch stays green)
//	TMUX set    → tmux backend
//	neither     → noop backend
func DetectFromEnv(getenv func(string) string) Multiplexer {
	// herdr backend (Phase 4): HERDR_ENV=1 → NewHerdrBackend(). Until the
	// herdr backend exists, fall through so detection stays correct for tmux.
	if getenv("TMUX") != "" {
		return NewTmuxBackend()
	}
	return NewNoopBackend()
}

// NewTmuxBackend returns the tmux-backed Multiplexer.
func NewTmuxBackend() Multiplexer { return tmuxBackend{} }

// NewNoopBackend returns a no-op Multiplexer used when no multiplexer is active.
func NewNoopBackend() Multiplexer { return noopBackend{} }
