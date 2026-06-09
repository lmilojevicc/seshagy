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

func TestFooterKeepsStatusOnOneLine(t *testing.T) {
	m := New()
	m.width = 80
	m.source = sessionmgr.ModeAll
	m.status = "loaded 1171 items"
	m.showHelp = false

	footer := m.renderFooter()
	if height := lipgloss.Height(footer); height != 2 {
		t.Fatalf("footer height = %d, want 2\n%s", height, sessionmgr.StripANSI(footer))
	}
	clean := sessionmgr.StripANSI(footer)
	lines := strings.Split(clean, "\n")
	if len(lines) != 2 {
		t.Fatalf("footer lines = %d, want 2\n%s", len(lines), clean)
	}
	if !strings.Contains(lines[0], "loaded 1171 items") {
		t.Fatalf("status wrapped or disappeared from first line:\n%s", clean)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width >= m.width {
			t.Fatalf("footer line %d width = %d, want less than terminal width %d", i, width, m.width)
		}
	}
}

func TestFooterWarningStatusesUseWarningStyle(t *testing.T) {
	s := defaultStyles()
	warnings := []string{
		"no integrations selected",
		"hook installation skipped",
		"rename cancelled",
		"yazi closed without a directory",
		"nothing selected",
		"delete only applies to sessions and agents",
		"rename only applies to sessions",
	}
	for _, status := range warnings {
		style := footerStatusStyle(s, status, false)
		if style.GetForeground() != s.warning.GetForeground() || !style.GetBold() {
			t.Fatalf("footerStatusStyle(%q) = foreground %v bold %v, want warning foreground %v bold true", status, style.GetForeground(), style.GetBold(), s.warning.GetForeground())
		}
		m := New()
		m.width = 80
		m.status = status
		m.showHelp = false
		if clean := sessionmgr.StripANSI(m.renderFooter()); !strings.Contains(strings.Split(clean, "\n")[0], status) {
			t.Fatalf("footer did not render warning status %q on first line:\n%s", status, clean)
		}
	}
}

func TestFooterStatusStylesKeepErrorsRedAndNormalMuted(t *testing.T) {
	s := defaultStyles()
	if style := footerStatusStyle(s, "loaded 1171 items", false); style.GetForeground() != s.muted.GetForeground() || style.GetBold() != s.muted.GetBold() {
		t.Fatalf("normal status style = foreground %v bold %v, want muted foreground %v bold %v", style.GetForeground(), style.GetBold(), s.muted.GetForeground(), s.muted.GetBold())
	}
	if style := footerStatusStyle(s, "nothing selected", true); style.GetForeground() != s.danger.GetForeground() || style.GetBold() != s.danger.GetBold() {
		t.Fatalf("error status style = foreground %v bold %v, want danger foreground %v bold %v", style.GetForeground(), style.GetBold(), s.danger.GetForeground(), s.danger.GetBold())
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
