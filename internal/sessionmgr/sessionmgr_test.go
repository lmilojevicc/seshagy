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

func TestNormalizeAgentState(t *testing.T) {
	tests := map[string]AgentState{"busy": AgentWorking, "permission": AgentBlocked, "cancelled": AgentAborted, "finished": AgentDone, "ready": AgentIdle, "weird": AgentUnknown}
	for in, want := range tests {
		if got := NormalizeAgentState(in); got != want {
			t.Fatalf("NormalizeAgentState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAgentsSkipsNonAgentsAndFormatsLocation(t *testing.T) {
	fields := []string{"%3", "work", "1", "0", "/Users/milo/Projects/seshagy", "1", "1", "1", "0", "claude", "busy", "needs ok", "123", "hook"}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	got := ParseAgents(raw, "")
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].AgentName != "claude" || got[0].AgentState != AgentWorking || got[0].Location != "work:1.0" || got[0].AgentMessage != "needs ok" {
		t.Fatalf("unexpected agent: %#v", got[0])
	}
}

func TestParseAgentsRequiresHookReportedAgentName(t *testing.T) {
	fields := []string{"%3", "work", "1", "0", "/Users/milo/Projects/seshagy", "1", "1", "1", "0", "", "busy", "needs ok", "123", "hook"}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	if got := ParseAgents(raw, ""); len(got) != 0 {
		t.Fatalf("expected unreported pane to be ignored, got %#v", got)
	}
}

func TestAgentPaneFromLine(t *testing.T) {
	line := IconAgent + " [idle]\tpi\twork:2.1\t~/Projects/x"
	if got := AgentPaneFromLine(line); got != "work:2.1" {
		t.Fatalf("pane = %q", got)
	}
}

func TestFormatLineColorsIconsButKeepsParseableText(t *testing.T) {
	line := FormatLine(Item{Kind: KindSession, Name: "demo"})
	if !strings.Contains(line, "\x1b[38;2;166;227;161m"+IconSession+"\x1b[0m") {
		t.Fatalf("session icon is not Catppuccin green: %q", line)
	}
	if clean := StripANSI(line); clean != IconSession+" demo" {
		t.Fatalf("StripANSI(%q) = %q", line, clean)
	}
	item, ok := ParseActionLine(line)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLine(%q) = %#v, %v", line, item, ok)
	}
}
