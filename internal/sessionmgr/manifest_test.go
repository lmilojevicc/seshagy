package sessionmgr

import (
	"context"
	"strings"
	"testing"
)

func TestDetectManifestClaudeBashPermissionPrompt(t *testing.T) {
	screen := "do you want to proceed?\n" +
		"bash command: rm -rf /tmp/test\n" +
		"❯ 1. Yes\n   2. No\n\n" +
		"Esc to cancel · Tab to amend · ctrl+e to explain\n"
	result := detectManifest("claude", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected manifest match")
	}
	if result.State != AgentBlocked {
		t.Fatalf("State = %q, want %q", result.State, AgentBlocked)
	}
	if !result.VisibleBlocker {
		t.Fatal("expected visible_blocker for blocked rule")
	}
	if result.RuleID != "bash_permission_prompt" {
		t.Fatalf("RuleID = %q, want bash_permission_prompt", result.RuleID)
	}
}

func TestDetectManifestClaudeTranscriptViewerSkipsStateUpdate(t *testing.T) {
	screen := "earlier output\nshowing detailed transcript\nctrl+o to toggle\n"
	result := detectManifest("claude", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected manifest match")
	}
	if !result.SkipStateUpdate {
		t.Fatal("expected skip_state_update for transcript_viewer")
	}
	if result.RuleID != "transcript_viewer" {
		t.Fatalf("RuleID = %q, want transcript_viewer", result.RuleID)
	}
	if result.State != AgentUnknown {
		t.Fatalf("State = %q, want %q", result.State, AgentUnknown)
	}
}

func TestDetectManifestCodexAfterLastPromptMarkerBlocker(t *testing.T) {
	screen := "tool output\n› \nAllow command? [y/n]\n"
	result := detectManifest("codex", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected manifest match")
	}
	if result.State != AgentBlocked {
		t.Fatalf("State = %q, want %q", result.State, AgentBlocked)
	}
	if result.RuleID != "live_strong_blocker" {
		t.Fatalf("RuleID = %q, want live_strong_blocker", result.RuleID)
	}
	if result.Region != "after_last_prompt_marker" {
		t.Fatalf("Region = %q, want after_last_prompt_marker", result.Region)
	}
}

func TestDetectManifestCodexTranscriptViewerSkipsStateUpdate(t *testing.T) {
	screen := "› \n↑/↓ to scroll · pgup/pgdn to page · home/end to jump · q to quit · esc to edit prev\n"
	result := detectManifest("codex", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected manifest match")
	}
	if !result.SkipStateUpdate {
		t.Fatal("expected skip_state_update for transcript_viewer")
	}
	if result.RuleID != "transcript_viewer" {
		t.Fatalf("RuleID = %q, want transcript_viewer", result.RuleID)
	}
}

func TestDetectManifestGrokPermissionScope(t *testing.T) {
	screen := "prompt text\nyes, proceed\nno, reject\nuse ← → to choose permission whitelist scope\n"
	result := detectManifest("grok-build", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected manifest match")
	}
	if result.State != AgentBlocked {
		t.Fatalf("State = %q, want %q", result.State, AgentBlocked)
	}
	if result.RuleID != "permission_scope_selector" {
		t.Fatalf("RuleID = %q, want permission_scope_selector", result.RuleID)
	}
}

func TestDetectManifestKnownAgentNoMatchStaysUnknown(t *testing.T) {
	result := detectManifest("claude", manifestDetectionInput{screen: "plain shell prompt\n"})
	if result.Matched {
		t.Fatal("expected no rule match")
	}
	if result.State != AgentUnknown {
		t.Fatalf("State = %q, want %q", result.State, AgentUnknown)
	}
	if result.FallbackReason != "" {
		t.Fatalf("FallbackReason = %q, want empty", result.FallbackReason)
	}
}

func TestDetectManifestUnknownAgentNoManifest(t *testing.T) {
	result := detectManifest("not-an-agent", manifestDetectionInput{screen: "anything\n"})
	if result.Matched || result.FallbackReason != "" {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestDetectManifestClaudeOSCWorking(t *testing.T) {
	result := detectManifest("claude", manifestDetectionInput{oscTitle: "⠂ project"})
	if !result.Matched || result.State != AgentWorking {
		t.Fatalf("expected osc_title working match, got %+v", result)
	}
	if result.RuleID != "osc_title_working" {
		t.Fatalf("RuleID = %q, want osc_title_working", result.RuleID)
	}
}

func TestManifestRegionBottomNonEmptyLines(t *testing.T) {
	screen := "line 1\n\nline 2\nline 3\n"
	got := manifestRegion(manifestDetectionInput{screen: screen}, "bottom_non_empty_lines(2)")
	want := "line 2\nline 3\n"
	if got != want {
		t.Fatalf("region = %q, want %q", got, want)
	}
}

func TestManifestRegionAfterLastHorizontalRule(t *testing.T) {
	screen := "header\n──────────\nblocked form\n"
	got := manifestRegion(manifestDetectionInput{screen: screen}, "after_last_horizontal_rule")
	if !strings.Contains(got, "blocked form") {
		t.Fatalf("region = %q, want content after horizontal rule", got)
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
			want:   true,
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
	screen := "tool output\n› \nAllow command? [y/n]\n"
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			return []byte(screen), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	got := manifestExplainLine(
		context.Background(),
		pane,
		"codex",
		"process",
		"llm-proxy",
		AgentUnknown,
	)
	if !strings.Contains(got, "rule live_strong_blocker") {
		t.Fatalf("manifestExplainLine() = %q, want matched rule for codex", got)
	}
	if !strings.Contains(got, "region after_last_prompt_marker") {
		t.Fatalf("manifestExplainLine() = %q, want region in explain output", got)
	}

	got = manifestExplainLine(context.Background(), pane, "gemini", "process", "", AgentUnknown)
	if got != "manifest skipped" {
		t.Fatalf("manifestExplainLine() = %q, want manifest skipped for gemini", got)
	}
}

func TestApplyManifestFallbackForLifecycleAgentWithSilentHooks(t *testing.T) {
	const pane = "%11"
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			return []byte(screen), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	items := []Item{{
		PaneID:      pane,
		AgentName:   "claude",
		AgentSource: "seshagy:claude",
		AgentState:  AgentUnknown,
	}}
	applyManifestFallback(context.Background(), items)
	if items[0].AgentState != AgentBlocked {
		t.Fatalf("AgentState = %q, want %q", items[0].AgentState, AgentBlocked)
	}
}

func TestApplyManifestFallbackSkipsStateUpdate(t *testing.T) {
	const pane = "%12"
	screen := "showing detailed transcript\nctrl+o to toggle\n"
	origOut := tmuxOutput
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 4 && args[0] == "capture-pane" && args[3] == pane {
			return []byte(screen), nil
		}
		return nil, nil
	}
	t.Cleanup(func() { tmuxOutput = origOut })

	items := []Item{{
		PaneID:     pane,
		AgentName:  "claude",
		AgentState: AgentUnknown,
	}}
	applyManifestFallback(context.Background(), items)
	if items[0].AgentState != AgentUnknown {
		t.Fatalf("AgentState = %q, want unchanged %q", items[0].AgentState, AgentUnknown)
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
	agents := []string{
		"amp", "agy", "antigravity", "claude", "claude-code", "cline", "codex", "cursor",
		"droid", "gemini", "copilot", "github-copilot", "grok", "grok-build", "hermes",
		"kilo", "kimi", "kiro", "opencode", "pi", "qodercli",
	}
	for _, agent := range agents {
		manifest, err := manifestForAgent(agent)
		if err != nil {
			t.Fatalf("manifestForAgent(%q) error = %v", agent, err)
		}
		if manifest == nil {
			t.Fatalf("manifestForAgent(%q) = nil", agent)
		}
	}
}
