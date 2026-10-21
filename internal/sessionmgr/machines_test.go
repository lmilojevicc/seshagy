package sessionmgr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lmilojevicc/seshagy/internal/logging"
)

// --- machine list / status parsers ---

func TestParseHerdrMachines(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Machine
		wantErr bool
	}{
		{
			name: "enabled and disabled profiles",
			input: `[{"id":"aaaa","label":"devbox","target":"me@devbox","session":"default",` +
				`"enabled":true,"selected":true},` +
				`{"id":"bbbb","label":"gpu-box","target":"me@gpu","session":"main",` +
				`"enabled":false,"selected":false}]`,
			want: []Machine{
				{
					ID: "aaaa", Label: "devbox", Target: "me@devbox",
					Session: "default", Enabled: true, Selected: true,
				},
				{
					ID: "bbbb", Label: "gpu-box", Target: "me@gpu",
					Session: "main", Enabled: false, Selected: false,
				},
			},
		},
		{
			name:  "label and target may be empty",
			input: `[{"id":"cccc","label":"","target":"","session":"","enabled":true}]`,
			want:  []Machine{{ID: "cccc", Enabled: true}},
		},
		{
			name:  "empty catalog",
			input: `[]`,
			want:  []Machine{},
		},
		{
			name:  "whitespace and newlines are tolerated",
			input: "\n [\n {\"id\":\"dddd\",\"label\":\"x\",\"enabled\":true}\n ]\n",
			want:  []Machine{{ID: "dddd", Label: "x", Enabled: true}},
		},
		{
			name:    "empty output",
			input:   "   \n\t",
			wantErr: true,
		},
		{
			name:    "garbage",
			input:   "not json at all",
			wantErr: true,
		},
		{
			name:    "result envelope is rejected",
			input:   `{"result":{"ssh":[]}}`,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrMachines([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
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

func TestParseHerdrMachineStatuses(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []MachineStatus
		wantErr bool
	}{
		{
			name: "error row carries herdr diagnostic",
			input: `[{"id":"aaaa","label":"devbox","status":"error",` +
				`"error":"remote platform detection failed"}]`,
			want: []MachineStatus{{
				ID: "aaaa", Label: "devbox", Status: "error",
				Error: "remote platform detection failed",
			}},
		},
		{
			name:  "disabled row has null error",
			input: `[{"id":"bbbb","label":"gpu","status":"disabled","error":null}]`,
			want:  []MachineStatus{{ID: "bbbb", Label: "gpu", Status: "disabled"}},
		},
		{
			name: "mixed rows",
			input: `[{"id":"a","label":"one","status":"reachable","error":null},` +
				`{"id":"b","label":"two","status":"auth required","error":"denied"}]`,
			want: []MachineStatus{
				{ID: "a", Label: "one", Status: "reachable"},
				{ID: "b", Label: "two", Status: "auth required", Error: "denied"},
			},
		},
		{
			name:  "empty probe result",
			input: `[]`,
			want:  []MachineStatus{},
		},
		{name: "empty output", input: "", wantErr: true},
		{name: "garbage", input: `[`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHerdrMachineStatuses([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
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

func TestMachineStatusClassification(t *testing.T) {
	tests := []struct {
		name          string
		status        string
		errText       string
		wantReachable bool
		wantDisabled  bool
		wantNeedsAuth bool
		wantUnusable  bool
	}{
		{name: "reachable", status: "reachable", wantReachable: true},
		{name: "reachable with surrounding space", status: "  Reachable ", wantReachable: true},
		{name: "ok alias", status: "ok", wantReachable: true},
		{name: "online alias", status: "online", wantReachable: true},
		{name: "disabled", status: "disabled", wantDisabled: true, wantUnusable: true},
		{
			name: "auth required", status: "auth required",
			wantNeedsAuth: true, wantUnusable: true,
		},
		{name: "error status", status: "error", wantUnusable: true},
		{name: "unknown value is not reachable", status: "brand-new-value"},
		{name: "empty status is not reachable", status: ""},
		{
			name:   "reachable status with an error is not reachable",
			status: "reachable", errText: "boom", wantUnusable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := MachineStatus{Status: tc.status, Error: tc.errText}
			if got := s.Reachable(); got != tc.wantReachable {
				t.Fatalf("Reachable() = %v, want %v", got, tc.wantReachable)
			}
			if got := s.Disabled(); got != tc.wantDisabled {
				t.Fatalf("Disabled() = %v, want %v", got, tc.wantDisabled)
			}
			if got := s.NeedsAuth(); got != tc.wantNeedsAuth {
				t.Fatalf("NeedsAuth() = %v, want %v", got, tc.wantNeedsAuth)
			}
			if got := s.Unusable(); got != tc.wantUnusable {
				t.Fatalf("Unusable() = %v, want %v", got, tc.wantUnusable)
			}
		})
	}
}

func TestMachineLabels(t *testing.T) {
	tests := []struct {
		name    string
		machine Machine
		want    string
		local   bool
	}{
		{name: "local machine", machine: LocalMachine(), want: "local", local: true},
		{name: "zero machine is local", machine: Machine{}, want: "local", local: true},
		{name: "labelled profile", machine: Machine{ID: "a", Label: "devbox"}, want: "devbox"},
		{name: "unlabelled profile falls back to id", machine: Machine{ID: "abc"}, want: "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.machine.DisplayLabel(); got != tc.want {
				t.Fatalf("DisplayLabel() = %q, want %q", got, tc.want)
			}
			if got := tc.machine.IsLocal(); got != tc.local {
				t.Fatalf("IsLocal() = %v, want %v", got, tc.local)
			}
		})
	}
}

// --- backend routing ---

// scopedCalls returns the recorded herdr invocations that were prefixed with
// `--machine <id>` and the ones that were not.
func scopedCalls(calls [][]string, id string) (scoped, unscoped [][]string) {
	for _, call := range calls {
		if len(call) >= 2 && call[0] == "--machine" && call[1] == id {
			scoped = append(scoped, call)
			continue
		}
		unscoped = append(unscoped, call)
	}
	return scoped, unscoped
}

func TestHerdrBackendScopesCommandsToMachine(t *testing.T) {
	const machineID = "abc123"
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			cmd := stripMachinePrefix(args)
			switch strings.Join(cmd, " ") {
			case "workspace list":
				return []byte(`{"result":{"type":"workspace_list","workspaces":[` +
					`{"workspace_id":"w1","label":"proj","cwd":"/proj","focused":true}]}}`), nil
			case "tab list":
				return []byte(`{"result":{"type":"tab_list","tabs":[` +
					`{"tab_id":"w1:t1","workspace_id":"w1","label":"main"}]}}`), nil
			case "pane list", "pane list --workspace w1":
				return []byte(`{"result":{"type":"pane_list","panes":[` +
					`{"pane_id":"w1:p1","workspace_id":"w1","tab_id":"w1:t1","cwd":"/proj","focused":true}]}}`), nil
			case "agent list":
				return []byte(`{"result":{"type":"agent_list","agents":[` +
					`{"agent":"claude","agent_status":"working","workspace_id":"w1","tab_id":"w1:t1","pane_id":"w1:p1","cwd":"/proj"}]}}`), nil
			case "pane process-info --pane w1:p1":
				return []byte(`{"result":{"type":"pane_process_info","process_info":` +
					`{"pane_id":"w1:p1","foreground_processes":[{"pid":42,"name":"claude"}]}}}`), nil
			case "pane read w1:p1 --source recent --ansi --lines 10":
				return []byte("screen"), nil
			case "workspace create --cwd /tmp/p --label p --focus":
				return []byte(`{"result":{"type":"workspace_created","workspace":` +
					`{"workspace_id":"w9","label":"p","cwd":"/tmp/p"}}}`), nil
			default:
				return nil, nil
			}
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	ctx := context.Background()
	mux := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: machineID})

	if _, err := mux.ListSessions(ctx); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if _, err := mux.ListAgents(ctx, ""); err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if _, err := mux.CaptureSession(ctx, "w1", 10); err != nil {
		t.Fatalf("CaptureSession: %v", err)
	}
	if _, err := mux.CaptureAgentPane(ctx, "w1:p1", 10); err != nil {
		t.Fatalf("CaptureAgentPane: %v", err)
	}
	if _, err := mux.AgentPID(ctx, Item{PaneID: "w1:p1"}); err != nil {
		t.Fatalf("AgentPID: %v", err)
	}
	if err := mux.RenameSession(ctx, "w1", "renamed"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	if err := mux.KillPane(ctx, "w1:p1"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if err := mux.RenameAgent(ctx, Item{PaneID: "w1:p1"}, "alias"); err != nil {
		t.Fatalf("RenameAgent: %v", err)
	}
	if _, _, err := mux.CreateSessionFromDir(ctx, "/tmp/p"); err != nil {
		t.Fatalf("CreateSessionFromDir: %v", err)
	}
	if err := mux.KillSession(ctx, "w1"); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	scoped, unscoped := scopedCalls(rec.calls, machineID)
	if len(unscoped) != 0 {
		t.Fatalf("unscoped herdr calls on a bound backend: %v", unscoped)
	}
	for _, call := range scoped {
		if len(call) < 3 {
			t.Fatalf("scoped call has no command after the prefix: %v", call)
		}
	}
	wantCommands := []string{
		"workspace list",
		"tab list",
		"pane list",
		"agent list",
		"pane list --workspace w1",
		"pane read w1:p1 --source recent --ansi --lines 10",
		"pane process-info --pane w1:p1",
		"workspace rename w1 renamed",
		"pane close w1:p1",
		"agent rename w1:p1 alias",
		"workspace create --cwd /tmp/p --label p --focus",
		"workspace close w1",
	}
	got := make(map[string]bool, len(scoped))
	for _, call := range scoped {
		got[strings.Join(call[2:], " ")] = true
	}
	for _, want := range wantCommands {
		if !got[want] {
			t.Fatalf("missing machine-scoped command %q; got %v", want, got)
		}
	}
}

// stripMachinePrefix drops a leading `--machine <id>` pair.
func stripMachinePrefix(args []string) []string {
	if len(args) >= 2 && args[0] == "--machine" {
		return args[2:]
	}
	return args
}

func TestHerdrBackendLocalCommandsUnchanged(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func([]string) ([]byte, error) {
			return []byte(`{"result":{"type":"workspace_list","workspaces":[]}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	// WithMachine(LocalMachine()) must be byte-identical to the plain backend.
	local := NewHerdrBackend().(MachineSelector).WithMachine(LocalMachine())
	if _, err := local.ListSessions(context.Background()); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if err := local.KillPane(context.Background(), "w1:p1"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	for _, call := range rec.calls {
		if call[0] == "--machine" {
			t.Fatalf("local backend must not pass --machine: %v", call)
		}
	}
}

func TestHerdrCurrentSessionIgnoresMachine(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "")
	rec := &herdrCmdRecorder{
		outputF: func([]string) ([]byte, error) {
			return []byte(`{"result":{"type":"pane_info","pane":` +
				`{"pane_id":"w1:p1","workspace_id":"wSelf"}}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: "remote"})
	got, err := mux.CurrentSession(context.Background())
	if err != nil {
		t.Fatalf("CurrentSession: %v", err)
	}
	if got != "wSelf" {
		t.Fatalf("CurrentSession() = %q, want wSelf", got)
	}
	if len(rec.calls) != 1 || strings.Join(rec.calls[0], " ") != "pane current" {
		t.Fatalf("calls = %v, want a bare `pane current`", rec.calls)
	}
}

func TestHerdrMachinesUsesLocalCommand(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			if len(args) > 0 && args[0] == "--machine" {
				return nil, errors.New("machine commands must not be machine-scoped")
			}
			if args[0] == "machine" && args[1] == "list" {
				return []byte(`[{"id":"aaaa","label":"devbox","enabled":true}]`), nil
			}
			return []byte(`[{"id":"aaaa","label":"devbox","status":"reachable","error":null}]`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	// Even the bound backend reads the machine catalog locally.
	bound := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: "aaaa"})
	selector, ok := bound.(MachineSelector)
	if !ok {
		t.Fatal("WithMachine() must keep the MachineSelector capability")
	}
	machines, err := selector.Machines(context.Background())
	if err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if len(machines) != 1 || machines[0].ID != "aaaa" || !machines[0].Enabled {
		t.Fatalf("Machines() = %+v", machines)
	}
	statuses, err := selector.MachineStatuses(context.Background(), "aaaa")
	if err != nil {
		t.Fatalf("MachineStatuses: %v", err)
	}
	if len(statuses) != 1 || !statuses[0].Reachable() {
		t.Fatalf("MachineStatuses() = %+v", statuses)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("calls = %v, want 2", rec.calls)
	}
	if strings.Join(rec.calls[0], " ") != "machine list --json" {
		t.Fatalf("calls[0] = %v", rec.calls[0])
	}
	if strings.Join(rec.calls[1], " ") != "machine status aaaa --json" {
		t.Fatalf("calls[1] = %v", rec.calls[1])
	}
}

func TestHerdrMachineListFailureDegrades(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func([]string) ([]byte, error) {
			return nil, errors.New("old herdr: unknown subcommand")
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	machines, err := NewHerdrBackend().(MachineSelector).Machines(context.Background())
	if err == nil || machines != nil {
		t.Fatalf("Machines() = (%+v, %v), want (nil, error)", machines, err)
	}
}

// TestHerdrMachineStatusParsesStdoutBeforeExitCode pins that herdr exiting 1
// (some row failed) still yields the parsed rows.
func TestHerdrMachineStatusParsesStdoutBeforeExitCode(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func([]string) ([]byte, error) {
			out := []byte(`[{"id":"aaaa","label":"devbox","status":"error","error":"boom"},` +
				`{"id":"bbbb","label":"gpu","status":"reachable","error":null}]`)
			return out, errors.New("exit status 1")
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	selector := NewHerdrBackend().(MachineSelector)
	statuses, err := selector.MachineStatuses(context.Background())
	if err != nil {
		t.Fatalf("MachineStatuses: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses = %+v, want 2", statuses)
	}
	if statuses[0].Reachable() || !statuses[1].Reachable() {
		t.Fatalf("classification = %+v", statuses)
	}
}

// TestHerdrMachineStatusFallsBackWhenSelectorRejected pins that a selector
// herdr refuses (disabled/unknown: exit 2, empty stdout) still returns data.
func TestHerdrMachineStatusFallsBackWhenSelectorRejected(t *testing.T) {
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			if len(args) > 2 && args[2] == "disabled-id" {
				return nil, errors.New("exit status 2")
			}
			return []byte(
				`[{"id":"disabled-id","label":"devbox","status":"disabled","error":null}]`,
			), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	selector := NewHerdrBackend().(MachineSelector)
	statuses, err := selector.MachineStatuses(context.Background(), "disabled-id")
	if err != nil {
		t.Fatalf("MachineStatuses: %v", err)
	}
	if len(statuses) != 1 || !statuses[0].Disabled() {
		t.Fatalf("statuses = %+v, want one disabled row", statuses)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("calls = %v, want a fallback probe", rec.calls)
	}
}

// TestHerdrMachineCommandTimeout verifies a bound command honours its context
// instead of hanging the picker on an unreachable machine.
func TestHerdrMachineCommandTimeout(t *testing.T) {
	var calls [][]string
	setHerdrHooksForTest(t, func(ctx context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		// Stand in for exec.CommandContext: the child only stops when the
		// context is done.
		<-ctx.Done()
		return nil, ctx.Err()
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	mux := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: "unreachable"})
	items, err := mux.ListSessions(ctx)
	if err == nil {
		t.Fatalf("ListSessions() = (%+v, nil), want a deadline error", items)
	}
	if items != nil {
		t.Fatalf("ListSessions() items = %+v, want nil (empty source)", items)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if len(calls) != 1 || calls[0][0] != "--machine" || calls[0][1] != "unreachable" {
		t.Fatalf("calls = %v, want one machine-scoped probe", calls)
	}
}

func TestHerdrItemsCarryMachine(t *testing.T) {
	const machineID = "abc123"
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			switch strings.Join(stripMachinePrefix(args), " ") {
			case "workspace list":
				return []byte(`{"result":{"type":"workspace_list","workspaces":[` +
					`{"workspace_id":"w1","label":"proj","cwd":"/proj"}]}}`), nil
			case "tab list":
				return []byte(`{"result":{"type":"tab_list","tabs":[]}}`), nil
			case "pane list":
				return []byte(`{"result":{"type":"pane_list","panes":[` +
					`{"pane_id":"w1:p1","workspace_id":"w1","cwd":"/proj"}]}}`), nil
			case "agent list":
				return []byte(`{"result":{"type":"agent_list","agents":[` +
					`{"agent":"claude","workspace_id":"w1","pane_id":"w1:p1"}]}}`), nil
			default:
				return nil, nil
			}
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: machineID})
	ctx := context.Background()

	sessions, err := mux.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Machine != machineID {
		t.Fatalf("sessions = %+v, want Machine %q", sessions, machineID)
	}

	agents, err := mux.ListAgents(ctx, "")
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].Machine != machineID {
		t.Fatalf("agents = %+v, want Machine %q", agents, machineID)
	}

	// Reusing a workspace rooted at the directory must keep the machine.
	reused, ok, err := mux.CreateSessionFromDir(ctx, "/proj")
	if err != nil || !ok {
		t.Fatalf("CreateSessionFromDir = (%+v, %v, %v), want reused", reused, ok, err)
	}
	if reused.Machine != machineID {
		t.Fatalf("reused.Machine = %q, want %q", reused.Machine, machineID)
	}
}

func TestItemKeyNamespacesMachine(t *testing.T) {
	tests := []struct {
		name string
		item Item
		want string
	}{
		{
			name: "local session keeps its historical key",
			item: Item{Kind: KindSession, Name: "proj"},
			want: "session:proj",
		},
		{
			name: "local agent keeps its historical key",
			item: Item{Kind: KindAgent, PaneID: "w1:p1"},
			want: "agent:w1:p1",
		},
		{
			name: "remote session is namespaced",
			item: Item{Kind: KindSession, Name: "proj", Machine: "aaa"},
			want: "machine:aaa:session:proj",
		},
		{
			name: "remote agent is namespaced",
			item: Item{Kind: KindAgent, PaneID: "w1:p1", Machine: "aaa"},
			want: "machine:aaa:agent:w1:p1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.Key(); got != tc.want {
				t.Fatalf("Key() = %q, want %q", got, tc.want)
			}
		})
	}
	// Two servers can both have w1:p1; the keys must differ.
	a := Item{Kind: KindAgent, PaneID: "w1:p1", Machine: "aaa"}
	b := Item{Kind: KindAgent, PaneID: "w1:p1", Machine: "bbb"}
	if a.Key() == b.Key() {
		t.Fatalf("keys collide across machines: %q", a.Key())
	}
}

func TestItemMachineKey(t *testing.T) {
	tests := []struct {
		machine string
		want    string
	}{
		{machine: "", want: "local"},
		{machine: "abc123", want: "abc123"},
	}
	for _, tc := range tests {
		item := Item{Machine: tc.machine}
		if got := item.MachineKey(); got != tc.want {
			t.Fatalf("MachineKey(%q) = %q, want %q", tc.machine, got, tc.want)
		}
	}
}

func TestHerdrKillSessionSkipsFocusRestoreWhenRemote(t *testing.T) {
	t.Setenv("HERDR_WORKSPACE_ID", "wSelf")
	rec := &herdrCmdRecorder{}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	mux := NewHerdrBackend().(MachineSelector).WithMachine(Machine{ID: "aaaa"})
	if err := mux.KillSession(context.Background(), "wOther"); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	want := "--machine\x00aaaa\x00workspace\x00close\x00wOther"
	if len(rec.calls) != 1 || strings.Join(rec.calls[0], "\x00") != want {
		t.Fatalf("calls = %v, want a single scoped close (no local focus restore)", rec.calls)
	}
}

func TestMachineSelectorNotImplementedByTmuxOrNoop(t *testing.T) {
	tests := []struct {
		name string
		mux  Multiplexer
	}{
		{name: "tmux", mux: NewTmuxBackend()},
		{name: "noop", mux: NewNoopBackend()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := tc.mux.(MachineSelector); ok {
				t.Fatalf("%s backend must not implement MachineSelector", tc.name)
			}
		})
	}
	// herdr does implement it, so the selector exists only under herdr.
	if _, ok := NewHerdrBackend().(MachineSelector); !ok {
		t.Fatal("herdr backend must implement MachineSelector")
	}
}

func TestHerdrWithMachineReturnsBoundBackend(t *testing.T) {
	local := NewHerdrBackend()
	bound := local.(MachineSelector).WithMachine(Machine{ID: "aaaa"})
	if bound.Kind() != BackendHerdr {
		t.Fatalf("Kind() = %q, want herdr", bound.Kind())
	}
	// Binding is not sticky on the shared value: the original stays local.
	rec := &herdrCmdRecorder{}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)
	if err := local.KillPane(context.Background(), "w1:p1"); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
	if len(rec.calls) != 1 || rec.calls[0][0] == "--machine" {
		t.Fatalf("calls = %v, want an unscoped close", rec.calls)
	}
}

// --- privacy ---

// TestHerdrMachineLabelsNeverLogged is the privacy sentinel for the machine
// axis: profile ids are logged raw at debug level only, and labels, targets,
// status strings and herdr's own stderr text never reach the JSONL file.
func TestHerdrMachineLabelsNeverLogged(t *testing.T) {
	const (
		labelSentinel  = "PRIVACY-SENTINEL-DEVBOX"
		targetSentinel = "PRIVACY-SENTINEL-USER@HOST"
		statusSentinel = "PRIVACY-SENTINEL-STATUS"
		path           = "machine.jsonl"
	)
	logPath := filepath.Join(t.TempDir(), path)
	resolved, err := logging.Resolve(
		logging.Config{Level: "debug", File: logPath},
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })

	const machineID = "0123456789abcdef0123456789abcdef"
	rec := &herdrCmdRecorder{
		outputF: func(args []string) ([]byte, error) {
			cmd := stripMachinePrefix(args)
			switch strings.Join(cmd, " ") {
			case "machine list --json":
				return []byte(`[{"id":"` + machineID + `","label":"` + labelSentinel +
					`","target":"` + targetSentinel + `","session":"default","enabled":true}]`), nil
			case "machine status --json":
				return []byte(`[{"id":"` + machineID + `","label":"` + labelSentinel +
					`","status":"` + statusSentinel + `","error":"` + targetSentinel + `"}]`), nil
			}
			return nil, errors.New("ssh: " + targetSentinel + " " + labelSentinel)
		},
		runF: func(args []string) error {
			return errors.New("ssh: " + targetSentinel + " " + labelSentinel)
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	ctx := context.Background()
	selector := NewHerdrBackend().(MachineSelector)
	if _, err := selector.Machines(ctx); err != nil {
		t.Fatalf("Machines: %v", err)
	}
	if _, err := selector.MachineStatuses(ctx); err != nil {
		t.Fatalf("MachineStatuses: %v", err)
	}
	bound := selector.WithMachine(Machine{ID: machineID})
	if _, err := bound.ListSessions(ctx); err == nil {
		t.Fatal("ListSessions() error = nil, want the simulated ssh failure")
	}
	if err := bound.KillPane(ctx, "w1:p1"); err == nil {
		t.Fatal("KillPane() error = nil, want the simulated ssh failure")
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{labelSentinel, targetSentinel, statusSentinel} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("user-controlled machine value %q leaked: %s", secret, data)
		}
	}
	if !strings.Contains(string(data), `"event":"machine.command_failed"`) {
		t.Fatalf("machine command failure event missing: %s", data)
	}
	if !strings.Contains(string(data), `"machine_id":"`+machineID+`"`) {
		t.Fatalf("raw machine id missing at debug level: %s", data)
	}
	// Every record must be valid JSON with the safe field set only.
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("record is not JSON: %v (%s)", err, line)
		}
		if record["event"] == "machine.command_failed" {
			if record["component"] != "sessionmgr" {
				t.Fatalf("component = %v", record["component"])
			}
			if record["error_class"] == "" || record["error_class"] == nil {
				t.Fatalf("error_class missing: %v", record)
			}
		}
	}
}

// TestHerdrItemActionsRouteByItemMachine pins that actions carrying an Item
// derive their machine from the item, so an action armed before a machine
// switch never targets the wrong server (the local one, in this case).
func TestHerdrItemActionsRouteByItemMachine(t *testing.T) {
	const remote = "remote-id"
	rec := &herdrCmdRecorder{
		outputF: func([]string) ([]byte, error) {
			return []byte(`{"result":{"type":"pane_process_info","process_info":` +
				`{"pane_id":"w1:p1","foreground_processes":[{"pid":7}]}}}`), nil
		},
	}
	setHerdrHooksForTest(t, rec.outputFn, rec.runFn)

	// A LOCAL backend handed a REMOTE item must still reach the remote server.
	local := NewHerdrBackend()
	item := Item{Kind: KindAgent, PaneID: "w1:p1", Machine: remote}

	if _, err := local.AgentPID(context.Background(), item); err != nil {
		t.Fatalf("AgentPID: %v", err)
	}
	if err := local.RenameAgent(context.Background(), item, "alias"); err != nil {
		t.Fatalf("RenameAgent: %v", err)
	}
	attach := local.AttachOrSwitchCommand(item)
	focus := local.FocusAgentCommand(item)

	wantOutput := "--machine\x00" + remote + "\x00pane\x00process-info\x00--pane\x00w1:p1"
	if len(rec.calls) != 2 || strings.Join(rec.calls[0], "\x00") != wantOutput {
		t.Fatalf("calls = %v, want the process-info call scoped to %q", rec.calls, remote)
	}
	wantRename := "--machine\x00" + remote + "\x00agent\x00rename\x00w1:p1\x00alias"
	if strings.Join(rec.calls[1], "\x00") != wantRename {
		t.Fatalf("calls[1] = %v, want %q", rec.calls[1], wantRename)
	}
	for name, cmd := range map[string][]string{
		"AttachOrSwitchCommand": attach.Args,
		"FocusAgentCommand":     focus.Args,
	} {
		if len(cmd) < 4 || cmd[0] != "herdr" || cmd[1] != "--machine" || cmd[2] != remote {
			t.Fatalf("%s args = %v, want a --machine %q prefix", name, cmd, remote)
		}
	}

	// A local item on a bound backend keeps targeting the bound server.
	rec.calls = nil
	bound := local.(MachineSelector).WithMachine(Machine{ID: "bound-id"})
	if _, err := bound.AgentPID(context.Background(), Item{PaneID: "w2:p1"}); err != nil {
		t.Fatalf("AgentPID: %v", err)
	}
	if len(rec.calls) != 1 || rec.calls[0][1] != "bound-id" {
		t.Fatalf("calls = %v, want the bound machine", rec.calls)
	}
}
