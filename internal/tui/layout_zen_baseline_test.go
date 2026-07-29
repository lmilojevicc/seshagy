package tui

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestZenLayoutMatchesApprovedFrames(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, tt := range []struct {
		name          string
		width, height int
	}{
		{name: "wide", width: 120, height: 32},
		{name: "narrow", width: 51, height: 24},
		{name: "short", width: 51, height: 16},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := readBase64FrameFixture(t, "testdata/zen_"+tt.name+".golden.b64")
			got := []byte(zenBaselineModel(tt.width, tt.height).View())
			if !bytes.Equal(got, want) {
				t.Fatalf(
					"Zen %s frame differs from approved baseline\ngot %d bytes, want %d",
					tt.name,
					len(got),
					len(want),
				)
			}

			clean := sessionmgr.StripANSI(string(got))
			for _, forbidden := range []string{
				"RAW_SESSION_ID_SENTINEL",
				"RAW_SESSION_TARGET",
				"RAW_PANE_ID_SENTINEL",
				"RAW_WINDOW_ID_SENTINEL",
				"RAW_PREVIEW_SENTINEL",
				"RAW_DETAILS_SENTINEL",
			} {
				if strings.Contains(clean, forbidden) {
					t.Fatalf("Zen %s fixture exposes %q\n%s", tt.name, forbidden, clean)
				}
			}
			if lipgloss.Height(string(got)) != tt.height {
				t.Fatalf(
					"Zen %s frame height = %d, want %d",
					tt.name,
					lipgloss.Height(string(got)),
					tt.height,
				)
			}
		})
	}
}

func TestMissingAndExplicitDefaultMatchExistingFrames(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	for _, layout := range []string{"", appconfig.LayoutDefault} {
		name := layout
		if name == "" {
			name = "missing"
		}
		for _, tt := range []struct {
			name  string
			width int
		}{
			{name: "wide", width: 140},
			{name: "narrow", width: 46},
		} {
			t.Run(name+"_"+tt.name, func(t *testing.T) {
				want := readBase64FrameFixture(
					t,
					"testdata/default_"+tt.name+".golden.b64",
				)
				got := []byte(approvedDefaultBaselineModelWithLayout(t, tt.width, layout).View())
				if !bytes.Equal(got, want) {
					t.Fatalf(
						"layout %q default %s frame changed\ngot %d bytes, want %d",
						layout,
						tt.name,
						len(got),
						len(want),
					)
				}
			})
		}
	}
}

func readBase64FrameFixture(t *testing.T, path string) []byte {
	t.Helper()
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func zenBaselineModel(width, height int) Model {
	cfg := appconfig.Default()
	cfg.TUI.Layout = appconfig.LayoutZen
	preview := true
	cfg.TUI.Preview = &preview
	m := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.width, m.height = width, height
	m.loading = false
	m.source = sessionmgr.ModeAgents
	m.showHelp = true
	m.showPreview = true
	m.spinnerFrame = 0
	m.spinnerActive = false
	m.notifications = nil
	m.agentsCurrentOnly = true
	m.currentSession = "RAW_SESSION_ID_SENTINEL"
	m.agentsStateFilter = sessionmgr.AgentBlocked
	m.query = "review"
	m.searchInput.SetValue(m.query)

	fixed := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	agents := []sessionmgr.Item{
		zenBaselineAgent(
			"pi",
			"review alpha",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"1",
			fixed,
		),
		zenBaselineAgent(
			"claude",
			"review beta",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"2",
			fixed,
		),
		zenBaselineAgent(
			"codex",
			"review gamma",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"3",
			fixed,
		),
		zenBaselineAgent(
			"droid",
			"review delta",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"4",
			fixed,
		),
		zenBaselineAgent(
			"pi",
			"review epsilon",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"5",
			fixed,
		),
		zenBaselineAgent(
			"claude",
			"review zeta",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"6",
			fixed,
		),
		zenBaselineAgent(
			"codex",
			"review eta",
			sessionmgr.AgentBlocked,
			"workspace alpha",
			"7",
			fixed,
		),
		zenBaselineAgent(
			"opencode",
			"builder",
			sessionmgr.AgentWorking,
			"workspace alpha",
			"8",
			fixed,
		),
		zenBaselineAgent("agy", "planner", sessionmgr.AgentWorking, "workspace beta", "9", fixed),
		zenBaselineAgent("claude", "finisher", sessionmgr.AgentDone, "workspace beta", "10", fixed),
		zenBaselineAgent("pi", "waiting", sessionmgr.AgentIdle, "workspace beta", "11", fixed),
		zenBaselineAgent(
			"cursor",
			"observer",
			sessionmgr.AgentUnknown,
			"workspace beta",
			"12",
			fixed,
		),
	}
	m.items = append([]sessionmgr.Item(nil), agents...)
	m.cursor = 1
	m.preview = "RAW_PREVIEW_SENTINEL"
	m.previewError = ""
	m.previewKey = "RAW_PANE_ID_SENTINEL"

	sessions := []sessionmgr.Item{
		{
			Kind: sessionmgr.KindSession, Name: "workspace alpha", Target: "RAW_SESSION_TARGET_A",
			Path: "example/workspace-alpha", Activity: fixed, Created: fixed,
		},
		{
			Kind: sessionmgr.KindSession, Name: "workspace beta", Target: "RAW_SESSION_TARGET_B",
			Path: "example/workspace-beta", Activity: fixed, Created: fixed,
		},
	}
	all := append(append([]sessionmgr.Item(nil), sessions...), agents...)
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeAll: {
			items:     all,
			fetchedAt: fixed,
		},
		sessionmgr.ModeSessions: {
			items:     sessions,
			fetchedAt: fixed,
		},
		sessionmgr.ModeZoxide: {
			items: []sessionmgr.Item{{
				Kind: sessionmgr.KindZoxide, Name: "example project", Path: "example/project",
			}},
			fetchedAt: fixed,
		},
		sessionmgr.ModeFD: {
			items: []sessionmgr.Item{{
				Kind: sessionmgr.KindFD, Name: "example archive", Path: "example/archive",
			}},
			fetchedAt: fixed,
		},
	}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	return m
}

func zenBaselineAgent(
	name string,
	displayName string,
	state sessionmgr.AgentState,
	location string,
	index string,
	updated time.Time,
) sessionmgr.Item {
	return sessionmgr.Item{
		Kind:             sessionmgr.KindAgent,
		AgentName:        name,
		AgentDisplayName: displayName,
		AgentState:       state,
		AgentUpdated:     updated,
		PaneID:           "RAW_PANE_ID_SENTINEL_" + index,
		Session:          "RAW_SESSION_ID_SENTINEL",
		Window:           "RAW_WINDOW_ID_SENTINEL",
		Pane:             index,
		Location:         location,
		Path:             "example/agent-" + index,
	}
}
