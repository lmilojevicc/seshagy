package sessionmgr

import (
	"strings"
	"testing"
	"time"
)

func TestSessionNameFromDirMatchesScriptConventions(t *testing.T) {
	tests := map[string]string{
		"/Users/milo/Projects/foo.bar": "foo_bar",
		"/tmp/.config":                 "dot_config",
		"/tmp/a:b":                     "a_b",
		"/tmp/a b":                     "a_b",
	}
	for in, want := range tests {
		if got := SessionNameFromDir(in); got != want {
			t.Fatalf("SessionNameFromDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSessions(t *testing.T) {
	raw := []byte("dev\x1f100\x1f120\x1f/Users/milo/dev\x1f1\x1f2\n")
	got := ParseSessions(raw)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name != "dev" || got[0].Path != "/Users/milo/dev" || !got[0].Attached || got[0].Windows != 2 {
		t.Fatalf("parsed unexpected session: %#v", got[0])
	}
	if !got[0].Created.Equal(time.Unix(100, 0)) || !got[0].Activity.Equal(time.Unix(120, 0)) {
		t.Fatalf("timestamps parsed incorrectly: %#v", got[0])
	}
}

func TestDetectAgentName(t *testing.T) {
	tests := []struct{ cmd, title, want string }{
		{"pi", "π seshagy", "pi"},
		{"claude", "", "claude"},
		{"zsh", "Claude Code", ""},
		{"codex", "", "codex"},
		{"cursor-agent", "", "cursor"},
		{"antigravity-cli", "", "agy"},
	}
	for _, tt := range tests {
		got, ok := DetectAgentName(tt.cmd, tt.title)
		if tt.want == "" && ok {
			t.Fatalf("DetectAgentName(%q,%q) = %q, true; want false", tt.cmd, tt.title, got)
		}
		if tt.want != "" && (!ok || got != tt.want) {
			t.Fatalf("DetectAgentName(%q,%q) = %q,%v; want %q,true", tt.cmd, tt.title, got, ok, tt.want)
		}
	}
}

func TestNormalizeAgentState(t *testing.T) {
	tests := map[string]AgentState{"busy": AgentWorking, "permission": AgentBlocked, "cancelled": AgentAborted, "finished": AgentDone, "ready": AgentIdle, "weird": AgentUnknown}
	for in, want := range tests {
		if got := NormalizeAgentState(in); got != want {
			t.Fatalf("NormalizeAgentState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAgentsSkipsNonAgentsAndFormatsLocation(t *testing.T) {
	fields := []string{"%3", "work", "1", "editor", "0", "claude", "/Users/milo/Projects/seshagy", "1", "1", "1", "0", "", "", "busy", "needs ok", "123", "hook"}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	got := ParseAgents(raw, "")
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].AgentName != "claude" || got[0].AgentState != AgentWorking || got[0].Location != "work:1.0" || got[0].AgentMessage != "needs ok" {
		t.Fatalf("unexpected agent: %#v", got[0])
	}
}

func TestAgentPaneFromLine(t *testing.T) {
	line := IconAgent + " [idle]\tpi\twork:2.1\t~/Projects/x"
	if got := AgentPaneFromLine(line); got != "work:2.1" {
		t.Fatalf("pane = %q", got)
	}
}
