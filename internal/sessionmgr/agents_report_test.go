package sessionmgr

import (
	"context"
	"testing"
)

func TestNormalizeAgentStateMapping(t *testing.T) {
	cases := []struct {
		input string
		want  AgentState
	}{
		{"working", AgentWorking},
		{"busy", AgentWorking},
		{"running", AgentWorking},
		{"thinking", AgentWorking},
		{"processing", AgentWorking},
		{"blocked", AgentBlocked},
		{"permission", AgentBlocked},
		{"question", AgentBlocked},
		{"waiting", AgentBlocked},
		{"done", AgentDone},
		{"completed", AgentDone},
		{"finished", AgentDone},
		{"idle", AgentIdle},
		{"ready", AgentIdle},
		{"", AgentIdle},
		{"bogus", AgentIdle},
		{"aborted", AgentIdle},
		{"error", AgentIdle},
	}
	for _, tc := range cases {
		if got := NormalizeAgentState(tc.input); got != tc.want {
			t.Errorf("NormalizeAgentState(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestReportAgentWritesOptionsAndSeq(t *testing.T) {
	ft := NewFakeTmux()
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		Name:   "pi",
		State:  AgentWorking,
		Source: "seshagy:pi",
		Seq:    5,
	})
	if err != nil {
		t.Fatalf("ReportAgent() error = %v", err)
	}
	if !applied {
		t.Fatal("ReportAgent() applied = false, want true")
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "working" {
		t.Errorf("@seshagy_agent_state = %q, want working", got)
	}
	if got := ft.Get("%5", "@seshagy_agent_seq"); got != "5" {
		t.Errorf("@seshagy_agent_seq = %q, want 5", got)
	}
	if got := ft.Get("%5", "@seshagy_agent_updated"); got == "" {
		t.Error("@seshagy_agent_updated is empty, want a timestamp")
	}
}

func TestReportAgentStaleSeqIgnored(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_seq", "5")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		State:  AgentWorking,
		Source: "test",
		Seq:    3, // stale: 3 <= 5
	})
	if err != nil {
		t.Fatalf("ReportAgent() error = %v", err)
	}
	if applied {
		t.Fatal("ReportAgent(seq=3, existing=5) applied = true, want false")
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "" {
		t.Errorf("@seshagy_agent_state = %q, want empty (stale ignored)", got)
	}
}

func TestReportAgentEqualSeqIgnored(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_seq", "5")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		State:  AgentWorking,
		Source: "test",
		Seq:    5, // equal: 5 <= 5 is stale
	})
	if err != nil {
		t.Fatalf("ReportAgent() error = %v", err)
	}
	if applied {
		t.Fatal("ReportAgent(seq=5, existing=5) applied = true, want false (strict >)")
	}
}

func TestReportAgentHigherSeqOverwrites(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_seq", "3")
	ft.Set("%5", "@seshagy_agent_state", "idle")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		State:  AgentWorking,
		Source: "test",
		Seq:    5,
	})
	if err != nil {
		t.Fatalf("ReportAgent() error = %v", err)
	}
	if !applied {
		t.Fatal("ReportAgent(seq=5, existing=3) applied = false, want true")
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "working" {
		t.Errorf("@seshagy_agent_state = %q, want working", got)
	}
	if got := ft.Get("%5", "@seshagy_agent_seq"); got != "5" {
		t.Errorf("@seshagy_agent_seq = %q, want 5", got)
	}
}

func TestReleaseAgentClearsAllStateOptions(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_seq", "5")
	ft.Set("%5", "@seshagy_agent_state", "working")
	ft.Set("%5", "@seshagy_agent_name", "pi")
	ft.Set("%5", "@seshagy_agent_source", "seshagy:pi")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReleaseAgent(ctx, AgentRelease{
		Pane:   "%5",
		Source: "seshagy:pi",
		Seq:    5, // equal seq is valid for release
	})
	if err != nil {
		t.Fatalf("ReleaseAgent() error = %v", err)
	}
	if !applied {
		t.Fatal("ReleaseAgent() applied = false, want true")
	}
	// State-bearing options must be cleared.
	for _, opt := range []string{"@seshagy_agent_state", "@seshagy_agent_name", "@seshagy_agent_message", "@seshagy_agent_updated", "@seshagy_agent_source", "@seshagy_agent_session_id"} {
		if got := ft.Get("%5", opt); got != "" {
			t.Errorf("%s = %q after release, want empty", opt, got)
		}
	}
	// @seshagy_agent_seq is retained as the tombstone high-water mark.
	if got := ft.Get("%5", "@seshagy_agent_seq"); got != "5" {
		t.Errorf("@seshagy_agent_seq = %q after release, want 5 (tombstone high-water)", got)
	}
}

func TestReleaseAgentStaleSeqIgnored(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_seq", "5")
	ft.Set("%5", "@seshagy_agent_state", "working")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	applied, err := ReleaseAgent(ctx, AgentRelease{
		Pane:   "%5",
		Source: "test",
		Seq:    3, // stale: 3 < 5
	})
	if err != nil {
		t.Fatalf("ReleaseAgent() error = %v", err)
	}
	if applied {
		t.Fatal("ReleaseAgent(seq=3, existing=5) applied = true, want false")
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "working" {
		t.Errorf("@seshagy_agent_state = %q, want working (stale release ignored)", got)
	}
}

func TestReleaseAgentTombstoneBlocksStaleResurrection(t *testing.T) {
	ft := NewFakeTmux()
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	// Report working at seq=5.
	applied, err := ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		Name:   "pi",
		State:  AgentWorking,
		Source: "test",
		Seq:    5,
	})
	if err != nil || !applied {
		t.Fatalf("ReportAgent(seq=5) failed: applied=%v err=%v", applied, err)
	}

	// Release at seq=10 — tombstone high-water.
	applied, err = ReleaseAgent(ctx, AgentRelease{
		Pane:   "%5",
		Source: "test",
		Seq:    10,
	})
	if err != nil || !applied {
		t.Fatalf("ReleaseAgent(seq=10) failed: applied=%v err=%v", applied, err)
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "" {
		t.Errorf("@seshagy_agent_state = %q after release, want empty", got)
	}
	if got := ft.Get("%5", "@seshagy_agent_seq"); got != "10" {
		t.Errorf("@seshagy_agent_seq = %q after release, want 10 (tombstone)", got)
	}

	// Late stale report at seq=7 (< 10 tombstone) — must NOT resurrect.
	applied, err = ReportAgent(ctx, AgentReport{
		Pane:   "%5",
		State:  AgentDone,
		Source: "test",
		Seq:    7, // stale: 7 <= 10 tombstone
	})
	if err != nil {
		t.Fatalf("ReportAgent(seq=7) error = %v", err)
	}
	if applied {
		t.Fatal(
			"ReportAgent(seq=7, tombstone=10) applied = true, want false (stale resurrection blocked)",
		)
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "" {
		t.Errorf("@seshagy_agent_state = %q after stale report, want empty (no resurrection)", got)
	}
}

func TestMarkAgentVisitedFlipsDoneToIdle(t *testing.T) {
	ft := NewFakeTmux()
	ft.Set("%5", "@seshagy_agent_state", "done")
	ft.Set("%5", "@seshagy_agent_seq", "5")
	SetTmuxHooksForTest(t, ft.output, ft.run)
	ctx := context.Background()

	flipped, err := MarkAgentVisited(ctx, "%5")
	if err != nil {
		t.Fatalf("MarkAgentVisited() error = %v", err)
	}
	if !flipped {
		t.Fatal("MarkAgentVisited() flipped = false, want true for done pane")
	}
	if got := ft.Get("%5", "@seshagy_agent_state"); got != "idle" {
		t.Errorf("@seshagy_agent_state = %q, want idle", got)
	}
	if got := ft.Get("%5", "@seshagy_agent_updated"); got == "" {
		t.Error("@seshagy_agent_updated is empty, want a timestamp")
	}
	if got := ft.Get("%5", "@seshagy_agent_last_seen"); got == "" {
		t.Error("@seshagy_agent_last_seen is empty, want a timestamp")
	}
	// Seq must be unchanged — the visit flip does not advance the epoch.
	if got := ft.Get("%5", "@seshagy_agent_seq"); got != "5" {
		t.Errorf("@seshagy_agent_seq = %q, want 5 (unchanged)", got)
	}
}

func TestMarkAgentVisitedSeqSafeNoClobber(t *testing.T) {
	base := NewFakeTmux()
	base.Set("%5", "@seshagy_agent_state", "done")
	base.Set("%5", "@seshagy_agent_seq", "5")
	s := NewStrictFakeTmux(t, base).AllowPaneOptions()
	// Simulate a higher-seq ReportAgent landing between the two seq reads:
	// the first read returns "5", the defensive re-read returns "9".
	var seqReads int
	s.HandleOutput(func(args []string) bool {
		return len(args) >= 4 && args[0] == "show-option" && args[3] == "@seshagy_agent_seq"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		seqReads++
		if seqReads >= 2 {
			return []byte("9"), nil
		}
		return []byte("5"), nil
	})
	s.Install(t)
	ctx := context.Background()

	flipped, err := MarkAgentVisited(ctx, "%5")
	if err != nil {
		t.Fatalf("MarkAgentVisited() error = %v", err)
	}
	if flipped {
		t.Fatal("MarkAgentVisited() flipped = true, want false (seq changed mid-write)")
	}
	if got := base.Get("%5", "@seshagy_agent_state"); got != "done" {
		t.Errorf("@seshagy_agent_state = %q, want done (not clobbered)", got)
	}
}

func TestMarkAgentVisitedSkipsNonDone(t *testing.T) {
	ctx := context.Background()
	for _, state := range []string{"working", "blocked", "idle", ""} {
		t.Run(state, func(t *testing.T) {
			ft := NewFakeTmux()
			ft.Set("%5", "@seshagy_agent_state", state)
			ft.Set("%5", "@seshagy_agent_seq", "5")
			SetTmuxHooksForTest(t, ft.output, ft.run)

			flipped, err := MarkAgentVisited(ctx, "%5")
			if err != nil {
				t.Fatalf("MarkAgentVisited() error = %v", err)
			}
			if flipped {
				t.Fatalf("MarkAgentVisited(state=%q) flipped = true, want false", state)
			}
			want := state
			if got := ft.Get("%5", "@seshagy_agent_state"); got != want {
				t.Errorf("@seshagy_agent_state = %q, want %q (untouched)", got, want)
			}
		})
	}
}

func TestResolvePaneByCwdExactMatch(t *testing.T) {
	s := NewStrictFakeTmux(t, nil)
	s.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "list-panes"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1\x1f/Users/milo/proj\n%2\x1f/Users/milo/other\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	got, err := ResolvePaneByCwd(ctx, "/Users/milo/proj")
	if err != nil {
		t.Fatalf("ResolvePaneByCwd: %v", err)
	}
	if want := "%1"; got != want {
		t.Errorf("ResolvePaneByCwd = %q, want %q", got, want)
	}
}

func TestResolvePaneByCwdParentChild(t *testing.T) {
	s := NewStrictFakeTmux(t, nil)
	s.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "list-panes"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		// cwd is a child of the pane path → prefix match.
		return []byte("%3\x1f/Users/milo/proj\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	got, err := ResolvePaneByCwd(ctx, "/Users/milo/proj/sub")
	if err != nil {
		t.Fatalf("ResolvePaneByCwd: %v", err)
	}
	if want := "%3"; got != want {
		t.Errorf("ResolvePaneByCwd = %q, want %q (parent/child)", got, want)
	}
}

func TestResolvePaneByCwdAmbiguousRefuses(t *testing.T) {
	s := NewStrictFakeTmux(t, nil)
	s.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "list-panes"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1\x1f/Users/milo/proj\n%2\x1f/Users/milo/proj\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	got, err := ResolvePaneByCwd(ctx, "/Users/milo/proj")
	if err != nil {
		t.Fatalf("ResolvePaneByCwd: %v", err)
	}
	if got != "" {
		t.Errorf("ResolvePaneByCwd = %q, want \"\" (ambiguous, refuse to guess)", got)
	}
}

func TestResolvePaneByCwdNoneFalse(t *testing.T) {
	s := NewStrictFakeTmux(t, nil)
	s.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "list-panes"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1\x1f/elsewhere\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	got, err := ResolvePaneByCwd(ctx, "/Users/milo/proj")
	if err != nil {
		t.Fatalf("ResolvePaneByCwd: %v", err)
	}
	if got != "" {
		t.Errorf("ResolvePaneByCwd = %q, want \"\" (no match)", got)
	}
}

func TestMarkActiveDoneAgentsIdleFlipsActiveDonePane(t *testing.T) {
	base := NewFakeTmux()
	base.Set("%1", "@seshagy_agent_state", "done")
	base.Set("%1", "@seshagy_agent_seq", "1")
	base.Set("%2", "@seshagy_agent_state", "done")
	base.Set("%2", "@seshagy_agent_seq", "2")
	s := NewStrictFakeTmux(t, base).AllowPaneOptions()
	s.HandleOutput(func(args []string) bool {
		// list-panes -a -f #{pane_active} -F #{pane_id} → only %1 is active.
		return len(args) >= 1 && args[0] == "list-panes"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1"), nil
	})
	s.Install(t)
	ctx := context.Background()

	items := []Item{
		{Kind: KindAgent, AgentState: AgentDone, PaneID: "%1"},
		{Kind: KindAgent, AgentState: AgentDone, PaneID: "%2"},
		{Kind: KindAgent, AgentState: AgentWorking, PaneID: "%3"},
	}
	MarkActiveDoneAgentsIdle(ctx, items)

	if got := base.Get("%1", "@seshagy_agent_state"); got != "idle" {
		t.Errorf("active done pane %%1 state = %q, want idle", got)
	}
	if got := base.Get("%2", "@seshagy_agent_state"); got != "done" {
		t.Errorf("inactive done pane %%2 state = %q, want done (untouched)", got)
	}
}

func TestMarkActiveDoneAgentsIdleNoopWithoutDone(t *testing.T) {
	called := false
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			called = true
		}
		return nil, nil
	}, nil)
	ctx := context.Background()

	MarkActiveDoneAgentsIdle(ctx, []Item{
		{Kind: KindAgent, AgentState: AgentWorking, PaneID: "%1"},
	})
	if called {
		t.Fatal("list-panes called when no done agents exist; want no tmux call")
	}
}
