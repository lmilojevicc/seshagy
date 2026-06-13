package sessionmgr

import (
	"context"
	"testing"
)

func TestDetectStateFromManifestClaudeBlocked(t *testing.T) {
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	match, ok := detectStateFromManifest("claude", screen)
	if !ok {
		t.Fatal("expected manifest match")
	}
	if match.State != AgentBlocked {
		t.Fatalf("State = %q, want %q", match.State, AgentBlocked)
	}
	if !match.VisibleBlocker {
		t.Fatal("expected visible_blocker for blocked rule")
	}
	if match.RuleID != "dynamic_workflow_prompt" {
		t.Fatalf("RuleID = %q, want dynamic_workflow_prompt", match.RuleID)
	}
}

func TestDetectStateFromManifestCodexBlocked(t *testing.T) {
	screen := "tool output\nAllow command? [y/n]\n"
	match, ok := detectStateFromManifest("codex", screen)
	if !ok {
		t.Fatal("expected manifest match")
	}
	if match.State != AgentBlocked {
		t.Fatalf("State = %q, want %q", match.State, AgentBlocked)
	}
	if match.RuleID != "live_strong_blocker" {
		t.Fatalf("RuleID = %q, want live_strong_blocker", match.RuleID)
	}
}

func TestDetectStateFromManifestGrokWorking(t *testing.T) {
	screen := "earlier lines\n\nctrl+c:cancel ctrl+enter:interject waiting for tool\n"
	match, ok := detectStateFromManifest("grok-build", screen)
	if !ok {
		t.Fatal("expected manifest match")
	}
	if match.State != AgentWorking {
		t.Fatalf("State = %q, want %q", match.State, AgentWorking)
	}
	if match.RuleID != "waiting_tool_working" {
		t.Fatalf("RuleID = %q, want waiting_tool_working", match.RuleID)
	}
}

func TestDetectStateFromManifestNoMatch(t *testing.T) {
	if _, ok := detectStateFromManifest("claude", "plain shell prompt\n"); ok {
		t.Fatal("expected no manifest match")
	}
}

func TestDetectStateFromManifestUnknownAgent(t *testing.T) {
	if _, ok := detectStateFromManifest("gemini", "do you want to proceed?\n"); ok {
		t.Fatal("expected no manifest for unsupported agent")
	}
}

func TestManifestRegionBottomNonEmptyLines(t *testing.T) {
	screen := "line 1\n\nline 2\nline 3\n"
	got := manifestRegion(screen, "bottom_non_empty_lines(2)")
	want := "line 2\nline 3\n"
	if got != want {
		t.Fatalf("region = %q, want %q", got, want)
	}
}

func TestShouldApplyManifestFallback(t *testing.T) {
	tests := []struct {
		name   string
		state  AgentState
		agent  string
		source string
		want   bool
	}{
		{
			name:  "unknown non-lifecycle",
			state: AgentUnknown,
			agent: "gemini",
			want:  true,
		},
		{
			name:   "unknown lifecycle authority",
			state:  AgentUnknown,
			agent:  "claude",
			source: "seshagy:claude",
			want:   false,
		},
		{
			name:  "hook reported working",
			state: AgentWorking,
			agent: "gemini",
			want:  false,
		},
		{
			name:  "hook reported blocked",
			state: AgentBlocked,
			agent: "gemini",
			want:  false,
		},
		{
			name:  "hook reported idle",
			state: AgentIdle,
			agent: "gemini",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldApplyManifestFallback(tt.state, tt.agent, tt.source); got != tt.want {
				t.Fatalf("shouldApplyManifestFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManifestExplainLineShowsMatchedRule(t *testing.T) {
	const pane = "%9"
	screen := "tool output\nAllow command? [y/n]\n"
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			return []byte(screen), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	got := manifestExplainLine(context.Background(), pane, "codex", "process", AgentUnknown)
	if got != "manifest skipped" {
		t.Fatalf("manifestExplainLine() = %q, want manifest skipped for lifecycle authority", got)
	}

	got = manifestExplainLine(context.Background(), pane, "gemini", "process", AgentUnknown)
	if got != "manifest skipped" {
		t.Fatalf("manifestExplainLine() = %q, want manifest skipped for unsupported manifest", got)
	}
}

func TestCaptureAgentPaneCachedReusesPaneCapture(t *testing.T) {
	const pane = "%10"
	calls := 0
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			calls++
			return []byte("cached screen\n"), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	cache := make(manifestCaptureCache)
	for range 2 {
		screen, err := captureAgentPaneCached(
			context.Background(),
			cache,
			pane,
			manifestCaptureLines,
		)
		if err != nil {
			t.Fatalf("captureAgentPaneCached() error = %v", err)
		}
		if screen != "cached screen\n" {
			t.Fatalf("screen = %q, want cached screen", screen)
		}
	}
	if calls != 1 {
		t.Fatalf("capture-pane calls = %d, want 1", calls)
	}
}

func TestBundledManifestsCompile(t *testing.T) {
	for _, agent := range []string{"claude", "claude-code", "codex", "grok", "grok-build"} {
		manifest, err := manifestForAgent(agent)
		if err != nil {
			t.Fatalf("manifestForAgent(%q) error = %v", agent, err)
		}
		if manifest == nil {
			t.Fatalf("manifestForAgent(%q) = nil", agent)
		}
	}
}
