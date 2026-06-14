package sessionmgr

import (
	"encoding/json"
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

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid json: %s", string(data))
	}
}

func TestItemsToJSONUsesModeToken(t *testing.T) {
	items := []Item{{Kind: KindSession, Name: "work", Target: "work"}}
	payload := ItemsToJSON(ModeAgents, items, IconSet{})
	if payload.Mode != "agents" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "session" {
		t.Fatalf("items = %#v", payload.Items)
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
	})
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
}

func strPtr(value string) *string {
	return &value
}
