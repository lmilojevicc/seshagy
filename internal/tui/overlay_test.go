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

func TestFooterCmdlineShowsTextInputInTile(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStyleCmdline
	m.width = 80
	m.height = 32

	// Search mode: the footer stacks the SEARCH input tile (3 lines) above the
	// HELP tile (3 lines). The textinput sits on the tile's content line.
	m.inputMode = modeSearch
	m.searchInput.SetValue("my-project")
	footer := sessionmgr.StripANSI(m.renderFooter())
	lines := strings.Split(footer, "\n")
	if len(lines) != 6 {
		t.Fatalf("footer lines = %d, want 6 (SEARCH tile + HELP tile)\n%s", len(lines), footer)
	}
	if !strings.Contains(lines[0], "SEARCH") {
		t.Fatalf("cmdline search top border missing SEARCH title: %q", lines[0])
	}
	if !strings.Contains(lines[1], "/ my-project") {
		t.Fatalf("cmdline search input = %q, want to contain / my-project", lines[1])
	}
	if strings.Contains(lines[1], "ready") {
		t.Fatalf("cmdline input line should not contain status text: %q", lines[1])
	}

	// Rename mode: content line starts with the old name + " -> ".
	m.inputMode = modeRename
	m.renameFrom = "old-name"
	m.renameInput.SetValue("new-name")
	footer = sessionmgr.StripANSI(m.renderFooter())
	lines = strings.Split(footer, "\n")
	if len(lines) != 6 {
		t.Fatalf(
			"rename footer lines = %d, want 6 (RENAME tile + HELP tile)\n%s",
			len(lines),
			footer,
		)
	}
	if !strings.Contains(lines[0], "RENAME") {
		t.Fatalf("cmdline rename top border missing RENAME title: %q", lines[0])
	}
	if !strings.Contains(lines[1], "old-name -> ") || !strings.Contains(lines[1], "new-name") {
		t.Fatalf("cmdline rename input = %q, want old-name -> new-name", lines[1])
	}

	// No footer line should exceed the safe width.
	for i, line := range lines {
		if w := lipgloss.Width(line); w > safeWidth(m.width) {
			t.Fatalf("cmdline footer line %d width = %d, want at most %d", i, w, safeWidth(m.width))
		}
	}
}

func TestCmdlineInputStyleHasFieldsetTitle(t *testing.T) {
	for _, tt := range []struct {
		name  string
		mode  inputMode
		title string
	}{
		{name: "search", mode: modeSearch, title: "SEARCH"},
		{name: "rename", mode: modeRename, title: "RENAME"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.config.TUI.InputStyle = appconfig.InputStyleCmdline
			m.width, m.height = 120, 32
			m.inputMode = tt.mode
			m.renameFrom = "old-name"
			m.searchInput.SetValue("proj")
			m.renameInput.SetValue("new-name")

			view := sessionmgr.StripANSI(m.View())
			// The input tile title sits on its fieldset top edge (╭─ TITLE ──╮).
			var edge string
			for _, line := range strings.Split(view, "\n") {
				if strings.HasPrefix(line, "╭─ ") && strings.Contains(line, tt.title) {
					edge = line
					break
				}
			}
			if edge == "" {
				t.Fatalf("cmdline view missing %q fieldset title edge\n%s", tt.title, view)
			}
			if !strings.HasSuffix(edge, "╮") {
				t.Fatalf("cmdline %q fieldset edge not closed: %q", tt.title, edge)
			}
			// The HELP tile must still render beneath the input tile.
			if !strings.Contains(view, "HELP") {
				t.Fatalf("cmdline view missing HELP tile\n%s", view)
			}
		})
	}
}

func TestFooterPopupStyleStillComposesStatus(t *testing.T) {
	m := newTestModel(t)
	// Default (popup) input style: the footer stays help-only. The search/rename
	// field renders as its own overlay, not in the footer, so even while searching
	// the footer has no status strip and no inline input.
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.width, m.height = 40, 4
	m.inputMode = modeSearch
	m.searchInput.SetValue("test")
	footer := sessionmgr.StripANSI(m.renderFooter())
	lines := strings.Split(footer, "\n")
	if len(lines) != 3 {
		t.Fatalf("popup footer should be a HELP tile (3 lines), got %d\n%s", len(lines), footer)
	}
	if strings.Contains(footer, "test") {
		t.Fatalf("popup footer must not embed the search input (it is an overlay)\n%s", footer)
	}
	if strings.Contains(footer, "✓") {
		t.Fatalf("popup footer must not show the backend indicator\n%s", footer)
	}
}
