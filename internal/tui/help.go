package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpKeyGap is the blank columns between a row's key hints and its label.
const helpKeyGap = 2

// helpColumnGap is the blank columns between the help overlay's two columns.
const helpColumnGap = 3

// helpBoxPad is the blank column the help box keeps inside each border.
const helpBoxPad = 1

// helpCloseHint is the help box's bottom-border hint, drawn after the scroll
// counter: the keys that take the box down. The help rows cannot name them — a
// close key is not a picker action.
const helpCloseHint = "f1/esc/q close"

// helpBlock is one group of the help overlay: its heading and its rows.
type helpBlock struct {
	group keyGroup
	rows  []helpRow
}

// helpRow is one row of a block: the key hints it names and the label after
// them, plus the filter state the row highlights. A row with no key draws its
// label across the column — the query example has no key to name.
type helpRow struct {
	key    string
	label  string
	value  string
	active bool
}

// text is the row's label as plain text, its value included: the text the
// column is sized from.
func (r helpRow) text() string {
	if r.value == "" {
		return r.label
	}
	return r.label + " · " + r.value
}

// helpWindow is the help overlay laid out for the current frame: the rows it
// draws, the box it draws them in, and the row its window opens on.
type helpWindow struct {
	lines  []string
	width  int
	height int
	top    int
}

// innerWidth and innerHeight are the columns and rows inside the box's border.
func (w helpWindow) innerWidth() int  { return max(0, w.width-2) }
func (w helpWindow) innerHeight() int { return max(0, w.height-2) }

// maxTop is the last row the window can open on.
func (w helpWindow) maxTop() int { return max(0, len(w.lines)-w.innerHeight()) }

// openHelp opens the help overlay on F1. The picker behind it keeps its query,
// filters, cursor and scroll.
func (m Model) openHelp() (tea.Model, tea.Cmd) {
	m.helpOpen = true
	m.helpTop = 0
	return m, nil
}

// handleHelpKey routes a key press while the help overlay is up: ↑/↓ move its
// window, F1, esc and q take it down, and every other key is swallowed, so
// nothing typed at the overlay ever reaches the picker behind it.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "f1", "esc", "q":
		m.helpOpen = false
		m.helpTop = 0
		return m, nil
	case "up":
		m.helpTop = max(0, m.helpTop-1)
		return m, nil
	case "down":
		m.helpTop = min(m.helpTop+1, m.helpWindow().maxTop())
		return m, nil
	}
	return m, nil
}

// helpBlocks is the help overlay's contents: the table's help rows, filed under
// the group headings in the order the overlay draws them. A binding the backend
// cannot honour is left out, so tmux and noop keep the overlay they always had.
func (m Model) helpBlocks() []helpBlock {
	blocks := make([]helpBlock, 0, len(keyGroups))
	for _, group := range keyGroups {
		var rows []helpRow
		for _, binding := range m.keys.helpBindings() {
			if binding.group != group || !m.showsBinding(binding) {
				continue
			}
			rows = append(rows, helpRow{
				key:    keyHint(binding.keys),
				label:  m.bindingLabel(binding),
				value:  bindingValue(m, binding),
				active: bindingActive(m, binding),
			})
		}
		if len(rows) > 0 {
			blocks = append(blocks, helpBlock{group: group, rows: rows})
		}
	}
	return blocks
}

// helpWindow lays the help overlay out for the frame. The groups draw in two
// columns while the pair fits the frame's width and in one column when it does
// not; the box is sized to the content and clamped to the frame the picker drew
// (safeWidth columns, so the box can never spill a line past it), and the window
// opens on the row the user scrolled to.
func (m Model) helpWindow() helpWindow {
	blocks := m.helpBlocks()
	split := helpSplit(blocks)
	left, right := blocks[:split], blocks[split:]
	leftKey, leftLabel := helpColumnMetrics(left)
	rightKey, rightLabel := helpColumnMetrics(right)
	twoColumns := len(right) > 0 &&
		helpBoxWidth(leftKey+leftLabel+helpColumnGap+rightKey+rightLabel) <= m.width

	var lines []string
	var contentWidth int
	if twoColumns {
		leftLines := m.renderHelpColumn(left, leftKey, leftKey+leftLabel)
		rightLines := m.renderHelpColumn(right, rightKey, rightKey+rightLabel)
		for i := range max(len(leftLines), len(rightLines)) {
			line := pad("", leftKey+leftLabel)
			if i < len(leftLines) {
				line = leftLines[i]
			}
			line += strings.Repeat(" ", helpColumnGap)
			if i < len(rightLines) {
				line += rightLines[i]
			}
			lines = append(lines, line)
		}
		contentWidth = leftKey + leftLabel + helpColumnGap + rightKey + rightLabel
	} else {
		// One column draws every block: the column is sized from all of them,
		// not from the blocks the split happened to leave on the left.
		key, label := helpColumnMetrics(blocks)
		lines = m.renderHelpColumn(blocks, key, key+label)
		contentWidth = key + label
	}

	w := helpWindow{
		lines:  lines,
		width:  min(helpBoxWidth(contentWidth), safeWidth(m.width)),
		height: min(len(lines)+2, max(0, m.height)),
	}
	w.top = clampVal(m.helpTop, 0, w.maxTop())
	return w
}

// helpBoxWidth is the box a content width draws in: a blank column inside each
// border, the borders themselves.
func helpBoxWidth(contentWidth int) int { return contentWidth + 2*helpBoxPad + 2 }

// helpSplit is the block index the second column starts at: the first column
// takes the blocks up to half the table's rows, so the two balance. It is
// len(blocks) when a second column would not balance them.
func helpSplit(blocks []helpBlock) int {
	if len(blocks) < 2 {
		return len(blocks)
	}
	half := helpBlocksHeight(blocks) / 2
	rows := 0
	for i, block := range blocks {
		if i > 0 {
			rows++
		}
		rows += 1 + len(block.rows)
		if rows >= half && i+1 < len(blocks) {
			return i + 1
		}
	}
	return len(blocks)
}

// helpBlocksHeight is the rows a column of blocks draws: a heading and its rows
// each, plus a blank row between blocks.
func helpBlocksHeight(blocks []helpBlock) int {
	rows := 0
	for i, block := range blocks {
		if i > 0 {
			rows++
		}
		rows += 1 + len(block.rows)
	}
	return rows
}

// helpColumnMetrics is the columns a column of blocks draws: the width its key
// hints take with the gap that follows them (zero when no row names a key), and
// the width of its widest heading or label.
func helpColumnMetrics(blocks []helpBlock) (keyWidth, labelWidth int) {
	for _, block := range blocks {
		labelWidth = max(labelWidth, lipgloss.Width(string(block.group)))
		for _, row := range block.rows {
			if row.key != "" {
				keyWidth = max(keyWidth, lipgloss.Width(row.key))
			}
			labelWidth = max(labelWidth, lipgloss.Width(row.text()))
		}
	}
	if keyWidth > 0 {
		keyWidth += helpKeyGap
	}
	return keyWidth, labelWidth
}

// renderHelpColumn draws a column of blocks: each heading, its rows, and a blank
// row between blocks, every line padded to the column's width so the columns of
// a two-column overlay line up.
func (m Model) renderHelpColumn(blocks []helpBlock, keyWidth, width int) []string {
	lines := make([]string, 0, helpBlocksHeight(blocks))
	active := make([]bool, 0, helpBlocksHeight(blocks))
	for i, block := range blocks {
		if i > 0 {
			lines = append(lines, "")
			active = append(active, false)
		}
		lines = append(lines, m.styles.emphasis.Render(string(block.group)))
		active = append(active, false)
		for _, row := range block.rows {
			lines = append(lines, m.renderHelpRow(row, keyWidth))
			active = append(active, row.active)
		}
	}
	for i, line := range lines {
		lines[i] = pad(line, width)
		if active[i] {
			lines[i] = paintBackground(lines[i], width, m.styles.selectionBG)
		}
	}
	return lines
}

// renderHelpRow draws one row: the key hints in the key style, the label muted,
// and the state the row filters on after it.
func (m Model) renderHelpRow(row helpRow, keyWidth int) string {
	label := m.styles.muted.Render(row.label)
	if row.value != "" {
		valueStyle := m.styles.muted
		if row.active {
			valueStyle = m.styles.emphasis
		}
		label += m.styles.muted.Render(" · ") + valueStyle.Render(row.value)
	}
	if row.key == "" {
		return label
	}
	return pad(m.styles.key.Render(row.key), keyWidth) + label
}

// renderHelpOverlay floats the help box over the picker frame, centered and
// clamped to it. The box is drawn on the rendered frame, so its own styling is
// not nested inside the frame's.
func (m Model) renderHelpOverlay(frame string) string {
	w := m.helpWindow()
	inner := max(0, w.innerWidth()-2*helpBoxPad)
	visible := w.lines[w.top:min(len(w.lines), w.top+w.innerHeight())]
	lines := make([]string, 0, len(visible))
	for _, line := range visible {
		lines = append(lines, " "+pad(line, inner)+" ")
	}
	bottom := helpScrollCounter(w)
	if bottom != "" {
		bottom += " · "
	}
	bottom += helpCloseHint
	box := strings.Join(
		drawWindow(
			m.styles.title.Render("help"),
			lines,
			w.width,
			w.height,
			m.styles.border,
			m.styles.muted.Render(bottom),
		),
		"\n",
	)
	return overlay(frame, box, (m.width-w.width)/2, (m.height-w.height)/2)
}

// helpScrollCounter spells the help rows the box hides above and below its
// window, for the box's bottom border.
func helpScrollCounter(w helpWindow) string {
	above := w.top
	below := max(0, len(w.lines)-(w.top+w.innerHeight()))
	return resultsScrollCounter(above, below)
}
