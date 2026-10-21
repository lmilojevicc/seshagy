package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

const (
	// horizontalMinWidth is the narrowest frame the horizontal layout keeps a
	// list column and a preview column side by side; a narrower frame stacks.
	horizontalMinWidth = 100
	// previewPercent is the preview column's share of a horizontal frame width.
	previewPercent = 45
	// previewMinCols is the narrowest usable preview column.
	previewMinCols = 28
	// promptWindowRows is the prompt window's height: top border, content row,
	// bottom border.
	promptWindowRows = 3
	// promptInset is the blank column the prompt content row keeps inside the
	// window border, matching the inset the results rows carry.
	promptInset = 1
	// resultsPercent is the stacked layouts' share of the rows left after the
	// prompt window; the preview window takes the rest.
	resultsPercent = 45
	// minResultsRows is the shortest results window that still shows a row.
	minResultsRows = 3
	// footerMinHeight is the shortest frame that keeps the footer key line.
	footerMinHeight = 6
	// titleMinWidth is the room a title keeps before its tab suffix is dropped.
	titleMinWidth = 12
	// itemNameFloor is the least room an item's name keeps before the badge
	// beside it is dropped, and itemBadgeGap the blank run that separates them.
	// A session's attached flag sits one column closer: it reads as part of the
	// name rather than as a state beside it.
	itemNameFloor   = 1
	itemBadgeGap    = 2
	sessionBadgeGap = 1
	// previewInset is the blank column the preview content starts after.
	previewInset = 1
	// hostIcon marks remote-machine rows.
	hostIcon = "󰒋 "

	placeholderText = "type to search"
)

// windowBox is one window of the frame: the cell it starts at and its size.
type windowBox struct {
	x, y          int
	width, height int
}

// viewLayout is one frame's window budget: the prompt, results, and preview
// windows plus the footer key line. It is resolved — the sub-horizontalMinWidth
// fallback is applied and the preview window is empty when the preview is off.
type viewLayout struct {
	width  int
	height int
	// layout is the effective layout: horizontal, vertical, or dropdown.
	layout string
	footer bool

	prompt  windowBox
	results windowBox
	preview windowBox
}

// listWidth is the results window's inner width: the columns its rows draw in.
func (l viewLayout) listWidth() int { return max(0, l.results.width-2) }

// listHeight is the results window's inner height: the lines it can show.
func (l viewLayout) listHeight() int { return max(0, l.results.height-2) }

// computeViewLayout splits a frame into the three telescope windows. The
// horizontal layout puts the prompt window over the results window in a left
// column and runs the preview down the right column; the vertical and dropdown
// layouts stack the three full width. The preview window stays empty when the
// preview is off, and the left column or the results window then take the room.
func computeViewLayout(width, height int, layout string, preview bool) viewLayout {
	l := viewLayout{
		width:  max(0, width),
		height: max(0, height),
		layout: effectiveLayout(layout, width),
	}
	if l.width == 0 || l.height == 0 {
		return l
	}
	l.footer = l.height >= footerMinHeight
	rows := l.height
	if l.footer {
		rows--
	}
	promptRows := min(promptWindowRows, rows)
	switch l.layout {
	case appconfig.LayoutVertical, appconfig.LayoutDropdown:
		rest := rows - promptRows
		l.prompt = windowBox{x: 0, y: 0, width: l.width, height: promptRows}
		l.results = windowBox{x: 0, y: promptRows, width: l.width, height: rest}
		if preview && rest > 0 {
			resultsRows := min(rest, max(minResultsRows, rest*resultsPercent/100))
			l.results.height = resultsRows
			l.preview = windowBox{
				x:      0,
				y:      promptRows + resultsRows,
				width:  l.width,
				height: rest - resultsRows,
			}
		}
	default:
		left := l.width
		if preview {
			previewCols := min(l.width, max(previewMinCols, l.width*previewPercent/100))
			left = max(1, l.width-previewCols-windowGap)
			l.preview = windowBox{
				x:      left + windowGap,
				y:      0,
				width:  l.width - left - windowGap,
				height: rows,
			}
		}
		l.prompt = windowBox{x: 0, y: 0, width: left, height: promptRows}
		l.results = windowBox{x: 0, y: promptRows, width: left, height: rows - promptRows}
	}
	return l
}

// effectiveLayout resolves a configured layout against the frame width: the
// horizontal split needs horizontalMinWidth columns, so a narrower frame stacks
// vertically instead. Unknown tokens are horizontal.
func effectiveLayout(layout string, width int) string {
	switch layout {
	case appconfig.LayoutVertical, appconfig.LayoutDropdown:
		return layout
	default:
		if width < horizontalMinWidth {
			return appconfig.LayoutVertical
		}
		return appconfig.LayoutHorizontal
	}
}

func (m Model) viewLayout() viewLayout {
	return computeViewLayout(safeWidth(m.width), m.height, m.layout, m.previewVisible())
}

// bodyHeight is the results window's inner height in lines; page-up/down turns
// it into the items it covers.
func (m Model) bodyHeight() int {
	return m.viewLayout().listHeight()
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	var frame string
	if m.installMenu.active {
		frame = m.renderInstallMenu(m.height)
	} else {
		frame = m.renderPicker()
		if m.helpOpen {
			// The help box floats over the rendered picker instead of joining it,
			// so the frame's styling cannot reach inside the box.
			frame = m.renderHelpOverlay(frame)
		}
	}
	return m.overlayNotifications(m.styles.app.Render(frame))
}

func (m Model) renderPicker() string {
	l := m.viewLayout()
	frame := blankFrame(l.width, l.height)
	place := func(lines []string, box windowBox) {
		if len(lines) == 0 {
			return
		}
		frame = overlay(frame, strings.Join(lines, "\n"), box.x, box.y)
	}
	place(m.renderPromptWindow(l), l.prompt)
	place(m.renderResultsWindow(l), l.results)
	if l.preview.height > 0 {
		place(m.renderPreviewWindow(l), l.preview)
	}
	if l.footer {
		footer := pad(clampStyledText(m.renderFooter(l.width), l.width), l.width)
		place([]string{footer}, windowBox{y: l.height - 1})
	}
	return frame
}

// blankFrame is the l.width x l.height canvas the windows are drawn on.
func blankFrame(width, height int) string {
	line := strings.Repeat(" ", max(0, width))
	lines := make([]string, max(0, height))
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// renderPromptWindow draws the prompt window: the active filters in the window
// title, the query row inside it.
func (m Model) renderPromptWindow(l viewLayout) []string {
	title := m.styles.titlePrompt.Render("seshagy") +
		m.styles.muted.Render(" · ") +
		m.styles.emphasis.Render(m.filterTitle())
	content := m.renderPromptContent(max(0, l.prompt.width-2))
	return drawWindow(
		title,
		[]string{content},
		l.prompt.width,
		l.prompt.height,
		m.styles.border,
		"",
	)
}

// renderResultsWindow draws the results window: the item rows inside it, the
// needs-you count in the title, and the scroll counters in the bottom border.
func (m Model) renderResultsWindow(l viewLayout) []string {
	rows := m.rows()
	lines, above, below := m.resultsRows(rows, l.listWidth(), l.listHeight())
	bottom := resultsScrollCounter(above, below)
	if bottom != "" {
		bottom = m.styles.muted.Render(bottom)
	}
	title := m.styles.titleResults.Render("Results")
	if n := needsYouCount(rows); n > 0 {
		title += m.styles.muted.Render(" · ") +
			m.styles.needsYou.Render(fmt.Sprintf("%d need you", n))
	}
	return drawWindow(title, lines, l.results.width, l.results.height, m.styles.border, bottom)
}

// itemHeight is the lines an item draws in the results window: two for an agent
// or a session, whose second line carries the context the name line cannot hold,
// and one for a host or a directory, which are their own context.
func itemHeight(item sessionmgr.Item) int {
	switch item.Kind {
	case sessionmgr.KindAgent, sessionmgr.KindSession:
		return 2
	}
	return 1
}

// itemLine is one drawn line of the results list: an item's first line, one of
// the lines an item continues onto, or the header that opens a section.
type itemLine struct {
	// item is the index of the item the line draws, or -1 for a section header.
	item int
	// detail marks an item's continuation line, which the window never opens on.
	detail bool
	// group is the section a header line titles.
	group group
}

// itemFlow lays the result rows out as the lines the results window draws and
// owns the scroll math over them. An item is one or two lines tall, and an empty
// query puts a header line above the first item of each section. The cursor and
// the ↑/↓ counters count items, never lines, so the cursor can never land on a
// header.
type itemFlow struct {
	lines []itemLine
	// items is how many items the flow covers.
	items int
	// firsts[i] is the line item i's first line draws on, heights[i] how many
	// lines the item takes.
	firsts  []int
	heights []int
}

func newItemFlow(rows []row, grouped bool) itemFlow {
	flow := itemFlow{
		lines:   make([]itemLine, 0, 2*len(rows)),
		items:   len(rows),
		firsts:  make([]int, len(rows)),
		heights: make([]int, len(rows)),
	}
	for i, r := range rows {
		if grouped && (i == 0 || r.group != rows[i-1].group) {
			flow.lines = append(flow.lines, itemLine{item: -1, group: r.group})
		}
		flow.firsts[i] = len(flow.lines)
		flow.heights[i] = itemHeight(r.item)
		for line := range flow.heights[i] {
			flow.lines = append(flow.lines, itemLine{item: i, detail: line > 0})
		}
	}
	return flow
}

// itemWindow is the slice of the flow one frame draws: the lines to draw in
// order, the items it shows, and the items hidden above and below them.
type itemWindow struct {
	lines []itemLine
	// from is the flow line the window opens on.
	from         int
	above, below int
	shown        int
}

// window is the slice a frame of the given line height draws when the window
// last opened on line top. The results window is a plain line viewport, like
// terminal scrollback or telescope: it keeps its position and only scrolls,
// line by line, as far as the cursor's item needs to be fully visible. Items
// cut by the top or bottom edge show their visible lines, so a step always
// scrolls by exactly the height of the item moved onto and the bottom line never
// flips between blank and drawn.
func (f itemFlow) window(cursor, top, height int) itemWindow {
	if height <= 0 || f.items == 0 {
		return itemWindow{}
	}
	cursor = max(0, min(cursor, f.items-1))
	if len(f.lines) <= height {
		return itemWindow{lines: f.lines, shown: f.items}
	}
	from := f.follow(top, cursor, height)
	end := min(len(f.lines), from+height)
	win := itemWindow{lines: f.lines[from:end], from: from}
	// An item counts as hidden only when none of its lines are in the window.
	win.above = f.itemsAbove(from)
	if from > 0 && f.lines[from].detail {
		win.above-- // the item continuing onto the first line is partly shown
	}
	win.below = f.items - f.itemsAbove(end)
	win.shown = f.items - win.above - win.below
	return win
}

// follow is the line a window of the given budget opens on so the cursor's item
// is fully visible, moving the previous top as little as possible. Scrolling up
// onto the first item of a section brings its header along when it fits.
func (f itemFlow) follow(top, cursor, budget int) int {
	if f.items == 0 || budget <= 0 || len(f.lines) <= budget {
		return 0
	}
	cursor = max(0, min(cursor, f.items-1))
	start, end := f.firsts[cursor], f.itemEnd(cursor)
	if end-start > budget {
		// The item is taller than the window: open on its first line.
		return min(start, len(f.lines)-budget)
	}
	if start > 0 && f.lines[start-1].item < 0 && end-(start-1) <= budget && start-1 < top {
		start--
	}
	if start < top {
		top = start
	}
	if end > top+budget {
		top = end - budget
	}
	return max(0, min(top, len(f.lines)-budget))
}

// itemEnd is the first line after the given item's lines.
func (f itemFlow) itemEnd(item int) int { return f.firsts[item] + f.heights[item] }

// itemsAbove is the number of items drawn entirely above the given line.
func (f itemFlow) itemsAbove(line int) int { return sort.SearchInts(f.firsts, line) }

// resultsRows draws the results window's inner rows and reports how many items
// the window hides above and below its position, for the bottom border.
func (m Model) resultsRows(
	rows []row,
	width, height int,
) (lines []string, above, below int) {
	if height <= 0 {
		return nil, 0, 0
	}
	if !m.loadedOnce {
		return centeredNote(width, height, m.styles.muted.Render("loading…")), 0, 0
	}
	if len(rows) == 0 {
		return m.renderEmptyState(width, height), 0, 0
	}
	grouped := m.grouped(rows)
	counts := groupCounts(rows)
	flow := newItemFlow(rows, grouped)
	win := flow.window(m.cursor, m.listTop, height)
	drawn := -1
	var itemLines []string
	for i, line := range win.lines {
		lineIndex := win.from + i
		if line.item < 0 {
			lines = append(lines, m.renderGroupHeader(line.group, counts[line.group], width))
			continue
		}
		if line.item != drawn {
			drawn = line.item
			itemLines = m.renderItem(rows[line.item], width, line.item == m.cursor)
		}
		// Draw this flow line of the item; an item cut by the window's edge
		// shows only the lines that fall inside it.
		if idx := lineIndex - flow.firsts[line.item]; idx < len(itemLines) {
			lines = append(lines, itemLines[idx])
		}
	}
	return lines, win.above, win.below
}

// groupCounts tallies the items each section holds, which is what a header line
// reports. Counts cover the whole list, not only the rows the window draws.
func groupCounts(rows []row) [groupCount]int {
	var counts [groupCount]int
	for _, r := range rows {
		counts[r.group]++
	}
	return counts
}

// renderGroupHeader draws a section's header: its lowercase label under the
// rows' icon column, in the muted header style — red and bold for the needs-you
// section — followed by the section's dim item count. It is a display-only line:
// the cursor never lands on one.
func (m Model) renderGroupHeader(g group, count, width int) string {
	labelStyle := m.styles.muted
	if g == groupNeedsYou {
		labelStyle = m.styles.needsYou
	}
	line := " " + labelStyle.Render(m.groupLabel(g)) +
		"  " + m.styles.groupCount.Render(strconv.Itoa(count))
	return pad(clampStyledText(line, width), width)
}

// groupLabel is the header a section draws: its fixed lowercase label, except
// the session section, which is named the way the active backend names a session
// ("sessions" under tmux, "workspaces" under herdr).
func (m Model) groupLabel(g group) string {
	if g == groupSessions {
		return m.terms.SessionPlural
	}
	return g.label()
}

// resultsScrollCounter spells the items hidden above and below the visible
// window, for the results window's bottom border.
func resultsScrollCounter(above, below int) string {
	parts := make([]string, 0, 2)
	if above > 0 {
		parts = append(parts, fmt.Sprintf("↑ %d more", above))
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("↓ %d more", below))
	}
	return strings.Join(parts, " · ")
}

// needsYouCount counts the rows waiting on the user: blocked agents, plus agents
// that finished a turn nobody has looked at yet.
func needsYouCount(rows []row) int {
	n := 0
	for _, r := range rows {
		if needsAttention(r.item) {
			n++
		}
	}
	return n
}

// renderPreviewWindow draws the preview window: the selected item's name as the
// window title, the captured content inside it, and the kind's footer in the
// bottom border.
func (m Model) renderPreviewWindow(l viewLayout) []string {
	title := m.styles.muted.Render("Preview")
	bottom := ""
	if item, ok := m.selectedItem(); ok {
		// The name comes from the multiplexer, so it is sanitized like the
		// capture itself: a control character must not reach the border.
		title = m.styles.titlePreview.Render(sanitizePreviewLine(previewTitle(item)))
		if footer := sanitizePreviewLine(m.previewFooter(item)); footer != "" {
			bottom = m.styles.muted.Render(footer)
		}
	}
	lines := m.renderPreviewContent(max(0, l.preview.width-2), max(0, l.preview.height-2))
	return drawWindow(title, lines, l.preview.width, l.preview.height, m.styles.border, bottom)
}

// previewTitle is the preview window's title: the selected item's name, and for
// an agent the workspace it runs in — the same pair its row shows as the title
// and the workspace column.
func previewTitle(item sessionmgr.Item) string {
	title := itemTitle(item)
	if item.Kind != sessionmgr.KindAgent || item.Location == "" || item.Location == title {
		return title
	}
	return title + " · " + item.Location
}

// previewFooter spells the dim footer the preview window keeps in its bottom
// border, one wording per kind: the pane and how long it has been running for an
// agent, the window count of a session, the entry count of a directory listing,
// and the transport of a host. It is empty when the item has nothing to report.
func (m Model) previewFooter(item sessionmgr.Item) string {
	switch item.Kind {
	case sessionmgr.KindAgent:
		parts := make([]string, 0, 2)
		if item.PaneID != "" {
			parts = append(parts, "pane "+item.PaneID)
		}
		if !item.StartedAt.IsZero() {
			parts = append(parts, agentAgeAt(item.StartedAt, time.Now()))
		}
		return strings.Join(parts, " · ")
	case sessionmgr.KindSession:
		if item.Windows <= 0 {
			return ""
		}
		return windowCountLabel(m.terms, item.Windows)
	case sessionmgr.KindHost:
		return item.Location
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		// A directory row previews a listing, so its entries are the lines that
		// came back. A capture that is still running, failed, or had nothing to
		// list has no count to report.
		if n, ok := m.previewEntryCount(); ok {
			return fmt.Sprintf("%d entries", n)
		}
	}
	return ""
}

// previewEntryCount counts the rows of the loaded directory-listing preview. It
// reports false while no listing is loaded, because neither an in-flight capture
// nor a failed one is a listing to count.
func (m Model) previewEntryCount() (int, bool) {
	if m.previewError != "" || m.preview == "" || m.preview == noPreviewAvailableText {
		return 0, false
	}
	n := 0
	for _, line := range strings.Split(m.preview, "\n") {
		if strings.TrimSpace(sessionmgr.StripANSI(line)) != "" {
			n++
		}
	}
	return n, true
}

// cursorBlock is the reverse-video cell that marks the typing position.
func (m Model) cursorBlock() string {
	return lipgloss.NewStyle().Reverse(true).Render(" ")
}

func spinnerFrame(frame int) string {
	frames := []rune(spinnerFrames)
	return string(frames[frame%len(frames)])
}

// renderPromptContent draws the prompt window's content row: one column of
// inset — the same the results rows carry — then the accent prompt and the query
// (or the muted placeholder) with the typing cursor, and the shown/total count
// right-aligned. A kill confirmation or an in-flight rename replaces the row.
func (m Model) renderPromptContent(width int) string {
	if width <= 0 {
		return ""
	}
	inset := strings.Repeat(" ", min(promptInset, width))
	row := width - promptInset
	s := m.styles
	switch {
	case m.killArmed:
		return inset + m.renderKillConfirm(row)
	case m.renameActive:
		left := s.title.Render("rename") + " " + s.emphasis.Render("❯") + " " +
			m.renameText + m.cursorBlock()
		return inset + clampStyledText(left, row)
	}
	prompt := s.emphasis.Render("❯") + " "
	var left string
	switch {
	case m.query != "":
		left = prompt + m.query + m.cursorBlock()
	default:
		// An empty query types into the first cell of the input, so the cursor
		// sits right behind the prompt with the placeholder following it.
		left = prompt + m.cursorBlock() + s.muted.Render(placeholderText)
	}
	// The count keeps the last cells: a long query is clipped with an ellipsis
	// rather than the count, which is the only place the row reports the size.
	right := m.promptTrailing(row)
	if right == "" {
		return inset + clampStyledText(left, row)
	}
	room := row - lipgloss.Width(right)
	return inset + clampStyledText(left, room) +
		strings.Repeat(" ", max(0, room-lipgloss.Width(left))) + right
}

// promptTrailing renders the dim right side of the prompt content row: the
// shown/total count and the spinner while the first load is still running. It is
// empty when the window is too narrow for the count, and the query then takes
// the whole row.
func (m Model) promptTrailing(width int) string {
	spinner := ""
	if !m.loadedOnce && m.spinnerActive {
		spinner = " " + m.styles.emphasis.Render(spinnerFrame(m.spinnerFrame))
	}
	right := m.promptCount() + spinner
	if width-1 < lipgloss.Width(right) {
		return ""
	}
	return right
}

// promptCount is the "shown/total" the prompt window reports: how many rows the
// active scope and query leave of the whole list. The shown count stays bright
// and the total dim, so the row always reads as a pair.
func (m Model) promptCount() string {
	shown, total := len(m.rows()), len(m.items)
	return m.styles.itemName.Bold(true).Render(fmt.Sprintf("%d", shown)) +
		m.styles.muted.Render(fmt.Sprintf("/%d", total))
}

// filterTitle names the active filters: the scope, the agent-state filter when
// one is active, and the machine the list is loaded from: "agents · blocked ·
// @devbox". The machine part only appears once a saved machine exists, so a
// picker without machines draws exactly the title it always has.
func (m Model) filterTitle() string {
	title := m.scopeLabel(m.scope)
	if m.stateFilter != stateFilterNone {
		title += " · " + m.stateFilter.label()
	}
	if machine := m.machineTitleSuffix(); machine != "" {
		title += " · @" + machine
	}
	return title
}

// scopeLabel names a scope for the prompt window title and the empty state. The
// sessions scope carries the backend's own word for a session.
func (m Model) scopeLabel(sc scope) string {
	if sc == scopeSessions {
		return m.terms.SessionPlural
	}
	return scopeLabels[sc]
}

// scopeAvailable reports whether the picker has a source for a scope: the hosts
// scope only exists once remote hosts are configured. The tab cycle steps over
// the scopes that have no source.
func (m Model) scopeAvailable(sc scope) bool {
	if sc == scopeHosts {
		return m.hasHosts()
	}
	return true
}

func (m Model) hasHosts() bool {
	for _, item := range m.items {
		if item.Kind == sessionmgr.KindHost {
			return true
		}
	}
	return false
}

// showsBinding reports whether the picker draws a binding at all. The machine
// filter is the one binding that belongs to a single backend: no saved machines
// exist outside herdr, so tmux and noop keep their frame exactly as it is.
func (m Model) showsBinding(binding keyBinding) bool {
	return binding.id != "machine" || m.selector != nil
}

// itemBadge is the run an item draws beside its name on the line the name sits
// on: an agent's colored state glyph and word, a session's green attached flag,
// or a host's dim transport and alias. A directory carries none.
type itemBadge struct {
	run string
	// gap is the blank run drawn between the name and the badge, before the
	// columns the badge's own padding already supplies.
	gap int
	// pad is the columns of the badge's own background that precede its text.
	pad int
}

// draw is the badge as the line draws it: the gap that separates it from the
// name, then the run itself.
func (b itemBadge) draw() string {
	if b.run == "" {
		return ""
	}
	return spaces(max(0, b.gap-b.pad)) + b.run
}

// width is the columns the badge takes beside the name.
func (b itemBadge) width() int { return lipgloss.Width(b.draw()) }

// itemBadge draws the run that follows an item's name. An agent shows its state
// — the configured glyph and word, both in the state's color, a blocked agent
// on the design's red block — and a session shows its green attached flag while
// it is attached. A host shows its transport and the alias herdr is handed, dim,
// with an alias that only repeats the name (a host discovered in ~/.ssh/config)
// dropped: the transport is then all such a host has to say. Directories show
// nothing: the path is the whole item.
func (m Model) itemBadge(item sessionmgr.Item) itemBadge {
	switch item.Kind {
	case sessionmgr.KindAgent:
		return m.agentStateBadge(item.AgentState)
	case sessionmgr.KindSession:
		if !item.Attached {
			return itemBadge{}
		}
		return itemBadge{run: m.styles.success.Render("• attached"), gap: sessionBadgeGap}
	case sessionmgr.KindHost:
		alias := item.Target
		if alias == item.DisplayName() {
			alias = ""
		}
		parts := make([]string, 0, 2)
		if item.Location != "" {
			parts = append(parts, item.Location)
		}
		if alias != "" {
			parts = append(parts, alias)
		}
		if len(parts) == 0 {
			return itemBadge{}
		}
		return itemBadge{
			run: m.styles.muted.Render(strings.Join(parts, " · ")),
			gap: itemBadgeGap,
		}
	default:
		return itemBadge{}
	}
}

// agentStateBadge draws an agent's state as its name-line badge: the glyph and
// word the configured icon set resolves for the state — both in icon mode, the
// word alone in text mode — in the state's color. State mode "none" hides agent
// state entirely, and then there is no badge. The word is drawn as its own run so
// a configured color covers it exactly as it covers the glyph.
func (m Model) agentStateBadge(state sessionmgr.AgentState) itemBadge {
	if m.icons.AgentStateHidden() {
		return itemBadge{}
	}
	icon := m.icons.ForAgentState(state)
	style := m.agentStateStyle(state)
	if !m.icons.AgentStateUsesIcons() {
		return itemBadge{run: style.Render(icon.ASCII), gap: itemBadgeGap}
	}
	if state == sessionmgr.AgentBlocked {
		// The blocked badge paints its own red background, and the selection bar
		// resumes after it: a selected item keeps the red block rather than
		// losing it to the item's background.
		return itemBadge{
			run: m.styles.badgeBlocked.Render(" " + icon.Text + " " + icon.ASCII + " "),
			gap: itemBadgeGap,
			pad: 1,
		}
	}
	return itemBadge{
		run: style.Render(icon.Text) + " " + style.Render(icon.ASCII),
		gap: itemBadgeGap,
	}
}

// agentNameStyle is the style an agent's kind label is drawn in: its brand
// color when the vendor has one, the default foreground otherwise.
func agentNameStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(agentColor(name))
}

// itemDetail is an item's second line: the dim context under its name — an
// agent's workspace and age, a session's window count and the agents running
// inside it. Directories and hosts draw no second line.
func (m Model) itemDetail(item sessionmgr.Item, width int) string {
	sep := m.styles.muted.Render(" · ")
	parts := make([]string, 0, 3)
	add := func(text string) {
		if text != "" {
			parts = append(parts, text)
		}
	}
	switch item.Kind {
	case sessionmgr.KindAgent:
		add(m.styles.muted.Render(m.workspaceLabel(item.Session)))
		if !item.StartedAt.IsZero() {
			add(m.styles.muted.Render(agentAgeAt(item.StartedAt, time.Now())))
		}
		// The logo says which agent this is; without one the brand key is the
		// only place the agent type can show.
		if key := agentKey(item.AgentName); key != "" && !m.showAgentLogos() &&
			!repeatsTitle(item, key) {
			add(agentNameStyle(item.AgentName).Render(key))
		}
	case sessionmgr.KindSession:
		if item.Windows > 0 {
			add(m.styles.muted.Render(windowCountLabel(m.terms, item.Windows)))
		}
		add(m.itemTally(item))
	}
	return clampStyledText(strings.Join(parts, sep), width)
}

// workspaceLabel names the workspace an agent runs in: the session item's label
// for the identifier the agent carries (the herdr workspace id, or the tmux
// session name), falling back to the identifier itself for a workspace the
// session source has not listed.
func (m Model) workspaceLabel(id string) string {
	if id == "" {
		return ""
	}
	for _, item := range m.items {
		if item.Kind != sessionmgr.KindSession {
			continue
		}
		if item.Name == id || item.Target == id {
			return item.Name
		}
	}
	return id
}

// itemTally is a session's agent counts for its second line ("● 1 ○ 1"): one
// entry per state the workspace hosts, each glyph in that state's color and its
// count in the item's own foreground, one space between a glyph and its number.
// The entries list the states most urgent first, and a hidden state mode drops
// the tally entirely.
func (m Model) itemTally(session sessionmgr.Item) string {
	if m.icons.AgentStateHidden() {
		return ""
	}
	counts := m.sessionAgentCounts(session)
	var out strings.Builder
	for _, state := range agentCountOrder {
		n := counts[state]
		if n == 0 {
			continue
		}
		if out.Len() > 0 {
			out.WriteString(" ")
		}
		// The glyph carries the state color; the count stays in the item's own
		// foreground so the runs of numbers read as data, not as more glyph.
		out.WriteString(m.agentStateStyle(state).Render(m.stateLabel(state)))
		out.WriteString(" " + strconv.Itoa(n))
	}
	return out.String()
}

// agentCountOrder lists the agent states a session's tally reports, most urgent
// first; an empty state (no agents) never appears.
var agentCountOrder = []sessionmgr.AgentState{
	sessionmgr.AgentWorking,
	sessionmgr.AgentBlocked,
	sessionmgr.AgentDone,
	sessionmgr.AgentIdle,
	sessionmgr.AgentUnknown,
}

// sessionAgentCounts tallies the loaded agents running inside a session by
// state. An agent belongs to the session its session field names, which tmux
// spells as the session name and herdr as the workspace id.
func (m Model) sessionAgentCounts(session sessionmgr.Item) map[sessionmgr.AgentState]int {
	counts := make(map[sessionmgr.AgentState]int)
	for _, item := range m.items {
		if item.Kind != sessionmgr.KindAgent {
			continue
		}
		if item.Session != session.Name && item.Session != session.ActionTarget() {
			continue
		}
		counts[item.AgentState]++
	}
	return counts
}

// stateLabel is how a state is named inside a count: its configured glyph when
// the icon set draws icons, and its configured word when it does not.
func (m Model) stateLabel(state sessionmgr.AgentState) string {
	icon := m.icons.ForAgentState(state)
	if m.icons.AgentStateUsesIcons() {
		return icon.Icon
	}
	return icon.ASCII
}

// renderItem draws one result item as the lines its kind takes — a name line,
// plus the dim context line an agent or a session carries — each exactly width
// cells. The selected item paints every line it draws with the selection bar,
// and a badge the window has no room for is dropped rather than squeezing the
// name away.
func (m Model) renderItem(r row, width int, selected bool) []string {
	prefix := m.itemPrefix(r.item)
	badge := m.itemBadge(r.item)
	room := max(0, width-lipgloss.Width(prefix))
	if badge.run != "" && room < itemNameFloor+badge.width() {
		badge = itemBadge{}
	}
	lines := []string{prefix + m.renderTitle(r, max(1, room-badge.width())) + badge.draw()}
	if itemHeight(r.item) > 1 {
		indent := lipgloss.Width(prefix)
		lines = append(lines, spaces(indent)+m.itemDetail(r.item, max(1, width-indent)))
	}
	for i, line := range lines {
		line = pad(clampStyledText(line, width), width)
		if selected {
			line = m.selectionBar(line, width)
		}
		lines[i] = line
	}
	return lines
}

// itemPrefix is the fixed lead every item line shares: one column of space
// inside the window border, the kind icon column, then the gap before the name.
func (m Model) itemPrefix(item sessionmgr.Item) string {
	iconWidth := m.rowIconWidth()
	icon := pad(clampStyledText(m.rowIcon(item), iconWidth), iconWidth)
	return " " + icon + " "
}

// selectionBar paints a drawn row with the configured selection background over
// its full width. A profile that draws no styling has no bar to paint, and the
// row keeps a plain marker instead: the cursor has to stay visible without
// color.
func (m Model) selectionBar(line string, width int) string {
	if backgroundOpen(m.styles.selectionBG) == "" {
		return m.styles.bar.Render("▌") + strings.TrimPrefix(line, " ")
	}
	return paintBackground(line, width, m.styles.selectionBG)
}

// rowIconWidth is the icon column's width. It is one cell wider when agent logos
// are drawn, because the icon font extends into the cell after the glyph.
func (m Model) rowIconWidth() int {
	if m.showAgentLogos() {
		return 3
	}
	return 2
}

// rowIcon is the 2-3 cell kind column: the agent's font logo, the configured
// kind icon, or the host marker.
func (m Model) rowIcon(item sessionmgr.Item) string {
	switch item.Kind {
	case sessionmgr.KindAgent:
		if !m.showAgentLogos() {
			return ""
		}
		logo := agentLogo(item.AgentName)
		if logo == "" {
			return ""
		}
		// The icon font extends into the next cell; reserve it plus one space
		// so the state glyph keeps an even gap on both sides.
		return lipgloss.NewStyle().Foreground(agentColor(item.AgentName)).Render(logo + "  ")
	case sessionmgr.KindHost:
		icons := m.icons
		if !icons.Enabled {
			return ""
		}
		if icons.ASCII {
			return m.styles.muted.Render("h ")
		}
		return m.styles.muted.Render(hostIcon)
	default:
		style := m.icons.For(item.Kind)
		if style.Text == "" {
			return ""
		}
		color := lipgloss.TerminalColor(lipgloss.NoColor{})
		if style.Color != "" {
			color = lipgloss.Color(style.Color)
		}
		return lipgloss.NewStyle().Foreground(color).Render(style.Text)
	}
}

// agentStateStyle is the style an agent's state glyph and word are drawn in: the
// configured color when the icon set has one, the semantic theme color
// otherwise (sessionmgr deliberately leaves an unconfigured color empty so the
// TUI can fill it).
func (m Model) agentStateStyle(state sessionmgr.AgentState) lipgloss.Style {
	if color := strings.TrimSpace(m.icons.ForAgentState(state).Color); color != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	}
	switch state {
	case sessionmgr.AgentWorking:
		return m.styles.success
	case sessionmgr.AgentBlocked:
		return m.styles.warning
	case sessionmgr.AgentDone:
		return m.styles.info
	default:
		return m.styles.muted
	}
}

// agentAgeAt renders how long an agent process has been running: seconds under a
// minute, minutes under an hour, then hours with zero-padded minutes, then days
// with hours.
func agentAgeAt(started, now time.Time) string {
	elapsed := now.Sub(started)
	if elapsed < 0 {
		elapsed = 0
	}
	switch {
	case elapsed < time.Minute:
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(elapsed.Hours()), int(elapsed.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(elapsed.Hours())/24, int(elapsed.Hours())%24)
	}
}

// agentTabSuffix returns the tab label an agent row still has to show, or ""
// when the label repeats the title or the workspace already drawn in the meta
// columns. A label that is not shown would only repeat the row ("pi — skills"
// beside a "skills" meta cell).
func agentTabSuffix(item sessionmgr.Item) string {
	label := strings.TrimSpace(item.TabLabel)
	if label == "" {
		return ""
	}
	repeats := func(other string) bool {
		return strings.EqualFold(label, strings.TrimSpace(other))
	}
	if repeatsTitle(item, label) || repeats(item.Location) {
		return ""
	}
	return label
}

// repeatsTitle reports whether label would only repeat the title the row already
// draws.
func repeatsTitle(item sessionmgr.Item, label string) bool {
	return strings.EqualFold(strings.TrimSpace(label), strings.TrimSpace(itemTitle(item)))
}

// renderTitle draws the item title in its column with the fuzzy-matched runes
// emphasized. An agent's or a session's name is the item's headline and is drawn
// bold, and an agent's tab label is appended muted when there is room; a
// directory's parent path is dimmed and its basename kept bright, which is what
// the path's item is about. The selection does not change the weight: the bar
// behind the item carries it.
func (m Model) renderTitle(r row, width int) string {
	title := itemTitle(r.item)
	suffix := ""
	if r.item.Kind == sessionmgr.KindAgent {
		if label := agentTabSuffix(r.item); label != "" {
			suffix = " — " + label
			if lipgloss.Width(suffix)+titleMinWidth > width {
				suffix = ""
			}
		}
	}
	base := m.styles.itemName
	switch r.item.Kind {
	case sessionmgr.KindAgent, sessionmgr.KindSession:
		base = base.Bold(true)
	}
	text := clampText(title, max(1, width-lipgloss.Width(suffix)))
	if r.item.Kind == sessionmgr.KindZoxide || r.item.Kind == sessionmgr.KindFD {
		runes := []rune(text)
		if cut := lastIndexRune(runes, '/'); cut >= 0 && cut+1 < len(runes) {
			parent, name := string(runes[:cut+1]), string(runes[cut+1:])
			return m.highlight(parent, shiftMatches(r.matches, 0, cut+1), m.styles.muted) +
				m.highlight(name, shiftMatches(r.matches, cut+1, len(runes)-cut-1), base)
		}
	}
	if suffix != "" {
		return m.highlight(text, r.matches, base) + m.styles.muted.Render(suffix)
	}
	return m.highlight(text, r.matches, base)
}

// lastIndexRune is the position of the last occurrence of r in runes, or -1. It
// works in runes so a path's split points stay valid for match highlighting.
func lastIndexRune(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

// highlight renders text in base style with the matched rune positions drawn in
// the accent match style (bold + underline so the subsequence reads clearly).
func (m Model) highlight(text string, matches []int, base lipgloss.Style) string {
	if len(matches) == 0 {
		return base.Render(text)
	}
	marked := make(map[int]bool, len(matches))
	for _, pos := range matches {
		marked[pos] = true
	}
	var out, run strings.Builder
	runMarked := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		style := base
		if runMarked {
			style = m.styles.match
		}
		out.WriteString(style.Render(run.String()))
		run.Reset()
	}
	for i, r := range []rune(text) {
		if marked[i] != runMarked {
			flush()
			runMarked = marked[i]
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}

// shiftMatches rebases match positions onto a substring drawn at offset and
// drops the ones that fall outside it.
func shiftMatches(matches []int, offset, size int) []int {
	out := make([]int, 0, len(matches))
	for _, pos := range matches {
		if shifted := pos - offset; shifted >= 0 && shifted < size {
			out = append(out, shifted)
		}
	}
	return out
}

// renderEmptyState centers the "nothing here" message in the results window.
func (m Model) renderEmptyState(width, height int) []string {
	var headline, hint string
	switch {
	case m.loadError != "":
		// A saved machine that could not be loaded: the window says so and names
		// the two ways out, instead of looking like an empty local list.
		headline = m.styles.emphasis.Render("machine @" + m.machineLabel() + " unavailable")
		hint = "alt+m switch machine · ^l retry"
	case m.query != "":
		headline = m.styles.emphasis.Render(`no matches for "` + m.query + `"`)
		hint = "⌫ backspace clears the query"
	case m.stateFilter != stateFilterNone:
		headline = m.styles.emphasis.Render("no " + m.stateFilter.label() + " agents")
		hint = "^s next state · ⌫ back to all"
	case m.scope != scopeAll:
		headline = m.styles.itemName.Render("nothing in ") +
			m.styles.emphasis.Render(m.scopeLabel(m.scope)) +
			m.styles.itemName.Render(" filter")
		hint = "⇥ next filter · ⌫ back to all"
	default:
		headline = m.styles.itemName.Render("nothing to jump to")
		hint = "start an agent or open a directory"
	}
	return centeredNote(width, height, headline, m.styles.muted.Render(hint))
}

// centeredNote centers one or more rendered lines in a window of the given size.
func centeredNote(width, height int, lines ...string) []string {
	out := make([]string, 0, height)
	top := max(0, (height-len(lines))/2)
	for i := 0; i < height; i++ {
		index := i - top
		if index < 0 || index >= len(lines) {
			out = append(out, "")
			continue
		}
		line := clampStyledText(lines[index], width)
		indent := max(0, (width-lipgloss.Width(line))/2)
		out = append(out, pad(strings.Repeat(" ", indent)+line, width))
	}
	return out
}

// renderPreviewContent draws the captured content of the preview window: the
// selected item's payload with its ANSI colors intact, bottom-anchored for
// multiplexer captures and top-anchored for directory listings, always exactly
// height lines wide enough for the window. width is the window's inner width, the
// content starting after the window's one column of padding.
func (m Model) renderPreviewContent(width, height int) []string {
	item, ok := m.selectedItem()
	if !ok || height <= 0 {
		return nil
	}
	content, style := m.preview, lipgloss.NewStyle()
	switch {
	case m.previewError != "":
		content, style = m.previewError, m.styles.danger
	case m.preview == "":
		content, style = "loading…", m.styles.muted
	}
	lines := strings.Split(content, "\n")
	tail := isTailPreviewKind(item.Kind)
	if len(lines) > height {
		if tail {
			lines = lines[len(lines)-height:]
		} else {
			lines = lines[:height]
		}
	}
	out := make([]string, 0, height)
	if tail {
		// A capture is a window onto a terminal: the newest output is on the
		// last row, so the padding goes above it.
		out = append(out, make([]string, height-len(lines))...)
	}
	padding := strings.Repeat(" ", previewInset)
	textWidth := max(0, width-previewInset)
	for _, line := range lines {
		// Captures are raw terminal output: sanitize before clipping so no
		// stray CR, tab, or cursor sequence reaches the frame. The cut is hard
		// — a preview is a window onto a terminal, and an ellipsis would read
		// as content the pane never printed. A reset closes any color the line
		// left open so it cannot bleed into the border or the next row.
		clipped := hardCutStyledText(sanitizePreviewLine(line), textWidth)
		if style.GetForeground() != nil {
			clipped = style.Render(clipped)
		}
		out = append(out, padding+clipped+previewReset)
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// isTailPreviewKind reports whether a kind's preview is sourced from a
// multiplexer capture (tmux capture-pane or herdr pane read), both of which are
// bottom-anchored, rather than a top-down directory listing.
func isTailPreviewKind(kind sessionmgr.Kind) bool {
	return kind == sessionmgr.KindSession || kind == sessionmgr.KindAgent
}

type footerHint struct {
	key   string
	label string
}

// footerHints are the footer's hints: the table's footer entries, in the order
// they draw, with the sessions hint named the way the active backend names a
// session. A hint the backend cannot honour is left out, so tmux and noop draw
// the footer they always have.
func (m Model) footerHints() []footerHint {
	bindings := m.keys.footerBindings()
	hints := make([]footerHint, 0, len(bindings))
	for i := 0; i < len(bindings); i++ {
		binding := bindings[i]
		if !m.showsBinding(binding) {
			continue
		}
		if binding.id == "up" && i+1 < len(bindings) && bindings[i+1].id == "down" {
			// Up and down share one hint, "↑↓ move", as long as both are bound.
			hints = append(hints, footerHint{
				key:   keyHint(binding.keys) + keyHint(bindings[i+1].keys),
				label: "move",
			})
			i++
			continue
		}
		hints = append(
			hints,
			footerHint{key: keyHint(binding.keys), label: m.bindingLabel(binding)},
		)
	}
	return hints
}

// footerPinnedHints is how many trailing hints the footer keeps while anything
// else fits: F1 help and esc quit, the two keys that reach everything else.
const footerPinnedHints = 2

// renderFooter draws the frame's bottom key line: the core action hints, the
// rename keys while a rename is in flight, or nothing but the confirm keys while
// a kill is armed. Hints drop from the right until the line fits, and the help
// and quit hints are pinned last.
func (m Model) renderFooter(width int) string {
	switch {
	case m.killArmed:
		return m.styles.key.Render("⏎/y") + " " + m.styles.muted.Render("confirm") +
			"  " + m.styles.key.Render("esc") + " " + m.styles.muted.Render("cancel")
	case m.renameActive:
		return m.styles.key.Render("⏎") + " " + m.styles.muted.Render("save") +
			"  " + m.styles.key.Render("esc") + " " + m.styles.muted.Render("cancel")
	}
	hints := m.footerHints()
	parts := make([]string, len(hints))
	for i, hint := range hints {
		parts[i] = m.styles.key.Render(hint.key) + " " + m.styles.muted.Render(hint.label)
	}
	// The droppable hints give way from the right; the pinned pair is measured
	// with them, so it survives as long as the line can hold it. The help hint
	// goes before the quit hint does. A config that unbinds footer actions can
	// leave fewer hints than the pinned pair, and then every hint left is pinned.
	pinnedCount := min(footerPinnedHints, len(parts))
	body := parts[:len(parts)-pinnedCount]
	pinned := parts[len(parts)-pinnedCount:]
	shownWidth := func(n, p int) int {
		width := hintsWidth(body[:n])
		if n > 0 && p > 0 {
			width += 2
		}
		return width + hintsWidth(pinned[len(pinned)-p:])
	}
	n, p := len(body), pinnedCount
	for n > 0 && shownWidth(n, p) > width {
		n--
	}
	if p > 0 && shownWidth(n, p) > width {
		p--
	}
	if shownWidth(n, p) > width {
		n, p = 0, 0
	}
	visible := append(make([]string, 0, n+p), body[:n]...)
	visible = append(visible, pinned[len(pinned)-p:]...)
	return clampStyledText(strings.Join(visible, "  "), width)
}

// hintsWidth is the columns a list of joined hint runs takes, including the
// two-space gap between entries.
func hintsWidth(parts []string) int {
	total := 0
	for i, part := range parts {
		if i > 0 {
			total += 2
		}
		total += lipgloss.Width(part)
	}
	return total
}

func (m Model) renderKillConfirm(width int) string {
	item := m.killPending
	question := fmt.Sprintf("kill %s %s?", m.terms.SessionNoun, item.DisplayName())
	if item.Kind == sessionmgr.KindAgent {
		question = fmt.Sprintf("kill %s %s?", m.terms.PaneNoun, item.DisplayName())
	}
	left := m.styles.danger.Render(clampText(question, max(1, width-30)))
	right := m.styles.muted.Render("⏎/y confirm · any other key cancels")
	if lipgloss.Width(left)+lipgloss.Width(right)+2 > width {
		return clampStyledText(left+"  "+right, width)
	}
	return left + "  " + right
}

func (m Model) renderInstallMenu(height int) string {
	s := m.styles
	width := max(54, min(88, m.width-4))
	innerW := max(44, width-4)
	names := integrations.Available()
	innerH := max(12, len(names)+8)

	lines := []string{
		s.title.Render("Install agent integrations"),
		s.muted.Render("Hooks/plugins report state to seshagy in real time."),
		s.muted.Render(clampText(m.integrationsNote(), innerW)),
		"",
	}
	for i, name := range names {
		cursor := "  "
		if i == m.installMenu.cursor {
			cursor = s.bar.Render("▌") + " "
		}
		status := m.installMenu.statuses[name]
		if status == "" {
			status = "idle"
		}
		var glyph string
		switch status {
		case "installing", "uninstalling":
			glyph = s.warning.Render("…")
		case "installed":
			glyph = s.success.Render("✓")
		case "failed":
			glyph = s.danger.Render("✗")
		default:
			glyph = s.muted.Render("○")
		}
		line := cursor + name + "  " + glyph
		if i == m.installMenu.cursor {
			line = paintBackground(line, innerW, s.selectionBG)
		}
		lines = append(lines, line)
	}
	if msg := m.installMenu.message; msg != "" {
		lines = append(lines, "", s.muted.Render(clampText(msg, innerW)))
	}
	helpParts := []string{
		s.key.Render("enter") + " install",
		s.key.Render("u") + " uninstall",
		s.key.Render("a") + " all",
		s.key.Render("esc") + " close",
		s.key.Render("q") + " quit",
	}
	lines = append(lines, "", strings.Join(helpParts, s.muted.Render(" · ")))
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	box := s.panePopup.Width(width - 2).Height(innerH).Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

// integrationsNote is the install menu's one-line caveat, drawn on every
// backend: the hooks/plugins report agent state to tmux only, and under herdr
// the backend detects agent state itself.
func (m Model) integrationsNote() string {
	if m.mux.Kind() == sessionmgr.BackendHerdr {
		return "tmux only — herdr detects agent state itself"
	}
	return "tmux only — under herdr, herdr detects agent state itself"
}

// overlayNotifications floats the toast stack in the frame's bottom-right
// corner; the frame itself is the dimmed or plain dashboard.
func (m Model) overlayNotifications(frame string) string {
	toast := m.renderNotificationToast(time.Now())
	if toast == "" {
		return frame
	}
	toastW := lipgloss.Width(toast)
	toastH := lipgloss.Height(toast)
	x := max(0, m.width-toastW-3)
	y := max(0, m.height-toastH-1)
	return overlay(frame, toast, x, y)
}

func (m Model) renderNotificationToast(now time.Time) string {
	live := make([]notification, 0, len(m.notifications))
	cutoff := now.Add(-notificationTTL)
	for _, n := range m.notifications {
		if n.at.After(cutoff) {
			live = append(live, n)
		}
	}
	if len(live) == 0 {
		return ""
	}

	maxDisplayW := min(max(12, m.width*2/5), max(1, m.width-2))
	minDisplayW := min(12, maxDisplayW)
	naturalW := 0
	rawLines := make([]string, len(live))
	for i, n := range live {
		marker := "•"
		switch n.sev {
		case sevWarning:
			marker = "!"
		case sevError:
			marker = "×"
		}
		text := strings.NewReplacer("\n", " ", "\r", " ").Replace(n.text)
		rawLines[i] = marker + " " + text
		naturalW = max(naturalW, lipgloss.Width(rawLines[i]))
	}
	displayW := min(max(naturalW+2, minDisplayW), maxDisplayW)
	contentW := max(1, displayW-2)
	lines := make([]string, len(live))
	for i, n := range live {
		style := m.styles.muted
		switch n.sev {
		case sevWarning:
			style = m.styles.warning
		case sevError:
			style = m.styles.danger
		}
		lines[i] = style.Render(clampText(rawLines[i], contentW))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.muted.GetForeground()).
		Width(contentW).
		Render(strings.Join(lines, "\n"))
}
