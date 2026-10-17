package tui

import (
	"fmt"
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
	// metaColumnMax caps each right-flush meta column.
	metaColumnMax = 18
	// titleMinWidth is the room a title keeps before its tab suffix is dropped.
	titleMinWidth = 12
	// rowRightMargin is the blank cell the right-flush columns keep off the
	// window's inner edge, rowGap the least blank run that separates them from
	// the name, and rowNameFloor the least room the name keeps before the
	// right-flush columns start being dropped.
	rowRightMargin = 1
	rowGap         = 1
	rowNameFloor   = 1
	// previewInset is the blank column the preview content starts after.
	previewInset = 1
	// hostIcon marks remote-machine rows.
	hostIcon = "󰒋 "

	placeholderText = "type to search"
	// promptMinQuery is the room the query keeps in the prompt window before the
	// sigil hint is dropped; the placeholder always fits while the hint is shown.
	promptMinQuery = len(placeholderText) + 2
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

// listHeight is the results window's inner height: the rows it can show.
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

// bodyHeight is the number of result rows a frame shows; page-up/down uses it.
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

// renderPromptWindow draws the prompt window: the active scope in the window
// title, the query row inside it.
func (m Model) renderPromptWindow(l viewLayout) []string {
	title := m.styles.titlePrompt.Render("seshagy") +
		m.styles.muted.Render(" · ") +
		m.styles.emphasis.Render(m.scope.chip())
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

// resultsRows draws the results window's inner rows and reports how many rows
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
	start, end := visibleWindow(len(rows), m.cursor, height)
	visible := rows[start:end]
	cells := make([]rowCells, len(visible))
	for i, r := range visible {
		cells[i] = m.rowCells(r)
	}
	grid := buildRowGrid(visible, cells, width, m.rowPrefixWidth())
	lines = make([]string, 0, len(visible))
	for i, r := range visible {
		lines = append(
			lines,
			m.renderRow(r, cells[i], grid[r.item.Kind], width, start+i == m.cursor),
		)
	}
	return lines, start, len(rows) - end
}

// resultsScrollCounter spells the rows hidden above and below the visible
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
// (or the muted placeholder) with the typing cursor, the dim sigil hint, and the
// shown/total count right-aligned. A kill confirmation or an in-flight rename
// replaces the row.
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

// promptTrailing renders the dim right side of the prompt content row: the sigil
// hint when the query keeps its room, the shown/total count, and the spinner
// while the first load is still running. It is empty when the window is too
// narrow for even the count, and the query then takes the whole row.
func (m Model) promptTrailing(width int) string {
	spinner := ""
	if !m.loadedOnce && m.spinnerActive {
		spinner = " " + m.styles.emphasis.Render(spinnerFrame(m.spinnerFrame))
	}
	right := m.promptCount() + spinner
	if sigils := m.sigilHint(); lipgloss.Width(sigils) > 0 &&
		width-lipgloss.Width(right)-2-lipgloss.Width(sigils) >= promptMinQuery {
		right = sigils + "  " + right
	}
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

// sigilHint is the dim "@ ! $ * ~ / #" reminder of the scopes that can be typed
// on an empty query. The active scope's sigil carries the chip accent.
func (m Model) sigilHint() string {
	scopes := m.visibleScopes()
	parts := make([]string, 0, len(scopes))
	for _, sc := range scopes {
		if sc.sigil() == 0 {
			continue
		}
		style := m.styles.chipIdle
		if sc == m.scope {
			style = m.styles.chipActive
		}
		parts = append(parts, style.Render(string(sc.sigil())))
	}
	return strings.Join(parts, " ")
}

// visibleScopes lists the scopes the prompt window's sigil hint shows, in cycle
// order. The hosts sigil only appears once host items exist.
func (m Model) visibleScopes() []scope {
	scopes := make([]scope, 0, int(scopeCount))
	for sc := scopeAll; sc < scopeCount; sc++ {
		if sc == scopeHosts && !m.hasHosts() {
			continue
		}
		scopes = append(scopes, sc)
	}
	return scopes
}

func (m Model) hasHosts() bool {
	for _, item := range m.items {
		if item.Kind == sessionmgr.KindHost {
			return true
		}
	}
	return false
}

// cellSep is the gap between two row cells: every cell of a group is separated
// by it, and a cell a row has no value for leaves its column blank instead of
// shifting the cells after it.
const cellSep = "  "

// metaCell is one cell of a row's trailing run: the plain text its column is
// measured from, the styled text drawn, and whether the column pads it on the
// left so a group of ages ends on the same column.
type metaCell struct {
	plain string
	text  string
	right bool
}

// metaCol builds one left-aligned cell from a style and its plain text.
func metaCol(style lipgloss.Style, text string) metaCell {
	return metaCell{plain: text, text: style.Render(text)}
}

// metaRightCol builds one right-aligned cell: a run of values of different
// lengths (the ages) ends on its column instead of starting on it.
func metaRightCol(style lipgloss.Style, text string) metaCell {
	return metaCell{plain: text, text: style.Render(text), right: true}
}

// rowCells is one row split by where its cells are drawn: the run that follows
// the title — next to the name, with no dot between them — and the columns flush
// against the window's right edge.
type rowCells struct {
	left  []metaCell
	right []metaCell
}

// rowCells builds a row's cells. Each kind lays out its own: an agent keeps its
// kind label and workspace beside its name and right-aligns its age, a session
// right-aligns the attached flag, its window count, and the agents inside it, a
// host right-aligns its alias and transport, and a directory has no cells at all.
func (m Model) rowCells(r row) rowCells {
	item := r.item
	switch item.Kind {
	case sessionmgr.KindAgent:
		cells := rowCells{}
		// The logo already says which agent this is; the colored kind label is
		// only worth the columns when no logo is drawn, and a label that only
		// repeats the name already drawn as the title is dropped, exactly like a
		// tab label that repeats it.
		if !m.showAgentLogos() {
			if kind := agentKey(item.AgentName); !repeatsTitle(item, kind) {
				cells.left = append(cells.left, metaCol(agentNameStyle(item.AgentName), kind))
			}
		}
		if item.Location != "" && item.Location != itemTitle(item) {
			cells.left = append(cells.left, metaCol(m.styles.muted, item.Location))
		}
		age := ""
		if !item.StartedAt.IsZero() {
			age = agentAgeAt(item.StartedAt, time.Now())
		}
		cells.right = append(cells.right, metaRightCol(m.styles.muted, age))
		if state, ok := m.agentStateCell(item.AgentState); ok {
			cells.right = append(cells.right, state)
		}
		return cells
	case sessionmgr.KindSession:
		// The attached flag comes first and keeps its column (blank while the
		// session is not attached) so the window count and the agent counts
		// after it line up on every session row.
		attached := metaCol(m.styles.muted, "")
		if item.Attached {
			attached = metaCol(m.styles.success, "attached")
		}
		count := ""
		if item.Windows > 0 {
			count = windowCountLabel(m.terms, item.Windows)
		}
		cells := rowCells{right: []metaCell{attached, metaCol(m.styles.muted, count)}}
		if counts, ok := m.sessionAgentCountsCell(item); ok {
			cells.right = append(cells.right, counts)
		}
		return cells
	case sessionmgr.KindHost:
		// The alias herdr is handed, then the transport. An alias that only
		// repeats the row's name (a host discovered in ~/.ssh/config) is dropped,
		// which leaves the transport as the row's only column. The raw target is
		// spelled out in the preview instead.
		alias := item.Target
		if alias == item.DisplayName() {
			alias = ""
		}
		return rowCells{right: []metaCell{
			metaCol(m.styles.muted, alias),
			metaCol(m.styles.muted, item.Location),
		}}
	default:
		// Directories are their own context: the path is the row.
		return rowCells{}
	}
}

// agentNameStyle is the style an agent's kind label is drawn in: its brand
// color when the vendor has one, the default foreground otherwise.
func agentNameStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(agentColor(name))
}

// agentStateCell is an agent row's state cell: the configured state glyph and
// word, in the configured state color. State mode "none" hides agent state
// entirely, and then the cell is dropped. The word is drawn as its own run so a
// configured color covers it exactly as it covers the glyph.
func (m Model) agentStateCell(state sessionmgr.AgentState) (metaCell, bool) {
	if m.icons.AgentStateHidden() {
		return metaCell{}, false
	}
	icon := m.icons.ForAgentState(state)
	style := m.agentStateStyle(state)
	if !m.icons.AgentStateUsesIcons() {
		return metaCol(style, icon.ASCII), true
	}
	return metaCell{
		plain: icon.Icon + " " + icon.ASCII,
		text:  style.Render(icon.Icon) + " " + style.Render(icon.ASCII),
	}, true
}

// sessionAgentCountsCell is a session row's last cell: the agents running inside
// the session, tallied per state as a colored glyph and its count ("● 1 ○ 1").
// States are listed in urgency order, and the cell is dropped when the session
// has no agents.
func (m Model) sessionAgentCountsCell(session sessionmgr.Item) (metaCell, bool) {
	counts := m.sessionAgentCounts(session)
	var plain, text strings.Builder
	for _, state := range agentCountOrder {
		n := counts[state]
		if n == 0 {
			continue
		}
		glyph := m.stateLabel(state)
		if plain.Len() > 0 {
			plain.WriteString(" ")
			text.WriteString(" ")
		}
		// The glyph carries the state color; the count stays in the row's own
		// foreground so the runs of numbers read as data, not as more glyph.
		plain.WriteString(glyph + " " + strconv.Itoa(n))
		text.WriteString(m.agentStateStyle(state).Render(glyph) + " " + strconv.Itoa(n))
	}
	if plain.Len() == 0 {
		return metaCell{}, false
	}
	return metaCell{plain: plain.String(), text: text.String()}, true
}

// agentCountOrder lists the agent states a session's count cell reports, most
// urgent first; an empty state (no agents) never appears.
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

// rowGrid is one frame's row layout: the right-flush columns of every kind the
// visible rows draw. Kinds without right cells (directories) are absent.
type rowGrid map[sessionmgr.Kind]metaGrid

// buildRowGrid measures the visible rows and resolves each kind's right-flush
// columns. Each kind resolves its own, so a long agent workspace widens the
// agent rows instead of pushing the session rows' counts around.
func buildRowGrid(rows []row, cells []rowCells, width, prefix int) rowGrid {
	kindCells := make(map[sessionmgr.Kind][][]metaCell)
	for i, r := range rows {
		kind := r.item.Kind
		kindCells[kind] = append(kindCells[kind], cells[i].right)
	}
	grid := make(rowGrid, len(kindCells))
	for kind, metas := range kindCells {
		grid[kind] = buildMetaGrid(metas, kindCellsBudget(width, prefix))
	}
	return grid
}

// kindCellsBudget is the room a row's right-flush columns may take: the window's
// inner width less the row's prefix, its right margin, the gap before the
// columns, and the least room the name on the left keeps.
func kindCellsBudget(width, prefix int) int {
	return max(1, width-prefix-rowRightMargin-rowGap-rowNameFloor)
}

// metaGrid is the resolved right-flush columns of one kind's visible rows: each
// column's width and whether it pads its cell on the left. The columns before
// keep are the ones a tight window dropped, starting with the leftmost.
type metaGrid struct {
	widths []int
	rights []bool
	keep   int
}

// total is the columns the grid draws: every kept column with the gap before it.
func (g metaGrid) total() int {
	total := 0
	for i := g.keep; i < len(g.widths); i++ {
		if i > g.keep {
			total += len(cellSep)
		}
		total += g.widths[i]
	}
	return total
}

// buildMetaGrid measures every right-flush column over the visible rows, capped
// at metaColumnMax, and then drops the leftmost columns until what is left fits
// the budget. The leftmost column is the one a row can lose and stay readable —
// the agent's age, a session's attached flag — so the name on the left is what
// the window clamps, and the state column is the last thing to go.
func buildMetaGrid(metas [][]metaCell, budget int) metaGrid {
	columns := 0
	for _, cells := range metas {
		columns = max(columns, len(cells))
	}
	grid := metaGrid{widths: make([]int, columns), rights: make([]bool, columns)}
	for _, cells := range metas {
		for i, cell := range cells {
			if w := min(metaColumnMax, lipgloss.Width(cell.plain)); w > grid.widths[i] {
				grid.widths[i] = w
			}
			grid.rights[i] = grid.rights[i] || cell.right
		}
	}
	for grid.keep < len(grid.widths) && grid.total() > budget {
		grid.keep++
	}
	return grid
}

// renderMetaCells draws a row's right-flush cells: every kept column is padded
// to its width, a cell a row has no value for stays blank so the columns after
// it line up, and a row that stops short of the grid still keeps its columns.
func renderMetaCells(cells []metaCell, grid metaGrid) string {
	var out strings.Builder
	for i := grid.keep; i < len(grid.widths); i++ {
		if i > grid.keep {
			out.WriteString(cellSep)
		}
		var cell metaCell
		if i < len(cells) {
			cell = cells[i]
		}
		out.WriteString(padMetaCell(cell.text, grid.widths[i], grid.rights[i]))
	}
	return out.String()
}

// padMetaCell draws one cell inside its column: a left-aligned cell starts on
// the column, a right-aligned one ends on it.
func padMetaCell(text string, width int, right bool) string {
	clamped := clampStyledText(text, width)
	padding := strings.Repeat(" ", max(0, width-lipgloss.Width(clamped)))
	if right {
		return padding + clamped
	}
	return clamped + padding
}

// renderRow draws one result row: the kind icon, the title, the cells that
// follow it, and the right-flush columns. Every line is exactly width cells,
// with one blank cell between the columns and the window's inner edge, and the
// selected row is painted with the selection bar across the full width.
func (m Model) renderRow(r row, cells rowCells, meta metaGrid, width int, selected bool) string {
	iconWidth := m.rowIconWidth()
	icon := pad(clampStyledText(m.rowIcon(r.item), iconWidth), iconWidth)
	prefix := " " + icon + " "
	right := renderMetaCells(cells.right, meta)
	room := width - lipgloss.Width(prefix)
	if right != "" {
		room -= lipgloss.Width(right) + rowRightMargin + rowGap
	}
	line := prefix + m.renderRowLeft(r, cells.left, max(0, room), selected)
	if right != "" {
		gap := max(rowGap, width-lipgloss.Width(line)-lipgloss.Width(right)-rowRightMargin)
		line += strings.Repeat(" ", gap) + right + strings.Repeat(" ", rowRightMargin)
	}
	line = pad(clampStyledText(line, width), width)
	if selected {
		line = m.selectionBar(line, width)
	}
	return line
}

// renderRowLeft draws the run that follows the icon: the title with its matches
// emphasized, then the cells an item keeps next to its name. The title keeps its
// room first, and a cell that no longer fits — a workspace in a narrow window —
// is clamped and then dropped rather than pushing the right-flush columns away.
func (m Model) renderRowLeft(r row, cells []metaCell, room int, selected bool) string {
	var out strings.Builder
	title := m.renderTitle(r, room, selected)
	out.WriteString(title)
	used := lipgloss.Width(title)
	for _, cell := range cells {
		if cell.plain == "" {
			continue
		}
		avail := room - used - len(cellSep)
		if avail < 1 {
			break
		}
		text := clampStyledText(cell.text, min(lipgloss.Width(cell.plain), avail))
		out.WriteString(cellSep)
		out.WriteString(text)
		used += len(cellSep) + lipgloss.Width(text)
	}
	return out.String()
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

// rowPrefixWidth is the fixed inset/icon/gap prefix every row shares: one
// column of space inside the window border, the kind icon, then the gap before
// the title.
func (m Model) rowPrefixWidth() int { return 1 + m.rowIconWidth() + 1 }

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
// emphasized. An agent's name is bright — bold while selected or while the agent
// waits on the user (blocked or done) — and its tab label is appended muted when
// there is room; a directory's parent path is dimmed and its basename kept
// bright, which is what the path's row is about.
func (m Model) renderTitle(r row, width int, selected bool) string {
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
	if selected || needsAttention(r.item) {
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
	case m.query != "":
		headline = m.styles.emphasis.Render(`no matches for "` + m.query + `"`)
		hint = "⌫ backspace clears the query"
	case m.scope != scopeAll:
		headline = m.styles.itemName.Render("nothing in ") +
			m.styles.emphasis.Render(m.scope.chip()) +
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

var pickerHints = []footerHint{
	{"↑↓", "move"},
	{"⏎", "open"},
	{"⇥", "filter"},
	{"^r", "rename"},
	{"^x", "kill"},
	{"^o", "preview"},
	{"^y", "yazi"},
	{"esc", "quit"},
}

// renderFooter draws the frame's bottom key line: the picker hints, the rename
// keys while a rename is in flight, or nothing but the confirm keys while a kill
// is armed. Hints drop from the end until the line fits.
func (m Model) renderFooter(width int) string {
	switch {
	case m.killArmed:
		return m.styles.key.Render("⏎/y") + " " + m.styles.muted.Render("confirm") +
			"  " + m.styles.key.Render("esc") + " " + m.styles.muted.Render("cancel")
	case m.renameActive:
		return m.styles.key.Render("⏎") + " " + m.styles.muted.Render("save") +
			"  " + m.styles.key.Render("esc") + " " + m.styles.muted.Render("cancel")
	}
	parts := make([]string, 0, len(pickerHints))
	for _, hint := range pickerHints {
		parts = append(parts, m.styles.key.Render(hint.key)+" "+m.styles.muted.Render(hint.label))
	}
	width2 := func() int {
		total := 0
		for i, part := range parts {
			if i > 0 {
				total += 2
			}
			total += lipgloss.Width(part)
		}
		return total
	}
	for len(parts) > 0 && width2() > width {
		parts = parts[:len(parts)-1]
	}
	return clampStyledText(strings.Join(parts, "  "), width)
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
	innerH := max(11, len(names)+7)

	lines := []string{
		s.title.Render("Install agent integrations"),
		s.muted.Render("Hooks/plugins report state to seshagy in real time."),
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
