package sessionmgr

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestItemToJSONIncludesStructuredAgentFields(t *testing.T) {
	item := Item{
		Kind:           KindAgent,
		Name:           "pi",
		Target:         "%13",
		Path:           "~/obsidian/agent-mind-repository",
		PaneID:         "%13",
		Session:        "agent-mind-repository",
		Window:         "1",
		Pane:           "1",
		Location:       "agent-mind-repository:1.1",
		AgentName:      "pi",
		AgentState:     AgentUnknown,
		AgentMessage:   "install integration",
		AgentSource:    "seshagy:pi",
		AgentUpdated:   "1718361600",
		AgentSessionID: "native-123",
		AgentSeq:       "42",
		PaneTitle:      "pi",
		Visible:        true,
	}
	icons := IconSet{Enabled: false}

	got := ItemToJSON(item, icons)
	if got.Kind != "agent" || got.PaneID != "%13" || got.State != AgentUnknown {
		t.Fatalf("ItemToJSON() = %#v", got)
	}
	if got.AgentName != "pi" || got.Message != "install integration" {
		t.Fatalf("agent fields = %#v", got)
	}
	if got.Key != "agent:%13" {
		t.Fatalf("key = %q", got.Key)
	}
	if got.Line == "" {
		t.Fatal("expected rendered line for backward compatibility")
	}
	if got.LinePlain == "" {
		t.Fatal("expected line_plain for scripting")
	}
	if strings.Contains(got.LinePlain, "\x1b[") {
		t.Fatalf("line_plain must not contain ANSI: %q", got.LinePlain)
	}
	if got.UpdatedAtRFC3339 == "" {
		t.Fatal("expected updated_at_rfc3339 for unix timestamp")
	}
	expectedRFC := time.Unix(1718361600, 0).UTC().Format(time.RFC3339)
	if got.UpdatedAtRFC3339 != expectedRFC {
		t.Fatalf("updated_at_rfc3339 = %q want %q", got.UpdatedAtRFC3339, expectedRFC)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid json: %s", string(data))
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if visible, ok := raw["visible"].(bool); !ok || !visible {
		t.Fatalf("visible must be emitted for agent items: %#v", raw["visible"])
	}
}

func TestItemsToJSONUsesModeToken(t *testing.T) {
	items := []Item{{Kind: KindSession, Name: "work", Target: "work"}}
	payload := ItemsToJSON(ModeAgents, items, IconSet{})
	if payload.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", payload.SchemaVersion)
	}
	if !payload.Ok {
		t.Fatal("ok must be true")
	}
	if payload.Mode != "agents" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "session" {
		t.Fatalf("items = %#v", payload.Items)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if attached, ok := raw["items"].([]any)[0].(map[string]any)["attached"].(bool); !ok {
		t.Fatalf("attached must be emitted for session items: %#v", raw)
	} else if attached {
		t.Fatal("attached should be false")
	}
}

func TestManifestUpdateOutputToJSONUsesVersionString(t *testing.T) {
	version, err := ParseManifestVersion("2026.06.10.3")
	if err != nil {
		t.Fatalf("ParseManifestVersion() error = %v", err)
	}
	output := ManifestUpdateOutput{
		Updated: []ManifestUpdateCommit{{
			AgentID: "codex",
			Version: version,
		}},
		Status: ManifestUpdateStatus{
			LastResult: strPtr("updated"),
		},
	}
	got := ManifestUpdateOutputToJSON(output)
	if !got.Ok || got.SchemaVersion != JSONSchemaVersion {
		t.Fatalf("envelope = ok:%v schema_version:%d", got.Ok, got.SchemaVersion)
	}
	if len(got.Updated) != 1 || got.Updated[0].Version != "2026.06.10.3" {
		t.Fatalf("updated = %#v", got.Updated)
	}
}

func TestAgentExplainToReportIncludesDetectedStatus(t *testing.T) {
	report := agentExplainToReport(agentExplain{
		PaneID:           "%3",
		Location:         "work:1.0",
		IdentitySource:   "hook (@agent_name)",
		AgentName:        "claude",
		StateSource:      "hook (@agent_state): working",
		HookStateRaw:     "working",
		EffectiveStatus:  AgentIdle,
		DetectedStatus:   AgentWorking,
		TrackingOverride: true,
		LastSeen:         "1718361600",
		Integration: &IntegrationExplainJSON{
			Label:     "Claude Code",
			Target:    "claude",
			State:     "current",
			Version:   3,
			Authority: "lifecycle",
		},
	})
	if !report.Ok || report.SchemaVersion != JSONSchemaVersion {
		t.Fatalf("envelope = ok:%v schema_version:%d", report.Ok, report.SchemaVersion)
	}
	if report.DetectedStatus != AgentWorking || report.EffectiveStatus != AgentIdle {
		t.Fatalf("status fields = %#v", report)
	}
	if report.LastSeenRFC3339 == "" {
		t.Fatalf("last_seen_rfc3339 missing: %#v", report)
	}
	expected := time.Unix(1718361600, 0).UTC().Format(time.RFC3339)
	if report.LastSeenRFC3339 != expected {
		t.Fatalf("last_seen_rfc3339 = %q want %q", report.LastSeenRFC3339, expected)
	}
	if report.Integration == nil || report.Integration.Target != "claude" {
		t.Fatalf("integration = %#v", report.Integration)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), `"integration":`) &&
		strings.Contains(string(data), `"integration_summary"`) {
		t.Fatalf("must not emit legacy integration summary field: %s", string(data))
	}
}

func strPtr(value string) *string {
	return &value
}
