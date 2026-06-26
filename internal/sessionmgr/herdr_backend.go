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
	out, _ := herdrOutput(ctx, "pane", "current", "--json") // graceful: no CLI → no current session
	pane, _ := parseHerdrPaneInfo(out)
	if pane == nil {
		return "", nil
	}
	return pane.WorkspaceID, nil
}

func (herdrBackend) ListSessions(ctx context.Context) ([]Item, error) {
	out, err := herdrOutput(ctx, "workspace", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("herdr workspace list: %w", err)
	}
	workspaces, err := parseHerdrWorkspaces(out)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(workspaces))
	for _, ws := range workspaces {
		name := ws.Label
		if name == "" {
			name = ws.WorkspaceID
		}
		items = append(items, Item{
			Kind:     KindSession,
			Name:     name,
			Target:   ws.WorkspaceID,
			Path:     ws.Cwd,
			Attached: ws.Focused,
			// Windows (tab count) left at 0 — counting requires a per-workspace
			// tab list call which is not cheap for a list refresh.
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
		"--json",
	)
	if err != nil {
		return Item{}, false, fmt.Errorf("herdr workspace create: %w", err)
	}
	workspaces, _ := parseHerdrWorkspaces(out) // create succeeded; parse failure → best-effort
	if len(workspaces) == 0 {
		// Create succeeded but no JSON body — return a best-effort item.
		return Item{Kind: KindSession, Name: label, Target: label}, false, nil
	}
	ws := workspaces[0]
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
		"--json",
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
	args := []string{"pane", "list", "--json"}
	if sessionFilter != "" {
		args = append(args, "--workspace", sessionFilter)
	}
	out, err := herdrOutput(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("herdr pane list: %w", err)
	}
	panes, err := parseHerdrPanes(out)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(panes))
	for _, p := range panes {
		// Only panes with a recognized agent are agents. PaneInfo.agent is
		// omitted for plain shells; display_agent is the friendly label.
		if p.Agent == "" && p.DisplayAgent == "" {
			continue
		}
		name := p.DisplayAgent
		if name == "" {
			name = p.Agent
		}
		if name == "" {
			continue
		}
		path := p.ForegroundCwd
		if path == "" {
			path = p.Cwd
		}
		items = append(items, Item{
			Kind:             KindAgent,
			Name:             name,
			AgentName:        p.Agent,
			AgentDisplayName: p.DisplayAgent,
			AgentState:       mapHerdrStatusToAgentState(p.AgentStatus),
			PaneID:           p.PaneID,
			Session:          p.WorkspaceID,
			Window:           p.TabID,
			Pane:             p.PaneID,
			Path:             path,
			Location:         fmt.Sprintf("%s:%s.%s", p.WorkspaceID, p.TabID, p.PaneID),
		})
	}
	// Do NOT apply local aliases under herdr — herdr labels are authoritative.
	return items, nil
}

// CaptureAgentPane reads recent pane output. herdr's "recent" source is
// tail/bottom-anchored, matching seshagy's bottom-anchored preview rendering.
func (herdrBackend) CaptureAgentPane(
	ctx context.Context,
	paneID string,
	lines int,
) (string, error) {
	args := []string{"pane", "read", paneID, "--source", "recent"}
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
	out, err := herdrOutput(ctx, "pane", "list", "--json")
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
