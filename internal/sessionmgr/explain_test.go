package sessionmgr

import (
	"context"
	"strings"
	"testing"
)

func agentExplainFields(paneID string, overrides map[int]string) []string {
	fields := []string{
		paneID,
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
		"working",
		"needs ok",
		"123",
		"seshagy:claude",
		"session-123",
		"42",
	}
	for idx, value := range overrides {
		fields[idx] = value
	}
	return fields
}

func installExplainFakeTmux(t *testing.T, pane string, fields []string) *fakeTmux {
	t.Helper()
	f := newFakeTmux()
	displayLine := strings.Join(fields, paneSep)
	origOut, origRun := tmuxOutput, tmuxRun
	tmuxOutput = func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) >= 5 && args[0] == "display-message" && args[1] == "-p" && args[2] == "-t" &&
			args[3] == pane {
			switch args[4] {
			case "#{pane_id}":
				return []byte(pane), nil
			case agentFormat:
				return []byte(displayLine), nil
			}
		}
		return f.output(ctx, args...)
	}
	tmuxRun = f.run
	t.Cleanup(func() {
		tmuxOutput = origOut
		tmuxRun = origRun
	})
	return f
}

func TestExplainAgentHookReportedPane(t *testing.T) {
	const pane = "%3"
	f := installExplainFakeTmux(t, pane, agentExplainFields(pane, nil))
	f.set(pane, "@agent_last_state", "working")
	f.set(pane, "@agent_last_status", "idle")
	f.set(pane, "@agent_last_seen", "1718380800")

	out, err := ExplainAgent(context.Background(), pane)
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	for _, want := range []string{
		"pane: %3",
		"location: work:1.0",
		"identity source: hook (@agent_name)",
		"agent name: claude",
		"state source: hook (@agent_state): working",
		"@agent_state: working",
		"effective status: idle (tracking override)",
		"@agent_source: seshagy:claude",
		"@agent_seq: 42",
		"@agent_session_id: session-123",
		"lifecycle authority: yes",
		"@agent_last_state: working",
		"@agent_last_status: idle",
		"@agent_last_seen: 1718380800",
		"integration:",
		"listed: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainAgent() missing %q in:\n%s", want, out)
		}
	}
}

func TestExplainAgentProcessDetectedPane(t *testing.T) {
	const pane = "%4"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		9:  "gemini",
		11: "",
		12: "",
		15: "",
		16: "",
		17: "",
	})
	f := installExplainFakeTmux(t, pane, fields)
	f.set(pane, "@agent_last_state", "unknown")
	f.set(pane, "@agent_last_status", "unknown")

	out, err := ExplainAgent(context.Background(), pane)
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	for _, want := range []string{
		"identity source: process detection (command/title)",
		"agent name: gemini",
		"state source: default (unknown)",
		"@agent_source: process",
		"lifecycle authority: no",
		"listed: true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainAgent() missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "integration:") {
		t.Fatalf("process-detected gemini should not show integration block:\n%s", out)
	}
}

func TestExplainAgentHookCapableWithoutHookReport(t *testing.T) {
	const pane = "%5"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		9:  "claude",
		11: "",
		12: "",
		15: "",
		16: "",
		17: "",
	})
	installExplainFakeTmux(t, pane, fields)

	out, err := ExplainAgent(context.Background(), pane)
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(
		out,
		"listed: false (hook-capable agent \"claude\" requires @agent_name from an integration)",
	) {
		t.Fatalf("expected hook-capable skip reason in:\n%s", out)
	}
	if !strings.Contains(out, "identity source: process detection (command/title)") {
		t.Fatalf("expected process identity source in:\n%s", out)
	}
}

func TestExplainAgentTitleInferenceStateSource(t *testing.T) {
	const pane = "%6"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		9:  "gemini",
		10: "⠋ planning",
		11: "",
		12: "",
		15: "",
		16: "",
		17: "",
	})
	installExplainFakeTmux(t, pane, fields)

	out, err := ExplainAgent(context.Background(), pane)
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(out, "state source: title inference: working") {
		t.Fatalf("expected title inference state source in:\n%s", out)
	}
}
