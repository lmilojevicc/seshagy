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
	WindowNoun    string
	WindowPlural  string
	PaneNoun      string
	KillVerbPast  string // past-tense destroy verb ("killed" / "closed")
}

func TmuxTerms() Terms {
	return Terms{
		BackendName:   "tmux",
		SessionNoun:   "session",
		SessionPlural: "sessions",
		WindowNoun:    "window",
		WindowPlural:  "windows",
		PaneNoun:      "pane",
		KillVerbPast:  "killed",
	}
}

func HerdrTerms() Terms {
	return Terms{
		BackendName:   "herdr",
		SessionNoun:   "workspace",
		SessionPlural: "workspaces",
		WindowNoun:    "tab",
		WindowPlural:  "tabs",
		PaneNoun:      "pane",
		KillVerbPast:  "closed",
	}
}

// NoneTerms is the neutral vocabulary used when no multiplexer is detected.
func NoneTerms() Terms {
	return TmuxTerms() // neutral fallback; the "outside tmux/herdr" footer is handled in tui/view.go
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
	KillPane(ctx context.Context, paneID string) error
	RenameSession(ctx context.Context, target, newName string) error
	CaptureSession(ctx context.Context, target string, lines int) (string, error)
	AttachOrSwitchCommand(item Item) *exec.Cmd

	ListAgents(ctx context.Context, sessionFilter string) ([]Item, error)
	// AgentPID returns the pid of the agent process in a pane, so the picker can
	// report its uptime. tmux already knows it from discovery (Item.AgentPID);
	// herdr resolves it with `pane process-info`.
	AgentPID(ctx context.Context, item Item) (int, error)
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

// MachineSelector is implemented only by backends that can target saved
// machines (herdr today). A backend that does not implement it has exactly one
// machine — the local one — so tmux, noop and every test fake keep working
// unchanged.
type MachineSelector interface {
	// Machines lists the saved profiles, including disabled ones so the caller
	// can explain why they are missing. It is a local, cheap call
	// (`herdr machine list --json`: a file read, no server, no SSH), safe on
	// startup and reload but pointless on a refresh tick.
	Machines(ctx context.Context) ([]Machine, error)
	// MachineStatuses probes profiles without prompting for authentication.
	// Every probe is an SSH round trip: call it only on an explicit user action.
	// An empty selector list probes every saved profile.
	MachineStatuses(ctx context.Context, selectors ...string) ([]MachineStatus, error)
	// WithMachine returns the same backend bound to m, so every forwarded
	// command carries `--machine <id>`. Passing the zero Machine (or the local
	// machine) returns the local backend.
	WithMachine(m Machine) Multiplexer
}

// Detect selects the active multiplexer from the process environment.
func Detect() Multiplexer { return DetectFromEnv(os.Getenv) }

// DetectFromEnv is the env-driven, testable form of Detect. Detection priority:
//
//	HERDR_ENV=1 → herdr backend (herdr owns agent state; seshagy is read-only)
//	TMUX set    → tmux backend
//	neither     → noop backend
func DetectFromEnv(getenv func(string) string) Multiplexer {
	if getenv("HERDR_ENV") == "1" {
		return NewHerdrBackend()
	}
	if getenv("TMUX") != "" {
		return NewTmuxBackend()
	}
	return NewNoopBackend()
}

// NewTmuxBackend returns the tmux-backed Multiplexer.
func NewTmuxBackend() Multiplexer { return tmuxBackend{} }

// NewNoopBackend returns a no-op Multiplexer used when no multiplexer is active.
func NewNoopBackend() Multiplexer { return noopBackend{} }

// NewHerdrBackend returns the herdr-backed Multiplexer.
func NewHerdrBackend() Multiplexer { return herdrBackend{} }
