package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestOverlayPlacesForegroundAndPreservesDimmedBackground(t *testing.T) {
	// Raw ANSI (not lipgloss) so the test is deterministic regardless of the
	// detected color profile: the overlay must preserve whatever styling the
	// background carries on the cells it does not overwrite.
	gray := "\x1b[38;5;242m"
	reset := "\x1b[0m"
	bg := strings.Join([]string{
		gray + "aaaaaaaaaaaa" + reset,
		gray + "bbbbbbbbbbbb" + reset,
	}, "\n")
	fg := strings.Join([]string{"XX", "YY"}, "\n")

	got := overlay(bg, fg, 4, 0)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d (%q)", len(lines), got)
	}
	// fg overwrites columns 4..5; left (4) + fg (2) + right (6) keep width 12.
	cases := []struct{ line, want string }{
		{lines[0], "aaaaXXaaaaaa"},
		{lines[1], "bbbbYYbbbbbb"},
	}
	for _, c := range cases {
		plain := ansi.Strip(c.line)
		if plain != c.want {
			t.Errorf("row content = %q, want %q", plain, c.want)
		}
		if w := lipgloss.Width(c.line); w != 12 {
			t.Errorf("row width = %d, want 12", w)
		}
		// Dim styling must survive on both background portions around the popup.
		left := ansi.Truncate(c.line, 4, "")
		right := ansi.TruncateLeft(c.line, 6, "")
		if !strings.Contains(left, gray) || !strings.Contains(left, reset) {
			t.Errorf("left bg lost gray styling: %q", left)
		}
		if !strings.Contains(right, gray) || !strings.Contains(right, reset) {
			t.Errorf("right bg lost gray styling: %q", right)
		}
	}
}

func TestOverlayYOffsetAndWideRunes(t *testing.T) {
	gray := "\x1b[38;5;242m"
	reset := "\x1b[0m"
	// "你好" = 4 visible columns (each CJK rune is width 2).
	bg := gray + "你好cd" + reset // width 6
	got := overlay(bg, "ZZ", 2, 0)
	// Column 2..3 -> replaces "好" with "ZZ"; "你" (cols 0-1) and "cd" (cols 4-5) stay.
	if plain := ansi.Strip(got); plain != "你ZZcd" {
		t.Errorf("wide-rune overlay = %q, want %q", plain, "你ZZcd")
	}

	// y-offset: a single fg row at y=1 only touches the second bg line.
	two := gray + "aaaa" + reset + "\n" + gray + "bbbb" + reset
	got2 := overlay(two, "ZZ", 1, 1)
	rows := strings.Split(got2, "\n")
	if ansi.Strip(rows[0]) != "aaaa" {
		t.Errorf("untouched row0 = %q, want aaaa", ansi.Strip(rows[0]))
	}
	if ansi.Strip(rows[1]) != "bZZb" {
		t.Errorf("overlaid row1 = %q, want bZZb", ansi.Strip(rows[1]))
	}
}

func TestInputPopupActiveGuardsSize(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.inputMode = modeSearch
	if !m.inputPopupActive() {
		t.Error("search mode on a large terminal should be popup-active")
	}
	m.width, m.height = 30, 32
	if m.inputPopupActive() {
		t.Error("narrow terminal should suppress the popup")
	}
	m.width, m.height = 120, 4
	if m.inputPopupActive() {
		t.Error("short terminal should suppress the popup")
	}
	m.inputMode = modeNormal
	m.width, m.height = 120, 32
	if m.inputPopupActive() {
		t.Error("normal mode must never be popup-active")
	}
}

func TestInputPopupInactiveForCmdline(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	m.width, m.height = 120, 32
	for _, mode := range []inputMode{modeSearch, modeRename, modeNormal} {
		m.inputMode = mode
		if m.inputPopupActive() {
			t.Errorf("cmdline input_style should never show the popup (mode %v)", mode)
		}
	}
}

func TestFooterCmdlineShowsTextInputOnLine1(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	m.width = 80
	m.height = 32

	// Search mode: line 1 is the textinput (prompt "/ " + value), not the
	// status line. The status style adds 1-col padding, so trim before checking.
	m.inputMode = modeSearch
	m.searchInput.SetValue("my-project")
	footer := sessionmgr.StripANSI(m.renderFooter())
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("footer lines = %d, want 2\n%s", len(lines), footer)
	}
	if trimmed := strings.TrimSpace(lines[0]); !strings.HasPrefix(trimmed, "/ my-project") {
		t.Fatalf("cmdline search line 1 = %q, want to start with / my-project", trimmed)
	}
	if strings.Contains(lines[0], "ready") {
		t.Fatalf("cmdline line 1 should not contain status text: %q", lines[0])
	}

	// Rename mode: line 1 starts with the old name + " -> ".
	m.inputMode = modeRename
	m.renameFrom = "old-name"
	m.renameInput.SetValue("new-name")
	footer = sessionmgr.StripANSI(m.renderFooter())
	lines = strings.Split(footer, "\n")
	if !strings.Contains(lines[0], "old-name -> ") || !strings.Contains(lines[0], "new-name") {
		t.Fatalf("cmdline rename line 1 = %q, want old-name -> new-name", lines[0])
	}

	// No footer line should exceed the safe width.
	for i, line := range lines {
		if w := lipgloss.Width(line); w > safeWidth(m.width) {
			t.Fatalf("cmdline footer line %d width = %d, want at most %d", i, w, safeWidth(m.width))
		}
	}
}

func TestFooterPopupStyleStillComposesStatus(t *testing.T) {
	m := newTestModel(t)
	// Default (popup) config: small terminal falls back to the inline field on
	// the right of the status line, so line 1 holds both the status source info
	// and the search input.
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.width, m.height = 40, 4
	m.inputMode = modeSearch
	m.searchInput.SetValue("test")
	footer := sessionmgr.StripANSI(m.renderFooter())
	lines := strings.Split(footer, "\n")
	if !strings.Contains(lines[0], "all") || !strings.Contains(lines[0], "/ test") {
		t.Fatalf("popup small-terminal line 1 should hold status + search input: %q", lines[0])
	}
}
