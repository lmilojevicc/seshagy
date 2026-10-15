package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

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

	writeTUIConfig(t, "[sources]\ndefault = \"sessions\"\n")
	if m := New(WithMultiplexer(newFakeMux())); !m.showPreview {
		t.Fatal("showPreview = false with no [tui] section, want true (default)")
	}

	writeTUIConfig(t, "[tui]\npreview = false\n")
	if m := New(WithMultiplexer(newFakeMux())); m.showPreview {
		t.Fatal("showPreview = true with preview = false, want false")
	}

	writeTUIConfig(t, "[tui]\npreview = true\n")
	if m := New(WithMultiplexer(newFakeMux())); !m.showPreview {
		t.Fatal("showPreview = false with preview = true, want true")
	}
}

func TestSelectedItemAndCursorClamp(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindFD, Path: "/tmp/b", Name: "/tmp/b"},
		{Kind: sessionmgr.KindSession, Name: "alpha", Target: "alpha"},
	})
	if m.selectedKey() != "session:alpha" {
		t.Fatalf(
			"selectedKey = %q, want session:alpha (sessions rank before dirs)",
			m.selectedKey(),
		)
	}
	m.cursor = 99
	if _, ok := m.selectedItem(); ok {
		t.Fatal("expected no selection for an out-of-range cursor")
	}
	if m.selectedKey() != "" {
		t.Fatal("expected empty selectedKey for an out-of-range cursor")
	}
}

func TestRequestPreviewReusesLoadedPayload(t *testing.T) {
	mux := newFakeMux()
	mux.preview = "captured"
	m := newTestModelWith(t, mux)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})

	m, cmd := m.requestPreview()
	if cmd == nil {
		t.Fatal("first request returned no capture command")
	}
	if m.previewPendingKey != "session:demo" {
		t.Fatalf("previewPendingKey = %q", m.previewPendingKey)
	}

	model, _ := m.Update(cmd())
	m = model.(Model)
	if m.previewKey != "session:demo" || m.preview != "captured" {
		t.Fatalf("preview after capture = %q / key = %q", m.preview, m.previewKey)
	}
	if _, again := m.requestPreview(); again != nil {
		t.Fatal("an already loaded payload was re-captured")
	}
}

func TestCurrentSessionLabelHerdrShowsWorkspaceLabel(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	m.items = []sessionmgr.Item{{
		Kind:      sessionmgr.KindAgent,
		AgentName: "pi",
		Session:   "wB",       // opaque herdr workspace id
		Location:  "frontend", // resolved workspace label
	}}
	m.currentSession = "wB"
	if label := m.currentSessionLabel(); label != "frontend" {
		t.Fatalf("currentSessionLabel = %q, want \"frontend\"", label)
	}

	m.items = nil
	if label := m.currentSessionLabel(); label != "wB" {
		t.Fatalf("fallback currentSessionLabel = %q, want \"wB\"", label)
	}
}

func TestInitAndEmptySourceCaches(t *testing.T) {
	m := newTestModel(t)
	if m.Init() == nil {
		t.Fatal("Init() returned no commands")
	}
	for _, source := range tuiSources {
		if m.cacheFresh(source) {
			t.Fatalf("%v cache reported fresh before any load", source)
		}
	}
	if m.loadedOnce {
		t.Fatal("loadedOnce should stay false until the first refresh result")
	}
}

func TestWindowSizeGatesPreviewColumn(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 99, 30
	if m.previewVisible() {
		t.Fatal("preview visible below the minimum width")
	}
	m.width = 100
	if !m.previewVisible() {
		t.Fatal("preview hidden at the minimum width")
	}
	m.showPreview = false
	if m.previewVisible() {
		t.Fatal("preview visible while disabled")
	}
}
