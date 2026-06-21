package sessionmgr

import "time"

const (
	IconSession = ""
	IconZoxide  = "󰉖"
	IconFD      = "󰥩"
)

type Kind string

const (
	KindSession Kind = "session"
	KindZoxide  Kind = "zoxide"
	KindFD      Kind = "fd"
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
}

func (i Item) Key() string {
	switch i.Kind {
	case KindSession:
		return string(i.Kind) + ":" + i.Name
	default:
		return string(i.Kind) + ":" + i.Path
	}
}

func (i Item) DisplayName() string {
	switch i.Kind {
	case KindZoxide, KindFD:
		return i.Path
	default:
		return i.Name
	}
}
