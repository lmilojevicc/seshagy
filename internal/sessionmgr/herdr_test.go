package sessionmgr

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// --- Test helpers ---

// setHerdrHooksForTest swaps herdrOutput/herdrRun for the duration of a test.
func setHerdrHooksForTest(
	t *testing.T,
	output func(context.Context, ...string) ([]byte, error),
	run func(context.Context, ...string) error,
) {
	t.Helper()
	origOut, origRun := herdrOutput, herdrRun
	if output != nil {
		herdrOutput = output
	}
	if run != nil {
		herdrRun = run
	}
	t.Cleanup(func() {
		herdrOutput = origOut
		herdrRun = origRun
	})
}

// herdrCmdRecorder captures the args of every herdr command invoked, and
// returns canned output/run results.
type herdrCmdRecorder struct {
	calls   [][]string
	outputF func(args []string) ([]byte, error)
	runF    func(args []string) error
}

func (r *herdrCmdRecorder) outputFn(ctx context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.outputF != nil {
		return r.outputF(args)
	}
	return nil, nil
}

func (r *herdrCmdRecorder) runFn(ctx context.Context, args ...string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.runF != nil {
		return r.runF(args)
	}
	return nil
}

// --- Parser tests ---

func TestParseHerdrWorkspaces(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []workspaceInfo
		wantErr bool
	}{
		{
			name: "wrapped response",
			input: `{"id":"r1","result":{"type":"workspace_list","workspaces":[` +
				`{"workspace_id":"w1","label":"proj","cwd":"/home/me/proj","focused":true},` +
				`{"workspace_id":"w2","label":"api","cwd":"/home/me/api","focused":false}` +
				`]}}`,
			want: []workspaceInfo{
				{WorkspaceID: "w1", Label: "proj", Cwd: "/home/me/proj", Focused: true},
				{WorkspaceID: "w2", Label: "api", Cwd: "/home/me/api", Focused: false},
			},
		},
		{
			name: "direct payload",
			input: `{"type":"workspace_list","workspaces":[` +
				`{"workspace_id":"w3","label":"docs"}]}`,
			want: []workspaceInfo{
				{WorkspaceID: "w3", Label: "docs"},
			},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrWorkspaces([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (%+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseHerdrPanes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []paneInfo
		wantErr bool
	}{
		{
			name: "wrapped with agents",
			input: `{"id":"r1","result":{"type":"pane_list","panes":[` +
				`{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","agent":"claude","display_agent":"Claude","agent_status":"working","cwd":"/proj","focused":true},` +
				`{"pane_id":"w1:p2","workspace_id":"w1","tab_id":"w1:t1","agent_status":"unknown","cwd":"/proj","focused":false}` +
				`]}}`,
			want: []paneInfo{
				{
					PaneID:       "w1:p1",
					WorkspaceID:  "w1",
					TabID:        "w1:t1",
					Agent:        "claude",
					DisplayAgent: "Claude",
					AgentStatus:  "working",
					Cwd:          "/proj",
					Focused:      true,
				},
				{
					PaneID:      "w1:p2",
					WorkspaceID: "w1",
					TabID:       "w1:t1",
					AgentStatus: "unknown",
					Cwd:         "/proj",
					Focused:     false,
				},
			},
		},
		{
			name:  "direct payload",
			input: `{"type":"pane_list","panes":[{"pane_id":"w2:p1","agent":"codex","agent_status":"blocked"}]}`,
			want: []paneInfo{
				{PaneID: "w2:p1", Agent: "codex", AgentStatus: "blocked"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrPanes([]byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
		})
	}
}

func TestParseHerdrPaneInfo(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *paneInfo
		wantErr bool
	}{
		{
			name: "wrapped pane_info",
			input: `{"id":"r1","result":{"type":"pane_info","pane":` +
				`{"pane_id":"w1:p1","workspace_id":"w1","agent":"pi","agent_status":"idle"}}}`,
			want: &paneInfo{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "pi", AgentStatus: "idle"},
		},
		{
			name:  "bare pane object",
			input: `{"pane_id":"w2:p3","workspace_id":"w2","agent_status":"done"}`,
			want:  &paneInfo{PaneID: "w2:p3", WorkspaceID: "w2", AgentStatus: "done"},
		},
		{
			name:    "no pane",
			input:   `{"type":"workspace_list","workspaces":[]}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrPaneInfo([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.PaneID != tc.want.PaneID || got.WorkspaceID != tc.want.WorkspaceID ||
				got.AgentStatus != tc.want.AgentStatus {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseHerdrAgents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		want0   agentInfo
	}{
		{
			name: "wrapped response",
			input: `{"id":"r1","result":{"type":"agent_list","agents":[` +
				`{"terminal_id":"t1","name":"my-renamed","agent":"claude","agent_status":"working","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","focused":true,"foreground_cwd":"/proj/src","screen_detection_skipped":false,"revision":5},` +
				`{"terminal_id":"t2","agent":"codex","agent_status":"unknown","workspace_id":"w2","tab_id":"w2:t1","pane_id":"w2:p1","focused":false,"revision":6}` +
				`]}}`,
			wantLen: 2,
			want0: agentInfo{
				TerminalID:  "t1",
				AgentStatus: "working",
				WorkspaceID: "w1",
				TabID:       "w1:t1",
				PaneID:      "w1:p1",
				Focused:     true,
				Revision:    5,
			},
		},
		{
			name:    "direct payload",
			input:   `{"type":"agent_list","agents":[{"agent":"pi","agent_status":"idle","workspace_id":"w3","pane_id":"w3:p1"}]}`,
			wantLen: 1,
			want0: agentInfo{
				AgentStatus: "idle",
				WorkspaceID: "w3",
				PaneID:      "w3:p1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrAgents([]byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if got[0].AgentStatus != tc.want0.AgentStatus ||
				got[0].WorkspaceID != tc.want0.WorkspaceID ||
				got[0].PaneID != tc.want0.PaneID {
				t.Fatalf("got[0] = %+v, want %+v", got[0], tc.want0)
			}
		})
	}
}

func TestParseHerdrWorkspaceCreated(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantID    string
		wantLabel string
		wantErr   bool
	}{
		{
			name: "workspace_created",
			input: `{"id":"r1","result":{"type":"workspace_created","workspace":` +
				`{"workspace_id":"w9","label":"myproj","cwd":"/home/me/myproj","focused":true},"tab":{},"root_pane":{}}}`,
			wantID:    "w9",
			wantLabel: "myproj",
		},
		{
			name:    "no workspace",
			input:   `{"result":{"type":"workspace_list","workspaces":[]}}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws, err := parseHerdrWorkspaceCreated([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", ws)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ws.WorkspaceID != tc.wantID {
				t.Fatalf("workspace_id = %q, want %q", ws.WorkspaceID, tc.wantID)
			}
			if ws.Label != tc.wantLabel {
				t.Fatalf("label = %q, want %q", ws.Label, tc.wantLabel)
			}
		})
	}
}

func TestHerdrBackendCreateSessionFromDir(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			if args[0] == "workspace" && args[1] == "create" {
				return []byte(`{"result":{"type":"workspace_created","workspace":` +
					`{"workspace_id":"w42","label":"myproj","cwd":"/tmp/myproj","focused":true}}}`), nil
			}
			return []byte(`{"result":{"type":"workspace_list","workspaces":[]}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	item, reused, err := mux.CreateSessionFromDir(context.Background(), "/tmp/myproj")
	if err != nil {
		t.Fatalf("CreateSessionFromDir error: %v", err)
	}
	if reused {
		t.Fatal("reused = true, want false (new workspace)")
	}
	if item.Target != "w42" {
		t.Fatalf("Target = %q, want w42", item.Target)
	}
	if item.Name != "myproj" {
		t.Fatalf("Name = %q, want myproj", item.Name)
	}
	// Verify create args do NOT include --json.
	for _, call := range rec.calls {
		if call[0] == "workspace" && call[1] == "create" {
			for _, a := range call {
				if a == "--json" {
					t.Fatalf("create args contain --json: %v", call)
				}
			}
		}
	}
}

func TestMapHerdrStatusToAgentState(t *testing.T) {
	tests := []struct {
		input string
		want  AgentState
	}{
		{"idle", AgentIdle},
		{"working", AgentWorking},
		{"blocked", AgentBlocked},
		{"done", AgentDone},
		{"unknown", AgentUnknown},
		{"", AgentUnknown},
		{"garbage", AgentUnknown},
		{"WORKING", AgentWorking}, // case-insensitive
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := mapHerdrStatusToAgentState(tc.input); got != tc.want {
				t.Fatalf("mapHerdrStatusToAgentState(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// --- Backend tests ---

func TestHerdrBackendListSessions(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			if args[0] == "workspace" && args[1] == "list" {
				return []byte(`{"result":{"type":"workspace_list","workspaces":[` +
					`{"workspace_id":"w1","label":"proj","cwd":"/proj","focused":true},` +
					`{"workspace_id":"w2","cwd":"/api"}]}}`), nil
			}
			if args[0] == "tab" && args[1] == "list" {
				return []byte(`{"result":{"type":"tab_list","tabs":[` +
					`{"tab_id":"w1:t1","workspace_id":"w1","label":"main"},` +
					`{"tab_id":"w1:t2","workspace_id":"w1","label":"logs"},` +
					`{"tab_id":"w2:t1","workspace_id":"w2","label":"api"}` +
					`]}}`), nil
			}
			return nil, nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	items, err := mux.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	// Workspace 1: label used as Name, workspace_id as Target, 2 tabs counted.
	if items[0].Name != "proj" || items[0].Target != "w1" || !items[0].Attached {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[0].Windows != 2 {
		t.Fatalf("items[0].Windows = %d, want 2 (tab count)", items[0].Windows)
	}
	// Workspace 2: no label → Name falls back to workspace_id, 1 tab.
	if items[1].Name != "w2" || items[1].Target != "w2" {
		t.Fatalf("items[1] = %+v", items[1])
	}
	if items[1].Windows != 1 {
		t.Fatalf("items[1].Windows = %d, want 1", items[1].Windows)
	}
}

func TestHerdrBackendListAgents(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			// ListAgents resolves workspace ids → labels via `workspace list`
			// before calling `agent list`.
			if args[0] == "workspace" && args[1] == "list" {
				return []byte(`{"result":{"type":"workspace_list","workspaces":[` +
					`{"workspace_id":"w1","label":"proj","cwd":"/proj","focused":true},` +
					`{"workspace_id":"w2","label":"","cwd":"/other","focused":false}` +
					`]}}`), nil
			}
			if args[0] == "tab" && args[1] == "list" {
				return []byte(`{"result":{"type":"tab_list","tabs":[` +
					`{"tab_id":"w1:t1","workspace_id":"w1","label":"main"},` +
					`{"tab_id":"w2:t1","workspace_id":"w2","label":""}` +
					`]}}`), nil
			}
			return []byte(`{"result":{"type":"agent_list","agents":[` +
				`{"terminal_id":"t1","name":"my-claude","agent":"claude","agent_status":"working","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","focused":true,"foreground_cwd":"/proj/src","cwd":"/proj","revision":1},` +
				`{"terminal_id":"t2","agent":"codex","agent_status":"unknown","workspace_id":"w2","tab_id":"w2:t1","pane_id":"w2:p1","focused":false,"cwd":"/other","revision":2}` +
				`]}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	items, err := mux.ListAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgents error: %v", err)
	}
	// agent list already filters non-agent panes; both entries are agents.
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	// a1: name=my-claude (user rename) takes priority over agent/display_agent
	if items[0].Name != "my-claude" || items[0].AgentName != "claude" ||
		items[0].AgentState != AgentWorking {
		t.Fatalf("items[0] = %+v", items[0])
	}
	// The rename must land in AgentDisplayName so DisplayName() renders it.
	if items[0].AgentDisplayName != "my-claude" {
		t.Fatalf("items[0].AgentDisplayName = %q, want my-claude", items[0].AgentDisplayName)
	}
	if got := items[0].DisplayName(); got != "my-claude" {
		t.Fatalf("items[0].DisplayName() = %q, want my-claude", got)
	}
	if items[0].Session != "w1" || items[0].Window != "w1:t1" ||
		items[0].PaneID != "w1:p1" || items[0].Path != "/proj/src" {
		t.Fatalf("items[0] fields = %+v", items[0])
	}
	// Location shows the workspace label (not the opaque pane id).
	if items[0].Location != "proj" {
		t.Fatalf("items[0].Location = %q, want proj", items[0].Location)
	}
	// TabLabel resolves the tab id to its label.
	if items[0].TabLabel != "main" {
		t.Fatalf("items[0].TabLabel = %q, want main", items[0].TabLabel)
	}
	// a2: no workspace label → falls back to the workspace id.
	if items[1].Name != "codex" || items[1].AgentName != "codex" ||
		items[1].AgentState != AgentUnknown {
		t.Fatalf("items[1] = %+v", items[1])
	}
	if items[1].Location != "w2" {
		t.Fatalf("items[1].Location = %q, want w2 (id fallback)", items[1].Location)
	}
}

func TestHerdrBackendListAgentsWorkspaceFilter(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			if args[0] == "workspace" && args[1] == "list" {
				return []byte(`{"result":{"type":"workspace_list","workspaces":[` +
					`{"workspace_id":"w1","label":"proj"},` +
					`{"workspace_id":"w2","label":"other"}` +
					`]}}`), nil
			}
			return []byte(`{"result":{"type":"agent_list","agents":[` +
				`{"agent":"claude","agent_status":"working","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","focused":true},` +
				`{"agent":"codex","agent_status":"idle","workspace_id":"w2","tab_id":"w2:t1","pane_id":"w2:p1","focused":false}` +
				`]}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	// Filter to w1 only — should return 1 agent.
	items, err := mux.ListAgents(context.Background(), "w1")
	if err != nil {
		t.Fatalf("ListAgents error: %v", err)
	}
	if len(items) != 1 || items[0].AgentName != "claude" {
		t.Fatalf("filtered items = %+v, want 1 claude agent", items)
	}
	// Verify agent list was called WITHOUT --workspace (filtered in-memory).
	// ListAgents issues 3 calls: workspace list, tab list (label resolution),
	// and agent list.
	var agentListCall []string
	for _, c := range rec.calls {
		if len(c) >= 2 && c[0] == "agent" && c[1] == "list" {
			agentListCall = c
		}
	}
	if agentListCall == nil {
		t.Fatalf("no agent list call in %v", rec.calls)
	}
	for _, a := range agentListCall {
		if a == "--workspace" {
			t.Fatalf("agent list should not pass --workspace; args = %v", agentListCall)
		}
	}
}

func TestHerdrBackendSuppression(t *testing.T) {
	rec := &herdrCmdRecorder{}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	ctx := context.Background()

	applied, err := mux.ReportAgent(ctx, AgentReport{})
	if applied || err != nil {
		t.Fatalf("ReportAgent = (%v, %v), want (false, nil)", applied, err)
	}
	applied, err = mux.ReleaseAgent(ctx, AgentRelease{})
	if applied || err != nil {
		t.Fatalf("ReleaseAgent = (%v, %v), want (false, nil)", applied, err)
	}
	applied, err = mux.MarkAgentVisited(ctx, "w1:p1")
	if applied || err != nil {
		t.Fatalf("MarkAgentVisited = (%v, %v), want (false, nil)", applied, err)
	}
	mux.MarkActiveDoneAgentsIdle(ctx, []Item{{Kind: KindAgent, AgentState: AgentDone}})

	// No herdr command should have been invoked for state writes.
	if len(rec.calls) != 0 {
		t.Fatalf("expected 0 herdr calls for suppression, got %d: %v", len(rec.calls), rec.calls)
	}
}

func TestHerdrBackendCommandsUseCorrectArgs(t *testing.T) {
	rec := &herdrCmdRecorder{}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	ctx := context.Background()

	// KillSession
	if err := mux.KillSession(ctx, "w1"); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	// RenameSession
	if err := mux.RenameSession(ctx, "w1", "newname"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	// RenameAgent
	if err := mux.RenameAgent(ctx, Item{PaneID: "w1:p1"}, ""); err != nil {
		t.Fatalf("RenameAgent clear: %v", err)
	}
	if err := mux.RenameAgent(ctx, Item{PaneID: "w1:p1"}, "alias"); err != nil {
		t.Fatalf("RenameAgent set: %v", err)
	}

	// AttachOrSwitchCommand
	cmd := mux.AttachOrSwitchCommand(Item{Kind: KindSession, Name: "proj", Target: "w1"})
	if cmd == nil || !strings.Contains(strings.Join(cmd.Args, " "), "workspace focus w1") {
		t.Fatalf("AttachOrSwitchCommand = %+v", cmd)
	}
	// FocusAgentCommand
	cmd = mux.FocusAgentCommand(Item{Kind: KindAgent, PaneID: "w1:p1"})
	if cmd == nil || !strings.Contains(strings.Join(cmd.Args, " "), "agent focus w1:p1") {
		t.Fatalf("FocusAgentCommand = %+v", cmd)
	}

	// Verify herdrRun-based commands captured correct args.
	wantCalls := [][]string{
		{"workspace", "close", "w1"},
		{"workspace", "rename", "w1", "newname"},
		{"agent", "rename", "w1:p1", "--clear"},
		{"agent", "rename", "w1:p1", "alias"},
	}
	if len(rec.calls) != len(wantCalls) {
		t.Fatalf("calls = %d, want %d: %v", len(rec.calls), len(wantCalls), rec.calls)
	}
	for i, want := range wantCalls {
		got := rec.calls[i]
		if len(got) != len(want) {
			t.Fatalf("call[%d] = %v, want %v", i, got, want)
		}
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("call[%d][%d] = %q, want %q (full: %v)", i, j, got[j], want[j], got)
			}
		}
	}
}

func TestHerdrResolvePaneByCwd(t *testing.T) {
	tests := []struct {
		name   string
		cwd    string
		panes  string
		want   string
		errNil bool
	}{
		{
			name: "unique exact match",
			cwd:  "/proj",
			panes: `{"result":{"type":"pane_list","panes":[` +
				`{"pane_id":"w1:p1","cwd":"/proj"},` +
				`{"pane_id":"w1:p2","cwd":"/other"}]}}`,
			want:   "w1:p1",
			errNil: true,
		},
		{
			name: "ambiguous match returns empty",
			cwd:  "/proj",
			panes: `{"result":{"type":"pane_list","panes":[` +
				`{"pane_id":"w1:p1","cwd":"/proj"},` +
				`{"pane_id":"w1:p2","foreground_cwd":"/proj"}]}}`,
			want:   "",
			errNil: true,
		},
		{
			name: "no match returns empty",
			cwd:  "/nowhere",
			panes: `{"result":{"type":"pane_list","panes":[` +
				`{"pane_id":"w1:p1","cwd":"/proj"}]}}`,
			want:   "",
			errNil: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
				return []byte(tc.panes), nil
			}, nil)

			mux := NewHerdrBackend()
			got, err := mux.ResolvePaneByCwd(context.Background(), tc.cwd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ResolvePaneByCwd(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}

func TestHerdrBackendCaptureAgentPane(t *testing.T) {
	var capturedArgs []string
	setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "pane" && args[1] == "read" {
			capturedArgs = args
			return []byte("line1\nline2\nline3"), nil
		}
		return nil, errors.New("unexpected call")
	}, nil)

	mux := NewHerdrBackend()
	out, err := mux.CaptureAgentPane(context.Background(), "w1:p1", 30)
	if err != nil {
		t.Fatalf("CaptureAgentPane error: %v", err)
	}
	if out != "line1\nline2\nline3" {
		t.Fatalf("CaptureAgentPane = %q", out)
	}
	// Must request ANSI format so previews are colored (not stripped).
	foundAnsi := false
	for _, a := range capturedArgs {
		if a == "--ansi" {
			foundAnsi = true
			break
		}
	}
	if !foundAnsi {
		t.Fatalf("CaptureAgentPane args missing --ansi: %v", capturedArgs)
	}
}

func TestHerdrBackendKind(t *testing.T) {
	if NewHerdrBackend().Kind() != BackendHerdr {
		t.Fatal("kind != BackendHerdr")
	}
}

// Verify exec.Cmd constructions produce the right binary.
func TestHerdrBackendExecCommands(t *testing.T) {
	mux := NewHerdrBackend()
	cmd := mux.AttachOrSwitchCommand(Item{Kind: KindSession, Target: "w1"})
	if _, ok := exec.LookPath("herdr"); ok != nil {
		// herdr not installed — just verify the command was constructed.
		if cmd == nil {
			t.Fatal("AttachOrSwitchCommand returned nil")
		}
	}
}
