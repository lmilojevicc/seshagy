package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// helpModel is a loaded picker at a frame size, with a query and filters the
// help overlay must leave untouched behind it.
func helpModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := newTestModel(t)
	m.width, m.height = width, height
	m.loadedOnce = true
	m = withItems(m, pickerItems())
	m.query = "fix"
	m.scope = scopeAgents
	m.stateFilter = stateFilterBlocked
	return m
}

// openHelp presses F1 and fails when the overlay does not open.
func openHelp(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, "f1")
	if !m.helpOpen {
		t.Fatal("F1 did not open the help overlay")
	}
	if m.helpTop != 0 {
		t.Fatalf("the overlay opened on row %d, want the top", m.helpTop)
	}
	return m
}

// helpBox is the rendered help box's position in the frame — the row its top
// border draws on, its left and right border columns — plus its lines. The
// frame itself is checked to be exactly the frame's width and height.
func helpBox(t *testing.T, m Model) (top, left, right int, box []string) {
	t.Helper()
	rows := frameLines(t, m)
	w := m.helpWindow()
	for i, row := range rows {
		title := strings.Index(row, " help ")
		if title < 0 {
			continue
		}
		corner := strings.LastIndex(row[:title], "╭")
		end := strings.Index(row[title:], "╮")
		if corner < 0 || end < 0 {
			continue
		}
		if i+w.height > len(rows) {
			t.Fatalf(
				"the help box runs past the frame: row %d, height %d, %d rows",
				i,
				w.height,
				len(rows),
			)
		}
		return i,
			ansi.StringWidth(row[:corner]),
			ansi.StringWidth(row[:title+end]),
			rows[i : i+w.height]
	}
	t.Fatalf("no help box in the frame:\n%s", strings.Join(rows, "\n"))
	return 0, 0, 0, nil
}

// hasHelpBox reports whether a frame draws the help box: a title row whose
// "help" sits inside a box border. The footer's "F1 help" hint is not one.
func hasHelpBox(frame string) bool {
	for _, row := range strings.Split(frame, "\n") {
		title := strings.Index(row, " help ")
		if title >= 0 && strings.Contains(row[:title], "╭") {
			return true
		}
	}
	return false
}

// helpTextColumn is the frame column one of the box's lines starts a text run
// at, for asserting which column a group or a row label is drawn in.
func helpTextColumn(t *testing.T, box []string, text string) int {
	t.Helper()
	for _, row := range box {
		if at := strings.Index(row, text); at >= 0 {
			return ansi.StringWidth(row[:at])
		}
	}
	t.Fatalf("the help box draws no %q:\n%s", text, strings.Join(box, "\n"))
	return 0
}

// pickerSnapshot is the picker state the help overlay must not touch: the query
// and filters it sits over, and the selection and scroll behind it.
type pickerSnapshot struct {
	query       string
	scope       scope
	stateFilter stateFilter
	cursor      int
	cursorKey   string
	cursorMoved bool
	listTop     int
	showPreview bool
	items       int
	helpOpen    bool
	helpTop     int
}

func snapshotPicker(m Model) pickerSnapshot {
	return pickerSnapshot{
		query:       m.query,
		scope:       m.scope,
		stateFilter: m.stateFilter,
		cursor:      m.cursor,
		cursorKey:   m.cursorKey,
		cursorMoved: m.cursorMoved,
		listTop:     m.listTop,
		showPreview: m.showPreview,
		items:       len(m.items),
		helpOpen:    m.helpOpen,
		helpTop:     m.helpTop,
	}
}

// TestHelpOpensOnF1AndClosesOnF1EscAndQ pins the close chords: F1 toggles the
// overlay, esc and q take it down, and a close leaves the picker exactly as it
// was behind the box.
func TestHelpOpensOnF1AndClosesOnF1EscAndQ(t *testing.T) {
	for _, close := range []string{"f1", "esc", "q"} {
		t.Run(close, func(t *testing.T) {
			m := helpModel(t, 120, 34)
			before := snapshotPicker(m)

			m = openHelp(t, m)
			if !hasHelpBox(sessionmgr.StripANSI(m.View())) {
				t.Fatalf("F1 did not draw the help box:\n%s", sessionmgr.StripANSI(m.View()))
			}

			closed, cmd := press(t, m, close)
			if closed.helpOpen {
				t.Fatalf("%s left the help overlay open", close)
			}
			if cmd != nil {
				t.Fatalf("%s closed the help with a command: %v", close, cmd)
			}
			if got := snapshotPicker(closed); got != before {
				t.Fatalf("%s changed the picker: %+v, want %+v", close, got, before)
			}
			if frame := sessionmgr.StripANSI(closed.View()); hasHelpBox(frame) ||
				!strings.Contains(frame, "F1 help") {
				t.Fatalf("%s did not restore the picker frame:\n%s", close, frame)
			}
		})
	}
}

// TestHelpSwallowsEveryOtherKey pins the overlay's swallowing: no picker key
// reaches the query, the filters, the cursor, or the scroll behind the box.
func TestHelpSwallowsEveryOtherKey(t *testing.T) {
	m := openHelp(t, helpModel(t, 120, 34))
	before := snapshotPicker(m)

	for _, key := range []string{
		"z", "space", "ctrl+a", "ctrl+s", "ctrl+t", "ctrl+z", "ctrl+f", "ctrl+e",
		"ctrl+g", "ctrl+l", "ctrl+u", "ctrl+w", "ctrl+o", "ctrl+y", "ctrl+r",
		"ctrl+x", "enter", "backspace", "delete", "tab", "shift+tab",
		"pgup", "pgdown", "home", "end", "alt+m",
	} {
		got, cmd := press(t, m, key)
		if !got.helpOpen {
			t.Fatalf("%q closed the help overlay", key)
		}
		if cmd != nil {
			t.Fatalf("%q sent a command through the overlay: %v", key, cmd)
		}
		if got.helpTop != 0 {
			t.Fatalf("%q scrolled the help window to %d", key, got.helpTop)
		}
		if snap := snapshotPicker(got); snap != before {
			t.Fatalf("%q reached the picker behind the help: %+v, want %+v", key, snap, before)
		}
		m = got
	}
}

// TestHelpBoxDrawsTheTableRowsTheFooterDoesNot pins the help box's contents
// against the one table: every row the table gives the overlay — each action's
// remaining keys, the rows the footer draws none of, and the query-token rows —
// in the table's row order within its group, and nothing the footer draws.
func TestHelpBoxDrawsTheTableRowsTheFooterDoesNot(t *testing.T) {
	m := helpModel(t, 120, 34)
	_, _, _, box := helpBox(t, openHelp(t, m))
	rendered := strings.Join(box, "\n")

	rows := 0
	for _, block := range m.helpBlocks() {
		if len(block.rows) == 0 {
			t.Fatalf("the box drew the empty group %q", block.group)
		}
		want := make([]keyBinding, 0, len(block.rows))
		for _, binding := range m.keys.helpBindings() {
			if binding.group == block.group && m.showsBinding(binding) {
				want = append(want, binding)
			}
		}
		if len(block.rows) != len(want) {
			t.Fatalf("group %q drew %d rows, want %d", block.group, len(block.rows), len(want))
		}
		for i, binding := range want {
			row := block.rows[i]
			rows++
			if row.key != keyHint(binding.keys) {
				t.Fatalf("row %d hints %q, want %q", i, row.key, keyHint(binding.keys))
			}
			if row.label != m.bindingLabel(binding) {
				t.Fatalf("row %d labels %q, want %q", i, row.label, m.bindingLabel(binding))
			}
			if row.value != bindingValue(m, binding) {
				t.Fatalf("row %d value %q, want %q", i, row.value, bindingValue(m, binding))
			}
			if row.active != bindingActive(m, binding) {
				t.Fatalf("row %d active = %v, want %v", i, row.active, bindingActive(m, binding))
			}
			if !strings.Contains(rendered, row.text()) {
				t.Fatalf("the box does not draw the row %q:\n%s", row.text(), rendered)
			}
		}
	}
	want := 0
	for _, binding := range m.keys.helpBindings() {
		if m.showsBinding(binding) {
			want++
		}
	}
	if rows != want {
		t.Fatalf("the box drew %d rows, want the table's %d", rows, want)
	}
	for _, token := range []string{"!name", "@ws", "#state", "/path", "fix @seshagy #idle"} {
		if !strings.Contains(rendered, token) {
			t.Fatalf("the box does not draw the token row %q:\n%s", token, rendered)
		}
	}
	for _, binding := range m.keys.footerBindings() {
		if binding.alsoHelp || !m.showsBinding(binding) {
			// The machine hint is deliberately named in the box too: the footer
			// cannot say which machine is selected, the help row can.
			continue
		}
		hint := keyHint(binding.keys) + " " + m.bindingLabel(binding)
		if strings.Contains(rendered, hint) {
			t.Fatalf("the box repeats the footer hint %q:\n%s", hint, rendered)
		}
	}
}

// TestHelpBoxIsCenteredAndClampedToTheseFrames pins the overlay's geometry at
// the sized frames: the frame keeps its exact width and height, the box is
// centered inside it, and its bottom border names the close chords.
func TestHelpBoxIsCenteredAndClampedToTheseFrames(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 34}, {80, 24}, {60, 20}} {
		t.Run(
			fmt.Sprintf("%dx%d", size.width, size.height),
			func(t *testing.T) {
				m := openHelp(t, helpModel(t, size.width, size.height))
				w := m.helpWindow()
				top, left, right, box := helpBox(t, m)

				if w.width > m.width || w.height > m.height {
					t.Fatalf("the box is %dx%d, larger than the frame", w.width, w.height)
				}
				if want := (m.width - w.width) / 2; left != want {
					t.Fatalf("the box starts at column %d, want %d", left, want)
				}
				if got := right - left + 1; got != w.width {
					t.Fatalf("the box is %d columns wide, want %d", got, w.width)
				}
				if want := (m.height - w.height) / 2; top != want {
					t.Fatalf("the box starts on row %d, want %d", top, want)
				}
				// The box is sized to its content: its widest line is exactly the
				// width inside the border, and no line overflows it.
				inner := w.innerWidth() - 2*helpBoxPad
				widest := 0
				for i, line := range w.lines {
					got := ansi.StringWidth(line)
					if got > inner {
						t.Fatalf(
							"box line %d is %d columns, wider than the %d inside the border: %q",
							i,
							got,
							inner,
							line,
						)
					}
					widest = max(widest, got)
				}
				if widest != inner {
					t.Fatalf(
						"the box's widest line is %d columns, want the %d inside its border",
						widest,
						inner,
					)
				}
				if bottom := box[len(box)-1]; !strings.Contains(bottom, helpCloseHint) {
					t.Fatalf("the box's bottom border does not name the close keys: %q", bottom)
				}
			},
		)
	}
}

// TestHelpBoxNeverSpillsPastTheFrame pins the box's clamp: the frame keeps its
// exact width and height with the help open at every size, including the widths
// where the box has to give up columns to fit inside it.
func TestHelpBoxNeverSpillsPastTheFrame(t *testing.T) {
	for _, height := range []int{4, 8, 20, 24, 34} {
		for _, width := range []int{1, 2, 4, 5, 40, 70, 71, 72, 73, 74, 75, 80, 119, 120} {
			m := openHelp(t, helpModel(t, width, height))
			frameLines(t, m)
			w := m.helpWindow()
			if w.width > m.viewLayout().width || w.height > m.height {
				t.Fatalf(
					"%dx%d: the box is %dx%d, larger than the frame",
					width,
					height,
					w.width,
					w.height,
				)
			}
		}
	}
}

// TestHelpBoxSwitchesFromTwoColumnsToTheNarrowOne pins the layout switch: a
// wide frame draws two columns, a narrow one the table's single column.
func TestHelpBoxSwitchesFromTwoColumnsToTheNarrowOne(t *testing.T) {
	for _, tt := range []struct {
		width, height int
		columns       int
	}{{120, 34, 2}, {80, 24, 2}, {60, 20, 1}} {
		t.Run(fmt.Sprintf("%dx%d", tt.width, tt.height), func(t *testing.T) {
			m := openHelp(t, helpModel(t, tt.width, tt.height))
			_, _, _, box := helpBox(t, m)

			// Two columns draw two group headings on one row; one column never
			// does.
			columns := 1
			for _, row := range box {
				headings := 0
				for _, group := range keyGroups {
					if strings.Contains(row, string(group)) {
						headings++
					}
				}
				if headings > 1 {
					columns = 2
				}
			}
			if columns != tt.columns {
				t.Fatalf(
					"the box drew %d columns, want %d:\n%s",
					columns,
					tt.columns,
					strings.Join(box, "\n"),
				)
			}

			if tt.columns == 2 {
				filters := helpTextColumn(t, box, string(groupFilters))
				navigation := helpTextColumn(t, box, string(groupNavigation))
				if navigation <= filters {
					t.Fatalf("the second column starts at %d, not after %d", navigation, filters)
				}
				return
			}
			// One column: every group heading starts at the box's inner left
			// edge, and no second column follows a row.
			want := helpTextColumn(t, box, string(groupFilters))
			if got := helpTextColumn(t, box, string(groupQueryTokens)); got != want {
				t.Fatalf("the second group starts at %d, want %d", got, want)
			}
		})
	}
}

// TestHelpBoxScrollsWhenTallerThanTheFrame pins the clamped box: at 60x20 the
// table is taller than the frame, so the box draws a window over it, ↓ and ↑
// move that window, and the bottom border counts what it hides.
func TestHelpBoxScrollsWhenTallerThanTheFrame(t *testing.T) {
	m := openHelp(t, helpModel(t, 60, 20))
	w := m.helpWindow()
	if w.height != 20 {
		t.Fatalf("box height = %d, want the whole frame's 20 rows", w.height)
	}
	if len(w.lines) <= w.innerHeight() {
		t.Fatalf(
			"the table has %d rows, want more than the %d the box draws",
			len(w.lines),
			w.innerHeight(),
		)
	}
	_, _, _, box := helpBox(t, m)
	if want := fmt.Sprintf(
		"↓ %d more",
		len(w.lines)-w.innerHeight(),
	); !strings.Contains(
		box[len(box)-1],
		want,
	) {
		t.Fatalf(
			"the box's bottom border does not count the rows below with %q: %q",
			want,
			box[len(box)-1],
		)
	}
	if strings.Contains(strings.Join(box, "\n"), "refresh") {
		t.Fatal("the box drew a row past its window")
	}

	m, _ = press(t, m, "down")
	if m.helpTop != 1 {
		t.Fatalf("↓ moved the window to row %d, want 1", m.helpTop)
	}
	for range w.maxTop() {
		m, _ = press(t, m, "down")
	}
	if m.helpTop != w.maxTop() {
		t.Fatalf("↓ moved the window to row %d, want its last row %d", m.helpTop, w.maxTop())
	}
	_, _, _, box = helpBox(t, m)
	if !strings.Contains(strings.Join(box, "\n"), "refresh") {
		t.Fatal("the box's last window does not draw the Actions rows")
	}
	if want := fmt.Sprintf("↑ %d more", w.maxTop()); !strings.Contains(box[len(box)-1], want) {
		t.Fatalf(
			"the box's bottom border does not count the rows above with %q: %q",
			want,
			box[len(box)-1],
		)
	}

	m, _ = press(t, m, "up")
	if m.helpTop != w.maxTop()-1 {
		t.Fatalf("↑ moved the window to row %d, want %d", m.helpTop, w.maxTop()-1)
	}
}

// TestHelpBoxHighlightsTheActiveFilterAndNamesTheState pins the two pieces of
// live picker state the box shows: the filter in effect is highlighted, and the
// agent-state row names the state ctrl+s is filtering on.
func TestHelpBoxHighlightsTheActiveFilterAndNamesTheState(t *testing.T) {
	withColor(t)
	m := openHelp(t, helpModel(t, 120, 34))

	for _, filter := range []struct {
		key    string
		active bool
		value  string
	}{
		{"ctrl+a", true, ""},
		{"ctrl+s", true, "blocked"},
		{"ctrl+z", false, ""},
	} {
		row := helpRowFor(t, m, filter.key)
		if row.active != filter.active || row.value != filter.value {
			t.Fatalf(
				"the %s row = %+v, want active %v and value %q",
				filter.key,
				row,
				filter.active,
				filter.value,
			)
		}
	}

	// With no filter in effect nothing is highlighted, and the agent-state row
	// names the absent state as none.
	plain := helpModel(t, 120, 34)
	plain.scope = scopeAll
	plain.stateFilter = stateFilterNone
	plain = openHelp(t, plain)
	if row := helpRowFor(t, plain, "ctrl+a"); row.active {
		t.Fatal("the agents row is highlighted while every filter is off")
	}
	if row := helpRowFor(t, plain, "ctrl+s"); row.active || row.value != "none" {
		t.Fatalf("the agent-state row = %+v, want an inactive none", row)
	}

	// The highlighted row is drawn with the selection bar behind it; the rows
	// whose filter is off are not. The bar is read from the box's own still-styled
	// lines, which the frame's stripped lines cannot show.
	bar := backgroundOpen(m.styles.selectionBG)
	if bar == "" {
		t.Fatal("the selection color renders no background")
	}
	lines := m.helpWindow().lines
	for _, draw := range []struct {
		text string
		bar  bool
	}{
		{"agents", true},
		{"agent state", true},
		{"zoxide", false},
		{"refresh", false},
	} {
		row := helpLineWith(t, lines, draw.text)
		if got := strings.Contains(row, bar); got != draw.bar {
			t.Fatalf("the %q row bar = %v, want %v: %q", draw.text, got, draw.bar, row)
		}
	}
}

// helpRowFor is the help box's row for one key, read through the same table the
// box draws its rows from.
func helpRowFor(t *testing.T, m Model, key string) helpRow {
	t.Helper()
	for _, binding := range m.keys.helpBindings() {
		names := false
		for _, candidate := range binding.keys {
			names = names || candidate == key
		}
		if !names {
			continue
		}
		for _, block := range m.helpBlocks() {
			for _, row := range block.rows {
				if row.key == keyHint(binding.keys) {
					return row
				}
			}
		}
	}
	t.Fatalf("the box has no row for %q", key)
	return helpRow{}
}

// helpLineWith is the box's line carrying a text run, still styled.
func helpLineWith(t *testing.T, lines []string, text string) string {
	t.Helper()
	for _, row := range lines {
		if strings.Contains(row, text) {
			return row
		}
	}
	t.Fatalf("the box draws no %q:\n%s", text, strings.Join(lines, "\n"))
	return ""
}
