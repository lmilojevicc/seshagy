package sessionmgr

import (
	"errors"
	"testing"
)

// setProcSnapshotForTest swaps the procSnapshot seam for the duration of a test.
func setProcSnapshotForTest(t *testing.T, fn func() (map[int32]procEntry, error)) {
	t.Helper()
	orig := procSnapshot
	procSnapshot = fn
	t.Cleanup(func() { procSnapshot = orig })
}

func TestDescendantsFindsChildrenAtDepth(t *testing.T) {
	table := map[int32]procEntry{
		1: {pid: 1, ppid: 0, comm: "zsh"},
		2: {pid: 2, ppid: 1, comm: "pi"},
		3: {pid: 3, ppid: 2, comm: "node"},
		4: {pid: 4, ppid: 1, comm: "claude"},
	}
	got := descendants(table, 1)
	want := map[int32]bool{2: true, 3: true, 4: true}
	if len(got) != len(want) {
		t.Fatalf("descendants(root=1) = %v, want %d pids", got, len(want))
	}
	for _, pid := range got {
		if !want[pid] {
			t.Errorf("descendants(root=1) contains unexpected pid %d", pid)
		}
	}
}

func TestDescendantsStopsAtMaxDepth(t *testing.T) {
	// Chain deeper than maxProcDepth so the bound is genuinely exercised:
	// 1->2->3->4->5->6->7->8 (7 levels of descent from root=1). With
	// maxProcDepth=4 the BFS returns generations 1-4 (pids 2-5) and must
	// exclude generations 5-7 (pids 6-8). This fails if the depth bound is
	// removed or raised beyond the chain.
	const chainLen = 8
	table := map[int32]procEntry{}
	for pid := int32(1); pid <= chainLen; pid++ {
		table[pid] = procEntry{pid: pid, ppid: pid - 1, comm: "proc"}
	}
	got := descendants(table, 1)
	gotSet := map[int32]bool{}
	for _, pid := range got {
		gotSet[pid] = true
	}
	// First maxProcDepth generations (pids 2..maxProcDepth+1) must be present.
	for pid := int32(2); pid <= int32(maxProcDepth)+1; pid++ {
		if !gotSet[pid] {
			t.Errorf("descendants(root=1) missing pid %d within maxProcDepth=%d", pid, maxProcDepth)
		}
	}
	// Deeper generations must be absent.
	for pid := int32(maxProcDepth) + 2; pid <= chainLen; pid++ {
		if gotSet[pid] {
			t.Errorf(
				"descendants(root=1) includes pid %d beyond maxProcDepth=%d",
				pid,
				maxProcDepth,
			)
		}
	}
}

func TestDescendantsHandlesCycle(t *testing.T) {
	table := map[int32]procEntry{
		1: {pid: 1, ppid: 0, comm: "shell"},
		2: {pid: 2, ppid: 3, comm: "a"}, // artificial cycle: 2's parent is 3
		3: {pid: 3, ppid: 2, comm: "b"},
	}
	got := descendants(table, 1)
	seen := map[int32]bool{}
	for _, pid := range got {
		if seen[pid] {
			t.Errorf("descendants returned duplicate pid %d (cycle not handled)", pid)
		}
		seen[pid] = true
	}
}

func TestDescendantsSkipsPidZeroOne(t *testing.T) {
	table := map[int32]procEntry{
		0: {pid: 0, ppid: 0, comm: "kernel"},
		1: {pid: 1, ppid: 0, comm: "launchd"},
		5: {pid: 5, ppid: 1, comm: "real"},
	}
	got := descendants(table, 1)
	for _, pid := range got {
		if pid <= 1 {
			t.Errorf("descendants returned pid %d (should skip 0 and 1)", pid)
		}
	}
}

func TestDetectAgentNodeResolvesToPiViaDescendants(t *testing.T) {
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		return map[int32]procEntry{
			100: {pid: 100, ppid: 99, comm: "zsh"},
			200: {pid: 200, ppid: 100, comm: "pi"},
		}, nil
	})
	cache := &snapshotCache{}
	got := detectAgent("node", "100", cache)
	if got != "pi" {
		t.Errorf("detectAgent(\"node\", \"100\", cache) = %q, want \"pi\"", got)
	}
}

func TestDetectAgentNodeResolvesCursorAgentViaBasename(t *testing.T) {
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		return map[int32]procEntry{
			100: {pid: 100, ppid: 99, comm: "zsh"},
			200: {pid: 200, ppid: 100, comm: "/Users/milo/.local/bin/cursor-agent"},
		}, nil
	})
	cache := &snapshotCache{}
	got := detectAgent("node", "100", cache)
	if got != "cursor" {
		t.Errorf("detectAgent(\"node\", \"100\", cache) = %q, want \"cursor\"", got)
	}
}

func TestDetectAgentFastPathSkipsSnapshot(t *testing.T) {
	calls := 0
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		calls++
		return nil, nil
	})
	cache := &snapshotCache{}
	got := detectAgent("claude", "100", cache)
	if got != "claude" {
		t.Fatalf("detectAgent(\"claude\", ...) = %q, want \"claude\"", got)
	}
	if calls != 0 {
		t.Errorf("procSnapshot called %d times on fast path, want 0", calls)
	}
}

func TestDetectAgentUnknownReturnsEmpty(t *testing.T) {
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		return map[int32]procEntry{
			100: {pid: 100, ppid: 99, comm: "zsh"},
			200: {pid: 200, ppid: 100, comm: "vim"},
		}, nil
	})
	cache := &snapshotCache{}
	got := detectAgent("bash", "100", cache)
	if got != "" {
		t.Errorf("detectAgent(\"bash\", ...) = %q, want \"\"", got)
	}
}

func TestParseAgentsSnapshotInvokedOnce(t *testing.T) {
	calls := 0
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		calls++
		return map[int32]procEntry{
			100: {pid: 100, ppid: 99, comm: "zsh"},
			200: {pid: 200, ppid: 100, comm: "pi"},
			300: {pid: 300, ppid: 99, comm: "zsh"},
			400: {pid: 400, ppid: 300, comm: "opencode"},
		}, nil
	})

	raw := agentPaneLine("%1", "work", "0", "0", "/home/work", "node", "100", "0") + "\n" +
		agentPaneLine("%2", "work", "1", "0", "/home/work", "node", "300", "0") + "\n" +
		agentPaneLine("%3", "work", "2", "0", "/home/work", "claude", "500", "0")

	items := ParseAgents([]byte(raw), "")
	if calls != 1 {
		t.Errorf("procSnapshot called %d times across multi-pane parse, want 1", calls)
	}
	if len(items) != 3 {
		t.Fatalf("ParseAgents() = %d items, want 3", len(items))
	}
	names := map[string]bool{}
	for _, item := range items {
		names[item.AgentName] = true
	}
	for _, want := range []string{"pi", "opencode", "claude"} {
		if !names[want] {
			t.Errorf("expected agent %q in results", want)
		}
	}
}

func TestDetectAgentPanePidAbsentFromSnapshot(t *testing.T) {
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		return map[int32]procEntry{
			100: {pid: 100, ppid: 99, comm: "zsh"},
			200: {pid: 200, ppid: 100, comm: "pi"},
		}, nil
	})
	cache := &snapshotCache{}
	// pane_pid 999 does not exist in the snapshot (process exited between
	// the tmux snapshot and the ps call).
	got := detectAgent("node", "999", cache)
	if got != "" {
		t.Errorf("detectAgent(\"node\", \"999\", cache) = %q, want \"\" for absent pane_pid", got)
	}
}

func TestDetectAgentSnapshotErrorReturnsEmpty(t *testing.T) {
	setProcSnapshotForTest(t, func() (map[int32]procEntry, error) {
		return nil, errors.New("ps: command not found")
	})
	cache := &snapshotCache{}
	got := detectAgent("node", "100", cache)
	if got != "" {
		t.Errorf("detectAgent(\"node\", \"100\", cache) = %q, want \"\" on snapshot error", got)
	}
}

func TestParsePsSnapshotParsesPidPpidComm(t *testing.T) {
	raw := []byte(" 100   99 pi\n" +
		"200 100 node\n" +
		"300   99 /usr/local/bin/cursor-agent\n" +
		"\n") // trailing empty line
	table := parsePsSnapshot(raw)
	if len(table) != 3 {
		t.Fatalf("parsePsSnapshot() = %d entries, want 3", len(table))
	}
	if table[100].comm != "pi" {
		t.Errorf("table[100].comm = %q, want pi", table[100].comm)
	}
	if table[100].ppid != 99 {
		t.Errorf("table[100].ppid = %d, want 99", table[100].ppid)
	}
	if table[300].comm != "/usr/local/bin/cursor-agent" {
		t.Errorf("table[300].comm = %q, want full path", table[300].comm)
	}
}
