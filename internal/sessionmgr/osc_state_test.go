package sessionmgr

import (
	"testing"
	"time"
)

func TestShouldSupplementStateFromTitle(t *testing.T) {
	tests := []struct {
		name         string
		hookStateRaw string
		state        AgentState
		agent        string
		source       string
		want         bool
	}{
		{
			name:         "lifecycle silent hooks allow title supplement",
			hookStateRaw: "",
			state:        AgentUnknown,
			agent:        "claude",
			source:       "seshagy:claude",
			want:         true,
		},
		{
			name:         "lifecycle hook working blocks title supplement",
			hookStateRaw: "working",
			state:        AgentWorking,
			agent:        "claude",
			source:       "seshagy:claude",
			want:         false,
		},
		{
			name:         "lifecycle hook blocked blocks title supplement",
			hookStateRaw: "blocked",
			state:        AgentBlocked,
			agent:        "claude",
			source:       "seshagy:claude",
			want:         false,
		},
		{
			name:         "lifecycle hook idle blocks title supplement",
			hookStateRaw: "idle",
			state:        AgentIdle,
			agent:        "claude",
			source:       "seshagy:claude",
			want:         false,
		},
		{
			name:         "non-lifecycle silent hooks allow title supplement",
			hookStateRaw: "",
			state:        AgentUnknown,
			agent:        "gemini",
			source:       "process",
			want:         true,
		},
		{
			name:         "non-lifecycle hook working blocks title supplement",
			hookStateRaw: "working",
			state:        AgentWorking,
			agent:        "gemini",
			source:       "process",
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSupplementStateFromTitle(
				tt.hookStateRaw,
				tt.state,
				tt.agent,
				tt.source,
				"",
				time.Time{},
			); got != tt.want {
				t.Fatalf("shouldSupplementStateFromTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAgentStateLifecycleSilentHooksUseTitle(t *testing.T) {
	got := resolveAgentState("", "claude", "seshagy:claude", "⠋ Thinking…", "", false)
	if got != AgentWorking {
		t.Fatalf("resolveAgentState() = %q, want %q", got, AgentWorking)
	}
}

func TestResolveAgentStateLifecycleHookWorkingIgnoresBlockedTitle(t *testing.T) {
	got := resolveAgentState("working", "claude", "seshagy:claude", "Action Required", "", false)
	if got != AgentWorking {
		t.Fatalf("resolveAgentState() = %q, want hook-reported %q", got, AgentWorking)
	}
}
