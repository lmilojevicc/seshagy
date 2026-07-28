package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func defaultTestView(m Model) layoutView {
	return m.projectLayout(layoutNeeds{Overview: true, Details: true, Preview: true})
}

func defaultTestTheme(m Model) defaultRenderTheme {
	return defaultRenderTheme{styles: m.styles, icons: m.config.IconSet()}
}

func defaultTestDetails(m Model, item sessionmgr.Item) *detailsView {
	clone := m
	clone.source = sessionmgr.ModeAll
	clone.items = []sessionmgr.Item{item}
	clone.cursor = 0
	clone.query = ""
	clone.agentsCurrentOnly = false
	clone.agentsStateFilter = ""
	return clone.projectDetails(time.Now())
}

func TestDefaultCompositionPreservesLoadingFilterMessage(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 100, 24
	m.source = sessionmgr.ModeSessions
	m.loading = true
	m.items = nil
	m.query = "missing"

	frame := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"refreshing…", "no matches for missing"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("loading filtered frame missing %q\n%s", want, frame)
		}
	}
}

func TestDefaultCompositionPreservesStalePreviewPayloads(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	base := newTestModel(t)
	base.width, base.height = 120, 32
	base.source = sessionmgr.ModeSessions
	base.loading = false
	base.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "current"}}
	base.previewKey = "previous"

	for _, tt := range []struct {
		name    string
		preview string
		err     error
		want    string
	}{
		{name: "ready", preview: "previous capture", want: "previous capture"},
		{name: "error", err: errors.New("previous failure"), want: "previous failure"},
		{name: "empty", preview: noPreviewAvailableText, want: noPreviewAvailableText},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			m.preview = tt.preview
			if tt.err != nil {
				m.previewError = tt.err.Error()
			}

			projected := m.projectLayout(layoutNeeds{Preview: true}).Preview
			if projected == nil {
				t.Fatal("requested stale Preview = nil")
			}
			raw := m.View()
			clean := sessionmgr.StripANSI(raw)
			if !strings.Contains(clean, tt.want) {
				t.Fatalf("stale %s frame missing %q\n%s", tt.name, tt.want, clean)
			}
			if strings.Contains(clean, "preview loading…") {
				t.Fatalf("stale %s payload replaced by loading chrome\n%s", tt.name, clean)
			}
			if tt.err != nil && !strings.Contains(raw, m.styles.danger.Render(tt.want)) {
				t.Fatalf("stale error lost danger styling: %q", raw)
			}
		})
	}
}

func TestDefaultSourceCountBadgeUsesTotalOnlyForQueryFiltering(t *testing.T) {
	view := sourcesView{VisibleCount: 1, TotalCount: 4}
	if got := defaultSourceCountBadge(view); got != "1" {
		t.Fatalf("non-query count badge = %q, want %q", got, "1")
	}

	view.ShowTotalCount = true
	if got := defaultSourceCountBadge(view); got != "1/4" {
		t.Fatalf("query-filtered count badge = %q, want %q", got, "1/4")
	}
}
