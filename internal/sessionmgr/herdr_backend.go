package sessionmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// herdrBackend implements Multiplexer against the herdr CLI. seshagy runs
// inside a herdr-managed pane, so "attach" is a server-side workspace focus.
// Agent state is READ-ONLY: herdr owns detection, so the report/release/visited
// methods are no-ops.
type herdrBackend struct{}

func (herdrBackend) Kind() BackendKind { return BackendHerdr }
func (herdrBackend) Terms() Terms      { return HerdrTerms() }

func (herdrBackend) InMultiplexer() bool { return os.Getenv("HERDR_ENV") == "1" }

func (herdrBackend) InMultiplexerPopup(context.Context) (bool, error) {
	return false, nil // no popup equivalent in v1
}

func (herdrBackend) CurrentSession(ctx context.Context) (string, error) {
	// Prefer the env var set inside managed panes — avoids a CLI round-trip.
	if id := os.Getenv("HERDR_WORKSPACE_ID"); id != "" {
		return id, nil
	}
	out, _ := herdrOutput(ctx, "pane", "current") // graceful: no CLI → no current session
	pane, _ := parseHerdrPaneInfo(out)
	if pane == nil {
		return "", nil
	}
	return pane.WorkspaceID, nil
}

func (herdrBackend) ListSessions(ctx context.Context) ([]Item, error) {
	out, err := herdrOutput(ctx, "workspace", "list")
	if err != nil {
		return nil, fmt.Errorf("herdr workspace list: %w", err)
	}
	workspaces, err := parseHerdrWorkspaces(out)
	if err != nil {
		return nil, err
	}
	// A single `herdr tab list` (no args) returns every tab across all
	// workspaces — group it by workspace_id to populate tab counts without a
	// per-workspace round-trip per refresh.
	tabCounts := map[string]int{}
	for _, t := range herdrTabs(ctx) {
		tabCounts[t.WorkspaceID]++
	}
	// Per-workspace representative path from panes: prefer the focused pane's
	// cwd, else the first pane's cwd. `workspace list` exposes no cwd.
	paths := herdrWorkspacePaths(ctx)
	items := make([]Item, 0, len(workspaces))
	for _, ws := range workspaces {
		name := ws.Label
		if name == "" {
			name = ws.WorkspaceID
		}
		path := paths[ws.WorkspaceID]
		if path == "" {
			path = ws.Cwd
		}
		items = append(items, Item{
			Kind:     KindSession,
			Name:     name,
			Target:   ws.WorkspaceID,
			Path:     path,
			Attached: ws.Focused,
			Windows:  tabCounts[ws.WorkspaceID],
			Panes:    ws.PaneCount,
		})
	}
	return items, nil
}

func (b herdrBackend) HasSession(ctx context.Context, target string) (bool, error) {
	sessions, err := b.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.Target == target {
			return true, nil
		}
	}
	return false, nil
}

func (b herdrBackend) CreateSessionFromDir(
	ctx context.Context,
	dir string,
) (Item, bool, error) {
	dir = ExpandHome(dir)
	sessions, err := b.ListSessions(ctx)
	if err != nil {
		return Item{}, false, err
	}
	// Reuse a workspace already rooted at dir (normalized path match).
	want := normalizePath(dir)
	for _, s := range sessions {
		if normalizePath(s.Path) == want {
			return Item{Kind: KindSession, Name: s.Name, Target: s.Target}, true, nil
		}
	}
	label := SessionNameFromDir(dir)
	out, err := herdrOutput(
		ctx,
		"workspace",
		"create",
		"--cwd",
		dir,
		"--label",
		label,
		"--focus",
	)
	if err != nil {
		return Item{}, false, fmt.Errorf("herdr workspace create: %w", err)
	}
	ws, _ := parseHerdrWorkspaceCreated(out) // create succeeded; parse failure → best-effort
	name := ws.Label
	if name == "" {
		name = label
	}
	return Item{Kind: KindSession, Name: name, Target: ws.WorkspaceID}, false, nil
}

func (herdrBackend) KillSession(ctx context.Context, target string) error {
	if err := herdrRun(ctx, "workspace", "close", target); err != nil {
		return fmt.Errorf("herdr workspace close: %w", err)
	}
	// herdr workspace close refocuses the client onto another workspace.
	// Restore focus to the workspace seshagy runs in so the user isn't moved —
	// unless they just closed that workspace themselves. Best-effort: the
	// close already succeeded. (No query between close and focus — fastest
	// restore, least flicker.)
	if cur, err := (herdrBackend{}).CurrentSession(ctx); err == nil && cur != "" && cur != target {
		_ = herdrRun(ctx, "workspace", "focus", cur)
	}
	return nil
}

func (herdrBackend) RenameSession(ctx context.Context, target, newName string) error {
	if err := herdrRun(ctx, "workspace", "rename", target, newName); err != nil {
		return fmt.Errorf("herdr workspace rename: %w", err)
	}
	return nil
}

// CaptureSession approximates tmux's session-level capture by reading the
// focused (or first) pane in the workspace. herdr has no session-level
// capture-pane equivalent.
func (b herdrBackend) CaptureSession(
	ctx context.Context,
	target string,
	lines int,
) (string, error) {
	out, _ := herdrOutput(
		ctx,
		"pane",
		"list",
		"--workspace",
		target,
	) // graceful degradation
	panes, _ := parseHerdrPanes(out)
	if len(panes) == 0 {
		return "", nil
	}
	// Prefer the focused pane; else the first.
	paneID := panes[0].PaneID
	for _, p := range panes {
		if p.Focused {
			paneID = p.PaneID
			break
		}
	}
	return b.CaptureAgentPane(ctx, paneID, lines)
}

func (herdrBackend) AttachOrSwitchCommand(item Item) *exec.Cmd {
	return exec.Command("herdr", "workspace", "focus", item.ActionTarget())
}

func (herdrBackend) ListAgents(
	ctx context.Context,
	sessionFilter string,
) ([]Item, error) {
	// `herdr agent list` returns only agent panes (no plain shells) and takes
	// no flags. Workspace filtering is done in-memory since the CLI does not
	// accept a --workspace flag for this command.
	out, err := herdrOutput(ctx, "agent", "list")
	if err != nil {
		return nil, fmt.Errorf("herdr agent list: %w", err)
	}
	agents, err := parseHerdrAgents(out)
	if err != nil {
		return nil, err
	}
	// Resolve workspace ids → labels and tab ids → labels once, via two cheap
	// global CLI calls (`workspace list`, `tab list`), so the trailing location
	// text and the detail panel show human-facing names, not opaque ids.
	workspaceLabels := herdrWorkspaceLabels(ctx)
	tabLabels := herdrTabLabels(ctx)
	items := make([]Item, 0, len(agents))
	for _, a := range agents {
		if sessionFilter != "" && a.WorkspaceID != sessionFilter {
			continue
		}
		agentLabel := ptrStr(a.Agent)
		// Display name priority: user rename (name) > presentation override
		// (display_agent). Route into AgentDisplayName so DisplayName() renders
		// the rename; AgentName stays the detected agent type.
		display := ptrStr(a.Name)
		if display == "" {
			display = ptrStr(a.DisplayAgent)
		}
		name := display
		if name == "" {
			name = agentLabel
		}
		if name == "" {
			name = "agent"
		}
		path := ptrStr(a.ForegroundCwd)
		if path == "" {
			path = ptrStr(a.Cwd)
		}
		location := workspaceLabels[a.WorkspaceID]
		if location == "" {
			location = a.WorkspaceID
		}
		items = append(items, Item{
			Kind:             KindAgent,
			Name:             name,
			AgentName:        agentLabel,
			AgentDisplayName: display,
			AgentState:       mapHerdrStatusToAgentState(a.AgentStatus),
			PaneID:           a.PaneID,
			Session:          a.WorkspaceID,
			Window:           a.TabID,
			Pane:             a.PaneID,
			Path:             path,
			Location:         location,
			TabLabel:         tabLabels[a.TabID],
		})
	}
	// Do NOT apply local aliases under herdr — herdr labels are authoritative.
	return items, nil
}

// herdrWorkspaceLabels fetches `herdr workspace list` and returns a map of
// workspace_id → label. Labels fall back to the workspace id in ListAgents when
// missing. Errors are swallowed (best-effort label resolution; the caller keeps
// working with ids if the lookup fails).
func herdrWorkspaceLabels(ctx context.Context) map[string]string {
	out, err := herdrOutput(ctx, "workspace", "list")
	if err != nil {
		return nil
	}
	workspaces, err := parseHerdrWorkspaces(out)
	if err != nil {
		return nil
	}
	labels := make(map[string]string, len(workspaces))
	for _, w := range workspaces {
		labels[w.WorkspaceID] = w.Label
	}
	return labels
}

// herdrTabs fetches `herdr tab list` (all workspaces, single call) and returns
// the slice of tabs. Errors are swallowed (best-effort; callers keep working
// with zero counts / empty labels if the lookup fails).
func herdrTabs(ctx context.Context) []tabInfo {
	out, err := herdrOutput(ctx, "tab", "list")
	if err != nil {
		return nil
	}
	tabs, err := parseHerdrTabs(out)
	if err != nil {
		return nil
	}
	return tabs
}

// herdrPanes fetches `herdr pane list` (all panes, single call) and returns the
// slice of panes. Errors are swallowed (best-effort, mirroring herdrTabs).
func herdrPanes(ctx context.Context) []paneInfo {
	out, err := herdrOutput(ctx, "pane", "list")
	if err != nil {
		return nil
	}
	panes, err := parseHerdrPanes(out)
	if err != nil {
		return nil
	}
	return panes
}

// herdrWorkspacePaths returns a representative cwd per workspace_id from the
// pane list: the focused pane's foreground_cwd/cwd if one exists, else the
// first pane's cwd. The workspace `cwd` field is usually empty under herdr, so
// panes are the practical path source. Errors are swallowed (best-effort).
func herdrWorkspacePaths(ctx context.Context) map[string]string {
	panes := herdrPanes(ctx)
	paths := make(map[string]string, len(panes))
	for _, p := range panes {
		cwd := p.ForegroundCwd
		if cwd == "" {
			cwd = p.Cwd
		}
		if cwd == "" {
			continue
		}
		if p.Focused {
			paths[p.WorkspaceID] = cwd
		} else if _, ok := paths[p.WorkspaceID]; !ok {
			paths[p.WorkspaceID] = cwd
		}
	}
	return paths
}

// herdrTabLabels returns a tab_id → label map for resolving opaque tab ids in
// agent detail. Falls back to empty (caller then omits the line).
func herdrTabLabels(ctx context.Context) map[string]string {
	tabs := herdrTabs(ctx)
	labels := make(map[string]string, len(tabs))
	for _, t := range tabs {
		labels[t.TabID] = t.Label
	}
	return labels
}

// CaptureAgentPane reads recent pane output. herdr's "recent" source is
// tail/bottom-anchored, matching seshagy's bottom-anchored preview rendering.
func (herdrBackend) CaptureAgentPane(
	ctx context.Context,
	paneID string,
	lines int,
) (string, error) {
	args := []string{"pane", "read", paneID, "--source", "recent", "--ansi"}
	if lines > 0 {
		args = append(args, "--lines", fmt.Sprintf("%d", lines))
	}
	out, err := herdrOutput(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("herdr pane read: %w", err)
	}
	return string(out), nil
}

func (herdrBackend) FocusAgentCommand(item Item) *exec.Cmd {
	return exec.Command("herdr", "agent", "focus", item.PaneID)
}

func (herdrBackend) RenameAgent(ctx context.Context, item Item, displayName string) error {
	args := []string{"agent", "rename", item.PaneID}
	if displayName == "" {
		args = append(args, "--clear")
	} else {
		args = append(args, displayName)
	}
	if err := herdrRun(ctx, args...); err != nil {
		return fmt.Errorf("herdr agent rename: %w", err)
	}
	return nil
}

func (herdrBackend) ResolvePane(_ context.Context, pane string) (string, error) {
	// herdr ids are opaque; pass through without validation (v1 simplicity).
	return pane, nil
}

func (herdrBackend) ResolvePaneByCwd(
	ctx context.Context,
	cwd string,
) (string, error) {
	if cwd == "" {
		return "", nil
	}
	cwd = filepath.Clean(cwd)
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	out, err := herdrOutput(ctx, "pane", "list")
	if err != nil {
		return "", err
	}
	panes, err := parseHerdrPanes(out)
	if err != nil {
		return "", err
	}
	var exact, prefix []string
	for _, p := range panes {
		for _, raw := range []string{p.Cwd, p.ForegroundCwd} {
			if raw == "" {
				continue
			}
			path := filepath.Clean(raw)
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				path = resolved
			}
			switch {
			case path == cwd:
				exact = append(exact, p.PaneID)
			case strings.HasPrefix(path, cwd+string(filepath.Separator)) ||
				strings.HasPrefix(cwd, path+string(filepath.Separator)):
				prefix = append(prefix, p.PaneID)
			}
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) == 0 && len(prefix) == 1 {
		return prefix[0], nil
	}
	return "", nil // 0 or ambiguous — refuse to guess
}

// --- Suppression no-ops: herdr owns agent state ---

func (herdrBackend) ReportAgent(context.Context, AgentReport) (bool, error) {
	// herdr owns agent state; seshagy does not report under herdr.
	return false, nil
}

func (herdrBackend) ReleaseAgent(context.Context, AgentRelease) (bool, error) {
	// herdr owns agent state; seshagy does not report under herdr.
	return false, nil
}

func (herdrBackend) MarkAgentVisited(context.Context, string) (bool, error) {
	// herdr owns agent state; seshagy does not report under herdr.
	return false, nil
}

func (herdrBackend) MarkActiveDoneAgentsIdle(context.Context, []Item) {
	// herdr owns agent state; seshagy does not report under herdr.
}
