package sessionmgr

import "testing"

func TestAdversarialWrappedCommandFalsePositives(t *testing.T) {
	cases := []struct {
		command string
		title   string
	}{
		{"python3 -m pip install gemini", ""},
		{"pip install google-gemini", ""},
		{"echo gemini", ""},
		{"node gemini", ""},
		{"bash gemini", ""},
		{"sh -c gemini", ""},
	}
	for _, tt := range cases {
		got := detectAgentName(tt.command, tt.title)
		if got == "gemini" {
			t.Errorf("false positive gemini from %q", tt.command)
		}
	}
}
