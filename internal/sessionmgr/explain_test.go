package sessionmgr

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExplainAgentHookReportedPane(t *testing.T) {
	const pane = "%3"
	f := InstallExplainFakeTmux(t, pane, agentExplainFields(pane, nil), "")
	f.Set(pane, "@agent_last_state", "working")
	f.Set(pane, "@agent_last_status", "idle")
	f.Set(pane, "@agent_last_seen", "1718380800")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
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
	f := InstallExplainFakeTmux(t, pane, fields, "")
	f.Set(pane, "@agent_last_state", "unknown")
	f.Set(pane, "@agent_last_status", "unknown")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
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
	InstallExplainFakeTmux(t, pane, fields, "")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	for _, want := range []string{
		"listed: true",
		"identity source: process detection (command/title)",
		"agent name: claude",
		"@agent_source: unhooked",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainAgent() missing %q in:\n%s", want, out)
		}
	}
}

func TestExplainAgentStaleHookState(t *testing.T) {
	const pane = "%10"
	now := time.Unix(1_700_000_000, 0)
	staleUpdated := strconv.FormatInt(now.Add(-10*time.Minute).Unix(), 10)
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		12: "working",
		14: staleUpdated,
	})
	InstallExplainFakeTmux(t, pane, fields, "")

	origNow := agentResolveNow
	agentResolveNow = func() time.Time { return now }
	t.Cleanup(func() { agentResolveNow = origNow })

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	for _, want := range []string{
		"state source: hook state stale (TTL exceeded)",
		"hook freshness: stale (TTL exceeded)",
		"@agent_state: working",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainAgent() missing %q in:\n%s", want, out)
		}
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
	InstallExplainFakeTmux(t, pane, fields, "")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(out, "state source: title inference: working") {
		t.Fatalf("expected title inference state source in:\n%s", out)
	}
}

func TestExplainAgentManifestFallbackFalseSkipsCapturePane(t *testing.T) {
	const pane = "%16"
	fields := agentExplainFields(pane, map[int]string{12: ""})
	InstallExplainFakeTmux(t, pane, fields, "")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{ManifestFallback: false})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if strings.Contains(out, "manifest fallback:") {
		t.Fatalf("expected no manifest fallback line in:\n%s", out)
	}
}

func TestExplainAgentManifestFallbackForLifecycleAgentWithSilentHooks(t *testing.T) {
	const pane = "%7"
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		12: "",
	})
	InstallExplainFakeTmux(t, pane, fields, screen)

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{ManifestFallback: true})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(
		out,
		"manifest fallback: rule dynamic_workflow_prompt (region whole_recent) → blocked",
	) {
		t.Fatalf("expected manifest fallback match for lifecycle agent in:\n%s", out)
	}
}

func TestExplainAgentManifestFallbackGeminiIdleFallback(t *testing.T) {
	const pane = "%8"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		9:  "gemini",
		11: "",
		12: "",
		15: "",
		16: "",
		17: "",
	})
	InstallExplainFakeTmux(t, pane, fields, "plain shell prompt\n")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{ManifestFallback: true})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(out, "manifest fallback: manifest skipped") {
		t.Fatalf("expected manifest skipped when no rule matches in:\n%s", out)
	}
}

func TestExplainAgentReportStructuredPayload(t *testing.T) {
	const pane = "%11"
	fields := agentExplainFields(pane, map[int]string{
		11: "claude",
		12: "working",
		15: "seshagy:claude",
	})
	f := InstallExplainFakeTmux(t, pane, fields, "")
	f.Set(pane, "@agent_last_state", "working")
	f.Set(pane, "@agent_last_status", "idle")
	f.Set(pane, "@agent_last_seen", "1718380800")

	report, err := ExplainAgentReport(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgentReport() error = %v", err)
	}
	if !report.Ok || report.SchemaVersion != JSONSchemaVersion || report.PaneID != pane {
		t.Fatalf("envelope = %#v", report)
	}
	if report.AgentName != "claude" || report.EffectiveStatus != AgentIdle ||
		report.DetectedStatus != AgentWorking || !report.TrackingOverride {
		t.Fatalf("status fields = %#v", report)
	}
	if report.Integration == nil || report.Integration.Target != "claude" {
		t.Fatalf("integration = %#v", report.Integration)
	}
	if report.LastSeenRFC3339 == "" {
		t.Fatalf("last_seen_rfc3339 missing: %#v", report)
	}
}

func TestExplainAgentIncludesManifestMeta(t *testing.T) {
	const pane = "%16"
	fields := agentExplainFields(pane, nil)
	InstallExplainFakeTmux(t, pane, fields, "")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(out, "manifest source:") {
		t.Fatalf("expected manifest metadata in:\n%s", out)
	}
}

func TestExplainAgentDeadPaneSkipped(t *testing.T) {
	const pane = "%15"
	fields := agentExplainFields(pane, map[int]string{8: "1"})
	InstallExplainFakeTmux(t, pane, fields, "")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if !strings.Contains(out, "listed: false (pane is dead)") {
		t.Fatalf("expected dead pane skip in:\n%s", out)
	}
}

func TestExplainAgentManifestFallbackSkipsTitleInference(t *testing.T) {
	const pane = "%9"
	fields := agentExplainFields(pane, map[int]string{
		0:  pane,
		9:  "claude",
		10: "⠋ Thinking…",
		11: "",
		12: "",
		15: "",
		16: "",
		17: "",
	})
	InstallExplainFakeTmux(t, pane, fields, "plain shell prompt\n")

	out, err := ExplainAgent(context.Background(), pane, LoadOptions{ManifestFallback: true})
	if err != nil {
		t.Fatalf("ExplainAgent() error = %v", err)
	}
	if strings.Contains(out, "state source: title inference:") {
		t.Fatalf(
			"expected title inference to be skipped when manifest fallback enabled in:\n%s",
			out,
		)
	}
	if !strings.Contains(out, "state source: default (unknown)") {
		t.Fatalf("expected default unknown state source in:\n%s", out)
	}
}
