package sessionmgr

import "testing"

func TestHasLifecycleAuthority(t *testing.T) {
	tests := []struct {
		agentName string
		source    string
		want      bool
	}{
		{"pi", "seshagy:pi", true},
		{"pi", "", true},
		{"pi", "hook", true},
		{"kimi", "", true},
		{"", "seshagy:opencode", true},
		{"claude", "seshagy:claude", false},
		{"claude", "", false},
		{"kimi", "seshagy:kimi", true},
		{"", "process", false},
		{"gemini", "process", false},
	}
	for _, tt := range tests {
		if got := HasLifecycleAuthority(tt.agentName, tt.source); got != tt.want {
			t.Fatalf(
				"HasLifecycleAuthority(%q, %q) = %v, want %v",
				tt.agentName,
				tt.source,
				got,
				tt.want,
			)
		}
	}
}
