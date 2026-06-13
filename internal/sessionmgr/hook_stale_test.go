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

func TestApplyManifestFallbackRecoversStaleHookState(t *testing.T) {
	const pane = "%13"
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			return []byte(screen), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	fields := []string{
		"%13",
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"claude",
		"",
		"claude",
		"working",
		"",
		staleUpdated,
		"seshagy:claude",
		"session-123",
		"42",
		"12345",
	}
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
