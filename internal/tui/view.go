package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// previewMinWidth gates the list+preview split.
const previewMinWidth = 110

// tabWidthUnset skips tab width limits when terminal width is unknown.
const tabWidthUnset = 9999

// safeWidth is one column shy of the terminal to avoid auto-wrap at the right edge.
func safeWidth(w int) int {
	if w <= 0 {
		return tabWidthUnset
	}
	return max(1, w-1)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	s := m.styles
	if m.setup.active {
		frame := s.app.Width(m.width).Height(m.height).Render(m.renderSetupPrompt(m.height))
		return m.overlayNotifications(frame)
	}
	if m.installMenu.active {
		frame := s.app.Width(m.width).Height(m.height).Render(m.renderInstallMenu(m.height))
		return m.overlayNotifications(frame)
	}
	header := m.renderTopRow()
	footer := m.renderFooter()
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	body := m.renderBody(bodyH)
	frame := joinFrame(header, body, footer, m.width, m.height)
	if !m.inputPopupActive() {
		return m.overlayNotifications(frame)
	}
	// Search/rename input floats as a centered popup over a dimmed copy of
	// the normal frame. Each bg line is independently grayed so the dim
	// survives the per-line ANSI-aware splice in overlay. Dimming can be
	// disabled via [tui].dim_background = false.
	bg := frame
	if m.config.TUI.DimBackground != nil && *m.config.TUI.DimBackground {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		frameLines := strings.Split(frame, "\n")
		for i, ln := range frameLines {
			frameLines[i] = dim.Render(ansi.Strip(ln))
		}
		bg = strings.Join(frameLines, "\n")
	}
	popup := m.renderInputPopup()
	x := max(0, (m.width-lipgloss.Width(popup))/2)
	y := max(0, (m.height-lipgloss.Height(popup))/2)
	return m.overlayNotifications(overlay(bg, popup, x, y))
}

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

// inputPopupActive reports whether the search/rename text input is currently
// shown as a floating popup rather than inline in the footer. Below a small
// terminal size the popup is suppressed and the legacy inline field is used.
// Cmdline input style always renders in the footer instead.
func (m Model) inputPopupActive() bool {
	if m.config.TUI.InputStyle == appconfig.InputStyleCmdline {
		return false
	}
	return (m.inputMode == modeSearch || m.inputMode == modeRename) &&
		m.width >= 34 && m.height >= 5
}

// renderInputPopup renders the bordered centered popup that hosts the
// search or rename text input plus a one-line help row.
func (m Model) renderInputPopup() string {
	s := m.styles
	boxW := min(60, m.width-4)
	if boxW < 30 {
		boxW = 30
	}
	// panePopup carries a rounded border (2 cols) + 0,1 padding (2 cols).
	contentW := boxW - 4
	if contentW < 1 {
		contentW = 1
	}
	var title, inputView, help string
	switch m.inputMode {
	case modeSearch:
		title = "SEARCH"
		si := m.searchInput
		if si.Width > contentW {
			si.Width = contentW
		}
		inputView = si.View()
		help = "enter to filter · esc to cancel"
	case modeRename:
		title = "RENAME"
		ri := m.renameInput
		ri.Prompt = clampText(m.renameFrom, contentW/2) + " -> "
		if ri.Width > contentW {
			ri.Width = contentW
		}
		inputView = ri.View()
		help = "enter to rename · esc to cancel"
	}
	content := inputView + "\n" + clampText(s.muted.Render(help), contentW)
	return paneWithTitle(s.panePopup, s.helpTileTitle, content, title, boxW, 0)
}

func (m Model) renderSetupPrompt(height int) string {
	s := m.styles
	width := max(54, min(88, m.width-4))
	innerW := max(44, width-4)
	innerH := 11
	title := "Choose startup input mode"
	if m.setup.manual {
		title = "Change input mode"
	}
	lines := []string{
		s.title.Render(title),
		s.muted.Render("Type-first mode lets normal typing filter immediately."),
		s.muted.Render(
			"App actions then require the configured prefix key (" + m.config.PrefixKey() + ").",
		),
		"",
	}
	choices := []struct {
		label string
		desc  string
	}{
		{"Enable type-first mode", "typing filters; " + m.config.PrefixKey() + " runs actions"},
		{"Keep classic mode", "/ starts filtering; action keys work directly"},
	}
	for i, choice := range choices {
		cursor := "  "
		if i == m.setup.cursor {
			cursor = s.bar.Render("▌") + " "
		}
		line := cursor + choice.label + s.muted.Render(" — "+choice.desc)
		if i == m.setup.cursor {
			line = s.selectedBG.Render(pad(line, innerW))
		}
		lines = append(lines, line)
	}
	helpParts := []string{
		s.key.Render("enter") + " select",
		s.key.Render("y") + " type-first",
		s.key.Render("n") + " classic",
	}
	if m.setup.manual {
		helpParts = append(helpParts, s.key.Render("esc")+" cancel")
	}
	helpParts = append(helpParts, s.key.Render("q")+" quit")
	lines = append(lines, "", strings.Join(helpParts, s.muted.Render(" · ")))
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	box := s.panePopup.Width(width - 2).Height(innerH).Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
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
		lines = append(lines, "", s.muted.Render(msg))
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

func (m Model) renderSourcesTile(width int) string {
	s := m.styles
	inner := max(1, width-4) // border (2) + horizontal padding (2)
	chips := m.renderSourceChips(inner)
	return paneWithTitle(s.tileSources, s.sourcesTileTitle, chips, "SOURCES", width, 0)
}

// renderSourceChips builds the source-tab chip line (active tab in the
// active_tab color, others muted), joined by a muted middot, with labels
// falling back key+name -> name -> key to fit, and a right-aligned visible-count
// badge (vis/total when filtering). The line is clamped to width.
func (m Model) renderSourceChips(width int) string {
	s := m.styles
	maxW := width
	tabs := m.sourceTabs()

	try := func(format string) string {
		parts := make([]string, 0, len(tabs))
		for _, tab := range tabs {
			var label string
			switch format {
			case "key-name":
				label = tab.key + " " + tab.name
			case "name":
				label = tab.name
			default:
				label = tab.key
			}
			if tab.mode == m.source {
				parts = append(parts, s.chipActive.Render(label))
			} else {
				parts = append(parts, s.chipIdle.Render(label))
			}
		}
		return strings.Join(parts, s.muted.Render(" | "))
	}

	line := try("key-name")
	if lipgloss.Width(line) > maxW {
		line = try("name")
	}
	if lipgloss.Width(line) > maxW {
		line = try("key")
	}

	// Right-aligned visible-count badge when room allows.
	items := m.visibleItems()
	count := fmt.Sprintf("%d", len(items))
	if m.query != "" {
		count = fmt.Sprintf("%d/%d", len(items), len(m.items))
	}
	if m.loading || m.refreshInflight(m.source) {
		frames := []rune(spinnerFrames)
		count += " " + string(frames[m.spinnerFrame%len(frames)])
	}
	return composeLine(line, count, maxW, s.muted)
}

type sourceTab struct {
	key  string
	name string
	mode sessionmgr.SourceMode
}

func (m Model) sourceTabs() []sourceTab {
	order := m.config.SourceOrder()
	tabs := make([]sourceTab, 0, len(order))
	for i, mode := range order {
		tabs = append(tabs, sourceTab{
			key:  fmt.Sprintf("%d", i+1),
			name: mode.DisplayNames(m.terms).Tab,
			mode: mode,
		})
	}
	return tabs
}

// renderTopRow renders the header as a single row of tiles: SOURCES (flex),
// AGENTS (compact), and WORKSPACES (compact) when the overview is active,
// otherwise just the SOURCES tile at full width. Collapsing the previous
// two-row header (overview + sources) reclaims a row for the list.
func (m Model) renderTopRow() string {
	s := m.styles
	usableW := safeWidth(m.width)
	overview := m.overviewItems()
	if len(overview) == 0 || m.height < 14 {
		return m.renderSourcesTile(usableW)
	}
	stats := aggregateOverviewStats(overview)
	icons := m.config.IconSet()

	gap := 1
	wsW := clampVal(22, 16, usableW/6)
	agentW := clampVal(34, 26, usableW/3)
	sourcesW := usableW - wsW - agentW - 2*gap
	if sourcesW < 20 {
		// Not enough room for all three tiles — fall back to SOURCES alone.
		return m.renderSourcesTile(usableW)
	}

	wsTitle := strings.ToUpper(m.terms.SessionPlural)
	wsContent := fmt.Sprintf(
		"%s %s",
		s.emphasis.Render(fmt.Sprintf("%d", stats.sessions)),
		s.muted.Render(fmt.Sprintf("(%d attached)", stats.attached)),
	)
	wsTile := paneWithTitle(s.tileWorkspace, s.workspaceTileTitle, wsContent, wsTitle, wsW, 0)

	agentContent := m.agentChips(icons, stats)
	agentTile := paneWithTitle(s.tileAgent, s.agentTileTitle, agentContent, "AGENTS", agentW, 0)

	sourcesTile := m.renderSourcesTile(sourcesW)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sourcesTile, strings.Repeat(" ", gap),
		agentTile, strings.Repeat(" ", gap),
		wsTile,
	)
}

// agentChips renders the colored agent-state chips for the overview tile,
// reusing the configured icon-set glyphs/colors via renderAgentState. All five
// states are always shown (with 0 counts) so the tile reads as a fixed legend
// rather than appearing empty.
func (m Model) agentChips(icons sessionmgr.IconSet, stats overviewStats) string {
	s := m.styles
	parts := make([]string, 0, len(agentStateOrder))
	for _, state := range agentStateOrder {
		count := stats.agents[state]
		glyph := renderAgentState(s, icons, state)
		iconStyle := icons.ForAgentState(state)
		cnt := renderAgentStateStyled(s, state, fmt.Sprintf("%d", count), iconStyle.Color)
		parts = append(parts, glyph+" "+cnt)
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderBody(height int) string {
	usableW := safeWidth(m.width)
	gap := 2
	if m.width < previewMinWidth || !m.showPreview {
		gap = 0
	}
	leftW := usableW
	rightW := 0
	if m.showPreview && m.width >= previewMinWidth {
		leftW = max(34, (usableW-gap)/2)
		if leftW > 72 {
			leftW = 72
		}
		rightW = usableW - leftW - gap
	}
	left := m.renderListPane(leftW, height)
	if rightW <= 0 {
		return left
	}
	right := m.renderRightPane(rightW, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

// titledTopEdge builds the top border line of a rounded pane at display width
// w with title overlaid fieldset-style: ╭─ title ──╮. The corners and dashes
// are rendered in borderFG; the title text is rendered in titleFG, so a pane
// can give its title a distinct color from its border. When titleFG == borderFG
// the edge is monochrome (the default). lipgloss v1.1.0 has no native
// border-title API, so this is composed by hand. Empty titles and very narrow
// widths fall back to a plain dashed edge in borderFG.
func titledTopEdge(title string, w int, borderFG, titleFG lipgloss.TerminalColor) string {
	if title == "" || w < 7 {
		return lipgloss.NewStyle().Foreground(borderFG).
			Render("╭" + strings.Repeat("─", max(0, w-2)) + "╮")
	}
	// Layout: ╭─ (3) + title + space (1) + dashes + ╮ (1); keep >= 1 trailing dash.
	clamped := clampText(title, w-6)
	dashes := w - 5 - lipgloss.Width(clamped)
	if dashes < 1 {
		dashes = 1
	}
	border := lipgloss.NewStyle().Foreground(borderFG)
	return border.Render("╭─ ") +
		lipgloss.NewStyle().Foreground(titleFG).Render(clamped) +
		border.Render(" "+strings.Repeat("─", dashes)+"╮")
}

// paneWithTitle renders a pane via style (width/height applied as for the
// other pane renderers) and overlays title onto its top border. The border
// line color comes from the style's own border foreground; the title text
// color comes from titleFG, which by default matches the pane border (via
// theme inheritance) so the edge is monochrome unless a distinct title color
// is configured.
func paneWithTitle(
	style lipgloss.Style,
	titleFG lipgloss.TerminalColor,
	content, title string,
	width, height int,
) string {
	boxStyle := style.Width(width - 2)
	if height > 0 {
		boxStyle = boxStyle.Height(height - 2)
	}
	box := boxStyle.Render(content)
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	w := lipgloss.Width(lines[0])
	if w < 3 {
		return box
	}
	lines[0] = titledTopEdge(title, w, style.GetBorderTopForeground(), titleFG)
	return strings.Join(lines, "\n")
}

func (m Model) renderListPane(width, height int) string {
	s := m.styles
	innerW := max(10, width-4)
	innerH := max(3, height-2)
	items := m.visibleItems()
	// Count the visible (filtered) items so the All-tab breakdown stays
	// consistent with the leading total when a filter is active.
	counts := sortedCounts(items)
	titleName := m.source.DisplayNames(m.terms).Title
	title := fmt.Sprintf("%s (%d", titleName, len(items))
	if m.query != "" {
		title += fmt.Sprintf("/%d match", len(m.items))
		if len(items) != 1 {
			title += "es"
		}
	}
	queryTitle := ""
	if m.query != "" && m.config.TypeFirst.Enabled && m.inputMode != modeSearch {
		queryTitle = " · " + clampText(m.query, 21)
	}
	title += queryTitle + ")"
	if m.source == sessionmgr.ModeAll {
		title = fmt.Sprintf(
			"All (%d · %d %s · %d agents · %d dirs)",
			len(items),
			counts[sessionmgr.KindSession],
			m.terms.SessionPlural,
			counts[sessionmgr.KindAgent],
			counts[sessionmgr.KindZoxide]+counts[sessionmgr.KindFD],
		)
		if queryTitle != "" {
			title = strings.TrimSuffix(title, ")") + queryTitle + ")"
		}
	}
	if m.source == sessionmgr.ModeAgents {
		scope := "all"
		if m.agentsCurrentOnly {
			if m.currentSession == "" {
				scope = "current " + m.terms.SessionNoun
			} else {
				scope = m.currentSessionLabel()
			}
		}
		title = fmt.Sprintf("Agents (%d · %s%s)", len(items), scope, queryTitle)
		if m.agentsStateFilter != "" {
			title += " · state: " + string(m.agentsStateFilter)
		}
	}
	lines := []string{}
	if m.loading {
		lines = append(lines, s.muted.Render("refreshing…"))
	}
	if len(items) == 0 {
		empty := "no items"
		if m.query != "" {
			empty = "no matches for " + m.query
		}
		lines = append(lines, "", s.muted.Render(empty))
	} else {
		start, end := visibleWindow(len(items), m.cursor, max(1, innerH-1))
		if start > 0 {
			lines = append(lines, s.muted.Render(fmt.Sprintf("  ↑ %d more", start)))
		}
		for idx := start; idx < end; idx++ {
			lines = append(lines, m.renderRow(items[idx], idx == m.cursor, innerW))
		}
		if end < len(items) {
			lines = append(lines, s.muted.Render(fmt.Sprintf("  ↓ %d more", len(items)-end)))
		}
	}
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	return paneWithTitle(s.paneList, s.listTitle, content, title, width, height)
}

func (m Model) renderRow(item sessionmgr.Item, selected bool, width int) string {
	s := m.styles
	prefix := "  "
	if selected {
		prefix = s.bar.Render("▌") + " "
	}
	primary, trailing := m.rowParts(item)
	bodyW := max(1, width-2)
	line := prefix + composeLine(primary, trailing, bodyW, s.muted)
	if selected {
		line = s.selectedBG.Render(pad(line, width))
	}
	return line
}

func (m Model) rowParts(item sessionmgr.Item) (string, string) {
	s := m.styles
	switch item.Kind {
	case sessionmgr.KindSession:
		icons := m.config.IconSet()
		var state string
		if !icons.TmuxStateHidden() {
			state = renderTmuxState(s, icons, item.Attached)
			if icons.TmuxStateUsesLabels() {
				state = renderTmuxStateLabel(s, icons, item.Attached)
			}
		}
		name := s.itemName.Render(item.Name)
		return rowText(m.iconFor(item.Kind), state, name), ago(item.Activity)
	case sessionmgr.KindZoxide:
		return rowText(m.iconFor(item.Kind), s.itemName.Render(item.Path)), "zoxide"
	case sessionmgr.KindFD:
		return rowText(m.iconFor(item.Kind), s.itemName.Render(item.Path)), "fd"
	case sessionmgr.KindAgent:
		icons := m.config.IconSet()
		var glyph string
		if !icons.AgentStateHidden() {
			glyph = renderAgentState(s, icons, item.AgentState)
			if !icons.AgentStateUsesIcons() {
				glyph = renderAgentStateLabel(s, icons, item.AgentState)
			}
		}
		name := s.itemName.Render(item.DisplayName())
		return rowText(glyph, name), item.Location
	default:
		return item.DisplayName(), ""
	}
}

func (m Model) iconFor(kind sessionmgr.Kind) string {
	icon := m.config.IconSet().For(kind)
	if icon.Text == "" {
		return ""
	}
	style := lipgloss.NewStyle().Bold(true)
	if icon.Color != "" {
		style = style.Foreground(lipgloss.Color(icon.Color))
	}
	return style.Render(icon.Text)
}

func rowText(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	var b strings.Builder
	for i, part := range kept {
		if i > 0 && !strings.HasSuffix(sessionmgr.StripANSI(b.String()), " ") &&
			!strings.HasPrefix(sessionmgr.StripANSI(part), " ") {
			b.WriteString(" ")
		}
		b.WriteString(part)
	}
	return b.String()
}

func (m Model) renderRightPane(width, height int) string {
	if !m.showPreview {
		return m.renderDetailPane(width, height)
	}
	// The detail pane adapts to its content (no trailing blank padding); the
	// preview pane fills the remainder. At short heights, cap the detail tile
	// so the preview keeps a three-row floor and the stacked panes never exceed
	// the body height passed down by View.
	const (
		paneGapH    = 1
		previewMinH = 3
	)
	detail := m.renderDetailPane(width, 0)
	detailH := lipgloss.Height(detail)
	maxDetailH := height - paneGapH - previewMinH
	if detailH > maxDetailH {
		detail = m.renderDetailPane(width, maxDetailH)
		detailH = lipgloss.Height(detail)
	}
	previewH := height - detailH - paneGapH
	preview := m.renderPreviewPane(width, previewH)
	return lipgloss.JoinVertical(lipgloss.Left, detail, "", preview)
}

func (m Model) renderDetailPane(width, height int) string {
	s := m.styles
	item, ok := m.selectedItem()
	title := "Details"
	var lines []string
	if !ok {
		lines = []string{s.muted.Render("select an item")}
	} else {
		title = m.detailTitle(item)
		lines = m.detailLines(item)
	}
	content := strings.Join(lines, "\n")
	// height <= 0 means "size to content": render at the body's natural height
	// (no trailing padding) so the preview pane can fill the remainder.
	if height > 0 {
		content = trimHeight(content, max(1, height-2))
	}
	return paneWithTitle(s.paneDetail, s.metadataTitle, content, title, width, height)
}

// detailTitle returns the title shown on the detail pane border: the selected
// item's name and kind (backend-aware vocabulary). The body drops these lines.
func (m Model) detailTitle(item sessionmgr.Item) string {
	switch item.Kind {
	case sessionmgr.KindSession:
		return item.Name + " · " + m.terms.BackendName + " " + m.terms.SessionNoun
	case sessionmgr.KindAgent:
		return item.DisplayName() + " · agent"
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return sessionmgr.SessionNameFromDir(item.Path) + " · " + string(item.Kind) + " directory"
	default:
		return item.DisplayName()
	}
}

func (m Model) detailLines(item sessionmgr.Item) []string {
	s := m.styles
	switch item.Kind {
	case sessionmgr.KindSession:
		icons := m.config.IconSet()
		attached := renderTmuxStateDetail(s, item.Attached, icons)
		lines := []string{
			kv(s, "path", sessionmgr.ContractHome(item.Path)),
			kv(s, "attached", attached),
			kv(s, m.terms.WindowPlural, fmt.Sprint(item.Windows)),
		}
		// Pane count and timestamps are optional: only herdr reports a pane count,
		// and herdr exposes no created/activity. Omit the rows when absent instead of
		// showing a misleading "unknown".
		if item.Panes > 0 {
			lines = append(lines, kv(s, m.terms.PanePlural, fmt.Sprint(item.Panes)))
		}
		if !item.Activity.IsZero() {
			lines = append(lines, kv(s, "activity", ago(item.Activity)))
		}
		if !item.Created.IsZero() {
			lines = append(lines, kv(s, "created", ago(item.Created)))
		}
		return lines
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return []string{
			kv(s, "path", item.Path),
			kv(s, "enter", "create/switch "+m.terms.BackendName+" "+m.terms.SessionNoun),
		}
	case sessionmgr.KindAgent:
		icons := m.config.IconSet()
		stateLabel := agentStateText(item.AgentState)
		if !icons.AgentStateHidden() {
			stateLabel = rowText(
				renderAgentState(s, icons, item.AgentState),
				agentStateText(item.AgentState),
			)
		}
		lines := []string{
			kv(s, "state", stateLabel),
			kv(s, "location", item.Location),
		}
		// Under herdr, item.Session is an opaque workspace id and the location
		// line already shows the resolved workspace label — skip the redundant
		// id-leaking row. Under tmux the session name is still useful context.
		if m.terms.BackendName != "herdr" {
			lines = append(lines, kv(s, m.terms.SessionNoun, item.Session))
		}
		// Under herdr, show which tab the agent lives in (resolved label, not
		// the opaque tab id stored in Window).
		if item.TabLabel != "" {
			lines = append(lines, kv(s, m.terms.WindowNoun, item.TabLabel))
		}
		lines = append(lines, kv(s, "path", sessionmgr.ContractHome(item.Path)))
		return lines
	default:
		return nil
	}
}

func (m Model) renderPreviewPane(width, height int) string {
	s := m.styles
	innerW := max(10, width-4)
	innerH := max(1, height-2)
	title := "Preview"
	if item, ok := m.selectedItem(); ok {
		title = "Preview · " + item.DisplayName()
	}
	content := m.preview
	if content == "" {
		content = s.muted.Render("preview loading…")
	}
	lines := []string{}
	contentH := max(1, innerH-len(lines))
	previewLines := strings.Split(content, "\n")
	// tmux-captured panes (sessions, agents) are bottom-anchored: when content
	// overflows, show the most recent lines so the preview starts from the
	// bottom of the tmux pane. Directory listings stay top-down.
	start := 0
	if item, ok := m.selectedItem(); ok && isTailPreviewKind(item.Kind) &&
		len(previewLines) > contentH {
		start = len(previewLines) - contentH
	}
	for i := start; i-start < contentH && i < len(previewLines); i++ {
		lines = append(lines, clampText(previewLines[i], innerW))
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return paneWithTitle(
		s.panePreview,
		s.previewTitle,
		trimHeight(strings.Join(lines, "\n"), innerH),
		title,
		width,
		height,
	)
}

// isTailPreviewKind reports whether a kind's preview is sourced from a
// multiplexer capture (tmux capture-pane or herdr pane read), both of which
// are bottom-anchored, rather than a top-down directory listing.
func isTailPreviewKind(kind sessionmgr.Kind) bool {
	return kind == sessionmgr.KindSession || kind == sessionmgr.KindAgent
}

func (m Model) renderFooter() string {
	s := m.styles
	var help string
	if m.showHelp {
		if m.config.TypeFirst.Enabled && !m.prefixArmed {
			help = strings.Join([]string{
				s.key.Render("type") + " filter",
				s.key.Render(m.config.PrefixKey()) + " actions",
				s.key.Render(m.config.PrefixKey()+" m") + " mode",
				s.key.Render("backspace") + " edit",
			}, s.muted.Render(" · "))
		} else {
			helpParts := []string{
				s.key.Render("?") + " help",
				s.key.Render("tab/⇧+tab") + " sections",
				s.key.Render("q") + " quit",
				s.key.Render("enter") + " attach/create/focus",
				s.key.Render("/") + " filter",
				s.key.Render("r") + " refresh",
				s.key.Render("p") + " preview",
				s.key.Render("m") + " mode",
				s.key.Render("h") + " install",
			}
			if m.source == sessionmgr.ModeAgents {
				helpParts = append(helpParts,
					s.key.Render("o")+" this session",
					s.key.Render("s")+" filter state",
					s.key.Render("R")+" rename",
				)
			} else {
				helpParts = append(helpParts,
					s.key.Render("R")+" rename",
					s.key.Render("x")+" kill",
					s.key.Render("y")+" yazi",
				)
			}
			help = strings.Join(helpParts, s.muted.Render(" · "))
		}
	} else {
		help = s.muted.Render("? help")
	}
	if m.prefixArmed {
		help = s.warning.Bold(true).Render("PREFIX") + " " + help
	}
	// The footer is just the help line. The old status strip (backend indicator,
	// loaded/refreshing status, errors) is gone because the overview tiles now
	// carry the counts and active source. The one exception is cmdline input
	// style: while actively searching/renaming, render the text input as a
	// one-line footer above the help (popup style uses its own overlay).
	footerW := safeWidth(m.width)
	contentW := max(1, footerW-2)
	helpLine := clampText(help, max(1, footerW-4))
	helpTile := paneWithTitle(s.tileHelp, s.helpTileTitle, helpLine, "HELP", footerW, 0)
	if (m.inputMode == modeSearch || m.inputMode == modeRename) &&
		m.config.TUI.InputStyle == appconfig.InputStyleCmdline {
		var ti textinput.Model
		switch m.inputMode {
		case modeSearch:
			ti = m.searchInput
		case modeRename:
			ti = m.renameInput
			ti.Prompt = clampText(m.renameFrom, contentW/2) + " -> "
		}
		if w := contentW - lipgloss.Width(ti.Prompt); w > 0 {
			ti.Width = w
		}
		inputLine := clampText(ti.View(), contentW)
		return inputLine + "\n" + helpTile
	}
	return helpTile
}

func renderTmuxState(s styles, icons sessionmgr.IconSet, attached bool) string {
	style := icons.ForTmuxState(attached)
	return renderTmuxStateStyled(s, attached, style.Icon, style.Color)
}

func renderTmuxStateLabel(s styles, icons sessionmgr.IconSet, attached bool) string {
	style := icons.ForTmuxState(attached)
	label := style.ASCII
	if label == "" {
		label = tmuxStateText(attached)
	}
	return renderTmuxStateStyled(s, attached, "["+label+"]", style.Color)
}

func renderTmuxStateRaw(s styles, icons sessionmgr.IconSet, attached bool) string {
	style := icons.ForTmuxState(attached)
	label := style.ASCII
	if label == "" {
		label = tmuxStateText(attached)
	}
	return renderTmuxStateStyled(s, attached, label, style.Color)
}

func renderTmuxStateStyled(s styles, attached bool, text, color string) string {
	if strings.TrimSpace(color) != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
	}
	if attached {
		return s.success.Render(text)
	}
	return s.muted.Render(text)
}

func renderTmuxStateDetail(
	s styles,
	attached bool,
	icons sessionmgr.IconSet,
) string {
	if icons.TmuxStateHidden() {
		if attached {
			return "yes"
		}
		return "no"
	}
	if icons.TmuxStateUsesIcons() {
		style := icons.ForTmuxState(attached)
		label := style.ASCII
		if label == "" {
			label = tmuxStateText(attached)
		}
		return rowText(renderTmuxState(s, icons, attached), label)
	}
	return renderTmuxStateRaw(s, icons, attached)
}

func tmuxStateText(attached bool) string {
	if attached {
		return "attached"
	}
	return "detached"
}

func renderAgentState(s styles, icons sessionmgr.IconSet, state sessionmgr.AgentState) string {
	style := icons.ForAgentState(state)
	return renderAgentStateStyled(s, state, style.Icon, style.Color)
}

func renderAgentStateLabel(s styles, icons sessionmgr.IconSet, state sessionmgr.AgentState) string {
	style := icons.ForAgentState(state)
	label := style.ASCII
	if label == "" {
		label = agentStateText(state)
	}
	return renderAgentStateStyled(s, state, "["+label+"]", style.Color)
}

func renderAgentStateStyled(s styles, state sessionmgr.AgentState, text, color string) string {
	if strings.TrimSpace(color) != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
	}
	return agentStateFallback(s, state, text)
}

func agentStateText(state sessionmgr.AgentState) string {
	return string(state)
}

func agentStateFallback(s styles, state sessionmgr.AgentState, text string) string {
	switch state {
	case sessionmgr.AgentWorking:
		return s.success.Render(text)
	case sessionmgr.AgentBlocked:
		return s.warning.Render(text)
	case sessionmgr.AgentDone:
		return s.info.Render(text)
	case sessionmgr.AgentUnknown:
		return s.muted.Render(text)
	default:
		return s.muted.Render(text)
	}
}

func kv(s styles, key, value string) string {
	return fmt.Sprintf("%-9s %s", s.muted.Render(key), value)
}

func composeLine(left, right string, width int, rightStyle lipgloss.Style) string {
	if right == "" {
		return clampText(left, width)
	}
	right = rightStyle.Render(right)
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	if leftW+rightW+1 > width {
		return clampText(left, width)
	}
	return left + strings.Repeat(" ", width-leftW-rightW) + right
}

func pad(line string, width int) string {
	if lipgloss.Width(line) >= width {
		return line
	}
	return line + strings.Repeat(" ", width-lipgloss.Width(line))
}

// joinFrame stacks UI blocks without lipgloss.JoinVertical, which pads every
// line to the widest line in the frame and can push the tab bar past the pane.
func joinFrame(header, body, footer string, width, height int) string {
	safeW := safeWidth(width)
	lines := make([]string, 0, height)
	appendBlock := func(block string) {
		for _, line := range strings.Split(block, "\n") {
			if len(lines) >= height {
				return
			}
			if lipgloss.Width(line) > safeW {
				line = clampText(line, safeW)
			}
			lines = append(lines, line)
		}
	}
	appendBlock(header)
	appendBlock(body)
	appendBlock(footer)
	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func trimHeight(s string, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func visibleWindow(total, cursor, height int) (int, int) {
	if total <= height {
		return 0, total
	}
	half := height / 2
	start := cursor - half
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = total - height
	}
	return start, start + height
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
