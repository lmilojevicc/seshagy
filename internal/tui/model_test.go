package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestTUIFirstRefreshSmoke(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := New()
	if m.Init() == nil {
		t.Fatal("Init() returned nil")
	}

	m.source = sessionmgr.ModeSessions
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}
	model, cmd := m.Update(refreshMsg{
		source: sessionmgr.ModeSessions,
		gen:    1,
		items: []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "dev"},
		},
	})
	m = model.(Model)
	if len(m.items) == 0 {
		t.Fatal("expected items after refresh")
	}
	if len(m.notifications) != 0 {
		t.Fatalf("notifications = %#v, want none for successful refresh", m.notifications)
	}
	if m.View() == "" {
		t.Fatal("expected non-empty view after refresh")
	}
	if cmd == nil {
		t.Fatal("expected preview command after refresh")
	}
}

func TestNotifyPushesAndTrimsToThree(t *testing.T) {
	m := Model{}
	for _, text := range []string{"one", "two", "three", "four"} {
		m.notify(text, sevInfo)
	}
	if len(m.notifications) != 3 {
		t.Fatalf("notifications = %d, want 3", len(m.notifications))
	}
	for i, want := range []string{"two", "three", "four"} {
		if got := m.notifications[i].text; got != want {
			t.Fatalf("notification %d = %q, want %q", i, got, want)
		}
	}
}

func latestNotificationText(m Model) string {
	if len(m.notifications) == 0 {
		return ""
	}
	return m.notifications[len(m.notifications)-1].text
}

func latestNotificationSeverity(m Model) notifSev {
	if len(m.notifications) == 0 {
		return sevInfo
	}
	return m.notifications[len(m.notifications)-1].sev
}

func TestShowPreviewFollowsConfig(t *testing.T) {
	writeTUIConfig := func(t *testing.T, body string) {
		t.Helper()
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "seshagy", "config.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// No [tui] section -> default preview ON.
	writeTUIConfig(t, "[sources]\ndefault = \"sessions\"\n")
	m := New()
	if !m.showPreview {
		t.Fatal("showPreview = false with no [tui] section, want true (default)")
	}

	// Explicit preview = false -> preview OFF.
	writeTUIConfig(t, "[tui]\npreview = false\n")
	m = New()
	if m.showPreview {
		t.Fatal("showPreview = true with preview = false, want false")
	}

	// Explicit preview = true -> preview ON.
	writeTUIConfig(t, "[tui]\npreview = true\n")
	m = New()
	if !m.showPreview {
		t.Fatal("showPreview = false with preview = true, want true")
	}
}

func TestSelectedKeyAndSortItems(t *testing.T) {
	m := New()
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindFD, Path: "/tmp/b"},
		{Kind: sessionmgr.KindSession, Name: "alpha"},
	}
	SortItems(m.items)
	if m.items[0].Kind != sessionmgr.KindSession {
		t.Fatalf("sort order = %#v", m.items)
	}

	m.cursor = 0
	if key := m.selectedKey(); key != "session:alpha" {
		t.Fatalf("selectedKey = %q, want session:alpha", key)
	}

	m.cursor = 99
	if m.selectedKey() != "" {
		t.Fatal("expected empty selectedKey for invalid cursor")
	}
}

func TestSortedCountsGroupsItemsByKind(t *testing.T) {
	counts := sortedCounts([]sessionmgr.Item{
		{Kind: sessionmgr.KindSession},
		{Kind: sessionmgr.KindSession},
	})
	if counts[sessionmgr.KindSession] != 2 {
		t.Fatalf("counts = %#v", counts)
	}
}
