package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
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
	if got[0].Name != "dev" || got[0].Path != "/Users/milo/dev" || !got[0].Attached ||
		got[0].Windows != 2 {
		t.Fatalf("parsed unexpected session: %#v", got[0])
	}
	if !got[0].Created.Equal(time.Unix(100, 0)) || !got[0].Activity.Equal(time.Unix(120, 0)) {
		t.Fatalf("timestamps parsed incorrectly: %#v", got[0])
	}
}

func TestNormalizeAgentState(t *testing.T) {
	tests := map[string]AgentState{
		"busy":       AgentWorking,
		"permission": AgentBlocked,
		"cancelled":  AgentAborted,
		"finished":   AgentDone,
		"ready":      AgentIdle,
		"weird":      AgentUnknown,
	}
	for in, want := range tests {
		if got := NormalizeAgentState(in); got != want {
			t.Fatalf("NormalizeAgentState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAgentsSkipsNonAgentsAndFormatsLocation(t *testing.T) {
	fields := []string{
		"%3",
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"claude",
		"",
		"claude",
		"busy",
		"needs ok",
		"123",
		"hook",
		"session-123",
		"42",
	}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	got := ParseAgents(raw, "")
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].AgentName != "claude" || got[0].AgentState != AgentWorking ||
		got[0].Location != "work:1.0" ||
		got[0].AgentMessage != "needs ok" ||
		got[0].AgentSessionID != "session-123" ||
		got[0].AgentSeq != "42" {
		t.Fatalf("unexpected agent: %#v", got[0])
	}
}

func TestParseAgentsToleratesLegacyFormatWithoutSessionMetadata(t *testing.T) {
	fields := []string{
		"%3",
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"claude",
		"",
		"claude",
		"busy",
		"needs ok",
		"123",
		"hook",
		"",
		"",
	}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	got := ParseAgents(raw, "")
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].AgentSessionID != "" || got[0].AgentSeq != "" {
		t.Fatalf(
			"legacy parse session metadata = %q/%q, want empty",
			got[0].AgentSessionID,
			got[0].AgentSeq,
		)
	}
}

func TestParseAgentsRequiresHookReportedAgentName(t *testing.T) {
	fields := []string{
		"%3",
		"work",
		"1",
		"0",
		"/Users/milo/Projects/seshagy",
		"1",
		"1",
		"1",
		"0",
		"",
		"busy",
		"needs ok",
		"123",
		"hook",
	}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	if got := ParseAgents(raw, ""); len(got) != 0 {
		t.Fatalf("expected unreported pane to be ignored, got %#v", got)
	}
}

func TestListFDirsWithCustomCommand(t *testing.T) {
	got, err := ListFDirsWithCommand(
		context.Background(),
		`printf '%s\n' /tmp/seshagy-fd-b /tmp/seshagy-fd-a`,
	)
	if err != nil {
		t.Fatalf("ListFDirsWithCommand() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Path != "/tmp/seshagy-fd-a" || got[1].Path != "/tmp/seshagy-fd-b" {
		t.Fatalf("custom fd dirs not sorted/parsed: %#v", got)
	}
	for _, item := range got {
		if item.Kind != KindFD || item.Target != item.Path {
			t.Fatalf("custom fd item = %#v", item)
		}
	}
}

func TestShouldApplyAgentSeq(t *testing.T) {
	tests := []struct {
		name         string
		existing     string
		incoming     int64
		incomingSeen bool
		want         bool
	}{
		{
			name:         "legacy no incoming seq applies",
			existing:     "99",
			incoming:     1,
			incomingSeen: false,
			want:         true,
		},
		{name: "empty existing applies", existing: "", incoming: 1, incomingSeen: true, want: true},
		{
			name:         "malformed existing applies",
			existing:     "bad",
			incoming:     1,
			incomingSeen: true,
			want:         true,
		},
		{name: "newer applies", existing: "41", incoming: 42, incomingSeen: true, want: true},
		{name: "equal applies", existing: "42", incoming: 42, incomingSeen: true, want: false},
		{name: "older ignored", existing: "43", incoming: 42, incomingSeen: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldApplyAgentSeq(
				tt.existing,
				tt.incoming,
				tt.incomingSeen,
			); got != tt.want {
				t.Fatalf(
					"shouldApplyAgentSeq(%q, %d, %v) = %v, want %v",
					tt.existing,
					tt.incoming,
					tt.incomingSeen,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestAgentPaneOptionsIncludeSessionMetadata(t *testing.T) {
	got := strings.Join(agentPaneOptions(), ",")
	for _, want := range []string{"@agent_session_id", "@agent_seq"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agentPaneOptions missing %s: %s", want, got)
		}
	}
}

func TestAgentLockPathIsPaneSpecificAndSafe(t *testing.T) {
	got := agentLockPath("%1:2.3")
	if !strings.Contains(got, "seshagy-agent-p1_2_3.lock") {
		t.Fatalf("agentLockPath() = %q, want sanitized pane id", got)
	}
}

func TestReleaseAgentWithSeqLeavesTombstoneForStaleReports(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := ReportAgent(
		ctx,
		AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      AgentWorking,
			Source:     "hook",
			SourceSeen: true,
			Seq:        100,
			SeqSeen:    true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "100" {
		t.Fatalf("initial @agent_seq = %q, want 100", got)
	}
	if err := ReleaseAgent(
		ctx,
		AgentRelease{Pane: pane, Source: "hook", SourceSeen: true, Seq: 101, SeqSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "101" {
		t.Fatalf("release @agent_seq = %q, want tombstone 101", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_name"); got != "" {
		t.Fatalf("release @agent_name = %q, want cleared", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_state"); got != "" {
		t.Fatalf("release @agent_state = %q, want cleared", got)
	}
	if err := ReleaseAgent(
		ctx,
		AgentRelease{Pane: pane, Source: "hook", SourceSeen: true, Seq: 102, SeqSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "102" {
		t.Fatalf("release after cleared pane @agent_seq = %q, want tombstone 102", got)
	}
	if err := ReportAgent(
		ctx,
		AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      AgentWorking,
			Source:     "hook",
			SourceSeen: true,
			Seq:        101,
			SeqSeen:    true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_name"); got != "" {
		t.Fatalf("stale report resurrected @agent_name = %q, want cleared", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_state"); got != "" {
		t.Fatalf("stale report resurrected @agent_state = %q, want cleared", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "102" {
		t.Fatalf("stale report changed @agent_seq = %q, want 102", got)
	}
}

func TestReleaseAgentWithSeqDoesNotTombstoneDifferentActiveSource(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := ReportAgent(
		ctx,
		AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      AgentWorking,
			Source:     "active",
			SourceSeen: true,
			Seq:        200,
			SeqSeen:    true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAgent(
		ctx,
		AgentRelease{Pane: pane, Source: "stale", SourceSeen: true, Seq: 201, SeqSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_name"); got != "pi" {
		t.Fatalf("different-source release changed @agent_name = %q, want pi", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_source"); got != "active" {
		t.Fatalf("different-source release changed @agent_source = %q, want active", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "200" {
		t.Fatalf("different-source release changed @agent_seq = %q, want 200", got)
	}
}

func TestReleaseAgentWithSeqDoesNotTombstoneActiveReportWithoutSource(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := ReportAgent(
		ctx,
		AgentReport{Pane: pane, Name: "pi", State: AgentWorking, Seq: 300, SeqSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAgent(
		ctx,
		AgentRelease{Pane: pane, Source: "hook", SourceSeen: true, Seq: 301, SeqSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_name"); got != "pi" {
		t.Fatalf("source-scoped release changed source-less @agent_name = %q, want pi", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_state"); got != "working" {
		t.Fatalf("source-scoped release changed source-less @agent_state = %q, want working", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "300" {
		t.Fatalf("source-scoped release changed source-less @agent_seq = %q, want 300", got)
	}
}

func TestReleaseAgentWithoutSeqClearsSeqForLegacyBehavior(t *testing.T) {
	ctx, pane := requireTestTmuxPane(t)
	if err := ReportAgent(
		ctx,
		AgentReport{
			Pane:       pane,
			Name:       "pi",
			State:      AgentWorking,
			Source:     "hook",
			SourceSeen: true,
			Seq:        100,
			SeqSeen:    true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseAgent(
		ctx,
		AgentRelease{Pane: pane, Source: "hook", SourceSeen: true},
	); err != nil {
		t.Fatal(err)
	}
	if got := paneOptionValue(ctx, pane, "@agent_seq"); got != "" {
		t.Fatalf("legacy release @agent_seq = %q, want cleared", got)
	}
	if got := paneOptionValue(ctx, pane, "@agent_name"); got != "" {
		t.Fatalf("legacy release @agent_name = %q, want cleared", got)
	}
}

func requireTestTmuxPane(t *testing.T) (context.Context, string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	session := fmt.Sprintf("seshagy_seq_%d", time.Now().UnixNano())
	if out, err := exec.Command("tmux", "new-session", "-d", "-s", session, "sleep 60").
		CombinedOutput(); err != nil {
		t.Skipf("tmux new-session failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() })
	out, err := exec.Command("tmux", "display-message", "-p", "-t", session+":0.0", "#{pane_id}").
		Output()
	if err != nil {
		t.Fatalf("tmux display pane id failed: %v", err)
	}
	pane := strings.TrimSpace(string(out))
	if pane == "" {
		t.Fatal("tmux returned empty pane id")
	}
	return context.Background(), pane
}

func paneOptionValue(ctx context.Context, pane, opt string) string {
	value, _ := showPaneOption(ctx, pane, opt)
	return value
}

func TestDetectTmuxPopup(t *testing.T) {
	tests := []struct {
		name        string
		envPane     string
		currentPane string
		want        bool
	}{
		{name: "normal pane", envPane: "%1", currentPane: "%1", want: false},
		{name: "popup has no pane env", envPane: "", currentPane: "%2", want: true},
		{
			name:        "popup differs from inherited pane env",
			envPane:     "%1",
			currentPane: "%2",
			want:        true,
		},
		{name: "empty current pane is inconclusive", envPane: "%1", currentPane: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectTmuxPopup(tt.envPane, tt.currentPane); got != tt.want {
				t.Fatalf(
					"detectTmuxPopup(%q, %q) = %v, want %v",
					tt.envPane,
					tt.currentPane,
					got,
					tt.want,
				)
			}
		})
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
	if !strings.Contains(line, "\x1b[38;5;10m"+IconSession+" \x1b[0m") {
		t.Fatalf("session icon does not use terminal bright green: %q", line)
	}
	if clean := StripANSI(line); clean != IconSession+" demo" {
		t.Fatalf("StripANSI(%q) = %q", line, clean)
	}
	item, ok := ParseActionLine(line)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLine(%q) = %#v, %v", line, item, ok)
	}
}

func TestFormatLineWithASCIIIcons(t *testing.T) {
	icons := DefaultIconSet()
	icons.ASCII = true
	icons.Session.ASCII = "S"
	icons.Session.Color = "9"
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if !strings.Contains(line, "\x1b[38;5;9mS\x1b[0m demo") {
		t.Fatalf("line does not use configured ascii icon/color: %q", line)
	}
	item, ok := ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", line, item, ok)
	}
}

func TestFormatLineWithHexIconColor(t *testing.T) {
	icons := DefaultIconSet()
	icons.Session.Color = "#a6e3a1"
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if !strings.Contains(line, "\x1b[38;2;166;227;161m"+IconSession+" \x1b[0mdemo") {
		t.Fatalf("line does not use truecolor hex escape: %q", line)
	}
}

func TestDefaultIconsCarryConfiguredDisplaySpacing(t *testing.T) {
	icons := DefaultIconSet()
	for name, style := range map[string]IconStyle{
		"session": icons.Session,
		"zoxide":  icons.Zoxide,
		"fd":      icons.FD,
	} {
		if !strings.HasSuffix(style.Icon, " ") || strings.HasSuffix(style.Icon, "  ") {
			t.Fatalf("%s default icon = %q, want exactly one trailing space", name, style.Icon)
		}
	}
	if !strings.HasSuffix(icons.Agent.Icon, "  ") || strings.HasSuffix(icons.Agent.Icon, "   ") {
		t.Fatalf("agent default icon = %q, want exactly two trailing spaces", icons.Agent.Icon)
	}

	line := FormatLine(Item{Kind: KindSession, Name: "demo"})
	if clean := StripANSI(line); clean != IconSession+" demo" {
		t.Fatalf("default icon spacing = %q, want one display space", clean)
	}
}

func TestFormatLineWithNoIconsOmitsSourcePrefixes(t *testing.T) {
	icons := DefaultIconSet()
	icons.Enabled = false
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if line != "demo" {
		t.Fatalf("no-icons session line = %q, want demo", line)
	}
	item, ok := ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", line, item, ok)
	}

	line = FormatLineWithIcons(Item{Kind: KindSession, Name: "Sdemo"}, icons)
	item, ok = ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "Sdemo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v, want full session name", line, item, ok)
	}

	agent := Item{
		Kind:       KindAgent,
		AgentName:  "pi",
		AgentState: AgentWorking,
		Location:   "work:2.1",
		Path:       "~/Projects/x",
	}
	line = FormatLineWithIcons(agent, icons)
	if !strings.HasPrefix(line, "[working]\tpi\twork:2.1") {
		t.Fatalf("no-icons agent line = %q, want state label without source prefix", line)
	}
	item, ok = ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindAgent || item.PaneID != "work:2.1" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", line, item, ok)
	}
}

func TestParseActionLineWithConfiguredIconsFallsBackToDefaults(t *testing.T) {
	icons := DefaultIconSet()
	icons.Session.Icon = "X"
	item, ok := ParseActionLineWithIcons(IconSession+" demo", icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("fallback parse = %#v, %v", item, ok)
	}
}

func TestDetectAgentNameFromCommand(t *testing.T) {
	tests := []struct{ command, want string }{
		{"pi", "pi"},
		{"claude", "claude"},
		{"claude-code", "claude"},
		{"codex", "codex"},
		{"codex-local", "codex"},
		{"codex-build", "codex"},
		{"opencode", "opencode"},
		{"gemini", "gemini"},
		{"cursor", "cursor"},
		{"cursor-agent", "cursor"},
		{"agy", "agy"},
		{"copilot", "copilot"},
		{"ghcs", "copilot"},
		{"kimi", "kimi"},
		{"kiro", "kiro"},
		{"droid", "droid"},
		{"droid-agent", "droid"},
		{"droid-build", "droid"},
		{"grok", "grok"},
		{"hermes", "hermes"},
		{"hermes-agent", "hermes"},
		{"kilo", "kilo"},
		{"qodercli", "qodercli"},
		{"bash", ""},
		{"zsh", ""},
		{"node", ""},
		{"python3", ""},
		{"vim", ""},
	}
	for _, tt := range tests {
		got := detectAgentName(tt.command, "")
		if got != tt.want {
			t.Errorf("detectAgentName(%q, \"\") = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func TestDetectAgentNameStripsExtensions(t *testing.T) {
	if got := detectAgentName("opencode.exe", ""); got != "opencode" {
		t.Errorf("got %q, want opencode", got)
	}
	if got := detectAgentName("codex.cmd", ""); got != "codex" {
		t.Errorf("got %q, want codex", got)
	}
	if got := detectAgentName("pi.js", ""); got != "pi" {
		t.Errorf("got %q, want pi", got)
	}
}
