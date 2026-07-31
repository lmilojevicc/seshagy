package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

var noOptionalTestLayout = layoutSpec{
	render: renderDefault,
}

type countingPreviewMux struct {
	sessionmgr.Multiplexer
	captures int
}

func (m *countingPreviewMux) CaptureSession(
	context.Context,
	string,
	int,
) (string, error) {
	m.captures++
	return "captured preview", nil
}

func TestDefaultLayoutSpec(t *testing.T) {
	if defaultLayout.id != layoutDefault || defaultLayout.render == nil ||
		defaultLayout.renderActions == nil || defaultLayout.renderInput == nil ||
		defaultLayout.inputContentWidth == nil {
		t.Fatalf("defaultLayout contract is incomplete: %#v", defaultLayout)
	}
	want := layoutNeeds{Overview: true, Details: true, Preview: true}
	if defaultLayout.needs != want {
		t.Fatalf("defaultLayout needs = %#v, want %#v", defaultLayout.needs, want)
	}
}

func TestZenLayoutSpec(t *testing.T) {
	if zenLayout.id != layoutZen || zenLayout.render == nil || zenLayout.renderActions == nil ||
		zenLayout.renderInput == nil || zenLayout.inputContentWidth == nil {
		t.Fatalf("zenLayout contract is incomplete: %#v", zenLayout)
	}
	want := layoutNeeds{Overview: true}
	if zenLayout.needs != want {
		t.Fatalf("zenLayout needs = %#v, want %#v", zenLayout.needs, want)
	}

	m := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
	)
	m.loading = false
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	view := m.projectLayout(zenLayout.needs)
	if len(view.Sources.Entries) == 0 || len(view.Collection.Rows) != 1 ||
		len(view.Actions.Hints) == 0 || view.Search.Mode != searchClassic {
		t.Fatalf("Zen projection omitted mandatory surfaces: %#v", view)
	}
	if view.Overview == nil || view.Details != nil || view.Preview != nil {
		t.Fatalf("Zen optional projections = %#v", view)
	}
}

func TestModelLayoutSpecDefaults(t *testing.T) {
	created := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	if created.layout.render == nil || created.layout.needs != defaultLayout.needs {
		t.Fatalf("New layout = %#v, want defaultLayout", created.layout)
	}
	initMsg := created.Init()()
	initBatch, ok := initMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("default Init result = %T, want tea.BatchMsg", initMsg)
	}
	if len(initBatch) != 7 {
		t.Fatalf("default ModeAll Init batch = %d commands, want 7", len(initBatch))
	}
	created.source = sessionmgr.ModeSessions
	created.inflightRefresh = map[sessionmgr.SourceMode]uint64{sessionmgr.ModeSessions: 1}
	created.refreshGen = map[sessionmgr.SourceMode]uint64{sessionmgr.ModeSessions: 1}
	initMsg = created.Init()()
	initBatch, ok = initMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("default non-All Init result = %T, want tea.BatchMsg", initMsg)
	}
	if len(initBatch) != 8 {
		t.Fatalf("default non-All Init batch = %d commands, want 8", len(initBatch))
	}

	zero := Model{}
	resolved := zero.layoutSpec()
	if resolved.render == nil || resolved.needs != defaultLayout.needs {
		t.Fatalf("zero Model layout = %#v, want defaultLayout fallback", resolved)
	}
}

func TestLayoutNeedsGateOptionalPreparationOnly(t *testing.T) {
	m := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.layout = noOptionalTestLayout
	m.source = sessionmgr.ModeSessions
	m.loading = false
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{
		sessionmgr.ModeSessions: 1,
	}

	initMsg := m.Init()()
	initBatch, ok := initMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init command result = %T, want tea.BatchMsg", initMsg)
	}
	if len(initBatch) != 7 {
		t.Fatalf(
			"no-optional Init batch = %d commands, want 7 without background ModeAll warm",
			len(initBatch),
		)
	}
	view := m.projectLayout(m.layoutSpec().needs)
	if view.Overview != nil || view.Details != nil || view.Preview != nil {
		t.Fatalf("no-optional projection leaked optional views: %#v", view)
	}
	if len(view.Sources.Entries) == 0 || len(view.Collection.Rows) != 1 ||
		len(view.Actions.Hints) == 0 {
		t.Fatalf("no-optional projection gated mandatory views: %#v", view)
	}
	if cmd := m.previewForSelection(); cmd != nil {
		t.Fatal("no-optional layout scheduled preview capture")
	}
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: m.items, fetchedAt: time.Now()},
	}
	model, _ := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if got.inflightRefresh[sessionmgr.ModeAll] != 0 {
		t.Fatal("no-optional tick started background ModeAll refresh")
	}

	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.cache = nil
	var cmd tea.Cmd
	model, cmd = m.switchSource(sessionmgr.ModeFD)
	got = model.(Model)
	if got.source != sessionmgr.ModeFD || got.inflightRefresh[sessionmgr.ModeFD] == 0 ||
		cmd == nil {
		t.Fatalf(
			"mandatory source refresh was gated: source=%v inflight=%v cmd=%v",
			got.source,
			got.inflightRefresh,
			cmd,
		)
	}
}

func TestLayoutNeedsDoNotGateSharedControllerBehavior(t *testing.T) {
	m := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.layout = noOptionalTestLayout
	m.width, m.height = 80, 16
	m.loading = false
	m.source = sessionmgr.ModeSessions
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}

	model, cmd := m.handleActionKey(keyMsg("/"))
	got := model.(Model)
	if got.inputMode != modeSearch || cmd == nil {
		t.Fatalf("shared search was gated: mode=%v cmd=%v", got.inputMode, cmd)
	}

	got.inputMode = modeNormal
	model, cmd = got.startRename()
	got = model.(Model)
	if got.inputMode != modeRename || got.renameTarget != "demo" || cmd == nil {
		t.Fatalf(
			"shared rename was gated: mode=%v target=%q cmd=%v",
			got.inputMode,
			got.renameTarget,
			cmd,
		)
	}

	got.inputMode = modeNormal
	got.notify("shared notification", sevInfo)
	if frame := sessionmgr.StripANSI(got.View()); !strings.Contains(frame, "shared notification") {
		t.Fatalf("shared notification was gated\n%s", frame)
	}
}

func TestLayoutAndPreferenceGatePreviewCaptureThroughWindowSizeUpdate(t *testing.T) {
	for _, tt := range []struct {
		name          string
		layout        string
		preview       bool
		wantAvailable bool
		wantCommand   bool
		wantCapture   int
	}{
		{name: "default enabled", layout: appconfig.LayoutDefault, preview: true, wantAvailable: true, wantCommand: true, wantCapture: 1},
		{name: "default disabled", layout: appconfig.LayoutDefault, preview: false, wantAvailable: true},
		{name: "Zen enabled preference", layout: appconfig.LayoutZen, preview: true},
		{name: "Zen disabled preference", layout: appconfig.LayoutZen, preview: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.TUI.Layout = tt.layout
			cfg.TUI.Preview = &tt.preview
			mux := &countingPreviewMux{Multiplexer: sessionmgr.NewNoopBackend()}
			m := New(WithConfig(cfg), WithMultiplexer(mux))
			m.loading = false
			m.source = sessionmgr.ModeSessions
			m.items = []sessionmgr.Item{{
				Kind: sessionmgr.KindSession,
				Name: "demo",
			}}
			if m.previewAvailable() != tt.wantAvailable {
				t.Fatalf("previewAvailable = %v, want %v", m.previewAvailable(), tt.wantAvailable)
			}

			_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
			if (cmd != nil) != tt.wantCommand {
				t.Fatalf("WindowSize command present = %v, want %v", cmd != nil, tt.wantCommand)
			}
			if cmd != nil {
				if msg := cmd(); msg == nil {
					t.Fatal("preview command returned nil message")
				}
			}
			if mux.captures != tt.wantCapture {
				t.Fatalf("CaptureSession calls = %d, want %d", mux.captures, tt.wantCapture)
			}
		})
	}
}

func TestActiveModeAllRefreshDoesNotRequireOverview(t *testing.T) {
	m := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.layout = noOptionalTestLayout
	m.source = sessionmgr.ModeAll
	m.loading = false
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.cache = nil

	model, cmd := m.Update(tickMsg(time.Now()))
	got := model.(Model)
	if cmd == nil || got.inflightRefresh[sessionmgr.ModeAll] == 0 {
		t.Fatalf("active ModeAll refresh was gated: inflight=%v cmd=%v", got.inflightRefresh, cmd)
	}
}

func TestLayoutRenderDispatchUsesActiveSpecAndDefaultFallback(t *testing.T) {
	called := 0
	custom := layoutSpec{
		render: func(view layoutView, _ layoutRenderTheme) renderedDashboard {
			called++
			return renderedDashboard{Header: "custom-header", Body: "custom-body"}
		},
	}
	m := newTestModel(t)
	m.width, m.height = 80, 12
	m.loading = false
	m.showHelp = false
	m.layout = custom
	frame := sessionmgr.StripANSI(m.renderNormalSurface())
	if called != 1 || !containsAll(frame, "custom-header", "custom-body") {
		t.Fatalf("custom layout dispatch = called:%d frame:%q", called, frame)
	}

	m.layout = layoutSpec{}
	if got := m.layoutSpec(); got.render == nil || got.needs != defaultLayout.needs {
		t.Fatalf("zero layout fallback = %#v, want defaultLayout", got)
	}
}

func containsAll(text string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(text, want) {
			return false
		}
	}
	return true
}
