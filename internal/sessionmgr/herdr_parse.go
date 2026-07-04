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
// `herdr pane list/get/current` (JSON is always emitted to stdout).
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
// `herdr workspace list` (JSON is always emitted to stdout).
type workspaceInfo struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Cwd         string `json:"cwd"`
	Focused     bool   `json:"focused"`
}

// tabInfo mirrors the TabInfo fields seshagy needs from
// `herdr tab list` (JSON is always emitted to stdout).
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
	Agents     []agentInfo     `json:"agents"`
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

// parseHerdrWorkspaces parses `herdr workspace list` output (JSON on stdout).
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

// parseHerdrPanes parses `herdr pane list` output (JSON on stdout).
func parseHerdrPanes(raw []byte) ([]paneInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	return res.Panes, nil
}

// parseHerdrPaneInfo parses `herdr pane get/current` output (single pane, JSON on stdout).
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

// agentInfo mirrors the AgentInfo fields seshagy needs from `herdr agent list`.
// Option<String> fields in the herdr source are omitted from JSON when None;
// pointer types here decode missing keys to nil.
type agentInfo struct {
	TerminalID             string  `json:"terminal_id"`
	Name                   *string `json:"name"`
	Agent                  *string `json:"agent"`
	AgentStatus            string  `json:"agent_status"`
	WorkspaceID            string  `json:"workspace_id"`
	TabID                  string  `json:"tab_id"`
	PaneID                 string  `json:"pane_id"`
	Focused                bool    `json:"focused"`
	DisplayAgent           *string `json:"display_agent"`
	Title                  *string `json:"title"`
	CustomStatus           *string `json:"custom_status"`
	ScreenDetectionSkipped bool    `json:"screen_detection_skipped"`
	Cwd                    *string `json:"cwd"`
	ForegroundCwd          *string `json:"foreground_cwd"`
	Revision               uint64  `json:"revision"`
}

// ptrStr safely dereferences a *string, returning "" for nil.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// parseHerdrTabs parses `herdr tab list` output (JSON on stdout).
func parseHerdrTabs(raw []byte) ([]tabInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	return res.Tabs, nil
}

// parseHerdrAgents parses `herdr agent list` output (JSON on stdout).
func parseHerdrAgents(raw []byte) ([]agentInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return nil, err
	}
	return res.Agents, nil
}

// parseHerdrWorkspaceCreated parses the `herdr workspace create` response,
// which embeds a single workspace under result.workspace.
func parseHerdrWorkspaceCreated(raw []byte) (workspaceInfo, error) {
	res, err := unwrapResult(raw)
	if err != nil {
		return workspaceInfo{}, err
	}
	if res.Workspace != nil {
		return *res.Workspace, nil
	}
	return workspaceInfo{}, fmt.Errorf("no workspace in herdr create response")
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
