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

// TestApplyManifestFallbackSkipsLifecycleFreshHook verifies that a lifecycle-
// authority agent (pi/opencode) with fresh hook state does NOT get captured
// (hooks own the state; manifest suppressed).
func TestApplyManifestFallbackSkipsLifecycleFreshHook(t *testing.T) {
	ctx := context.Background()

	var captureCalls int
	fake := NewStrictFakeTmux(t, nil)
	fake.AllowOutput(func(args []string) bool {
		if len(args) >= 1 && args[0] == "capture-pane" {
			captureCalls++
			return true
		}
		return false
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:         KindAgent,
			AgentName:    "opencode",
			PaneID:       "%1",
			AgentState:   AgentWorking,
			AgentUpdated: time.Now(), // fresh lifecycle hook
		},
	}
	ApplyManifestFallback(ctx, items)
	if captureCalls != 0 {
		t.Fatalf("capture-pane called %d times for fresh lifecycle agent, want 0", captureCalls)
	}
	if items[0].AgentState != AgentWorking {
		t.Fatalf("fresh lifecycle working overridden to %s", items[0].AgentState)
	}
}

// TestApplyManifestFallbackRunsForPartialHookAgentEvenWhenFresh verifies that a
// non-lifecycle agent (codex/claude/droid) with FRESH hook state is still
// captured and overwritten when the screen matches a different state (the
// ESC/approval-lag fix).
func TestApplyManifestFallbackRunsForPartialHookAgentEvenWhenFresh(t *testing.T) {
	ctx := context.Background()
	freshPane := "%5"

	var captured bool
	fake := NewStrictFakeTmux(t, nil)
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == freshPane {
				captured = true
				// cursor blocked screen (codex/claude/droid share no manifest, so
				// use cursor which has a blocked manifest rule).
				return []byte("waiting for approval\nrun this command?\nskip (esc or n)"), nil
			}
		}
		return []byte(""), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:         KindAgent,
			AgentName:    "cursor",
			PaneID:       freshPane,
			AgentState:   AgentWorking, // fresh hook says working
			AgentUpdated: time.Now(),   // fresh
		},
	}
	ApplyManifestFallback(ctx, items)
	if !captured {
		t.Fatal("expected capture-pane call for fresh non-lifecycle agent")
	}
	// Screen matched blocked → overwrite stale hook 'working' with 'blocked'.
	if items[0].AgentState != AgentBlocked {
		t.Fatalf(
			"fresh non-lifecycle working should be overwritten to blocked, got %s",
			items[0].AgentState,
		)
	}
}

// TestApplyManifestFallbackNoMatchPreservesFreshHookState verifies that when the
// screen manifest has NO matching rule for a non-lifecycle agent, the fresh hook
// state is preserved (not clobbered to idle).
func TestApplyManifestFallbackNoMatchPreservesFreshHookState(t *testing.T) {
	ctx := context.Background()
	fake := NewStrictFakeTmux(t, nil)
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		// Screen content that doesn't match any rule.
		return []byte("some random output\nthat doesn't match"), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:         KindAgent,
			AgentName:    "cursor",
			PaneID:       "%1",
			AgentState:   AgentWorking,
			AgentUpdated: time.Now(), // fresh
		},
	}
	ApplyManifestFallback(ctx, items)
	if items[0].AgentState != AgentWorking {
		t.Fatalf("fresh working clobbered to %s on no-match screen", items[0].AgentState)
	}
}

// TestApplyManifestFallbackRunsForLifecycleAgentWhenStale verifies that a
// lifecycle-authority agent (pi) with STALE hook state falls back to manifest.
func TestApplyManifestFallbackRunsForLifecycleAgentWhenStale(t *testing.T) {
	ctx := context.Background()
	var captured bool
	fake := NewStrictFakeTmux(t, nil)
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		captured = true
		return []byte(""), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:       KindAgent,
			AgentName:  "opencode",
			PaneID:     "%1",
			AgentState: AgentWorking,
			// AgentUpdated is zero → stale → manifest should run.
		},
	}
	ApplyManifestFallback(ctx, items)
	if !captured {
		t.Fatal("expected capture-pane call for stale lifecycle agent")
	}
}
