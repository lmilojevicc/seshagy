package tui

import (
	"fmt"
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
	// resultsPercent is the stacked layouts' share of the rows left after the
	// prompt window; the preview window takes the rest.
	resultsPercent = 45
	// minResultsRows is the shortest results window that still shows a row.
	minResultsRows = 3
	// footerMinHeight is the shortest frame that keeps the footer key line.
	footerMinHeight = 6
	// metaColumnMax caps each right-aligned meta column.
	metaColumnMax = 18
	// titleMinWidth is the room a title keeps before meta columns are dropped.
	titleMinWidth = 12
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
	metas := make([][]metaCell, len(visible))
	for i, r := range visible {
		metas[i] = m.rowMeta(r)
	}
	grid := buildMetaGrid(metas, width-m.rowPrefixWidth()-titleMinWidth-2)
	lines = make([]string, 0, len(visible))
	for i, r := range visible {
		lines = append(lines, m.renderRow(r, metas[i], grid, width, start+i == m.cursor))
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
// window title and the captured content inside it.
func (m Model) renderPreviewWindow(l viewLayout) []string {
	title := m.styles.muted.Render("Preview")
	if item, ok := m.selectedItem(); ok {
		// The name comes from the multiplexer, so it is sanitized like the
		// capture itself: a control character must not reach the border.
		title = m.styles.titlePreview.Render(sanitizePreviewLine(itemTitle(item)))
	}
	lines := m.renderPreviewContent(max(0, l.preview.width-2), max(0, l.preview.height-2))
	return drawWindow(title, lines, l.preview.width, l.preview.height, m.styles.border, "")
}

// cursorBlock is the reverse-video cell that marks the typing position.
func (m Model) cursorBlock() string {
	return lipgloss.NewStyle().Reverse(true).Render(" ")
}

func spinnerFrame(frame int) string {
	frames := []rune(spinnerFrames)
	return string(frames[frame%len(frames)])
}

// renderPromptContent draws the prompt window's content row: the accent prompt
// and the query (or the muted placeholder) with the typing cursor, the dim sigil
// hint, and the shown/total count right-aligned. A kill confirmation or an
// in-flight rename replaces the row.
func (m Model) renderPromptContent(width int) string {
	s := m.styles
	switch {
	case m.killArmed:
		return m.renderKillConfirm(width)
	case m.renameActive:
		left := s.title.Render("rename") + " " + s.emphasis.Render("❯") + " " +
			m.renameText + m.cursorBlock()
		return clampStyledText(left, width)
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
	right := m.promptTrailing(width)
	if right == "" {
		return clampStyledText(left, width)
	}
	room := width - lipgloss.Width(right)
	return clampStyledText(left, room) +
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

// promptCount is the "shown/total" the prompt window reports, collapsed to
// "total" when nothing is filtered out.
func (m Model) promptCount() string {
	shown, total := len(m.rows()), len(m.items)
	if shown == total {
		return m.styles.muted.Render(fmt.Sprintf("%d", total))
	}
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

// metaCell is one right-aligned row column: the plain text used for width
// computation and the styled text actually drawn.
type metaCell struct {
	plain string
	text  string
}

func styledCell(style lipgloss.Style, text string) metaCell {
	return metaCell{plain: text, text: style.Render(text)}
}

func (m Model) mutedCell(text string) metaCell {
	return styledCell(m.styles.muted, text)
}

// rowMeta builds the right-aligned meta columns for a row.
func (m Model) rowMeta(r row) []metaCell {
	item := r.item
	switch item.Kind {
	case sessionmgr.KindAgent:
		cells := make([]metaCell, 0, 4)
		if item.Location != "" && item.Location != itemTitle(item) {
			cells = append(cells, m.mutedCell(item.Location))
		}
		if !item.StartedAt.IsZero() {
			cells = append(cells, m.mutedCell(agentAgeAt(item.StartedAt, time.Now())))
		}
		// The logo already says which agent this is; the colored kind label is
		// only worth the columns when no logo is drawn.
		if !m.showAgentLogos() {
			if label := agentKey(item.AgentName); label != "" {
				cells = append(cells,
					styledCell(lipgloss.NewStyle().Foreground(agentColor(item.AgentName)), label))
			}
		}
		if !m.icons.AgentStateHidden() {
			state := m.icons.ForAgentState(item.AgentState)
			cells = append(cells, styledCell(m.agentStateStyle(item.AgentState), state.ASCII))
		}
		return cells
	case sessionmgr.KindSession:
		// The attached flag comes first and the window count last, with an empty
		// placeholder in the attached column when the session is not attached, so
		// the count sits in the last column on every session row.
		cells := make([]metaCell, 0, 2)
		if item.Attached {
			cells = append(cells, styledCell(m.styles.success, "attached"))
		} else {
			cells = append(cells, metaCell{})
		}
		if item.Windows > 0 {
			cells = append(cells, m.mutedCell(windowCountLabel(m.terms, item.Windows)))
		}
		return cells
	case sessionmgr.KindZoxide:
		return []metaCell{m.mutedCell("zoxide")}
	case sessionmgr.KindFD:
		label := "fd"
		if !sessionmgr.FDAvailable() {
			label = "find"
		}
		return []metaCell{m.mutedCell(label)}
	case sessionmgr.KindHost:
		cells := make([]metaCell, 0, 2)
		if item.Location != "" {
			cells = append(cells, m.mutedCell(item.Location))
		}
		// The row shows the raw target; the alias handed to herdr is the item's
		// action target and is spelled out in the preview instead.
		target := item.Path
		if target == "" {
			target = item.ActionTarget()
		}
		if target != "" && target != item.DisplayName() {
			cells = append(cells, m.mutedCell(target))
		}
		return cells
	default:
		return nil
	}
}

// metaColumnMin is the width a meta column can shrink to before its text is
// ellipsized; metaColumnMax caps a column so one long value cannot dominate.
const metaColumnMin = 6

// metaGrid is the right-aligned table the visible rows' meta cells share: the
// rightmost cell of every row sits in the last column, so rows that carry fewer
// cells (a session has no agent kind) leave the leading columns blank instead of
// shifting their own cells left.
type metaGrid struct {
	widths []int
}

func (g metaGrid) total() int {
	total := 0
	for i, w := range g.widths {
		if i > 0 {
			total += 2
		}
		total += w
	}
	return total
}

// buildMetaGrid measures every cell in its right-aligned column (capped at
// metaColumnMax) and then shrinks the least specific leftmost columns until the
// titles keep their minimum room. The rightmost column — the agent state or the
// session's window count — is never squeezed below metaColumnMin.
func buildMetaGrid(metas [][]metaCell, budget int) metaGrid {
	columns := 0
	for _, cells := range metas {
		if len(cells) > columns {
			columns = len(cells)
		}
	}
	grid := metaGrid{widths: make([]int, columns)}
	for _, cells := range metas {
		offset := columns - len(cells)
		for i, cell := range cells {
			if w := min(metaColumnMax, lipgloss.Width(cell.plain)); w > grid.widths[offset+i] {
				grid.widths[offset+i] = w
			}
		}
	}
	for i := 0; i < len(grid.widths) && grid.total() > budget; i++ {
		shortfall := min(grid.widths[i]-metaColumnMin, grid.total()-budget)
		if shortfall > 0 {
			grid.widths[i] -= shortfall
		}
	}
	return grid
}

// renderMetaCells places a row's cells in the grid's trailing columns; the
// columns the row lacks are kept blank so the block stays flush right.
func renderMetaCells(cells []metaCell, grid metaGrid) string {
	parts := make([]string, 0, len(grid.widths))
	offset := len(grid.widths) - len(cells)
	for i, w := range grid.widths {
		index := i - offset
		if index < 0 || index >= len(cells) {
			parts = append(parts, strings.Repeat(" ", w))
			continue
		}
		cell := cells[index]
		parts = append(parts, pad(clampStyledText(cell.text, w), w))
	}
	return strings.Join(parts, "  ")
}

// renderRow draws one item: selection marker, kind icon (agents add their logo),
// agent state glyph, highlighted title, then the right-aligned meta columns.
func (m Model) renderRow(r row, meta []metaCell, grid metaGrid, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = m.styles.bar.Render("▌") + " "
	}
	prefixWidth := m.rowPrefixWidth()
	iconWidth := prefixWidth - 4
	icon := pad(clampStyledText(m.rowIcon(r.item), iconWidth), iconWidth)
	state := pad(clampStyledText(m.rowStateGlyph(r.item), 2), 2)
	titleWidth := max(1, width-prefixWidth-grid.total()-2)
	left := marker + icon + state + m.renderTitle(r, titleWidth, selected)
	if len(grid.widths) == 0 {
		return pad(clampStyledText(left, width), width)
	}
	left = pad(clampStyledText(left, max(1, width-grid.total()-1)), max(1, width-grid.total()))
	return pad(clampStyledText(left+renderMetaCells(meta, grid), width), width)
}

// rowPrefixWidth is the fixed marker/icon/state prefix every row shares. The
// icon field is one cell wider when agent logos are drawn, because the icon
// font extends into the cell after the glyph.
func (m Model) rowPrefixWidth() int {
	iconWidth := 2
	if m.showAgentLogos() {
		iconWidth = 3
	}
	return 2 + iconWidth + 2
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

// rowStateGlyph is the 2-cell agent state column; in text/hidden icon modes the
// state is worded in the meta columns instead.
func (m Model) rowStateGlyph(item sessionmgr.Item) string {
	if item.Kind != sessionmgr.KindAgent || !m.icons.AgentStateUsesIcons() {
		return ""
	}
	glyph := m.icons.ForAgentState(item.AgentState).Icon
	return m.agentStateStyle(item.AgentState).Render(glyph)
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
	if repeats(itemTitle(item)) || repeats(item.Location) {
		return ""
	}
	return label
}

// renderTitle draws the item title with the fuzzy-matched runes emphasized. A
// directory's parent path is dimmed and its basename kept in the normal color;
// an agent's tab label is appended muted when there is room.
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
	if r.item.Kind == sessionmgr.KindAgent && r.item.AgentState == sessionmgr.AgentWorking {
		base = m.agentStateStyle(sessionmgr.AgentWorking)
	}
	if selected {
		base = base.Bold(true)
	}
	text := clampText(title, max(1, width-lipgloss.Width(suffix)))
	switch r.item.Kind {
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		if cut := strings.LastIndex(text, "/"); cut >= 0 && cut+1 < len(text) {
			parent, name := text[:cut+1], text[cut+1:]
			return m.highlight(parent, shiftMatches(r.matches, 0, cut+1), m.styles.muted) +
				m.highlight(name, shiftMatches(r.matches, cut+1, len(name)), base) +
				m.styles.muted.Render(suffix)
		}
	}
	return m.highlight(text, r.matches, base) + m.styles.muted.Render(suffix)
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
// height lines wide enough for the window.
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
	for _, line := range lines {
		// Captures are raw terminal output: sanitize before clipping so no
		// stray CR, tab, or cursor sequence reaches the frame. The cut is hard
		// — a preview is a window onto a terminal, and an ellipsis would read
		// as content the pane never printed. A reset closes any color the line
		// left open so it cannot bleed into the border or the next row.
		clipped := hardCutStyledText(sanitizePreviewLine(line), width)
		if style.GetForeground() != nil {
			clipped = style.Render(clipped)
		}
		out = append(out, clipped+previewReset)
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
			line = s.selectedBG.Render(pad(line, innerW))
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
