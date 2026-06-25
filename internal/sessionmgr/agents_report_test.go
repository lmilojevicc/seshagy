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
