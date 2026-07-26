package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestProjectLayoutOptionalNeedsGateOnlyOptionalViews(t *testing.T) {
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeAll
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	m.preview = "captured"

	for _, tt := range []struct {
		name string
		need layoutNeeds
	}{
		{name: "none"},
		{name: "overview", need: layoutNeeds{Overview: true}},
		{name: "details", need: layoutNeeds{Details: true}},
		{name: "preview", need: layoutNeeds{Preview: true}},
		{name: "all", need: layoutNeeds{Overview: true, Details: true, Preview: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := m.projectLayout(tt.need)
			if (got.Overview != nil) != tt.need.Overview {
				t.Errorf("Overview presence = %v, want %v", got.Overview != nil, tt.need.Overview)
			}
			if (got.Details != nil) != tt.need.Details {
				t.Errorf("Details presence = %v, want %v", got.Details != nil, tt.need.Details)
			}
			if (got.Preview != nil) != tt.need.Preview {
				t.Errorf("Preview presence = %v, want %v", got.Preview != nil, tt.need.Preview)
			}
			if len(got.Sources.Entries) == 0 || len(got.Collection.Rows) != 1 ||
				len(got.Actions.Hints) == 0 {
				t.Fatalf("mandatory views were gated: %#v", got)
			}
		})
	}
}

func TestProjectOverviewLifecycleAndCounts(t *testing.T) {
	base := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	base.loading = false
	base.source = sessionmgr.ModeSessions
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "one"},
		{Kind: sessionmgr.KindSession, Name: "two"},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentBlocked},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentDone},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentIdle},
		{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentUnknown},
		{Kind: sessionmgr.KindFD, Path: "/ignored"},
	}

	for _, tt := range []struct {
		name       string
		mutate     func(*Model)
		wantState  overviewState
		wantWarn   string
		wantError  string
		wantCounts bool
	}{
		{name: "pre-load", wantState: overviewLoading},
		{
			name: "empty", wantState: overviewEmpty,
			mutate: func(m *Model) {
				m.cache = map[sessionmgr.SourceMode]modeCache{sessionmgr.ModeAll: {}}
			},
		},
		{
			name: "warning", wantState: overviewWarning, wantWarn: "partial", wantCounts: true,
			mutate: func(m *Model) {
				m.cache = map[sessionmgr.SourceMode]modeCache{
					sessionmgr.ModeAll: {items: items, warning: "partial"},
				}
			},
		},
		{
			name: "error", wantState: overviewError, wantError: "failed",
			mutate: func(m *Model) {
				m.cache = map[sessionmgr.SourceMode]modeCache{
					sessionmgr.ModeAll: {err: errors.New("failed")},
				}
			},
		},
		{
			name: "ready", wantState: overviewReady, wantCounts: true,
			mutate: func(m *Model) {
				m.cache = map[sessionmgr.SourceMode]modeCache{
					sessionmgr.ModeAll: {items: items},
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			m.cache = nil
			if tt.mutate != nil {
				tt.mutate(&m)
			}
			beforeNotifications := len(m.notifications)
			got := m.projectLayout(layoutNeeds{Overview: true}).Overview
			if got == nil || got.State != tt.wantState || got.Warning != tt.wantWarn ||
				got.Error != tt.wantError {
				t.Fatalf(
					"Overview = %#v, want state=%v warning=%q error=%q",
					got,
					tt.wantState,
					tt.wantWarn,
					tt.wantError,
				)
			}
			if len(m.notifications) != beforeNotifications {
				t.Fatal("Overview projection changed notification state")
			}
			if tt.wantCounts {
				wantAgents := overviewAgentCountsView{
					Working: 1,
					Blocked: 1,
					Done:    1,
					Idle:    1,
					Unknown: 1,
				}
				if got.Sessions != 2 || got.Agents != wantAgents {
					t.Fatalf("Overview counts = sessions:%d agents:%#v", got.Sessions, got.Agents)
				}
			}
		})
	}

	active := base
	active.source = sessionmgr.ModeAll
	active.loading = true
	active.items = nil
	if got := active.projectLayout(
		layoutNeeds{Overview: true},
	).Overview; got.State != overviewLoading {
		t.Fatalf("active pre-load Overview = %#v, want loading", got)
	}
}

func TestProjectDetailsLifecycleKindsAndHerdrRedaction(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	base := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	base.loading = false
	base.source = sessionmgr.ModeAll

	if got := base.projectLayoutAtWithNeeds(
		now,
		layoutNeeds{Details: true},
	).Details; got == nil || got.State != detailsNoSelection || got.Title != "Details" ||
		len(got.Fields) != 0 {
		t.Fatalf("no-selection Details = %#v", got)
	}

	session := base
	session.items = []sessionmgr.Item{{
		Kind: sessionmgr.KindSession, Name: "demo", Path: "/home/test/demo", Attached: true,
		Windows: 2, Panes: 3, Activity: now.Add(-2 * time.Hour), Created: now.Add(-24 * time.Hour),
	}}
	gotSession := session.projectLayoutAtWithNeeds(now, layoutNeeds{Details: true}).Details
	wantSessionFields := []detailFieldView{
		{Label: "path", Value: "/home/test/demo"},
		{
			Label:     "attached",
			Value:     "attached",
			Indicator: projectTmuxStateIndicator(session.config.IconSet(), true),
		},
		{Label: "windows", Value: "2"},
		{Label: "panes", Value: "3"},
		{Label: "activity", Value: "2h"},
		{Label: "created", Value: "1d"},
	}
	if gotSession.State != detailsReady || gotSession.Title != "demo · tmux session" ||
		!reflect.DeepEqual(gotSession.Fields, wantSessionFields) {
		t.Fatalf("session Details = %#v, want fields %#v", gotSession, wantSessionFields)
	}

	hiddenConfig := appconfig.Default()
	hiddenConfig.Icons.TmuxStateMode = appconfig.StateDisplayModeNone
	hiddenConfig.Normalize()
	hidden := New(
		WithConfig(hiddenConfig),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	hidden.loading = false
	hidden.source = sessionmgr.ModeSessions
	hidden.items = append([]sessionmgr.Item(nil), session.items...)
	hiddenDetails := hidden.projectLayoutAtWithNeeds(now, layoutNeeds{Details: true}).Details
	if field := hiddenDetails.Fields[1]; field.Value != "yes" ||
		field.Indicator.Mode != displayHidden {
		t.Fatalf("hidden attached Details field = %#v, want yes without indicator", field)
	}

	directory := base
	directory.items = []sessionmgr.Item{{Kind: sessionmgr.KindFD, Path: "/src/demo"}}
	gotDirectory := directory.projectLayoutAtWithNeeds(now, layoutNeeds{Details: true}).Details
	wantDirectoryFields := []detailFieldView{
		{Label: "path", Value: "/src/demo"},
		{Label: "enter", Value: "create/switch tmux session"},
	}
	if gotDirectory.Title != "demo · fd directory" ||
		!reflect.DeepEqual(gotDirectory.Fields, wantDirectoryFields) {
		t.Fatalf("directory Details = %#v", gotDirectory)
	}

	tmuxAgent := base
	tmuxAgent.items = []sessionmgr.Item{{
		Kind: sessionmgr.KindAgent, AgentName: "pi", AgentDisplayName: "reviewer",
		AgentState: sessionmgr.AgentWorking, Location: "demo:1", Session: "demo",
		Path: "/home/test/demo", TabLabel: "editor",
	}}
	gotTmuxAgent := tmuxAgent.projectLayoutAtWithNeeds(now, layoutNeeds{Details: true}).Details
	if gotTmuxAgent.Title != "reviewer · agent" || detailValue(gotTmuxAgent, "session") != "demo" ||
		detailValue(
			gotTmuxAgent,
			"window",
		) != "editor" || detailValue(gotTmuxAgent, "state") != "working" {
		t.Fatalf("tmux agent Details = %#v", gotTmuxAgent)
	}

	herdr := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewHerdrBackend()))
	herdr.loading = false
	herdr.source = sessionmgr.ModeAgents
	herdr.items = []sessionmgr.Item{{
		Kind: sessionmgr.KindAgent, AgentName: "pi", AgentDisplayName: "reviewer",
		AgentState: sessionmgr.AgentWorking, Location: "Frontend", Path: "/src/frontend",
		Session: "WORKSPACE_OPAQUE", Window: "TAB_OPAQUE", PaneID: "PANE_OPAQUE",
		Pane: "PANE_NUMBER_OPAQUE", TabLabel: "Editor",
	}}
	gotHerdr := herdr.projectLayoutAtWithNeeds(now, layoutNeeds{Details: true}).Details
	projected := allProjectedStrings(*gotHerdr)
	for _, forbidden := range []string{"WORKSPACE_OPAQUE", "TAB_OPAQUE", "PANE_OPAQUE", "PANE_NUMBER_OPAQUE"} {
		if strings.Contains(projected, forbidden) {
			t.Errorf("herdr Details leaks %q in %q", forbidden, projected)
		}
	}
	if detailValue(gotHerdr, "workspace") != "" || detailValue(gotHerdr, "tab") != "Editor" ||
		detailValue(gotHerdr, "location") != "Frontend" {
		t.Fatalf("herdr Details labels/redaction = %#v", gotHerdr)
	}
}

func detailValue(view *detailsView, label string) string {
	for _, field := range view.Fields {
		if field.Label == label {
			return field.Value
		}
	}
	return ""
}

func TestProjectPreviewLifecycleTitleAnchorAndWidthIndependence(t *testing.T) {
	cfg := appconfig.Default()
	m := New(WithConfig(cfg), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeSessions
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}

	if got := m.projectLayout(layoutNeeds{}).Preview; got != nil {
		t.Fatalf("unrequested Preview = %#v, want nil", got)
	}
	m.showPreview = false
	if got := m.projectLayout(layoutNeeds{Preview: true}).Preview; got != nil {
		t.Fatalf("toggle-disabled Preview = %#v, want nil", got)
	}
	m.showPreview = true
	loading := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if loading == nil || loading.State != previewLoading || loading.Title != "Preview · demo" ||
		loading.Anchor != previewAnchorBottom {
		t.Fatalf("loading session Preview = %#v", loading)
	}

	m.preview = "captured"
	m.previewKey = m.selectedKey()
	ready := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if ready.State != previewReady || ready.Content != "captured" ||
		ready.Anchor != previewAnchorBottom {
		t.Fatalf("ready session Preview = %#v", ready)
	}

	m.preview = noPreviewAvailableText
	empty := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if empty.State != previewEmpty || empty.Content != noPreviewAvailableText {
		t.Fatalf("empty Preview = %#v", empty)
	}

	m.preview = "old capture"
	m.previewKey = "session:other"
	staleReady := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if staleReady.State != previewLoading || staleReady.Content != "old capture" {
		t.Fatalf("selection-changing ready Preview = %#v", staleReady)
	}

	m.preview = ""
	m.previewError = "old failure"
	staleError := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if staleError.State != previewLoading || staleError.Error != "old failure" {
		t.Fatalf("selection-changing error Preview = %#v", staleError)
	}

	m.preview = noPreviewAvailableText
	m.previewError = ""
	staleEmpty := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if staleEmpty.State != previewLoading || staleEmpty.Content != noPreviewAvailableText {
		t.Fatalf("selection-changing empty Preview = %#v", staleEmpty)
	}

	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindFD, Path: "/src/demo"}}
	m.preview = "file.txt"
	m.previewKey = m.selectedKey()
	directory := m.projectLayout(layoutNeeds{Preview: true}).Preview
	if directory.State != previewReady || directory.Title != "Preview · /src/demo" ||
		directory.Anchor != previewAnchorTop {
		t.Fatalf("directory Preview = %#v", directory)
	}

	off := false
	cfg.TUI.Preview = &off
	configuredOff := New(WithConfig(cfg), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	configuredOff.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	if got := configuredOff.projectLayout(layoutNeeds{Preview: true}).Preview; got != nil {
		t.Fatalf("config-disabled Preview = %#v, want nil", got)
	}
}

func TestOptionalProjectionValuesDoNotAliasLaterProjections(t *testing.T) {
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeAll
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo", Path: "/src/demo"}}
	m.preview = "captured"
	needs := layoutNeeds{Overview: true, Details: true, Preview: true}

	first := m.projectLayout(needs)
	first.Overview.Sessions = 99
	first.Details.Fields[0].Value = "mutated"
	first.Preview.Content = "mutated"
	second := m.projectLayout(needs)
	if second.Overview.Sessions != 1 || second.Details.Fields[0].Value != "/src/demo" ||
		second.Preview.Content != "captured" {
		t.Fatalf("optional projections alias across calls: %#v", second)
	}
}

func TestOptionalProjectionValuesAreWidthIndependent(t *testing.T) {
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeAll
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
	m.preview = "captured"
	m.previewKey = m.selectedKey()
	needs := layoutNeeds{Overview: true, Details: true, Preview: true}

	m.width, m.height = 20, 3
	compact := m.projectLayout(needs)
	m.width, m.height = 200, 80
	wide := m.projectLayout(needs)

	if !reflect.DeepEqual(compact.Overview, wide.Overview) ||
		!reflect.DeepEqual(compact.Details, wide.Details) ||
		!reflect.DeepEqual(compact.Preview, wide.Preview) {
		t.Fatalf(
			"optional projections depend on terminal dimensions: compact=%#v wide=%#v",
			compact,
			wide,
		)
	}
}

func TestProjectPreviewErrorIsSemanticAndRecovers(t *testing.T) {
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.width, m.height = 120, 32
	m.loading = false
	m.source = sessionmgr.ModeSessions
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "alpha"}}

	model, cmd := m.Update(previewMsg{key: m.selectedKey(), err: errors.New("preview failed")})
	got := model.(Model)
	if cmd != nil {
		t.Fatalf("preview error command = %v, want nil", cmd)
	}
	preview := got.projectLayout(layoutNeeds{Preview: true}).Preview
	if preview == nil || preview.State != previewError || preview.Error != "preview failed" ||
		preview.Content != "" {
		t.Fatalf("semantic preview error = %#v", preview)
	}
	model, _ = got.Update(previewMsg{key: got.selectedKey(), preview: "recovered"})
	recovered := model.(Model)
	if recovered.previewError != "" || recovered.preview != "recovered" {
		t.Fatalf(
			"preview success did not clear error: preview=%q error=%q",
			recovered.preview,
			recovered.previewError,
		)
	}
}
