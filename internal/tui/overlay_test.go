package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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

func TestInputLinePreservesPlaceholderUnicodeScrollingAndCursorBlink(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prevProfile) })

	m := newTestModel(t)
	m.inputMode = modeSearch

	placeholder := sessionmgr.StripANSI(m.renderInputLine(40))
	if !strings.Contains(placeholder, "/ filter sessions, directories") {
		t.Fatalf("search placeholder changed: %q", placeholder)
	}

	m.searchInput.SetValue("αβ你好omega")
	m.searchInput.SetCursor(4)
	m.searchInput.Focus()
	unicodeRaw := m.renderInputLine(40)
	unicodeLine := sessionmgr.StripANSI(unicodeRaw)
	if !strings.Contains(unicodeLine, "αβ你好omega") {
		t.Fatalf("Unicode input changed: %q", unicodeLine)
	}
	if !strings.Contains(unicodeRaw, "\x1b[7mo\x1b[0m") {
		t.Fatalf("Unicode cursor is not rendered on rune at position 4: %q", unicodeRaw)
	}
	if width := lipgloss.Width(unicodeRaw); width > 40 {
		t.Fatalf("Unicode input width = %d, want at most 40: %q", width, unicodeLine)
	}

	model, _ := m.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	m = model.(Model)
	m.inputMode = modeSearch
	m.searchInput.SetValue("prefix-" + strings.Repeat("x", 40) + "-tail")
	m.searchInput.CursorEnd()
	m.searchInput.Focus()
	longLine := sessionmgr.StripANSI(m.renderInputLine(18))
	if want := "/ xxxxxxxxxxxxxxx…"; longLine != want {
		t.Fatalf("long input viewport = %q, want current clipped output %q", longLine, want)
	}
	if width := lipgloss.Width(longLine); width > 18 {
		t.Fatalf("long input width = %d, want at most 18: %q", width, longLine)
	}

	m.searchInput.SetValue("abc")
	m.searchInput.SetCursor(1)
	m.searchInput.Cursor.Blink = false
	visibleCursor := m.renderInputLine(20)
	m.searchInput.Cursor.Blink = true
	hiddenCursor := m.renderInputLine(20)
	if visibleCursor == hiddenCursor {
		t.Fatalf("cursor blink states rendered identically: %q", visibleCursor)
	}
	if sessionmgr.StripANSI(visibleCursor) != sessionmgr.StripANSI(hiddenCursor) {
		t.Fatalf(
			"cursor blink changed input text: visible=%q hidden=%q",
			sessionmgr.StripANSI(visibleCursor),
			sessionmgr.StripANSI(hiddenCursor),
		)
	}
	if !strings.Contains(visibleCursor, "\x1b[7m") || strings.Contains(hiddenCursor, "\x1b[7m") {
		t.Fatalf(
			"cursor reverse-video state changed: visible=%q hidden=%q",
			visibleCursor,
			hiddenCursor,
		)
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

func TestPopupStyleRendersInlineInputWhenSuppressedByNarrowWidth(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.width, m.height = 24, 12
	m.inputMode = modeSearch
	m.searchInput.SetValue("needle")

	view := sessionmgr.StripANSI(m.View())
	if !strings.Contains(view, "SEARCH") || !strings.Contains(view, "needle") {
		t.Fatalf("narrow popup-style view missing inline SEARCH input\n%s", view)
	}
}

func TestPopupStyleRendersInlineRenameWhenSuppressedByNarrowWidth(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.width, m.height = 24, 12
	m.inputMode = modeRename
	m.renameFrom = "old"
	m.renameInput.SetValue("new-name")

	view := sessionmgr.StripANSI(m.View())
	if !strings.Contains(view, "RENAME") || !strings.Contains(view, "new-name") {
		t.Fatalf("narrow popup-style view missing inline RENAME input\n%s", view)
	}
}

func TestPopupStyleRendersInlineInputWhenSuppressedByShortHeight(t *testing.T) {
	for _, tt := range []struct {
		name  string
		mode  inputMode
		title string
		value string
	}{
		{name: "search", mode: modeSearch, title: "SEARCH", value: "short-query"},
		{name: "rename", mode: modeRename, title: "RENAME", value: "short-name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.config.TUI.InputStyle = appconfig.InputStylePopup
			m.width, m.height = 80, 4
			m.inputMode = tt.mode
			m.renameFrom = "old"
			m.searchInput.SetValue(tt.value)
			m.renameInput.SetValue(tt.value)

			view := sessionmgr.StripANSI(m.View())
			if !strings.Contains(view, tt.title) || !strings.Contains(view, tt.value) {
				t.Fatalf("short popup-style view missing inline %s input\n%s", tt.title, view)
			}
			if strings.Contains(view, "HELP") {
				t.Fatalf("short popup-style view should prioritize input over HELP\n%s", view)
			}
		})
	}
}

func TestViewPreservesInlineInputAtHeightBoundaries(t *testing.T) {
	for _, style := range []struct {
		name  string
		value string
		width int
	}{
		{name: "cmdline", value: appconfig.InputStyleCmdline, width: 80},
		{name: "narrow-popup", value: appconfig.InputStylePopup, width: 24},
	} {
		for _, mode := range []struct {
			name  string
			value inputMode
		}{
			{name: "search", value: modeSearch},
			{name: "rename", value: modeRename},
		} {
			for _, height := range []int{1, 4, 5, 8, 9, 10} {
				t.Run(fmt.Sprintf("%s/%s/%d", style.name, mode.name, height), func(t *testing.T) {
					m := newTestModel(t)
					m.config.TUI.InputStyle = style.value
					m.width, m.height = style.width, height
					m.inputMode = mode.value
					m.renameFrom = "old"
					m.searchInput.SetValue("typed-value")
					m.renameInput.SetValue("typed-value")

					view := m.View()
					clean := sessionmgr.StripANSI(view)
					if height == 1 {
						want := m.renderInputLine(safeWidth(m.width))
						if view != want {
							t.Fatalf(
								"height-one view differs from input line: got=%q want=%q",
								sessionmgr.StripANSI(view),
								sessionmgr.StripANSI(want),
							)
						}
					}
					if !strings.Contains(clean, "typed-value") {
						t.Fatalf("typed value missing at height %d\n%s", height, clean)
					}
					if got := lipgloss.Height(view); got > height {
						t.Fatalf("view height = %d, terminal height = %d\n%s", got, height, clean)
					}
				})
			}
		}
	}
}

func TestRenamePopupDoesNotWrapAtNarrowWidth(t *testing.T) {
	m := newTestModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 34, Height: 20})
	m = model.(Model)
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.inputMode = modeRename
	m.renameFrom = "old-name"
	m.renameInput.SetValue("new-name")

	popup := sessionmgr.StripANSI(m.renderInputPopup())
	if got := lipgloss.Height(popup); got != 4 {
		t.Fatalf("rename popup height = %d, want 4 (2 content rows)\n%s", got, popup)
	}
	if !strings.Contains(popup, "old-name -> new-name") {
		t.Fatalf("rename popup split prompt and value across rows\n%s", popup)
	}
}

func TestPopupStyleDoesNotRenderDuplicateInlineInputAtPopupSize(t *testing.T) {
	m := newTestModel(t)
	m.config.TUI.InputStyle = appconfig.InputStylePopup
	m.width, m.height = 80, 20
	m.inputMode = modeSearch
	m.searchInput.SetValue("popup-query")

	footer := sessionmgr.StripANSI(m.renderFooter())
	if strings.Contains(footer, "SEARCH") || strings.Contains(footer, "popup-query") {
		t.Fatalf("popup-capable footer must not duplicate the SEARCH input\n%s", footer)
	}
	view := sessionmgr.StripANSI(m.View())
	if !strings.Contains(view, "SEARCH") || !strings.Contains(view, "popup-query") {
		t.Fatalf("popup-capable view missing the SEARCH overlay\n%s", view)
	}
}
