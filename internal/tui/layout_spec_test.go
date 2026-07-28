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
	if defaultLayout.render == nil {
		t.Fatal("defaultLayout renderer is nil")
	}
	want := layoutNeeds{Overview: true, Details: true, Preview: true}
	if defaultLayout.needs != want {
		t.Fatalf("defaultLayout needs = %#v, want %#v", defaultLayout.needs, want)
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
	m.showPreview = false
	model, cmd := m.handleActionKey(keyMsg("p"))
	got := model.(Model)
	if !got.showPreview || cmd != nil {
		t.Fatalf(
			"no-optional preview toggle = show:%v cmd:%v, want shared toggle without capture",
			got.showPreview,
			cmd,
		)
	}

	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: m.items, fetchedAt: time.Now()},
	}
	model, _ = m.Update(tickMsg(time.Now()))
	got = model.(Model)
	if got.inflightRefresh[sessionmgr.ModeAll] != 0 {
		t.Fatal("no-optional tick started background ModeAll refresh")
	}

	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.cache = nil
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

func TestLayoutNeedsGatePreviewCaptureThroughWindowSizeUpdate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		layout      layoutSpec
		wantCommand bool
		wantCapture int
	}{
		{name: "default", layout: defaultLayout, wantCommand: true, wantCapture: 1},
		{name: "no optional", layout: noOptionalTestLayout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mux := &countingPreviewMux{Multiplexer: sessionmgr.NewNoopBackend()}
			m := New(WithConfig(appconfig.Default()), WithMultiplexer(mux))
			m.layout = tt.layout
			m.loading = false
			m.source = sessionmgr.ModeSessions
			m.items = []sessionmgr.Item{{
				Kind: sessionmgr.KindSession,
				Name: "demo",
			}}

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
