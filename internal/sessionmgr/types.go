package sessionmgr

import "time"

const (
	IconSession   = "󰙀"
	IconZoxide    = "󰉖"
	IconFD        = "󰥩"
	IconWorkspace = "󰙀" // same glyph as session; separate name for herdr config
)

type Kind string

const (
	KindSession Kind = "session"
	KindZoxide  Kind = "zoxide"
	KindFD      Kind = "fd"
	KindAgent   Kind = "agent"
	// KindHost is a remote machine (ssh, iroh, tailscale, ...) the user can
	// switch a full-screen herdr client to via `seshagy run`.
	KindHost Kind = "host"
)

// AgentState is the lifecycle state of a discovered agent pane.
type AgentState string

const (
	AgentIdle    AgentState = "idle"
	AgentWorking AgentState = "working"
	AgentBlocked AgentState = "blocked"
	AgentDone    AgentState = "done"
	AgentUnknown AgentState = "unknown"
)

// AgentStates returns the canonical lifecycle values accepted by agent reports.
func AgentStates() []AgentState {
	return []AgentState{AgentIdle, AgentWorking, AgentBlocked, AgentDone, AgentUnknown}
}

type Item struct {
	Kind Kind

	Name   string
	Target string
	Path   string

	Created  time.Time
	Activity time.Time
	Attached bool
	Windows  int
	Panes    int

	PaneID   string
	Session  string
	Window   string
	Pane     string
	Location string
	TabLabel string

	AgentName        string
	AgentDisplayName string
	AgentState       AgentState
	AgentUpdated     time.Time
	AgentSeq         int64
	AgentSource      string
	// AgentPID is the agent process discovery matched in the pane (tmux's
	// process-tree walk). herdr panes leave it zero and resolve it on demand.
	AgentPID int
	// StartedAt is when the agent process started. It is not part of discovery:
	// the caller resolves it with one batched ps call and stamps it on the item.
	StartedAt time.Time

	// Machine is the herdr saved profile the item lives on (opaque id); empty is
	// the local server. Actions must route through it so an in-flight action
	// never targets the wrong server after a machine switch. It is always empty
	// under tmux/noop, where machines do not exist.
	Machine string
}

// MachineKey names the server an item belongs to, for caches keyed by
// multiplexer id — herdr pane/workspace ids are only unique within one server.
func (i Item) MachineKey() string {
	if i.Machine == "" {
		return "local"
	}
	return i.Machine
}

// ActionTarget returns the multiplexer-actionable identifier for an item: the
// session target (or display name) for sessions, and the pane id for agents.
// tmux session items set Target == Name; herdr workspace items set Target to
// the workspace id (stable) and Name to the display label.
func (i Item) ActionTarget() string {
	if i.Target != "" {
		return i.Target
	}
	switch i.Kind {
	case KindAgent:
		return i.PaneID
	default:
		return i.Name
	}
}

// Key identifies an item within the picker. Local items keep their historical
// key shape; an item on a saved herdr machine is namespaced by that machine
// because two servers can both have `w1:p1`.
func (i Item) Key() string {
	key := i.localKey()
	if i.Machine == "" {
		return key
	}
	return "machine:" + i.Machine + ":" + key
}

func (i Item) localKey() string {
	switch i.Kind {
	case KindSession:
		return string(i.Kind) + ":" + i.Name
	case KindAgent:
		return string(i.Kind) + ":" + i.PaneID
	case KindHost:
		return string(i.Kind) + ":" + i.Name
	default:
		return string(i.Kind) + ":" + i.Path
	}
}

func (i Item) DisplayName() string {
	switch i.Kind {
	case KindAgent:
		if i.AgentDisplayName != "" {
			return i.AgentDisplayName
		}
		return i.AgentName
	case KindZoxide, KindFD:
		return i.Path
	default:
		return i.Name
	}
}
