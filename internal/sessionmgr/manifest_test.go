package sessionmgr

import (
	"strings"
	"testing"
)

func TestBundledManifestsCompile(t *testing.T) {
	for _, name := range []string{"opencode", "cursor", "antigravity", "agy", "grok"} {
		m, ok := manifestForAgent(name)
		if !ok {
			t.Errorf("manifestForAgent(%q): not found", name)
			continue
		}
		if len(m.rules) == 0 {
			t.Errorf("manifest %q has no rules", name)
		}
	}
}

func TestDetectManifestNoMatchIsIdle(t *testing.T) {
	result := detectManifest("agy", manifestDetectionInput{
		screen: "some random text\nwith no matching patterns",
	})
	if result.State != AgentIdle {
		t.Fatalf("no-match state = %s, want idle", result.State)
	}
	if result.Matched {
		t.Fatal("no-match should not be matched")
	}
}

func TestDetectManifestBlockedStrict(t *testing.T) {
	tests := []struct {
		name   string
		agent  string
		screen string
	}{
		{
			name:   "antigravity permission",
			agent:  "agy",
			screen: "Requesting permission for: bash\nDo you want to proceed?",
		},
		{
			name:  "grok permission scope",
			agent: "grok",
			screen: "yes, proceed    no, reject\n" +
				"use ← → to choose permission whitelist scope",
		},
		{
			name:   "opencode permission required",
			agent:  "opencode",
			screen: "△ Permission required",
		},
		{
			name:  "cursor write approval",
			agent: "cursor",
			screen: "write to this file?\nproceed (y)\n" +
				"reject & propose changes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectManifest(tt.agent, manifestDetectionInput{screen: tt.screen})
			if !result.Matched {
				t.Fatalf("expected match, got unmatched")
			}
			if result.State != AgentBlocked {
				t.Fatalf("state = %s, want blocked", result.State)
			}
		})
	}
}

func TestDetectManifestBlockedStrictRejectsNonMatching(t *testing.T) {
	// A screen that doesn't match any permission pattern must NOT be blocked.
	result := detectManifest("agy", manifestDetectionInput{
		screen: "Generating response...\nSome output here",
	})
	if result.State == AgentBlocked {
		t.Fatal("non-matching screen should not be blocked")
	}
}

func TestDetectManifestWorking(t *testing.T) {
	// antigravity spinner: braille chars + word ending in -ing
	spinner := "⠋ Generating response\n"
	result := detectManifest("agy", manifestDetectionInput{screen: spinner})
	if !result.Matched {
		t.Fatal("expected match for spinner")
	}
	if result.State != AgentWorking {
		t.Fatalf("state = %s, want working", result.State)
	}
}

func TestDetectManifestWorkingOpencodeInterrupt(t *testing.T) {
	screen := "esc to interrupt\n"
	result := detectManifest("opencode", manifestDetectionInput{screen: screen})
	if !result.Matched {
		t.Fatal("expected match")
	}
	if result.State != AgentWorking {
		t.Fatalf("state = %s, want working", result.State)
	}
}

func TestCompiledGateMatches(t *testing.T) {
	tests := []struct {
		name string
		gate manifestGate
		text string
		want bool
	}{
		{
			name: "contains AND all present",
			gate: manifestGate{Contains: []string{"hello", "world"}},
			text: "hello brave new world",
			want: true,
		},
		{
			name: "contains AND one missing",
			gate: manifestGate{Contains: []string{"hello", "missing"}},
			text: "hello brave new world",
			want: false,
		},
		{
			name: "any OR first matches",
			gate: manifestGate{Any: []manifestGate{
				{Contains: []string{"first"}},
				{Contains: []string{"second"}},
			}},
			text: "first one here",
			want: true,
		},
		{
			name: "any OR none match",
			gate: manifestGate{Any: []manifestGate{
				{Contains: []string{"first"}},
				{Contains: []string{"second"}},
			}},
			text: "neither here",
			want: false,
		},
		{
			name: "not suppresses match",
			gate: manifestGate{
				Contains: []string{"hello"},
				Not:      []manifestGate{{Contains: []string{"goodbye"}}},
			},
			text: "hello and goodbye",
			want: false,
		},
		{
			name: "not allows when absent",
			gate: manifestGate{
				Contains: []string{"hello"},
				Not:      []manifestGate{{Contains: []string{"goodbye"}}},
			},
			text: "hello world",
			want: true,
		},
		{
			name: "nested all inside any",
			gate: manifestGate{Any: []manifestGate{
				{All: []manifestGate{
					{Contains: []string{"a"}},
					{Contains: []string{"b"}},
				}},
			}},
			text: "a and b together",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := compileManifestGate(tt.gate, "test", 0)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			got := compiledGateMatches(compiled, tt.text)
			if got != tt.want {
				t.Fatalf("compiledGateMatches = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegexNormalization(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{
			name:    "alphabetic to letter",
			pattern: `\p{Alphabetic}+ing`,
			text:    "Generating",
			want:    true,
		},
		{
			name:    "braille range u form",
			pattern: `[\u2800-\u28FF]+`,
			text:    "⠋",
			want:    true,
		},
		{
			name:    "braille range x form",
			pattern: `[\x{2800}-\x{28FF}]+`,
			text:    "⠙",
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re, err := compileManifestRegex(tt.pattern, "test", "regex")
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := re.MatchString(tt.text); got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManifestRegionBottomNonEmptyLines(t *testing.T) {
	content := "line1\n\nline2\n\nline3\nline4"
	input := manifestDetectionInput{screen: content}

	got := manifestRegion(input, "bottom_non_empty_lines(2)")
	lines := manifestLines(got)
	// Should be the last 2 non-empty lines.
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), got)
	}
	if strings.TrimSpace(lines[0]) != "line3" {
		t.Fatalf("first line = %q, want line3", lines[0])
	}
}

func TestManifestRegionBottomNonEmptyLines8(t *testing.T) {
	// Verify N=8 works (used by cursor manifest).
	content := "a\nb\nc\nd\ne\nf"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "bottom_non_empty_lines(8)")
	if got == "" {
		t.Fatal("expected non-empty result for N=8 with 6 lines")
	}
}

func TestManifestRegionAfterLastPromptMarker(t *testing.T) {
	content := "hello\n› prompt\nbody line\nend"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "after_last_prompt_marker")
	if !strings.Contains(got, "body line") {
		t.Fatalf("after_last_prompt_marker = %q, want content after the › line", got)
	}
	if strings.Contains(got, "prompt") {
		t.Fatalf("after_last_prompt_marker = %q, must not include the prompt line", got)
	}
}

func TestManifestRegionAfterLastPromptMarkerNoMarker(t *testing.T) {
	content := "hello\nworld"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "after_last_prompt_marker")
	if got != content {
		t.Fatalf("no-marker case = %q, want whole buffer", got)
	}
}

func TestManifestRegionAfterLastHorizontalRule(t *testing.T) {
	content := "top\n───────────\nbottom"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "after_last_horizontal_rule")
	if !strings.Contains(got, "bottom") {
		t.Fatalf("after_last_horizontal_rule = %q, want content after the rule", got)
	}
	if strings.Contains(got, "top") {
		t.Fatalf("after_last_horizontal_rule = %q, must not include pre-rule content", got)
	}
}

func TestManifestRegionPromptBoxBody(t *testing.T) {
	content := "──────────\n❯ input\n──────────"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "prompt_box_body")
	if !strings.Contains(got, "input") {
		t.Fatalf("prompt_box_body = %q, want content inside the box", got)
	}
}

func TestManifestRegionPromptBoxBodyMissing(t *testing.T) {
	content := "plain text\nno box here"
	input := manifestDetectionInput{screen: content}
	got := manifestRegion(input, "prompt_box_body")
	if got != "" {
		t.Fatalf("prompt_box_body = %q, want empty when no box", got)
	}
}

func TestManifestRegionOscProgress(t *testing.T) {
	input := manifestDetectionInput{oscProgress: "spin"}
	if got := manifestRegion(input, "osc_progress"); got != "spin" {
		t.Fatalf("osc_progress = %q, want spin", got)
	}
}

func TestValidateManifestRegionAcceptsNewRegions(t *testing.T) {
	for _, region := range []string{
		"after_last_prompt_marker",
		"after_last_horizontal_rule",
		"prompt_box_body",
		"osc_progress",
	} {
		if err := validateManifestRegion(region); err != nil {
			t.Errorf("validateManifestRegion(%q) = %v, want nil", region, err)
		}
	}
}
