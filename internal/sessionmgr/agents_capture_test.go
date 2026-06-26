package sessionmgr

import (
	"context"
	"testing"
	"time"
)

func TestHasFreshHookState(t *testing.T) {
	tests := []struct {
		name    string
		updated time.Time
		want    bool
	}{
		{"zero", time.Time{}, false},
		{"recent", time.Now(), true},
		{"old", time.Now().Add(-2 * time.Minute), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Item{AgentUpdated: tt.updated}
			if got := hasFreshHookState(item); got != tt.want {
				t.Fatalf("hasFreshHookState = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyManifestFallbackRunsOnlyForStalePanes(t *testing.T) {
	ctx := context.Background()
	stalePane := "%1"
	freshPane := "%2"

	var captureCalls []string
	fake := NewStrictFakeTmux(t, nil)
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == stalePane {
				captureCalls = append(captureCalls, stalePane)
				// antigravity blocked screen
				return []byte("Requesting permission for: bash\nDo you want to proceed?"), nil
			}
		}
		captureCalls = append(captureCalls, "other")
		return []byte(""), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:       KindAgent,
			AgentName:  "antigravity",
			PaneID:     stalePane,
			AgentState: AgentIdle, // stale → should be classified
		},
		{
			Kind:         KindAgent,
			AgentName:    "claude",
			PaneID:       freshPane,
			AgentState:   AgentWorking,
			AgentUpdated: time.Now(), // fresh → should NOT be captured
		},
	}

	ApplyManifestFallback(ctx, items)

	// Stale pane should have been captured and classified.
	foundCapture := false
	for _, c := range captureCalls {
		if c == stalePane {
			foundCapture = true
		}
	}
	if !foundCapture {
		t.Fatal("expected capture-pane call for stale pane")
	}

	// Fresh pane should NOT have been captured.
	for _, c := range captureCalls {
		if c == "other" {
			t.Fatal("fresh pane should not be captured")
		}
	}

	// Stale pane classified to blocked.
	if items[0].AgentState != AgentBlocked {
		t.Fatalf("stale pane state = %s, want blocked", items[0].AgentState)
	}
	// Fresh pane unchanged.
	if items[1].AgentState != AgentWorking {
		t.Fatalf("fresh pane state = %s, want working (unchanged)", items[1].AgentState)
	}
}

func TestApplyManifestFallbackPreservesFreshHookState(t *testing.T) {
	ctx := context.Background()
	fake := NewStrictFakeTmux(t, nil)
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		// Even if capture returned something that looks idle, fresh state wins.
		return []byte(""), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:         KindAgent,
			AgentName:    "antigravity",
			PaneID:       "%1",
			AgentState:   AgentWorking,
			AgentUpdated: time.Now(), // fresh
		},
	}
	ApplyManifestFallback(ctx, items)
	if items[0].AgentState != AgentWorking {
		t.Fatalf("fresh working overridden to %s", items[0].AgentState)
	}
}
