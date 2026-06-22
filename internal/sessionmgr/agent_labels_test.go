package sessionmgr

import (
	"path/filepath"
	"testing"
)

func agentLabelTestEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	_ = filepath.Join(dir, appStateDir)
}

func TestSaveAndLoadAgentLabel(t *testing.T) {
	agentLabelTestEnv(t)
	if err := SaveAgentLabel("pi", "seshagy", "frontend-bot"); err != nil {
		t.Fatalf("SaveAgentLabel() error = %v", err)
	}

	store := LoadAgentLabels()
	if got := store.Labels["pi:seshagy"]; got != "frontend-bot" {
		t.Fatalf("LoadAgentLabels()[pi:seshagy] = %q, want frontend-bot", got)
	}

	items := ApplyAgentLabels([]Item{
		{Kind: KindAgent, AgentName: "pi", Session: "seshagy"},
	})
	if items[0].AgentDisplayName != "frontend-bot" {
		t.Fatalf("AgentDisplayName = %q, want frontend-bot", items[0].AgentDisplayName)
	}
}

func TestSaveAgentLabelEmptyClears(t *testing.T) {
	agentLabelTestEnv(t)
	if err := SaveAgentLabel("claude", "app", "my-helper"); err != nil {
		t.Fatalf("SaveAgentLabel() error = %v", err)
	}
	if err := SaveAgentLabel("claude", "app", ""); err != nil {
		t.Fatalf("SaveAgentLabel(clear) error = %v", err)
	}

	store := LoadAgentLabels()
	if _, ok := store.Labels["claude:app"]; ok {
		t.Fatal("label was not cleared after empty save")
	}

	items := ApplyAgentLabels([]Item{
		{Kind: KindAgent, AgentName: "claude", Session: "app"},
	})
	if items[0].AgentDisplayName != "" {
		t.Fatalf("AgentDisplayName = %q, want empty after clear", items[0].AgentDisplayName)
	}
}

func TestApplyAgentLabelsSetsDisplayName(t *testing.T) {
	agentLabelTestEnv(t)
	if err := SaveAgentLabel("codex", "work", "backend"); err != nil {
		t.Fatalf("SaveAgentLabel() error = %v", err)
	}

	items := ApplyAgentLabels([]Item{
		{Kind: KindAgent, AgentName: "codex", Session: "work"},
		{Kind: KindAgent, AgentName: "pi", Session: "work"},
		{Kind: KindSession, Name: "work"},
	})
	if items[0].AgentDisplayName != "backend" {
		t.Fatalf("items[0].AgentDisplayName = %q, want backend", items[0].AgentDisplayName)
	}
	if items[1].AgentDisplayName != "" {
		t.Fatalf("items[1].AgentDisplayName = %q, want empty (no alias)", items[1].AgentDisplayName)
	}
	if items[2].AgentDisplayName != "" {
		t.Fatalf(
			"items[2].AgentDisplayName = %q, want empty (non-agent)",
			items[2].AgentDisplayName,
		)
	}
}
