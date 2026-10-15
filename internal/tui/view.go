package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

const (
	// previewMinWidth gates the list+preview split.
	previewMinWidth = 100
	// previewPercent is the preview column's share of the frame width.
	previewPercent = 45
	// previewMinCols is the narrowest usable preview column.
	previewMinCols = 28
	// metaColumnMax caps each right-aligned meta column.
	metaColumnMax = 18
	// titleMinWidth is the room a title keeps before meta columns are dropped.
	titleMinWidth = 12
	// hostIcon marks remote-machine rows.
	hostIcon = "󰒋 "

	placeholderText = "type to search"
)

// viewLayout is one frame's row/column budget: a prompt row, optional rules, an
// optional footer, and the body in between.
type viewLayout struct {
	width        int
	bodyHeight   int
	listWidth    int
	previewWidth int
	rules        bool
	footer       bool
}

func computeViewLayout(width, height int, preview bool) viewLayout {
	rules := height >= 8
	footer := height >= 6
	chrome := 1 // prompt row (query + filter chips)
	if rules {
		chrome += 2
	}
	if footer {
		chrome++
	}
	l := viewLayout{
		width:      width,
		bodyHeight: max(1, height-chrome),
		listWidth:  width,
		rules:      rules,
		footer:     footer,
	}
	if preview {
		l.previewWidth = max(previewMinCols, width*previewPercent/100)
		l.listWidth = max(1, width-l.previewWidth-3) // " │ "
	}
	return l
}

func (m Model) viewLayout() viewLayout {
	return computeViewLayout(safeWidth(m.width), m.height, m.previewVisible())
}

// bodyHeight is the number of list rows a frame shows; page-up/down uses it.
func (m Model) bodyHeight() int {
	return m.viewLayout().bodyHeight
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
	lines := make([]string, 0, m.height)
	lines = append(lines, m.renderPromptRow(l.width))
	if l.rules {
		lines = append(lines, m.renderRule(l.width))
	}
	lines = append(lines, m.renderBody(l)...)
	if l.rules {
		lines = append(lines, m.renderRule(l.width))
	}
	if l.footer {
		lines = append(lines, m.renderFooter(l.width))
	}
	return trimHeight(strings.Join(lines, "\n"), max(1, m.height))
}

func (m Model) renderRule(width int) string {
	return m.styles.rule.Render(strings.Repeat("─", max(0, width)))
}

// composeLine left-aligns left and right-aligns right inside width, dropping the
// right side when both cannot fit.
func composeLine(left, right string, width int, rightStyle lipgloss.Style) string {
	if right == "" {
		return clampStyledText(left, width)
	}
	right = rightStyle.Render(right)
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > width {
		return clampStyledText(left, width)
	}
	return left + strings.Repeat(" ", width-lipgloss.Width(left)-lipgloss.Width(right)) + right
}

// cursorBlock is the reverse-video cell that marks the typing position.
func (m Model) cursorBlock() string {
	return lipgloss.NewStyle().Reverse(true).Render(" ")
}

func spinnerFrame(frame int) string {
	frames := []rune(spinnerFrames)
	return string(frames[frame%len(frames)])
}

// renderPromptRow draws the type-first prompt: the query (or the muted
// placeholder) with the cursor block on the left, and the filter chips
// right-aligned on the same row.
func (m Model) renderPromptRow(width int) string {
	s := m.styles
	prompt := s.emphasis.Render("❯") + " "
	var left string
	switch {
	case m.renameActive:
		left = s.title.Render("rename") + " " + prompt + m.renameText + m.cursorBlock()
	case m.query != "":
		left = prompt + m.query + m.cursorBlock()
	default:
		// An empty query types into the first cell of the input, so the cursor
		// sits right behind the prompt with the placeholder following it.
		left = prompt + m.cursorBlock() + s.muted.Render(placeholderText)
	}
	// The query always wins the row: the chips only get whatever is left.
	available := width - lipgloss.Width(left) - 1
	spinner := ""
	if !m.loadedOnce && m.spinnerActive {
		spinner = spinnerFrame(m.spinnerFrame)
		if available >= lipgloss.Width(spinner)+1 {
			available -= lipgloss.Width(spinner) + 1
		} else {
			spinner = ""
		}
	}
	right := m.renderChips(available)
	if spinner != "" {
		if right == "" {
			right = spinner
		} else {
			right += " " + spinner
		}
	}
	return composeLine(left, right, width, s.emphasis)
}

// visibleScopes lists the chips the prompt row shows, in cycle order. The
// hosts chip only appears once host items exist.
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

// chipSeparator joins the filter chips in the prompt row.
const chipSeparator = "  "

// renderChips renders the filter chips that fit in available columns. Inactive
// chips drop from the end first; the active chip is kept whenever it fits at
// all, so the current filter never disappears from the row.
func (m Model) renderChips(available int) string {
	scopes := m.visibleScopes()
	if available <= 0 || len(scopes) == 0 {
		return ""
	}
	active := 0
	for i, sc := range scopes {
		if sc == m.scope {
			active = i
		}
	}
	kept := make([]bool, len(scopes))
	for i := range kept {
		kept[i] = true
	}
	width := func() int {
		total := 0
		for i, sc := range scopes {
			if !kept[i] {
				continue
			}
			if total > 0 {
				total += len(chipSeparator)
			}
			total += len([]rune(sc.chip()))
		}
		return total
	}
	for i := len(scopes) - 1; i >= 0 && width() > available; i-- {
		if i == active {
			continue
		}
		kept[i] = false
	}
	if width() > available {
		return ""
	}
	parts := make([]string, 0, len(scopes))
	for i, sc := range scopes {
		if !kept[i] {
			continue
		}
		if i == active {
			parts = append(parts, m.styles.chipActive.Render(sc.chip()))
			continue
		}
		parts = append(parts, m.styles.chipIdle.Render(sc.chip()))
	}
	return strings.Join(parts, chipSeparator)
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

func (m Model) renderBody(l viewLayout) []string {
	if !m.loadedOnce {
		return centeredNote(l, m.styles.muted.Render("loading…"))
	}
	rows := m.rows()
	if len(rows) == 0 {
		return m.renderEmptyState(l)
	}
	start, end, counterSlots := listWindow(len(rows), m.cursor, l.bodyHeight)
	visible := rows[start:end]
	metas := make([][]metaCell, len(visible))
	for i, r := range visible {
		metas[i] = m.rowMeta(r)
	}
	grid := buildMetaGrid(metas, l.listWidth-m.rowPrefixWidth()-titleMinWidth-2)
	list := make([]string, 0, l.bodyHeight)
	if start > 0 && counterSlots > 0 {
		counterSlots--
		list = append(list, m.styles.muted.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for i, r := range visible {
		list = append(list, m.renderRow(r, metas[i], grid, l.listWidth, start+i == m.cursor))
	}
	if end < len(rows) && counterSlots > 0 {
		list = append(list, m.styles.muted.Render(fmt.Sprintf("  ↓ %d more", len(rows)-end)))
	}
	var preview []string
	if l.previewWidth > 0 {
		preview = m.renderPreview(l.previewWidth, l.bodyHeight)
	}
	out := make([]string, 0, l.bodyHeight)
	separator := m.styles.rule.Render("│")
	for i := 0; i < l.bodyHeight; i++ {
		line := ""
		if i < len(list) {
			line = list[i]
		}
		line = pad(clampStyledText(line, l.listWidth), l.listWidth)
		if l.previewWidth > 0 {
			cell := ""
			if i < len(preview) {
				cell = preview[i]
			}
			line += " " + separator + " " + pad(
				clampStyledText(cell, l.previewWidth),
				l.previewWidth,
			)
		}
		out = append(out, line)
	}
	return out
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

// renderEmptyState centers the "nothing here" message in the list area.
func (m Model) renderEmptyState(l viewLayout) []string {
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
	return centeredNote(l, headline, m.styles.muted.Render(hint))
}

// centeredNote centers one or more rendered lines in the list area.
func centeredNote(l viewLayout, lines ...string) []string {
	out := make([]string, 0, l.bodyHeight)
	top := max(0, (l.bodyHeight-len(lines))/2)
	for i := 0; i < l.bodyHeight; i++ {
		index := i - top
		if index < 0 || index >= len(lines) {
			out = append(out, "")
			continue
		}
		line := clampStyledText(lines[index], l.listWidth)
		indent := max(0, (l.listWidth-lipgloss.Width(line))/2)
		out = append(out, pad(strings.Repeat(" ", indent)+line, l.listWidth))
	}
	return out
}

// renderPreview draws the preview column: a muted header, then the captured
// content with its ANSI colors intact, bottom-anchored for multiplexer captures
// and top-anchored for directory listings.
func (m Model) renderPreview(width, height int) []string {
	item, ok := m.selectedItem()
	if !ok || height <= 0 {
		return nil
	}
	header := m.styles.muted.Render(hardCutStyledText(sanitizePreviewLine(itemTitle(item)), width))
	out := make([]string, 0, height)
	out = append(out, header)
	room := height - 1
	if room <= 0 {
		return out
	}
	content, style := m.preview, lipgloss.NewStyle()
	switch {
	case m.previewError != "":
		content, style = m.previewError, m.styles.danger
	case m.preview == "":
		content, style = "loading…", m.styles.muted
	}
	lines := strings.Split(content, "\n")
	if isTailPreviewKind(item.Kind) {
		if len(lines) > room {
			lines = lines[len(lines)-room:]
		}
		out = append(out, make([]string, max(0, room-len(lines)))...)
	} else if len(lines) > room {
		lines = lines[:room]
	}
	for _, line := range lines {
		// Captures are raw terminal output: sanitize before clipping so no
		// stray CR, tab, or cursor sequence reaches the frame. The cut is hard
		// — a preview is a window onto a terminal, and an ellipsis would read
		// as content the pane never printed. A reset closes any color the line
		// left open so it cannot bleed into the separator, the padding, or the
		// next row.
		clipped := hardCutStyledText(sanitizePreviewLine(line), width)
		if style.GetForeground() != nil {
			clipped = style.Render(clipped)
		}
		out = append(out, clipped+previewReset)
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

func (m Model) renderFooter(width int) string {
	switch {
	case m.killArmed:
		return m.renderKillConfirm(width)
	case m.renameActive:
		return m.styles.key.Render("⏎") + " " + m.styles.muted.Render("save") +
			"  " + m.styles.key.Render("esc") + " " + m.styles.muted.Render("cancel")
	}
	counter := m.footerCounter()
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
	for len(parts) > 0 && width2()+lipgloss.Width(counter)+2 > width {
		parts = parts[:len(parts)-1]
	}
	return composeLine(strings.Join(parts, "  "), counter, width, m.styles.muted)
}

// footerCounter is "shown/total", collapsed to "total" when nothing is filtered
// out.
func (m Model) footerCounter() string {
	shown, total := len(m.rows()), len(m.items)
	if shown == total {
		return fmt.Sprintf("%d", total)
	}
	return fmt.Sprintf("%d/%d", shown, total)
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
