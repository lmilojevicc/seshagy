package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
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

// TestLayoutFollowsConfig pins the layout the model exposes: the configured
// token, with legacy and unknown values normalizing to horizontal.
func TestLayoutFollowsConfig(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "vertical", configured: appconfig.LayoutVertical, want: appconfig.LayoutVertical},
		{name: "dropdown", configured: appconfig.LayoutDropdown, want: appconfig.LayoutDropdown},
		{
			name:       "horizontal",
			configured: appconfig.LayoutHorizontal,
			want:       appconfig.LayoutHorizontal,
		},
		{name: "unset", configured: "", want: appconfig.LayoutHorizontal},
		{name: "legacy zen", configured: "zen", want: appconfig.LayoutHorizontal},
		{name: "unknown", configured: "tree", want: appconfig.LayoutHorizontal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.TUI.Layout = test.configured
			m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
			if m.layout != test.want {
				t.Fatalf("layout for %q = %q, want %q", test.configured, m.layout, test.want)
			}
		})
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

	// The dropdown layout starts compact: the preview is hidden until ctrl+o.
	writeTUIConfig(t, "[tui]\nlayout = \"dropdown\"\n")
	if m := New(WithMultiplexer(newFakeMux())); m.showPreview {
		t.Fatal("showPreview = true with layout = dropdown, want false")
	}

	writeTUIConfig(t, "[tui]\nlayout = \"dropdown\"\npreview = true\n")
	if m := New(WithMultiplexer(newFakeMux())); !m.showPreview {
		t.Fatal("showPreview = false with layout = dropdown and preview = true, want true")
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

// TestPreviewVisibilityFollowsTheToggle pins the preview's own switch. Where the
// preview window goes — the right column or a band under the results — is the
// layout's business, so a narrow frame shows it rather than hiding it.
func TestPreviewVisibilityFollowsTheToggle(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 24
	if !m.previewVisible() {
		t.Fatal("preview hidden while enabled")
	}
	m.showPreview = false
	if m.previewVisible() {
		t.Fatal("preview visible while disabled")
	}
	m.showPreview = true
	m.width = 0
	if m.previewVisible() {
		t.Fatal("preview visible before the first size message")
	}
}

func TestAgentStartTimesResolveOncePerPaneAndPid(t *testing.T) {
	m := newTestModel(t)
	pane := agentItem("pi", "%1", "work")
	pane.AgentPID = 4242
	m = withItems(m, []sessionmgr.Item{pane})

	if got := len(m.pendingAgentStarts()); got != 1 {
		t.Fatalf("pending = %d, want the new pane", got)
	}
	m, cmd := m.resolveAgentStarts()
	if cmd == nil || !m.agentStartPending {
		t.Fatal("resolution was not scheduled")
	}

	started := time.Now().Add(-2 * time.Hour)
	m = m.applyAgentStarts(agentStartedMsg{starts: map[string]agentStart{
		"%1": {pid: 4242, started: started},
	}})
	if m.agentStartPending {
		t.Fatal("pending flag not cleared by the result")
	}
	if got := m.pendingAgentStarts(); len(got) != 0 {
		t.Fatalf("pending = %d after resolving, want none", len(got))
	}
	if got := m.items[0].StartedAt; !got.Equal(started) {
		t.Fatalf("StartedAt = %v, want the resolved %v", got, started)
	}
	if again, _ := m.resolveAgentStarts(); again.agentStartPending {
		t.Fatal("a cached pane was resolved again")
	}

	// A pane that restarted its agent (a new pid) is resolved again.
	restarted := pane
	restarted.AgentPID = 5150
	m = withItems(m, []sessionmgr.Item{restarted})
	if got := len(m.pendingAgentStarts()); got != 1 {
		t.Fatalf("pending = %d, want the restarted pane", got)
	}
	if !m.items[0].StartedAt.IsZero() {
		t.Fatal("a restarted agent inherited the previous process's age")
	}
}

func TestAgentStartTimesSkipPanesWithoutAPid(t *testing.T) {
	m := newTestModel(t)
	mux := newFakeMux()
	m.mux = mux
	// herdr agents carry no pid: AgentPID resolves it (and records the pane).
	pane := agentItem("pi", "%7", "work")
	m = withItems(m, []sessionmgr.Item{pane})

	if got := len(m.pendingAgentStarts()); got != 1 {
		t.Fatalf("pending = %d, want the pane without a pid", got)
	}
	if m.agentStartPending {
		t.Fatal("pending flag set before the resolution was scheduled")
	}
}

func TestAgentStartTimesCmdResolvesTheAgentPid(t *testing.T) {
	mux := newFakeMux()
	mux.agentPID = os.Getpid()
	mux.agentPIDErr = nil
	pane := agentItem("pi", "%3", "work")

	msg, ok := agentStartTimesCmd(mux, []sessionmgr.Item{pane})().(agentStartedMsg)
	if !ok {
		t.Fatal("agentStartTimesCmd did not produce an agentStartedMsg")
	}
	if len(mux.pidQueries) != 1 || mux.pidQueries[0] != "%3" {
		t.Fatalf("pid queries = %v, want one for %%3", mux.pidQueries)
	}
	start, found := msg.starts["%3"]
	if !found {
		t.Skip("ps did not report this process; the ps path is covered by sessionmgr tests")
	}
	if start.pid != os.Getpid() {
		t.Fatalf("resolved pid = %d, want %d", start.pid, os.Getpid())
	}
	if time.Since(start.started) < 0 {
		t.Fatalf("resolved start %v is in the future", start.started)
	}
}

func TestAgentStartTimesCmdOmitsUnresolvablePanes(t *testing.T) {
	mux := newFakeMux()
	mux.agentPIDErr = errors.New("herdr: not running")
	pane := agentItem("pi", "%9", "work")

	msg, ok := agentStartTimesCmd(mux, []sessionmgr.Item{pane})().(agentStartedMsg)
	if !ok {
		t.Fatal("agentStartTimesCmd did not produce an agentStartedMsg")
	}
	if len(msg.starts) != 0 {
		t.Fatalf("starts = %v, want none for an unresolvable pane", msg.starts)
	}
}
