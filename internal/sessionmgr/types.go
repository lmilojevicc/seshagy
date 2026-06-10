package sessionmgr

import "time"

const (
	IconSession = ""
	IconZoxide  = "󰉖"
	IconFD      = "󰥩"
	IconAgent   = ""
)

type Kind string

const (
	KindSession Kind = "session"
	KindAgent   Kind = "agent"
	KindZoxide  Kind = "zoxide"
	KindFD      Kind = "fd"
)

type AgentState string

const (
	AgentWorking AgentState = "working"
	AgentBlocked AgentState = "blocked"
	AgentAborted AgentState = "aborted"
	AgentDone    AgentState = "done"
	AgentIdle    AgentState = "idle"
	AgentUnknown AgentState = "unknown"
)

type Item struct {
	Kind Kind

	Name   string
	Target string
	Path   string

	Created  time.Time
	Activity time.Time
	Attached bool
	Windows  int

	PaneID   string
	Session  string
	Window   string
	Pane     string
	Location string

	AgentName    string
	AgentState   AgentState
	AgentMessage string
	AgentSource  string
	AgentUpdated string
	Visible      bool
}

func (i Item) Key() string {
	switch i.Kind {
	case KindSession:
		return string(i.Kind) + ":" + i.Name
	case KindAgent:
		return string(i.Kind) + ":" + i.PaneID
	default:
		return string(i.Kind) + ":" + i.Path
	}
}

func (i Item) DisplayName() string {
	switch i.Kind {
	case KindAgent:
		if i.AgentName != "" {
			return i.AgentName
		}
		return i.PaneID
	case KindZoxide, KindFD:
		return i.Path
	default:
		return i.Name
	}
}
