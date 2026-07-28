package tui

import (
	"bytes"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// TestDefaultLayoutMatchesPreRefactorFrames compares complete raw frames
// generated from main@4067ac3 before the projection/renderer cutover. The
// fixtures were captured from a read-only git archive with the same fixed
// model data and TrueColor profile used below.
func TestDefaultLayoutMatchesPreRefactorFrames(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, tt := range []struct {
		name  string
		width int
	}{
		{name: "wide", width: 140},
		{name: "narrow", width: 46},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture, err := os.ReadFile("testdata/default_" + tt.name + ".golden.b64")
			if err != nil {
				t.Fatal(err)
			}
			want, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(fixture)))
			if err != nil {
				t.Fatal(err)
			}
			got := []byte(preRefactorBaselineModel(t, tt.width).View())
			if !bytes.Equal(got, want) {
				t.Fatalf(
					"default %s frame differs from pre-refactor baseline\ngot %d bytes, want %d",
					tt.name,
					len(got),
					len(want),
				)
			}
		})
	}
}

func preRefactorBaselineModel(t *testing.T, width int) Model {
	t.Helper()
	cfg := appconfig.Default()
	preview := true
	cfg.TUI.Preview = &preview
	m := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.width, m.height = width, 24
	m.loading = false
	m.source = sessionmgr.ModeAll
	m.showHelp = true
	m.showPreview = true
	m.spinnerFrame = 0
	m.spinnerActive = false

	fixed := time.Date(2000, 1, 2, 3, 4, 5, 0, time.Local)
	m.items = []sessionmgr.Item{
		{
			Kind: sessionmgr.KindSession, Name: "alpha", Path: "/tmp/alpha",
			Attached: true, Windows: 2, Panes: 3,
		},
		{Kind: sessionmgr.KindSession, Name: "beta", Path: "/tmp/beta", Windows: 1},
		{
			Kind: sessionmgr.KindAgent, AgentName: "pi", AgentDisplayName: "frontend",
			AgentState: sessionmgr.AgentWorking, PaneID: "%1", Session: "alpha",
			Window: "1", Pane: "1", Location: "alpha:1.1", Path: "/tmp/alpha",
			AgentUpdated: fixed,
		},
		{
			Kind: sessionmgr.KindAgent, AgentName: "claude", AgentState: sessionmgr.AgentIdle,
			PaneID: "%2", Session: "beta", Window: "1", Pane: "1",
			Location: "beta:1.1", Path: "/tmp/beta", AgentUpdated: fixed,
		},
		{Kind: sessionmgr.KindZoxide, Name: "~/src/project", Path: "~/src/project"},
		{Kind: sessionmgr.KindFD, Name: "/tmp/example", Path: "/tmp/example"},
	}
	m.cursor = 0
	m.previewKey = m.selectedKey()
	m.preview = "preview line one\npreview line two"
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {
			items:     append([]sessionmgr.Item(nil), m.items...),
			fetchedAt: fixed,
		},
	}
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	return m
}
