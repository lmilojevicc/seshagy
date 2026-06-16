package sessionmgr

import (
	"context"
	"testing"
	"time"
)

// fakeTmux is an in-memory stand-in for a tmux server's pane options, used to
// exercise pane-option-driven logic without a live tmux.
type fakeTmux struct {
	opts map[string]map[string]string
}

func newFakeTmux() *fakeTmux {
	return &fakeTmux{opts: map[string]map[string]string{}}
}

func (f *fakeTmux) get(pane, opt string) string {
	if m, ok := f.opts[pane]; ok {
		return m[opt]
	}
	return ""
}

func (f *fakeTmux) set(pane, opt, value string) {
	if f.opts[pane] == nil {
		f.opts[pane] = map[string]string{}
	}
	f.opts[pane][opt] = value
}

func (f *fakeTmux) output(_ context.Context, args ...string) ([]byte, error) {
	if len(args) >= 4 && args[0] == "show-option" {
		return []byte(f.get(args[2], args[3])), nil
	}
	return nil, nil
}

func (f *fakeTmux) run(_ context.Context, args ...string) error {
	switch {
	case len(args) >= 5 && args[0] == "set-option" && args[1] == "-qpt":
		f.set(args[2], args[3], args[4])
	case len(args) >= 4 && args[0] == "set-option" && args[1] == "-qupt":
		if m, ok := f.opts[args[2]]; ok {
			delete(m, args[3])
		}
	}
	return nil
}

func installFakeTmux(t *testing.T, f *fakeTmux) {
	t.Helper()
	origOut, origRun := tmuxOutput, tmuxRun
	tmuxOutput = f.output
	tmuxRun = f.run
	t.Cleanup(func() {
		tmuxOutput = origOut
		tmuxRun = origRun
	})
}

func installFixedTrackTime(t *testing.T, now time.Time) {
	t.Helper()
	orig := agentTrackNow
	agentTrackNow = func() time.Time { return now }
	t.Cleanup(func() { agentTrackNow = orig })
}

func seedPendingIdleConfirmed(f *fakeTmux, pane string, now time.Time) {
	f.set(pane, "@agent_pending_idle_since", formatUnix(now.Add(-agentPendingIdleDebounce)))
	f.set(pane, "@agent_pending_idle_count", "2")
}

func seedStartupGraceExpired(f *fakeTmux, pane string, now time.Time) {
	f.set(pane, "@agent_name", "claude")
	f.set(pane, "@agent_startup_grace", formatUnix(now.Add(-agentStartupGraceWindow)))
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
			installFixedTrackTime(t, now)
			f := newFakeTmux()
			if tt.lastState != "" {
				f.set(pane, "@agent_last_state", string(tt.lastState))
			}
			if tt.lastStatus != "" {
				f.set(pane, "@agent_last_status", string(tt.lastStatus))
			}
			if tt.wantStatus == AgentDone {
				seedStartupGraceExpired(f, pane, now)
				seedPendingIdleConfirmed(f, pane, now)
			}
			installFakeTmux(t, f)

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
			if persisted := f.get(pane, "@agent_last_status"); persisted != string(tt.wantStatus) {
				t.Fatalf("@agent_last_status = %q, want %q", persisted, tt.wantStatus)
			}
			if seen := f.get(pane, "@agent_last_seen") != ""; seen != tt.wantLastSeen {
				t.Fatalf("@agent_last_seen present = %v, want %v", seen, tt.wantLastSeen)
			}
		})
	}
}

func TestUpdateAgentStatusTrackingEmptyPane(t *testing.T) {
	f := newFakeTmux()
	installFakeTmux(t, f)
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
	installFixedTrackTime(t, now)
	f := newFakeTmux()
	f.set(pane, "@agent_name", "claude")
	f.set(pane, "@agent_last_state", string(AgentWorking))
	f.set(pane, "@agent_last_status", string(AgentWorking))
	f.set(pane, "@agent_startup_grace", formatUnix(now.Add(-agentStartupGraceWindow)))
	installFakeTmux(t, f)

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
	if f.get(pane, "@agent_last_state") != string(AgentWorking) {
		t.Fatalf("@agent_last_state = %q, want working", f.get(pane, "@agent_last_state"))
	}
	if f.get(pane, "@agent_pending_idle_count") != "1" {
		t.Fatalf("@agent_pending_idle_count = %q, want 1", f.get(pane, "@agent_pending_idle_count"))
	}

	got, err = UpdateAgentStatusTracking(context.Background(), pane, AgentIdle, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentWorking {
		t.Fatalf("second idle status = %q, want %q", got, AgentWorking)
	}

	now = now.Add(agentPendingIdleDebounce)
	installFixedTrackTime(t, now)
	got, err = UpdateAgentStatusTracking(context.Background(), pane, AgentIdle, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != AgentDone {
		t.Fatalf("confirmed idle status = %q, want %q", got, AgentDone)
	}
	if f.get(pane, "@agent_pending_idle_count") != "" {
		t.Fatalf(
			"pending idle count should be cleared, got %q",
			f.get(pane, "@agent_pending_idle_count"),
		)
	}
}

func TestUpdateAgentStatusTrackingStartupGraceSkipsDoneInference(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_000, 0)
	installFixedTrackTime(t, now)
	f := newFakeTmux()
	f.set(pane, "@agent_name", "claude")
	f.set(pane, "@agent_last_state", string(AgentWorking))
	f.set(pane, "@agent_last_status", string(AgentWorking))
	seedPendingIdleConfirmed(f, pane, now)
	installFakeTmux(t, f)

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
	if grace := f.get(pane, "@agent_startup_grace"); grace == "" {
		t.Fatal("expected @agent_startup_grace to be set")
	}
}

func TestUpdateAgentStatusTrackingWorkingClearsPendingIdle(t *testing.T) {
	const pane = "%1"
	now := time.Unix(1_700_000_000, 0)
	installFixedTrackTime(t, now)
	f := newFakeTmux()
	f.set(pane, "@agent_last_state", string(AgentWorking))
	f.set(pane, "@agent_pending_idle_since", formatUnix(now))
	f.set(pane, "@agent_pending_idle_count", "2")
	installFakeTmux(t, f)

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
	if f.get(pane, "@agent_pending_idle_count") != "" {
		t.Fatalf(
			"pending idle count should be cleared, got %q",
			f.get(pane, "@agent_pending_idle_count"),
		)
	}
}
