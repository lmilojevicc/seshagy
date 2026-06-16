package sessionmgr

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIsHookStateStale(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fresh := strconv.FormatInt(now.Add(-2*time.Minute).Unix(), 10)
	stale := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)

	tests := []struct {
		name    string
		state   AgentState
		updated string
		want    bool
	}{
		{"fresh working", AgentWorking, fresh, false},
		{"stale working", AgentWorking, stale, true},
		{"stale blocked", AgentBlocked, stale, true},
		{"stale idle ignored", AgentIdle, stale, false},
		{"missing timestamp", AgentWorking, "", false},
		{"legacy timestamp", AgentWorking, "123", false},
		{"invalid timestamp", AgentWorking, "not-a-ts", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHookStateStale(tt.state, tt.updated, now); got != tt.want {
				t.Fatalf("isHookStateStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAgentStateStaleHookAllowsTitleInference(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)

	got := resolveAgentStateAt(
		"working",
		"claude",
		"seshagy:claude",
		"Action Required",
		staleUpdated,
		false,
		now,
	)
	if got != AgentBlocked {
		t.Fatalf("resolveAgentStateAt() = %q, want %q", got, AgentBlocked)
	}
}

func TestResolveAgentStateStaleHookWithoutInferenceReturnsUnknown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)

	got := resolveAgentStateAt(
		"working",
		"claude",
		"seshagy:claude",
		"Claude Code",
		staleUpdated,
		true,
		now,
	)
	if got != AgentUnknown {
		t.Fatalf("resolveAgentStateAt() = %q, want %q", got, AgentUnknown)
	}
}

func TestApplyManifestFallbackStaleHookNoMatchStaysUnknown(t *testing.T) {
	const pane = "%14"
	screen := "plain shell output with no manifest markers\n"
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if MatchCapturePane(pane)(args) {
			return []byte(screen), nil
		}
		return nil, nil
	}, nil)

	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	fields := agentExplainFields(pane, map[int]string{
		12: "working",
		14: staleUpdated,
	})
	raw := []byte(strings.Join(fields, paneSep) + "\n")

	origNow := agentResolveNow
	agentResolveNow = func() time.Time { return now }
	t.Cleanup(func() { agentResolveNow = origNow })

	got := ParseAgents(raw, "", LoadOptions{ManifestFallback: true})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].AgentState != AgentUnknown {
		t.Fatalf("AgentState before fallback = %q, want %q", got[0].AgentState, AgentUnknown)
	}
	applyManifestFallback(context.Background(), got)
	if got[0].AgentState != AgentUnknown {
		t.Fatalf(
			"AgentState = %q, want %q when stale hook screen matches no rule",
			got[0].AgentState,
			AgentUnknown,
		)
	}
}

func TestApplyManifestFallbackRecoversStaleHookState(t *testing.T) {
	const pane = "%13"
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if MatchCapturePane(pane)(args) {
			return []byte(screen), nil
		}
		return nil, nil
	}, nil)

	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	fields := agentExplainFields(pane, map[int]string{
		12: "working",
		14: staleUpdated,
	})
	raw := []byte(strings.Join(fields, paneSep) + "\n")

	origNow := agentResolveNow
	agentResolveNow = func() time.Time { return now }
	t.Cleanup(func() { agentResolveNow = origNow })

	got := ParseAgents(raw, "", LoadOptions{ManifestFallback: true})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	applyManifestFallback(context.Background(), got)
	if got[0].AgentState != AgentBlocked {
		t.Fatalf(
			"AgentState = %q, want %q after stale hook recovery",
			got[0].AgentState,
			AgentBlocked,
		)
	}
}
