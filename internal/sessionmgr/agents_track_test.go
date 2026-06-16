package sessionmgr

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func seedPendingIdleConfirmed(f *FakeTmux, pane string, now time.Time) {
	f.Set(pane, "@agent_pending_idle_since", formatUnix(now.Add(-agentPendingIdleDebounce)))
	f.Set(pane, "@agent_pending_idle_count", "2")
}

func seedStartupGraceExpired(f *FakeTmux, pane string, now time.Time) {
	f.Set(pane, "@agent_name", "claude")
	f.Set(pane, "@agent_startup_grace", formatUnix(now.Add(-agentStartupGraceWindow)))
}

func TestUpdateAgentStatusTracking(t *testing.T) {
	const pane = "%1"
	tests := []struct {
		name               string
		detected           AgentState
		visible            bool
		lifecycleAuthority bool
		lastState          AgentState // seeds @agent_last_state
		lastStatus         AgentState // seeds @agent_last_status
		wantStatus         AgentState
		wantLastSeen       bool
	}{
		{
			name:               "done visible reports idle",
			detected:           AgentDone,
			visible:            true,
			lifecycleAuthority: true,
			wantStatus:         AgentIdle,
			wantLastSeen:       true,
		},
		{
			name:               "done background stays done",
			detected:           AgentDone,
			visible:            false,
			lifecycleAuthority: true,
			wantStatus:         AgentDone,
		},
		{
			name:               "aborted visible reports idle",
			detected:           AgentAborted,
			visible:            true,
			lifecycleAuthority: true,
			wantStatus:         AgentIdle,
			wantLastSeen:       true,
		},
		{
			name:               "aborted background stays aborted",
			detected:           AgentAborted,
			visible:            false,
			lifecycleAuthority: true,
			wantStatus:         AgentAborted,
		},
		{
			name:               "idle visible stays idle",
			detected:           AgentIdle,
			visible:            true,
			lifecycleAuthority: true,
			wantStatus:         AgentIdle,
			wantLastSeen:       true,
		},
		{
			name:               "idle background after done keeps done",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: true,
			lastStatus:         AgentDone,
			wantStatus:         AgentDone,
		},
		{
			name:               "idle background after aborted keeps aborted",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: true,
			lastStatus:         AgentAborted,
			wantStatus:         AgentAborted,
		},
		{
			name:               "idle background after working becomes done",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: true,
			lastState:          AgentWorking,
			wantStatus:         AgentDone,
		},
		{
			name:               "idle background after blocked becomes done",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: true,
			lastState:          AgentBlocked,
			wantStatus:         AgentDone,
		},
		{
			name:               "session-only idle background after working stays idle",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: false,
			lastState:          AgentWorking,
			wantStatus:         AgentIdle,
		},
		{
			name:               "idle background fresh stays idle",
			detected:           AgentIdle,
			visible:            false,
			lifecycleAuthority: true,
			wantStatus:         AgentIdle,
		},
		{
			name:               "working passes through",
			detected:           AgentWorking,
			visible:            false,
			lifecycleAuthority: true,
			wantStatus:         AgentWorking,
		},
		{
			name:               "blocked passes through",
			detected:           AgentBlocked,
			visible:            true,
			lifecycleAuthority: true,
			wantStatus:         AgentBlocked,
			wantLastSeen:       true,
		},
		{
			name:               "unknown passes through",
			detected:           AgentUnknown,
			visible:            false,
			lifecycleAuthority: false,
			lastState:          AgentWorking,
			wantStatus:         AgentUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			SetFixedTrackTime(t, now)
			f := NewFakeTmux()
			if tt.lastState != "" {
				f.Set(pane, "@agent_last_state", string(tt.lastState))
			}
			if tt.lastStatus != "" {
				f.Set(pane, "@agent_last_status", string(tt.lastStatus))
			}
			if tt.wantStatus == AgentDone {
				seedStartupGraceExpired(f, pane, now)
				seedPendingIdleConfirmed(f, pane, now)
			}
			InstallTrackFakeTmux(t, f)

			got, err := UpdateAgentStatusTracking(
				context.Background(),
				pane,
				tt.detected,
				tt.visible,
				tt.lifecycleAuthority,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got, tt.wantStatus)
			}
			if persisted := f.Get(pane, "@agent_last_status"); persisted != string(tt.wantStatus) {
				t.Fatalf("@agent_last_status = %q, want %q", persisted, tt.wantStatus)
			}
			if seen := f.Get(pane, "@agent_last_seen") != ""; seen != tt.wantLastSeen {
				t.Fatalf("@agent_last_seen present = %v, want %v", seen, tt.wantLastSeen)
			}
		})
	}
}

func TestUpdateAgentStatusTrackingEmptyPane(t *testing.T) {
	f := NewFakeTmux()
	InstallTrackFakeTmux(t, f)
	got, err := UpdateAgentStatusTracking(context.Background(), "", AgentWorking, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentWorking {
		t.Fatalf("status = %q, want %q", got, AgentWorking)
	}
}

func TestUpdateAgentStatusTrackingPendingIdleDebounce(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_000, 0)
	SetFixedTrackTime(t, now)
	f := NewFakeTmux()
	f.Set(pane, "@agent_name", "claude")
	f.Set(pane, "@agent_last_state", string(AgentWorking))
	f.Set(pane, "@agent_last_status", string(AgentWorking))
	f.Set(pane, "@agent_startup_grace", formatUnix(now.Add(-agentStartupGraceWindow)))
	InstallTrackFakeTmux(t, f)

	got, err := UpdateAgentStatusTracking(
		context.Background(),
		pane,
		AgentIdle,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentWorking {
		t.Fatalf("first idle status = %q, want %q", got, AgentWorking)
	}
	if f.Get(pane, "@agent_last_state") != string(AgentWorking) {
		t.Fatalf("@agent_last_state = %q, want working", f.Get(pane, "@agent_last_state"))
	}
	if f.Get(pane, "@agent_pending_idle_count") != "1" {
		t.Fatalf("@agent_pending_idle_count = %q, want 1", f.Get(pane, "@agent_pending_idle_count"))
	}

	got, err = UpdateAgentStatusTracking(context.Background(), pane, AgentIdle, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentWorking {
		t.Fatalf("second idle status = %q, want %q", got, AgentWorking)
	}

	now = now.Add(agentPendingIdleDebounce)
	SetFixedTrackTime(t, now)
	got, err = UpdateAgentStatusTracking(context.Background(), pane, AgentIdle, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentDone {
		t.Fatalf("confirmed idle status = %q, want %q", got, AgentDone)
	}
	if f.Get(pane, "@agent_pending_idle_count") != "" {
		t.Fatalf(
			"pending idle count should be cleared, got %q",
			f.Get(pane, "@agent_pending_idle_count"),
		)
	}
}

func TestUpdateAgentStatusTrackingStartupGraceSkipsDoneInference(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_000, 0)
	SetFixedTrackTime(t, now)
	f := NewFakeTmux()
	f.Set(pane, "@agent_name", "claude")
	f.Set(pane, "@agent_last_state", string(AgentWorking))
	f.Set(pane, "@agent_last_status", string(AgentWorking))
	seedPendingIdleConfirmed(f, pane, now)
	InstallTrackFakeTmux(t, f)

	got, err := UpdateAgentStatusTracking(
		context.Background(),
		pane,
		AgentIdle,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentIdle {
		t.Fatalf("status = %q, want %q", got, AgentIdle)
	}
	if grace := f.Get(pane, "@agent_startup_grace"); grace == "" {
		t.Fatal("expected @agent_startup_grace to be set")
	}
}

func TestUpdateAgentStatusTrackingWorkingClearsPendingIdle(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_000, 0)
	SetFixedTrackTime(t, now)
	f := NewFakeTmux()
	f.Set(pane, "@agent_last_state", string(AgentWorking))
	f.Set(pane, "@agent_pending_idle_since", formatUnix(now))
	f.Set(pane, "@agent_pending_idle_count", "2")
	InstallTrackFakeTmux(t, f)

	got, err := UpdateAgentStatusTracking(
		context.Background(),
		pane,
		AgentWorking,
		false,
		true,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentWorking {
		t.Fatalf("status = %q, want %q", got, AgentWorking)
	}
	if f.Get(pane, "@agent_pending_idle_count") != "" {
		t.Fatalf(
			"pending idle count should be cleared, got %q",
			f.Get(pane, "@agent_pending_idle_count"),
		)
	}
}

func TestMarkAgentSeenIdleResetsState(t *testing.T) {
	const pane = "%1"
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_state", string(AgentAborted))

	seen, err := MarkAgentSeen(context.Background(), pane)
	if err != nil {
		t.Fatalf("MarkAgentSeen() err = %v", err)
	}
	if !seen {
		t.Fatal("MarkAgentSeen() seen = false, want true")
	}

	if got := f.Get(pane, "@agent_state"); got != string(AgentIdle) {
		t.Fatalf("@agent_state = %q, want idle", got)
	}
	if got := f.Get(pane, "@agent_last_status"); got != string(AgentIdle) {
		t.Fatalf("@agent_last_status = %q, want idle", got)
	}
}

func TestMarkAgentSeenUpdatesTimestamp(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_100, 0)
	SetFixedTrackTime(t, now)
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_state", string(AgentWorking))

	seen, err := MarkAgentSeen(context.Background(), pane)
	if err != nil {
		t.Fatalf("MarkAgentSeen() err = %v", err)
	}
	if !seen {
		t.Fatal("MarkAgentSeen() seen = false, want true")
	}

	want := formatUnix(now)
	if got := f.Get(pane, "@agent_last_seen"); got != want {
		t.Fatalf("@agent_last_seen = %q, want %q", got, want)
	}
}

func TestMarkAgentSeenSkipsStateResetWhenSeqTombstonePresent(t *testing.T) {
	const pane = "%2"
	now := time.Unix(1_700_000_100, 0)
	SetFixedTrackTime(t, now)
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_state", string(AgentAborted))
	f.Set(pane, "@agent_seq", "101")
	f.Set(pane, "@agent_last_status", string(AgentAborted))

	seen, err := MarkAgentSeen(context.Background(), pane)
	if err != nil {
		t.Fatalf("MarkAgentSeen() err = %v", err)
	}
	if !seen {
		t.Fatal("MarkAgentSeen() seen = false, want true")
	}

	if got := f.Get(pane, "@agent_state"); got != string(AgentAborted) {
		t.Fatalf("@agent_state = %q, want aborted (seq tombstone blocks reset)", got)
	}
	if got := f.Get(pane, "@agent_last_status"); got != string(AgentAborted) {
		t.Fatalf("@agent_last_status = %q, want aborted (seq tombstone blocks reset)", got)
	}
	if got := f.Get(pane, "@agent_last_seen"); got != formatUnix(now) {
		t.Fatalf("@agent_last_seen = %q, want updated timestamp", got)
	}
}

func TestMarkAgentSeenConcurrentWithReportAgent(t *testing.T) {
	const pane = "%4"
	ctx := context.Background()
	now := time.Unix(1_700_000_100, 0)
	SetFixedTrackTime(t, now)
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_state", string(AgentAborted))

	var wg sync.WaitGroup
	var seenErr, reportErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, seenErr = MarkAgentSeen(ctx, pane)
	}()
	go func() {
		defer wg.Done()
		_, reportErr = ReportAgent(ctx, AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      AgentWorking,
			Source:     "hook",
			SourceSeen: true,
			Seq:        500,
			SeqSeen:    true,
		})
	}()
	wg.Wait()
	if seenErr != nil {
		t.Fatalf("MarkAgentSeen() err = %v", seenErr)
	}
	if reportErr != nil {
		t.Fatalf("ReportAgent() err = %v", reportErr)
	}

	if got := f.Get(pane, "@agent_state"); got != "working" {
		t.Fatalf("@agent_state = %q, want working from hook report", got)
	}
	if got := f.Get(pane, "@agent_seq"); got != "500" {
		t.Fatalf("@agent_seq = %q, want 500", got)
	}
	if got := f.Get(pane, "@agent_last_seen"); got == "" {
		t.Fatal("@agent_last_seen should be updated by MarkAgentSeen")
	}
}

func TestMarkAgentSeenSkipsWhenSeqCorrupted(t *testing.T) {
	const pane = "%16"
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_name", "pi")
	f.Set(pane, "@agent_state", string(AgentAborted))
	f.Set(pane, "@agent_last_status", string(AgentAborted))
	f.Set(pane, "@agent_seq", "bad")

	seen, err := MarkAgentSeen(context.Background(), pane)
	if err != nil {
		t.Fatalf("MarkAgentSeen() err = %v", err)
	}
	if seen {
		t.Fatal("MarkAgentSeen() seen = true, want false when @agent_seq is malformed")
	}
	if got := f.Get(pane, "@agent_state"); got != string(AgentAborted) {
		t.Fatalf("@agent_state = %q, want unchanged", got)
	}
	if got := f.Get(pane, "@agent_last_status"); got != string(AgentAborted) {
		t.Fatalf("@agent_last_status = %q, want unchanged", got)
	}
	if got := f.Get(pane, "@agent_last_seen"); got != "" {
		t.Fatalf("@agent_last_seen = %q, want untouched", got)
	}
}

func TestMarkAgentSeenReleasedPaneReturnsFalse(t *testing.T) {
	const pane = "%3"
	f := InstallReportFakeTmux(t, pane)
	f.Set(pane, "@agent_seq", "77")

	seen, err := MarkAgentSeen(context.Background(), pane)
	if err != nil {
		t.Fatalf("MarkAgentSeen() err = %v", err)
	}
	if seen {
		t.Fatal("MarkAgentSeen() seen = true, want false for released pane")
	}
	if got := f.Get(pane, "@agent_last_seen"); got != "" {
		t.Fatalf("@agent_last_seen = %q, want untouched", got)
	}
}

func TestFocusAgentAfterSeqReleaseDoesNotWriteAgentState(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	SetFixedTrackTime(t, time.Unix(1_700_000_000, 0))

	applied, err := ReportAgent(ctx, AgentReport{
		Pane:       pane,
		Name:       "pi",
		State:      AgentWorking,
		Source:     "hook",
		SourceSeen: true,
		Seq:        70,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("report seq 70: %v", err)
	}
	if !applied {
		t.Fatal("report seq 70 should apply")
	}
	released, err := ReleaseAgent(ctx, AgentRelease{
		Pane:       pane,
		Source:     "hook",
		SourceSeen: true,
		Seq:        71,
		SeqSeen:    true,
	})
	if err != nil {
		t.Fatalf("release seq 71: %v", err)
	}
	if !released {
		t.Fatal("release seq 71 should apply")
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "71" {
		t.Fatalf("@agent_seq = %q, want tombstone 71", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_state"); got != "" {
		t.Fatalf("@agent_state = %q, want empty after release", got)
	}

	cmd := FocusAgentCommand(pane)
	_ = cmd.Run() // attach/switch-client may fail outside an attached client; state check runs first.
	if got := paneOptionValue(ctx, pane, "@agent_state"); got != "" {
		t.Fatalf("focus wrote @agent_state = %q after seq tombstone release", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_last_seen"); got == "" {
		t.Fatal("focus should still update @agent_last_seen")
	}
}

func TestFocusAgentCommandSkipsStateResetWhenSeqPresent(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := setPaneOption(ctx, pane, "@agent_name", "pi"); err != nil {
		t.Fatalf("set @agent_name: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_seq", "101"); err != nil {
		t.Fatalf("set @agent_seq: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_state", string(AgentAborted)); err != nil {
		t.Fatalf("set @agent_state: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_last_status", string(AgentAborted)); err != nil {
		t.Fatalf("set @agent_last_status: %v", err)
	}

	cmd := FocusAgentCommand(pane)
	_ = cmd.Run() // attach/switch-client may fail outside an attached client; state check runs first.

	if got := paneOptionValue(ctx, pane, "@agent_state"); got != string(AgentAborted) {
		t.Fatalf("@agent_state = %q, want aborted (seq tombstone blocks reset)", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_last_status"); got != string(AgentAborted) {
		t.Fatalf("@agent_last_status = %q, want aborted (seq tombstone blocks reset)", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_last_seen"); got == "" {
		t.Fatal("@agent_last_seen should be updated")
	}
}

func TestFocusAgentCommandSkipsWhenSeqCorrupted(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := setPaneOption(ctx, pane, "@agent_name", "pi"); err != nil {
		t.Fatalf("set @agent_name: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_state", string(AgentAborted)); err != nil {
		t.Fatalf("set @agent_state: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_last_status", string(AgentAborted)); err != nil {
		t.Fatalf("set @agent_last_status: %v", err)
	}
	if err := setPaneOption(ctx, pane, "@agent_seq", "bad"); err != nil {
		t.Fatalf("set @agent_seq: %v", err)
	}

	cmd := FocusAgentCommand(pane)
	_ = cmd.Run() // attach/switch-client may fail outside an attached client; state check runs first.

	if got := paneOptionValue(ctx, pane, "@agent_state"); got != string(AgentAborted) {
		t.Fatalf("@agent_state = %q, want unchanged", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_last_status"); got != string(AgentAborted) {
		t.Fatalf("@agent_last_status = %q, want unchanged", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_last_seen"); got != "" {
		t.Fatalf("@agent_last_seen = %q, want untouched", got)
	}
}

func TestFocusAgentCommandBuildsShellScript(t *testing.T) {
	cmd := FocusAgentCommand("%4")
	if filepath.Base(cmd.Path) != "sh" || len(cmd.Args) < 3 || cmd.Args[1] != "-c" {
		t.Fatalf("FocusAgentCommand() = %#v", cmd)
	}
	if !strings.Contains(cmd.Args[2], "select-pane -t") ||
		!strings.Contains(cmd.Args[2], `case "${state}" in`) ||
		!strings.Contains(cmd.Args[2], `working|busy|running`) {
		t.Fatalf("focus script missing expected tmux commands: %q", cmd.Args[2])
	}
}
