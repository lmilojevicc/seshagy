package sessionmgr

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReportAgentRequiresName(t *testing.T) {
	const pane = "%15"
	ctx := context.Background()
	InstallReportFakeTmux(t, pane)

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
	})
	if err == nil {
		t.Fatal("ReportAgent() expected name required error")
	}
	if !strings.Contains(err.Error(), "--agent/--name is required") {
		t.Fatalf("ReportAgent() error = %v", err)
	}
	if applied {
		t.Fatal("ReportAgent() applied = true, want false on validation error")
	}
}

func TestReportAgentRejectsStaleSeq(t *testing.T) {
	const pane = "%1"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	report := func(seq int64, state AgentState) (bool, error) {
		return ReportAgent(ctx, AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      state,
			Source:     "hook",
			SourceSeen: true,
			Seq:        seq,
			SeqSeen:    true,
		})
	}

	applied, err := report(10, AgentWorking)
	if err != nil {
		t.Fatalf("report seq 10: %v", err)
	}
	if !applied {
		t.Fatal("report seq 10 should apply")
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after seq 10 = %q, want working", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "10" {
		t.Fatalf("@agent_seq after seq 10 = %q, want 10", got)
	}

	applied, err = report(9, AgentIdle)
	if err != nil {
		t.Fatalf("report seq 9: %v", err)
	}
	if applied {
		t.Fatal("stale seq 9 should not apply")
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("stale seq changed @agent_state = %q, want working", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "10" {
		t.Fatalf("stale seq changed @agent_seq = %q, want 10", got)
	}
}

func TestReportAgentRejectsEqualSeq(t *testing.T) {
	const pane = "%5"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	report := func(seq int64, state AgentState, message string) (bool, error) {
		return ReportAgent(ctx, AgentReport{
			Pane:        pane,
			Name:        "pi",
			State:       state,
			Message:     message,
			MessageSeen: true,
			Source:      "hook",
			SourceSeen:  true,
			Seq:         seq,
			SeqSeen:     true,
		})
	}

	applied, err := report(10, AgentWorking, "busy")
	if err != nil {
		t.Fatalf("report seq 10: %v", err)
	}
	if !applied {
		t.Fatal("report seq 10 should apply")
	}
	snap := map[string]string{
		"@agent_name":    f.Get(pane, "@agent_name"),
		"@agent_state":   f.Get(pane, "@agent_state"),
		"@agent_message": f.Get(pane, "@agent_message"),
		"@agent_seq":     f.Get(pane, "@agent_seq"),
	}

	applied, err = report(10, AgentIdle, "duplicate")
	if err != nil {
		t.Fatalf("report equal seq 10: %v", err)
	}
	if applied {
		t.Fatal("equal seq 10 should not apply")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("equal seq changed %s = %q, want %q", opt, got, want)
		}
	}
}

func TestReportAgentClearsMessageWhenEmptyMessageSeen(t *testing.T) {
	const pane = "%6"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:        pane,
		Name:        "pi",
		State:       AgentWorking,
		Message:     "busy",
		MessageSeen: true,
		Source:      "hook",
		SourceSeen:  true,
		Seq:         10,
		SeqSeen:     true,
	}); err != nil {
		t.Fatalf("report with message: %v", err)
	}
	if got := f.Get(pane, "@agent_message"); got != "busy" {
		t.Fatalf("@agent_message = %q, want busy", got)
	}

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:        pane,
		Name:        "pi",
		State:       AgentWorking,
		Message:     "",
		MessageSeen: true,
		Source:      "hook",
		SourceSeen:  true,
		Seq:         11,
		SeqSeen:     true,
	}); err != nil {
		t.Fatalf("report clearing message: %v", err)
	}
	if got := f.Get(pane, "@agent_message"); got != "" {
		t.Fatalf("@agent_message = %q, want cleared", got)
	}
}

func TestReportAgentAppliesNewerSeq(t *testing.T) {
	const pane = "%2"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        10,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 10: %v", err)
	}
	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentBlocked,
		Source:     "hook",
		SourceSeen: true,
		Seq:        11,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 11: %v", err)
	}
	if got := f.Get(pane, "@agent_state"); got != "blocked" {
		t.Fatalf("@agent_state after seq 11 = %q, want blocked", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "11" {
		t.Fatalf("@agent_seq after seq 11 = %q, want 11", got)
	}
}

func TestReportAgentDoesNotResurrectAfterReleaseTombstone(t *testing.T) {
	const pane = "%3"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        100,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 100: %v", err)
	}
	if !applied {
		t.Fatal("report seq 100 should apply")
	}
	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        101,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release seq 101: %v", err)
	}
	if !released {
		t.Fatal("release seq 101 should apply")
	}
	if got := f.Get(pane, "@agent_seq"); got != "101" {
		t.Fatalf("tombstone @agent_seq = %q, want 101", got)
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("release @agent_name = %q, want cleared", got)
	}

	applied, err = ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        100,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("stale report seq 100: %v", err)
	}
	if applied {
		t.Fatal("stale report seq 100 should not apply")
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("stale report resurrected @agent_name = %q", got)
	}
	if got := f.Get(pane, "@agent_state"); got != "" {
		t.Fatalf("stale report resurrected @agent_state = %q", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "101" {
		t.Fatalf("stale report changed @agent_seq = %q, want tombstone 101", got)
	}
}

func TestReportAgentSeqRejectionLeavesOptionsSnapshot(t *testing.T) {
	const pane = "%4"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	before := func() map[string]string {
		snap := map[string]string{}
		for _, opt := range []string{"@agent_name", "@agent_state", "@agent_message", "@agent_seq"} {
			snap[opt] = f.Get(pane, opt)
		}
		return snap
	}

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:        pane,
		Name:        "pi",
		State:       AgentWorking,
		Message:     "busy",
		MessageSeen: true,
		Source:      "hook",
		SourceSeen:  true,
		Seq:         10,
		SeqSeen:     true,
	})
	if err != nil {
		t.Fatalf("report seq 10: %v", err)
	}
	if !applied {
		t.Fatal("report seq 10 should apply")
	}
	snap := before()

	applied, err = ReportAgent(ctx, AgentReport{
		Pane:        pane,
		Name:        "pi",
		State:       AgentIdle,
		Message:     "stale",
		MessageSeen: true,
		Source:      "hook",
		SourceSeen:  true,
		Seq:         9,
		SeqSeen:     true,
	})
	if err != nil {
		t.Fatalf("report seq 9: %v", err)
	}
	if applied {
		t.Fatal("stale seq 9 should not apply")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("stale seq changed %s = %q, want %q", opt, got, want)
		}
	}
	if strings.Contains(f.Get(pane, "@agent_message"), "stale") {
		t.Fatalf("stale seq updated message = %q", f.Get(pane, "@agent_message"))
	}
}

func TestReportAgentRejectsSeqLessWhenSeqExists(t *testing.T) {
	const pane = "%11"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        50,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 50: %v", err)
	}
	if !applied {
		t.Fatal("report seq 50 should apply")
	}

	applied, err = ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentIdle,
		Source:     "hook",
		SourceSeen: true,
	})
	if err != nil {
		t.Fatalf("seq-less report: %v", err)
	}
	if applied {
		t.Fatal("seq-less report should not apply when @agent_seq exists")
	}
	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state after seq-less report = %q, want working", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "50" {
		t.Fatalf("@agent_seq after seq-less report = %q, want 50", got)
	}
}

func TestReleaseAgentRejectsStaleSeq(t *testing.T) {
	const pane = "%7"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        20,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 20: %v", err)
	}
	snap := map[string]string{
		"@agent_name":  f.Get(pane, "@agent_name"),
		"@agent_state": f.Get(pane, "@agent_state"),
		"@agent_seq":   f.Get(pane, "@agent_seq"),
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        19,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release stale seq 19: %v", err)
	}
	if released {
		t.Fatal("stale release should not apply")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("stale release changed %s = %q, want %q", opt, got, want)
		}
	}
}

func TestReleaseAgentRejectsEqualSeq(t *testing.T) {
	const pane = "%8"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        30,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 30: %v", err)
	}
	snap := map[string]string{
		"@agent_name":  f.Get(pane, "@agent_name"),
		"@agent_state": f.Get(pane, "@agent_state"),
		"@agent_seq":   f.Get(pane, "@agent_seq"),
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        30,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release equal seq 30: %v", err)
	}
	if released {
		t.Fatal("equal release should not apply")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("equal release changed %s = %q, want %q", opt, got, want)
		}
	}
}

func TestReleaseAgentRejectsDifferentActiveSource(t *testing.T) {
	const pane = "%9"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook-a",
		SourceSeen: true,
		Seq:        40,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 40: %v", err)
	}
	snap := map[string]string{
		"@agent_name":   f.Get(pane, "@agent_name"),
		"@agent_state":  f.Get(pane, "@agent_state"),
		"@agent_source": f.Get(pane, "@agent_source"),
		"@agent_seq":    f.Get(pane, "@agent_seq"),
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook-b",
		SourceSeen: true,
		Seq:        41,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release from different source: %v", err)
	}
	if released {
		t.Fatal("different-source release should not apply")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("different-source release changed %s = %q, want %q", opt, got, want)
		}
	}
}

func TestConcurrentReportAgentSeqOrdering(t *testing.T) {
	// withAgentPaneLock serializes concurrent ReportAgent calls per pane; the
	// higher seq must win regardless of which goroutine acquires the lock first.
	const pane = "%10"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	type reportOutcome struct {
		seq int64
		err error
	}

	report := func(seq int64, state AgentState) reportOutcome {
		_, err := ReportAgent(ctx, AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      state,
			Source:     "hook",
			SourceSeen: true,
			Seq:        seq,
			SeqSeen:    true,
		})
		return reportOutcome{seq: seq, err: err}
	}

	results := make(chan reportOutcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		results <- report(99, AgentWorking)
	}()
	go func() {
		defer wg.Done()
		results <- report(100, AgentBlocked)
	}()
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("report seq %d: %v", r.seq, r.err)
		}
	}

	if got := f.Get(pane, "@agent_seq"); got != "100" {
		t.Fatalf("@agent_seq = %q, want 100", got)
	}
	if got := f.Get(pane, "@agent_state"); got != "blocked" {
		t.Fatalf("@agent_state = %q, want blocked from higher seq", got)
	}
}

func TestReportAfterReleaseSeqOrdering(t *testing.T) {
	// Release first, then report with lower seq — release tombstone must win.
	const pane = "%12"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        201,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release seq 201: %v", err)
	}
	if !released {
		t.Fatal("release seq 201 should apply")
	}

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        200,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 200: %v", err)
	}
	if applied {
		t.Fatal("report seq 200 should not apply after release tombstone")
	}
	if got := f.Get(pane, "@agent_seq"); got != "201" {
		t.Fatalf("@agent_seq = %q, want release tombstone 201", got)
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("@agent_name = %q, want cleared by release", got)
	}
	if got := f.Get(pane, "@agent_state"); got != "" {
		t.Fatalf("@agent_state = %q, want cleared by release", got)
	}
}

func TestReportThenReleaseSeqOrdering(t *testing.T) {
	// Report first, then release with higher seq — release tombstone must win.
	const pane = "%16"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        300,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 300: %v", err)
	}
	if !applied {
		t.Fatal("report seq 300 should apply when it runs first")
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        301,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release seq 301: %v", err)
	}
	if !released {
		t.Fatal("release seq 301 should apply after report")
	}
	if got := f.Get(pane, "@agent_seq"); got != "301" {
		t.Fatalf("@agent_seq = %q, want 301", got)
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("@agent_name = %q, want cleared by release", got)
	}
}

func TestReleaseThenReportSeqOrdering(t *testing.T) {
	// Release completes before report; stale report must not resurrect metadata.
	const pane = "%17"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        401,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("release seq 401 should apply")
	}

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        400,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if applied {
		t.Fatal("report seq 400 should not apply after release seq 401 tombstone")
	}
	if got := f.Get(pane, "@agent_seq"); got != "401" {
		t.Fatalf("@agent_seq = %q, want 401", got)
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("@agent_name = %q, want cleared", got)
	}
}

func TestReleaseAgentRejectsSeqLessWhenSeqExists(t *testing.T) {
	const pane = "%18"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	if _, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        70,
		SeqSeen:    true,
	}); err != nil {
		t.Fatalf("report seq 70: %v", err)
	}
	snap := map[string]string{
		"@agent_name":  f.Get(pane, "@agent_name"),
		"@agent_state": f.Get(pane, "@agent_state"),
		"@agent_seq":   f.Get(pane, "@agent_seq"),
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
	})
	if err != nil {
		t.Fatalf("seq-less release: %v", err)
	}
	if released {
		t.Fatal("seq-less release should not apply when @agent_seq exists")
	}
	for opt, want := range snap {
		if got := f.Get(pane, opt); got != want {
			t.Fatalf("seq-less release changed %s = %q, want %q", opt, got, want)
		}
	}
}

func TestReleaseAgentWithoutSourceClearsForeignMetadata(t *testing.T) {
	// Release without --source skips source matching and clears metadata even when
	// @agent_source was set by a different hook integration.
	const pane = "%13"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook-a",
		SourceSeen: true,
		Seq:        60,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 60: %v", err)
	}
	if !applied {
		t.Fatal("report seq 60 should apply")
	}
	if got := f.Get(pane, "@agent_source"); got != "hook-a" {
		t.Fatalf("@agent_source = %q, want hook-a", got)
	}

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:    pane,
		Seq:     61,
		SeqSeen: true,
	})
	if err != nil {
		t.Fatalf("release without source seq 61: %v", err)
	}
	if !released {
		t.Fatal("release without source should apply")
	}
	for _, opt := range []string{"@agent_name", "@agent_state", "@agent_source"} {
		if got := f.Get(pane, opt); got != "" {
			t.Fatalf("release without source left %s = %q, want cleared", opt, got)
		}
	}
	if got := f.Get(pane, "@agent_seq"); got != "61" {
		t.Fatalf("@agent_seq = %q, want tombstone 61", got)
	}
}

func TestReportAgentRejectsWhenSeqCorrupted(t *testing.T) {
	const pane = "%14"
	ctx := context.Background()
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))
	f := InstallReportFakeTmux(t, pane)

	f.Set(pane, "@agent_seq", "bad")

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        10,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 10: %v", err)
	}
	if applied {
		t.Fatal("report should not apply when existing @agent_seq is malformed")
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("@agent_name = %q, want unchanged", got)
	}
}

func TestReleaseAgentRejectsWhenSeqCorrupted(t *testing.T) {
	const pane = "%15"
	ctx := context.Background()
	f := InstallReportFakeTmux(t, pane)

	f.Set(pane, "@agent_name", "pi")
	f.Set(pane, "@agent_state", string(AgentWorking))
	f.Set(pane, "@agent_source", "hook")
	f.Set(pane, "@agent_seq", "bad")

	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        11,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release seq 11: %v", err)
	}
	if released {
		t.Fatal("release should not apply when existing @agent_seq is malformed")
	}
	if got := f.Get(pane, "@agent_name"); got != "pi" {
		t.Fatalf("@agent_name = %q, want unchanged", got)
	}
}
