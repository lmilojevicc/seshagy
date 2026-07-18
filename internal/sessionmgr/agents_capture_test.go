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
	isolateManifestCache(t)
	ctx := context.Background()

	var captureCalls int
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
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
	isolateManifestCache(t)
	ctx := context.Background()
	freshPane := "%5"

	var captured bool
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
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
	isolateManifestCache(t)
	ctx := context.Background()
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
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
	isolateManifestCache(t)
	ctx := context.Background()
	var captured bool
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
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

// TestParseOSCSequences verifies extraction of OSC title and progress payloads
// from a capture-pane stream that preserved escape sequences.
func TestParseOSCSequences(t *testing.T) {
	tests := []struct {
		name      string
		screen    string
		wantTitle string
		wantProg  string
	}{
		{
			name:      "osc0_title_bel",
			screen:    "line1\n\x1b]0;\x1b[32m⠋\x1b[0m Working\x07\ntail\n",
			wantTitle: "⠋ Working",
			wantProg:  "",
		},
		{
			name:      "osc2_title_st",
			screen:    "\x1b]2;Action Required\x1b\\\n",
			wantTitle: "Action Required",
			wantProg:  "",
		},
		{
			name:      "osc4_progress_bel",
			screen:    "\x1b]4;0\x07\n",
			wantTitle: "",
			wantProg:  "4;0",
		},
		{
			name:      "title_then_progress_last_wins",
			screen:    "\x1b]0;old\x07\n\x1b]0;new\x07\n\x1b]4;0\x07\n",
			wantTitle: "new",
			wantProg:  "4;0",
		},
		{
			name:      "none",
			screen:    "plain text no escapes\n",
			wantTitle: "",
			wantProg:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTitle, gotProg := parseOSCSequences(tt.screen)
			if gotTitle != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tt.wantTitle)
			}
			if gotProg != tt.wantProg {
				t.Errorf("progress = %q, want %q", gotProg, tt.wantProg)
			}
		})
	}
}

// TestApplyManifestFallbackCodexOSCTitleWorking verifies that a codex pane
// whose capture contains an OSC title spinner (but NO screen-text working
// indicator) is classified working via the osc_title_working rule — proving
// OSC regions are now populated (previously always empty → dead rule).
func TestApplyManifestFallbackCodexOSCTitleWorking(t *testing.T) {
	isolateManifestCache(t)
	ctx := context.Background()
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		// Screen has an OSC braille spinner title but no working text body.
		return []byte("some output\n\x1b]0;\x1b[32m⠋\x1b[0m Thinking\x07\n"), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:       KindAgent,
			AgentName:  "codex",
			PaneID:     "%1",
			AgentState: AgentIdle, // no fresh hook → default idle
		},
	}
	ApplyManifestFallback(ctx, items)
	if items[0].AgentState != AgentWorking {
		t.Fatalf("osc_title_working rule should classify working, got %s", items[0].AgentState)
	}
}

// TestApplyManifestFallbackClaudeBlockedSurvivesIdleRule is the regression for
// blocker #2: a claude pane with fresh hook `blocked` and a captured screen
// containing BOTH a prompt-box ❯ line AND permission text must stay blocked
// (NOT be overwritten to idle by the demoted live_prompt_box rule).
func TestApplyManifestFallbackClaudeBlockedSurvivesIdleRule(t *testing.T) {
	isolateManifestCache(t)
	ctx := context.Background()
	fake := NewStrictFakeTmux(t, nil).AllowPaneOptions()
	fake.HandleOutput(func(args []string) bool {
		return len(args) >= 1 && args[0] == "capture-pane"
	}, func(_ context.Context, args ...string) ([]byte, error) {
		// Screen contains a horizontal-rule prompt box with a ❯ cursor AND
		// permission wording. The live_prompt_box idle rule must NOT win.
		return []byte("│ do you want to proceed?              │\n" +
			"│ esc to cancel                       │\n" +
			"│ tab to amend                        │\n" +
			"─\n" +
			"❯ \n" +
			"─\n"), nil
	})
	fake.Install(t)

	items := []Item{
		{
			Kind:         KindAgent,
			AgentName:    "claude",
			PaneID:       "%1",
			AgentState:   AgentBlocked, // fresh hook says blocked
			AgentUpdated: time.Now(),   // fresh
		},
	}
	ApplyManifestFallback(ctx, items)
	if items[0].AgentState != AgentBlocked {
		t.Fatalf("claude blocked clobbered to %s by idle rule", items[0].AgentState)
	}
}

func TestApplyManifestFallbackSuppressesRecentlyReleased(t *testing.T) {
	isolateManifestCache(t)
	captureCalled := false
	base := NewFakeTmux()
	// Released 1 second ago — within the 10s suppression window.
	base.Set(
		"%5",
		"@seshagy_agent_released_at",
		time.Now().Add(-1*time.Second).Format(time.RFC3339Nano),
	)
	s := NewStrictFakeTmux(t, base).AllowPaneOptions()
	s.HandleOutput(func(args []string) bool {
		return len(args) > 0 && args[0] == "capture-pane"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		captureCalled = true
		return []byte("  ctrl+c to stop\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	items := []Item{
		{Kind: KindAgent, AgentName: "cursor", PaneID: "%5", AgentState: AgentIdle},
	}
	// Non-lifecycle agent (cursor) -> shouldRunManifest=true, but
	// isRecentlyReleased must suppress and NOT call capture-pane.
	ApplyManifestFallback(ctx, items)
	if captureCalled {
		t.Fatal("capture-pane was called despite recent release; manifest should be suppressed")
	}
	if items[0].AgentState != AgentIdle {
		t.Errorf("state = %q, want idle (release suppression)", items[0].AgentState)
	}
}

func TestApplyManifestFallbackRunsAfterReleaseWindow(t *testing.T) {
	isolateManifestCache(t)
	base := NewFakeTmux()
	// Released 30 seconds ago — past the 10s suppression window.
	base.Set(
		"%5",
		"@seshagy_agent_released_at",
		time.Now().Add(-30*time.Second).Format(time.RFC3339Nano),
	)
	s := NewStrictFakeTmux(t, base).AllowPaneOptions()
	s.HandleOutput(func(args []string) bool {
		return len(args) > 0 && args[0] == "capture-pane"
	}, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("  ctrl+c to stop\n"), nil
	})
	s.Install(t)
	ctx := context.Background()

	items := []Item{
		{Kind: KindAgent, AgentName: "cursor", PaneID: "%5", AgentState: AgentIdle},
	}
	ApplyManifestFallback(ctx, items)
	if items[0].AgentState != AgentWorking {
		t.Errorf("state = %q, want working (suppression window expired)", items[0].AgentState)
	}
}
