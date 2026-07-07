package sessionmgr

import (
	"context"
	"os"
	"strings"
)

// TmuxFocusLost reports whether the tmux pane/window/session hosting seshagy
// has lost focus. It mirrors the dismissed() logic of the former bash launcher:
// dismiss when the pane no longer exists, its window is no longer active, or no
// client remains attached to the session.
//
// paneID defaults to $TMUX_PANE; sessionName is resolved once from the pane
// (via `display-message -t <pane> '#{session_name}'`) when empty, which also
// serves as a pane-existence probe.
func TmuxFocusLost(ctx context.Context, paneID, sessionName string) bool {
	if paneID == "" {
		paneID = os.Getenv("TMUX_PANE")
		if paneID == "" {
			return false // not inside a tmux pane — cannot determine focus
		}
	}
	if sessionName == "" {
		out, err := tmuxOutput(ctx, "display-message", "-p", "-t", paneID, "#{session_name}")
		if err != nil {
			return true // pane gone / unresolvable → dismiss
		}
		sessionName = strings.TrimSpace(string(out))
		if sessionName == "" {
			return true
		}
	}
	// Window still the active window of its session?
	winOut, err := tmuxOutput(ctx, "display-message", "-p", "-t", paneID, "#{window_active}")
	if err != nil {
		return true
	}
	if strings.TrimSpace(string(winOut)) != "1" {
		return true
	}
	// Any client still attached to the session?
	clientsOut, err := tmuxOutput(ctx, "list-clients", "-F", "#{client_session}")
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(clientsOut), "\n") {
		if strings.TrimSpace(line) == sessionName {
			return false
		}
	}
	return true
}

// ResolveHerdrEphemeralTarget finds the pane/workspace this seshagy process
// runs in, for use by HerdrFocusLost. It prefers $HERDR_PANE_ID /
// $HERDR_WORKSPACE_ID (set by herdr when launching a pane command); when either
// is unset it discovers the currently focused pane via `herdr pane list`
// (seshagy's pane at launch). ok is false when neither works.
func ResolveHerdrEphemeralTarget(ctx context.Context) (paneID, workspaceID string, ok bool) {
	paneID = os.Getenv("HERDR_PANE_ID")
	workspaceID = os.Getenv("HERDR_WORKSPACE_ID")
	if paneID != "" && workspaceID != "" {
		return paneID, workspaceID, true
	}
	raw, err := herdrOutput(ctx, "pane", "list")
	if err != nil {
		return "", "", false
	}
	panes, err := parseHerdrPanes(raw)
	if err != nil {
		return "", "", false
	}
	for _, p := range panes {
		if p.Focused {
			return p.PaneID, p.WorkspaceID, true
		}
	}
	return "", "", false
}

// HerdrFocusLost reports whether the herdr pane/workspace hosting seshagy has
// lost focus. It mirrors the dismissed() logic of the former bash launcher's
// herdr branch: dismiss when the pane is no longer focused, or the focused
// workspace has changed away from workspaceID.
//
// A `herdr pane get` failure is treated as focus lost (dismiss). A
// `herdr workspace list` failure is ignored (keep polling), matching the
// script's `|| true` on the workspace lookup.
func HerdrFocusLost(ctx context.Context, paneID, workspaceID string) bool {
	if paneID == "" {
		return false // cannot identify the pane — do not dismiss
	}
	raw, err := herdrOutput(ctx, "pane", "get", paneID)
	if err != nil {
		return true // pane query failed → treat as lost focus
	}
	pane, err := parseHerdrPaneInfo(raw)
	if err != nil || !pane.Focused {
		return true
	}
	if workspaceID == "" {
		return false // no workspace to compare → keep
	}
	wsRaw, err := herdrOutput(ctx, "workspace", "list")
	if err != nil {
		return false // workspace lookup failed → ignore this check
	}
	workspaces, err := parseHerdrWorkspaces(wsRaw)
	if err != nil {
		return false
	}
	for _, ws := range workspaces {
		if ws.Focused && ws.WorkspaceID != workspaceID {
			return true
		}
	}
	return false
}
