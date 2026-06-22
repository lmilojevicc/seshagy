package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// previewMinWidth gates list+preview split and full tab labels in renderTabs.
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
		return s.app.Width(m.width).Height(m.height).Render(m.renderSetupPrompt(m.height))
	}
	header := m.renderTabs()
	footer := m.renderFooter()
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 1 {
		bodyH = 1
	}
	body := m.renderBody(bodyH)
	return joinFrame(header, body, footer, m.width, m.height)
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
	box := s.paneFocus.Width(width - 2).Height(innerH).Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderTabs() string {
	s := m.styles
	tabs := m.sourceTabs()
	all := make([]int, len(tabs))
	for i := range tabs {
		all[i] = i
	}
	maxW := safeWidth(m.width)
	tabPad := lipgloss.NewStyle().Padding(0, 1)
	render := func(full bool, sep string, brand bool, visible []int) string {
		parts := make([]string, 0, len(visible)+1)
		if brand {
			parts = append(parts, s.emphasis.Render("seshagy"))
		}
		for _, i := range visible {
			tab := tabs[i]
			label := fmt.Sprintf("[%s] %s", tab.key, tab.name)
			if !full {
				label = fmt.Sprintf("[%s]", tab.key)
			}
			if tab.mode == m.source {
				parts = append(parts, s.tabActive.Render(label))
			} else {
				parts = append(parts, s.tabInactive.Render(label))
			}
		}
		line := strings.Join(parts, sep)
		if brand || sep != "" || full {
			line = tabPad.Render(line)
		}
		return line
	}
	fitsOneLine := func(content string) bool {
		return lipgloss.Width(content) <= maxW && lipgloss.Height(content) <= 1
	}
	fitsTwoLine := func(content string) bool {
		return lipgloss.Width(content) <= maxW && lipgloss.Height(content) <= 2
	}
	tryFull := m.width <= 0 || !m.showPreview || m.width >= previewMinWidth
	if tryFull {
		line := render(true, "  ", true, all)
		if fitsOneLine(line) {
			return line
		}
	}
	layouts := []struct {
		sep   string
		brand bool
	}{
		{"  ", true},
		{" ", true},
		{" ", false},
		{"", false},
	}
	for _, layout := range layouts {
		line := render(false, layout.sep, layout.brand, all)
		if fitsOneLine(line) {
			return line
		}
	}
	brandLine := tabPad.Render(s.emphasis.Render("seshagy"))
	tabRow := tabPad.Render(render(false, "", false, all))
	twoLine := brandLine + "\n" + tabRow
	if fitsTwoLine(twoLine) {
		return twoLine
	}
	visible := append([]int(nil), all...)
	for len(visible) > 1 {
		line := render(false, "", false, visible)
		if fitsOneLine(line) {
			return line
		}
		removed := false
		for i := len(visible) - 1; i >= 0; i-- {
			if tabs[visible[i]].mode != m.source {
				visible = append(visible[:i], visible[i+1:]...)
				removed = true
				break
			}
		}
		if !removed {
			break
		}
	}
	return render(false, "", false, visible)
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
			name: mode.Names().Tab,
			mode: mode,
		})
	}
	return tabs
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

func (m Model) renderListPane(width, height int) string {
	s := m.styles
	innerW := max(10, width-4)
	innerH := max(3, height-2)
	items := m.visibleItems()
	// Count the visible (filtered) items so the All-tab breakdown stays
	// consistent with the leading total when a filter is active.
	counts := sortedCounts(items)
	titleName := m.source.Names().Title
	title := fmt.Sprintf("%s (%d", titleName, len(items))
	if m.query != "" {
		title += fmt.Sprintf("/%d match", len(m.items))
		if len(items) != 1 {
			title += "es"
		}
	}
	title += ")"
	if m.source == sessionmgr.ModeAll {
		title = fmt.Sprintf(
			"All (%d · %d sessions · %d agents · %d dirs)",
			len(items),
			counts[sessionmgr.KindSession],
			counts[sessionmgr.KindAgent],
			counts[sessionmgr.KindZoxide]+counts[sessionmgr.KindFD],
		)
	}
	lines := []string{s.title.Render(clampText(title, innerW))}
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
		start, end := visibleWindow(len(items), m.cursor, max(1, innerH-2))
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
	return s.paneFocus.Width(width - 2).Height(height - 2).Render(content)
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
		name := s.tabActive.Render(item.Name)
		return rowText(m.iconFor(item.Kind), state, name), ago(item.Activity)
	case sessionmgr.KindZoxide:
		return rowText(m.iconFor(item.Kind), item.Path), "zoxide"
	case sessionmgr.KindFD:
		return rowText(m.iconFor(item.Kind), item.Path), "fd"
	case sessionmgr.KindAgent:
		icons := m.config.IconSet()
		var glyph string
		if !icons.AgentStateHidden() {
			glyph = renderAgentState(s, icons, item.AgentState)
			if !icons.AgentStateUsesIcons() {
				glyph = renderAgentStateLabel(s, icons, item.AgentState)
			}
		}
		name := s.tabActive.Render(item.DisplayName())
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
	detailH := min(10, max(7, height/3))
	if !m.showPreview {
		detailH = height
	}
	detail := m.renderDetailPane(width, detailH)
	if !m.showPreview {
		return detail
	}
	previewH := height - lipgloss.Height(detail) - 1
	if previewH < 5 {
		previewH = 5
	}
	preview := m.renderPreviewPane(width, previewH)
	return lipgloss.JoinVertical(lipgloss.Left, detail, "", preview)
}

func (m Model) renderDetailPane(width, height int) string {
	s := m.styles
	innerH := max(4, height-2)
	item, ok := m.selectedItem()
	var lines []string
	if !ok {
		lines = []string{s.title.Render("Details"), "", s.muted.Render("select an item")}
	} else {
		lines = m.detailLines(item)
	}
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	return s.pane.Width(width - 2).Height(height - 2).Render(content)
}

func (m Model) detailLines(item sessionmgr.Item) []string {
	s := m.styles
	switch item.Kind {
	case sessionmgr.KindSession:
		icons := m.config.IconSet()
		attached := renderTmuxStateDetail(s, item.Attached, icons)
		return []string{
			s.title.Render(item.Name),
			s.muted.Render("tmux session"),
			"",
			kv(s, "path", sessionmgr.ContractHome(item.Path)),
			kv(s, "attached", attached),
			kv(s, "windows", fmt.Sprint(item.Windows)),
			kv(s, "activity", ago(item.Activity)),
			kv(s, "created", ago(item.Created)),
		}
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return []string{
			s.title.Render(sessionmgr.SessionNameFromDir(item.Path)),
			s.muted.Render(string(item.Kind) + " directory"),
			"",
			kv(s, "path", item.Path),
			kv(s, "enter", "create/switch tmux session"),
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
		return []string{
			s.title.Render(item.DisplayName()),
			s.muted.Render("agent · " + item.AgentName),
			"",
			kv(s, "state", stateLabel),
			kv(s, "location", item.Location),
			kv(s, "session", item.Session),
			kv(s, "path", sessionmgr.ContractHome(item.Path)),
		}
	default:
		return []string{s.title.Render(item.DisplayName())}
	}
}

func (m Model) renderPreviewPane(width, height int) string {
	s := m.styles
	innerW := max(10, width-4)
	innerH := max(4, height-2)
	title := "Preview"
	if item, ok := m.selectedItem(); ok {
		title = "Preview · " + item.DisplayName()
	}
	content := m.preview
	if content == "" {
		content = s.muted.Render("preview loading…")
	}
	lines := []string{s.title.Render(clampText(title, innerW)), ""}
	contentH := max(1, innerH-len(lines))
	previewLines := strings.Split(content, "\n")
	for i := 0; i < contentH && i < len(previewLines); i++ {
		lines = append(lines, clampText(previewLines[i], innerW))
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	return s.pane.Width(width - 2).
		Height(height - 2).
		Render(trimHeight(strings.Join(lines, "\n"), innerH))
}

func (m Model) renderFooter() string {
	s := m.styles
	var input string
	inputStyle := s.muted
	switch m.inputMode {
	case modeSearch:
		input = m.searchInput.View()
	case modeRename:
		input = m.renameInput.View()
	default:
		input = m.status
		if m.backgroundRefreshing() {
			if input == "" || input == "ready" {
				input = "refreshing…"
			} else {
				input += " · refreshing…"
			}
		}
		if input == "" {
			input = "ready"
		}
		inputStyle = footerStatusStyle(s, input, m.err != nil)
	}
	statusLeft := []string{}
	if sessionmgr.InTmux() {
		statusLeft = append(statusLeft, s.success.Render("✓ tmux"))
	} else {
		statusLeft = append(statusLeft, s.warning.Render("outside tmux"))
	}
	statusLeft = append(statusLeft, s.info.Render(m.source.Names().List))
	if m.config.TypeFirst.Enabled {
		statusLeft = append(statusLeft, s.emphasis.Render("type-first"))
		if m.prefixArmed {
			statusLeft = append(statusLeft, s.warning.Render("prefix"))
		}
	}
	if m.query != "" {
		statusLeft = append(statusLeft, s.emphasis.Render("/"+m.query))
	}
	left := strings.Join(statusLeft, "  ")
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
				s.key.Render("q") + " quit",
				s.key.Render("enter") + " attach/create/focus",
				s.key.Render("/") + " filter",
			}
			helpParts = append(helpParts,
				s.key.Render("m")+" mode",
				s.key.Render("r")+" refresh",
				s.key.Render("R")+" rename",
				s.key.Render("x")+" kill",
				s.key.Render("y")+" yazi",
				s.key.Render("p")+" preview",
			)
			help = strings.Join(helpParts, s.muted.Render(" · "))
		}
	} else {
		help = s.muted.Render("? help")
	}
	// The status style has one column of horizontal padding on each side. Keep
	// the composed text inside that content area, and render one column shy of
	// the terminal width to avoid terminal auto-wrap at the right edge.
	footerW := safeWidth(m.width)
	contentW := max(1, footerW-2)
	line1 := composeLine(left, input, contentW, inputStyle)
	line2 := clampText(help, contentW)
	return s.status.Width(footerW).Render(line1 + "\n" + line2)
}

func footerStatusStyle(s styles, status string, hasError bool) lipgloss.Style {
	if hasError {
		return s.danger
	}
	if isWarningStatus(status) {
		return s.warning
	}
	return s.muted
}

func isWarningStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if strings.HasPrefix(status, "press ") && strings.HasSuffix(status, " before actions") {
		return true
	}
	switch status {
	case "no integrations selected",
		"hook installation skipped",
		"input mode change cancelled",
		"rename cancelled",
		"yazi closed without a directory",
		"nothing selected",
		"delete only applies to sessions",
		"rename only applies to sessions and agents":
		return true
	default:
		return false
	}
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
