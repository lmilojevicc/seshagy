//go:build windows

package remote

import (
	"context"
	"errors"
)

// Launcher control needs unix domain sockets and process signals, so the
// `seshagy run` launcher is unix-only. These stubs keep the CLI compiling on
// windows and fail with an actionable message.

// ErrNoLauncher reports that launcher control is unavailable on this platform.
var ErrNoLauncher = errors.New("`seshagy run` is not supported on windows")

// SwitchRequest asks a running launcher to switch its herdr client.
type SwitchRequest struct {
	Host    string `json:"host"`
	Session string `json:"session,omitempty"`
}

// Target identifies the herdr server a launcher is attached to.
type Target struct {
	Alias       string
	Session     string
	Keybindings string
}

// LaunchOptions configures RunLauncher.
type LaunchOptions struct {
	Initial      Target
	LocalSession string
	Hosts        []Host
}

// Launcher attaches a herdr client to a target (unix only).
type Launcher struct{}

// RunLauncher is unsupported on windows.
func RunLauncher(context.Context, LaunchOptions) error {
	return ErrNoLauncher
}

// RequestSwitch is unsupported on windows.
func RequestSwitch(context.Context, SwitchRequest, string) error {
	return ErrNoLauncher
}
