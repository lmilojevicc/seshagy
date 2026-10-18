package sessionmgr

import "testing"

func TestSourceModeConfigToken(t *testing.T) {
	tests := []struct {
		mode SourceMode
		want string
	}{
		{ModeAll, "all"},
		{ModeSessions, "sessions"},
		{ModeZoxide, "zoxide"},
		{ModeFD, "fd"},
		{ModeAgents, "agents"},
		{ModeCurrentAgents, "current-agents"},
		{SourceMode(99), "all"},
	}
	for _, test := range tests {
		if got := test.mode.ConfigToken(); got != test.want {
			t.Fatalf("ConfigToken(%d) = %q, want %q", test.mode, got, test.want)
		}
	}
}
