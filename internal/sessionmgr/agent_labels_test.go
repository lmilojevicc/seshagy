package sessionmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

func StateHomeForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	return dir
}

func testAgentLabelsFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(xdg.StateHome(), "seshagy", "agent-labels.json")
}

func TestAgentLabelsSetAndGet(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("LoadAgentLabels() error = %v", err)
	}
	if err := store.Set("%1", "my pi", "pi"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got := store.Get("%1", "pi"); got != "my pi" {
		t.Fatalf("Get() = %q, want my pi", got)
	}
	reloaded, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if got := reloaded.Get("%1", "pi"); got != "my pi" {
		t.Fatalf("reloaded Get() = %q, want my pi", got)
	}
}

func TestAgentLabelsClearOnEmpty(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("LoadAgentLabels() error = %v", err)
	}
	if err := store.Set("%1", "label", "pi"); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear("%1"); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if got := store.Get("%1", "pi"); got != "" {
		t.Fatalf("Get() after clear = %q, want empty", got)
	}
	data, err := os.ReadFile(testAgentLabelsFile(t))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]AgentLabelEntry
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if _, ok := onDisk["%1"]; ok {
		t.Fatalf("cleared pane should be removed from file: %#v", onDisk)
	}
}

func TestAgentLabelsForAgentBindingMismatch(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("LoadAgentLabels() error = %v", err)
	}
	if err := store.Set("%1", "pi bot", "pi"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get("%1", "claude"); got != "" {
		t.Fatalf("Get() with mismatched agent = %q, want empty", got)
	}
	items := []Item{{Kind: KindAgent, PaneID: "%1", AgentName: "claude"}}
	ApplyAgentLabels(items, "")
	if items[0].AgentDisplayName != "" {
		t.Fatalf("ApplyAgentLabels() = %q, want no label on agent swap", items[0].AgentDisplayName)
	}
}

func TestAgentLabelsAtomicWrite(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("LoadAgentLabels() error = %v", err)
	}
	for i := range 20 {
		label := strings.Repeat("x", i+1)
		if err := store.Set("%1", label, "pi"); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
		data, err := os.ReadFile(testAgentLabelsFile(t))
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		var parsed map[string]AgentLabelEntry
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("on-disk JSON invalid after write %d: %v\ndata=%q", i, err, data)
		}
	}
}

func TestAgentLabelsPruneOrphans(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatalf("LoadAgentLabels() error = %v", err)
	}
	if err := store.Set("%1", "keep", "pi"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("%9", "drop", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune([]string{"%1"}); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if got := store.Get("%9", "claude"); got != "" {
		t.Fatalf("orphan Get() = %q, want empty", got)
	}
	reloaded, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("%1", "pi"); got != "keep" {
		t.Fatalf("kept label = %q, want keep", got)
	}
	if got := reloaded.Get("%9", "claude"); got != "" {
		t.Fatalf("pruned label still present = %q", got)
	}
}

func TestParseAgentsAppliesDisplayLabel(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("%3", "custom pi", "pi"); err != nil {
		t.Fatal(err)
	}
	fields := []string{
		"%3", "work", "1", "0", "/tmp/demo", "1", "1", "1", "0", "pi", "", "pi",
		"busy", "", "123", "hook", "session-123", "42",
	}
	raw := []byte(strings.Join(fields, paneSep) + "\n")
	items := ParseAgents(raw, "", LoadOptions{})
	ApplyAgentLabels(items, "")
	if len(items) != 1 {
		t.Fatalf("len = %d, want 1", len(items))
	}
	if got := items[0].AgentDisplayName; got != "custom pi" {
		t.Fatalf("AgentDisplayName = %q, want custom pi", got)
	}
	if got := items[0].DisplayName(); got != "custom pi" {
		t.Fatalf("DisplayName() = %q, want custom pi", got)
	}
}

func TestItemDisplayNamePrefersDisplayLabel(t *testing.T) {
	item := Item{Kind: KindAgent, AgentName: "pi", AgentDisplayName: "my bot", PaneID: "%1"}
	if got := item.DisplayName(); got != "my bot" {
		t.Fatalf("DisplayName() = %q, want my bot", got)
	}
}

func TestApplyAgentLabelsPruneSkipsSessionFiltered(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("%1", "session-a bot", "pi"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("%9", "session-b bot", "claude"); err != nil {
		t.Fatal(err)
	}
	items := []Item{{Kind: KindAgent, PaneID: "%1", AgentName: "pi", Session: "work"}}
	ApplyAgentLabels(items, "work")
	reloaded, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("%9", "claude"); got != "session-b bot" {
		t.Fatalf("filtered apply pruned other session label = %q, want session-b bot", got)
	}
}

func TestApplyAgentLabelsPruneSkipsEmptyList(t *testing.T) {
	StateHomeForTest(t)
	store, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("%1", "keep me", "pi"); err != nil {
		t.Fatal(err)
	}
	ApplyAgentLabels(nil, "")
	reloaded, err := LoadAgentLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get("%1", "pi"); got != "keep me" {
		t.Fatalf("empty list pruned label = %q, want keep me", got)
	}
}

func TestAgentLabelsLoadCorruptJSONReturnsError(t *testing.T) {
	StateHomeForTest(t)
	path := testAgentLabelsFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadAgentLabels()
	if err == nil {
		t.Fatal("LoadAgentLabels() error = nil, want corrupt JSON error")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "{not json" {
		t.Fatalf("corrupt file was overwritten: %q", data)
	}
}
