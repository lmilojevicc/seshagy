package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestViewRendersDashboardChromeAndRows(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = model.(Model)
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Path: "/tmp/demo", Windows: 1, Activity: time.Now(), Created: time.Now()},
		{Kind: sessionmgr.KindAgent, Name: "pi", AgentName: "pi", AgentState: sessionmgr.AgentWorking, PaneID: "%1", Location: "demo:1.1", Path: "~/demo"},
		{Kind: sessionmgr.KindZoxide, Name: "~/code/demo", Path: "~/code/demo"},
	}
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"seshagy", "[1] All", "All (3", "demo", "pi", "Preview"} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() missing %q\n%s", want, out)
		}
	}
}

func TestFilterVisibleItems(t *testing.T) {
	m := New()
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "api"},
		{Kind: sessionmgr.KindSession, Name: "web"},
		{Kind: sessionmgr.KindAgent, AgentName: "pi", Location: "api:1.1"},
	}
	m.query = "api"
	got := m.visibleItems()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
}

func TestDefaultStylesUseTerminalPalette(t *testing.T) {
	s := defaultStyles()
	if _, ok := s.app.GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf("app foreground should use terminal default, got %T", s.app.GetForeground())
	}
	if _, ok := s.app.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("app background should use terminal default, got %T", s.app.GetBackground())
	}
	if _, ok := s.status.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("status background should use terminal default, got %T", s.status.GetBackground())
	}
	if !s.selectedBG.GetReverse() {
		t.Fatal("selected rows should use reverse video so selection follows terminal colors")
	}

	for name, color := range map[string]lipgloss.TerminalColor{
		"session": s.p.green,
		"zoxide":  s.p.sky,
		"fd":      s.p.peach,
		"agent":   s.p.mauve,
	} {
		value, ok := color.(lipgloss.Color)
		if !ok {
			t.Fatalf("%s icon color should come from ANSI terminal palette, got %T", name, color)
		}
		if strings.HasPrefix(string(value), "#") {
			t.Fatalf("%s icon color should not be fixed truecolor: %s", name, value)
		}
	}
}

func TestIntegrationPromptRendersToggleRows(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = model.(Model)
	m.integrationPrompt = true
	m.integrationRows = []integrations.Recommendation{{Target: integrations.TargetPi, Label: "Pi", AgentAvailable: true, Installable: true, State: integrations.StatusNotInstalled}}
	m.integrationSelected[integrations.TargetPi] = true
	out := sessionmgr.StripANSI(m.View())
	for _, want := range []string{"Install agent state hooks?", "[x] Pi", "space toggle", "pane text or process", "inspection"} {
		if !strings.Contains(out, want) {
			t.Fatalf("integration prompt missing %q\n%s", want, out)
		}
	}
}
