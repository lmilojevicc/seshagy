package sessionmgr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// herdr CLI JSON response shapes. Herdr ids (pane_id, workspace_id, tab_id,
// terminal_id) are treated as opaque strings — the id format is
// version-sensitive, so nothing here parses separators.

// paneInfo mirrors the PaneInfo fields seshagy needs from
// `herdr pane list/get/current --json`.
type paneInfo struct {
	PaneID        string `json:"pane_id"`
	TerminalID    string `json:"terminal_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	Label         string `json:"label"`
	Agent         string `json:"agent"`
	DisplayAgent  string `json:"display_agent"`
	AgentStatus   string `json:"agent_status"`
	Focused       bool   `json:"focused"`
}

// workspaceInfo mirrors the WorkspaceInfo fields seshagy needs from
// `herdr workspace list --json`.
type workspaceInfo struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Cwd         string `json:"cwd"`
	Focused     bool   `json:"focused"`
}

// tabInfo mirrors the TabInfo fields seshagy needs from
// `herdr tab list --json`.
type tabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// herdrResult is the unified envelope. The herdr CLI may print a wrapped
// response ({"id":"...","result":{"type":"pane_list","panes":[...]}}) or the
// result payload directly ({"type":"pane_list","panes":[...]}). The Type
// discriminator plus the collection/single fields let one struct handle both.
type herdrResult struct {
	Type       string          `json:"type"`
	Workspaces []workspaceInfo `json:"workspaces"`
	Tabs       []tabInfo       `json:"tabs"`
	Panes      []paneInfo      `json:"panes"`
	Pane       *paneInfo       `json:"pane"`
	Workspace  *workspaceInfo  `json:"workspace"`
}

// herdrResponse is the full wrapped envelope with a top-level "result" object.
type herdrResponse struct {
	Result herdrResult `json:"result"`
}

// unwrapResult decodes raw herdr CLI output into a herdrResult, tolerating both
// the wrapped {"result":{...}} form and a direct result payload.
func unwrapResult(raw []byte) (herdrResult, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return herdrResult{}, fmt.Errorf("empty herdr response")
	}
	// Try wrapped form first.
	var wrapped herdrResponse
	if err := json.Unmarshal(trimmed, &wrapped); err == nil && wrapped.Result.Type != "" {
		return wrapped.Result, nil
	}
	// Direct payload form.
	var direct herdrResult
	if err := json.Unmarshal(trimmed, &direct); err != nil {
		return herdrResult{}, fmt.Errorf("parse herdr response: %w", err)
	}
	return direct, nil
}

// parseHerdrWorkspaces parses `herdr workspace list/create --json` output.
func parseHerdrWorkspaces(raw []byte) ([]workspaceInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	// `workspace create` returns a single workspace under "workspace".
	if res.Workspace != nil && len(res.Workspaces) == 0 {
		return []workspaceInfo{*res.Workspace}, nil
	}
	return res.Workspaces, nil
}

// parseHerdrPanes parses `herdr pane list --json` output.
func parseHerdrPanes(raw []byte) ([]paneInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	return res.Panes, nil
}

// parseHerdrPaneInfo parses `herdr pane get/current --json` output (single pane).
func parseHerdrPaneInfo(raw []byte) (*paneInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	if res.Pane != nil {
		return res.Pane, nil
	}
	// Some responses embed the pane directly without a "result" wrapper and
	// without a type discriminator — try decoding as a bare paneInfo.
	if res.Type == "" && len(res.Panes) == 0 {
		var pane paneInfo
		if err := json.Unmarshal(bytes.TrimSpace(raw), &pane); err == nil && pane.PaneID != "" {
			return &pane, nil
		}
	}
	return nil, fmt.Errorf("no pane info in herdr response")
}

// mapHerdrStatusToAgentState maps a herdr agent_status wire value to the
// internal AgentState enum. Unknown/empty values map to AgentUnknown (NOT idle)
// — under herdr, "unknown" is a real wire value meaning "no detected agent
// state", and must not be conflated with idle.
func mapHerdrStatusToAgentState(status string) AgentState {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "idle":
		return AgentIdle
	case "working":
		return AgentWorking
	case "blocked":
		return AgentBlocked
	case "done":
		return AgentDone
	default: // "unknown", "", or any unrecognized token
		return AgentUnknown
	}
}
