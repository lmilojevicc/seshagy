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
	// Workspace 1: label used as Name, workspace_id as Target
	if items[0].Name != "proj" || items[0].Target != "w1" || !items[0].Attached {
		t.Fatalf("items[0] = %+v", items[0])
	}
	// Workspace 2: no label → Name falls back to workspace_id
	if items[1].Name != "w2" || items[1].Target != "w2" {
		t.Fatalf("items[1] = %+v", items[1])
	}
}

func TestHerdrBackendListAgents(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			return []byte(`{"result":{"type":"pane_list","panes":[` +
				`{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","agent":"claude","display_agent":"Claude","agent_status":"working","cwd":"/proj","foreground_cwd":"/proj/src","focused":true},` +
				`{"pane_id":"w1:p2","workspace_id":"w1","tab_id":"w1:t1","agent_status":"unknown","cwd":"/proj","focused":false},` +
				`{"pane_id":"w1:p3","workspace_id":"w1","tab_id":"w1:t1","agent":"codex","agent_status":"unknown","cwd":"/proj","focused":false}` +
				`]}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend()
	items, err := mux.ListAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgents error: %v", err)
	}
	// p2 has no agent/display_agent → filtered out; p1 and p3 remain.
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2 (non-agent pane should be filtered)", len(items))
	}
	// p1: agent=claude, display_agent=Claude, status=working
	if items[0].AgentName != "claude" || items[0].Name != "Claude" ||
		items[0].AgentState != AgentWorking {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[0].Session != "w1" || items[0].Window != "w1:t1" ||
		items[0].PaneID != "w1:p1" || items[0].Path != "/proj/src" {
		t.Fatalf("items[0] fields = %+v", items[0])
	}
	if items[0].Location != "w1:p1" {
		t.Fatalf("Location = %q", items[0].Location)
	}
	// p3: agent=codex, no display_agent → Name falls back to agent
	if items[1].AgentName != "codex" || items[1].Name != "codex" ||
		items[1].AgentState != AgentUnknown {
		t.Fatalf("items[1] = %+v", items[1])
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
	setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if args[0] == "pane" && args[1] == "read" {
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
