package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestResultsWindowScrollsOnlyWhenTheCursorLeavesIt walks the cursor down and
// back up a grouped list of one- and two-line items. The window keeps its top
// line until the cursor's item would leave it and then moves just far enough,
// so it never jumps back and forth between key presses (re-centring every
// frame made two-line items scroll by uneven amounts and looked like flicker).
func TestResultsWindowScrollsOnlyWhenTheCursorLeavesIt(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 14
	m.showPreview = false
	m = withItems(m, append(groupedItems(), mixedHeightItems()...))
	m.query = ""

	rows := m.rows()
	if len(rows) < 6 {
		t.Fatalf("fixture has %d rows, want enough to scroll", len(rows))
	}
	flow := newItemFlow(rows, m.grouped(rows))
	height := m.bodyHeight()
	if len(flow.lines) <= height {
		t.Fatalf(
			"flow has %d lines in a %d line window, want it to scroll",
			len(flow.lines),
			height,
		)
	}

	step := func(msg tea.KeyMsg) {
		t.Helper()
		before := m.listTop
		next, _ := m.Update(msg)
		m = next.(Model)
		top := m.listTop
		start, end := flow.firsts[m.cursor], flow.itemEnd(m.cursor)
		if start < top || end > top+height {
			t.Fatalf("cursor item %d (lines %d-%d) outside the window [%d, %d)",
				m.cursor, start, end, top, top+height)
		}
		if flow.lines[top].detail {
			t.Fatalf("window opens on the continuation line %d of an item", top)
		}
		// The window only moves when the cursor's item would leave the window it
		// had before the key press.
		if start >= before && end <= before+height && top != before {
			t.Fatalf("window moved %d -> %d although item %d was visible", before, top, m.cursor)
		}
		switch msg.Type {
		case tea.KeyDown:
			if top < before {
				t.Fatalf("moving down scrolled the window up: %d -> %d", before, top)
			}
		case tea.KeyUp:
			if top > before {
				t.Fatalf("moving up scrolled the window down: %d -> %d", before, top)
			}
		}
	}

	for range len(rows) - 1 {
		step(downMsg())
	}
	if m.cursor != len(rows)-1 {
		t.Fatalf("cursor = %d after walking down, want %d", m.cursor, len(rows)-1)
	}
	for range len(rows) - 1 {
		step(upMsg())
	}
	if m.cursor != 0 || m.listTop != 0 {
		t.Fatalf("after walking back up cursor = %d, top = %d, want 0, 0", m.cursor, m.listTop)
	}
}

// TestItemFlowFollowBringsTheSectionHeaderAlong scrolls up onto the first item
// of a section and expects its header to come into view with it.
func TestItemFlowFollowBringsTheSectionHeaderAlong(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, groupedItems())
	rows := m.rows()
	flow := newItemFlow(rows, m.grouped(rows))
	for item := range flow.items {
		first := flow.firsts[item]
		if first == 0 || flow.lines[first-1].item >= 0 {
			continue
		}
		top := flow.follow(len(flow.lines), item, flow.heights[item]+1)
		if top != first-1 {
			t.Fatalf(
				"follow opened on line %d for item %d, want its header line %d",
				top,
				item,
				first-1,
			)
		}
	}
}
