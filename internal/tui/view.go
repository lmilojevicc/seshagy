package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	s := m.styles
	availableH := max(8, m.height)
	if m.setupPrompt {
		return s.app.Width(m.width).Height(availableH).Render(m.renderSetupPrompt(availableH))
	}
	if m.integrationPrompt {
		return s.app.Width(m.width).Height(availableH).Render(m.renderIntegrationPrompt(availableH))
	}
	header := m.renderTabs()
	footer := m.renderFooter()
	bodyH := availableH - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 6 {
		bodyH = 6
	}
	body := m.renderBody(bodyH)
	return s.app.Width(m.width).Height(availableH).Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}

func (m Model) renderSetupPrompt(height int) string {
	s := m.styles
	width := max(54, min(88, m.width-4))
	innerW := max(44, width-4)
	innerH := 11
	lines := []string{
		s.title.Render("Choose startup input mode"),
		s.muted.Render("Type-first mode lets normal typing filter immediately."),
		s.muted.Render("App actions then require the configured prefix key (" + m.config.PrefixKey() + ")."),
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
		if i == m.setupCursor {
			cursor = s.bar.Render("▌") + " "
		}
		line := cursor + choice.label + s.muted.Render(" — "+choice.desc)
		if i == m.setupCursor {
			line = s.selectedBG.Render(pad(line, innerW))
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", strings.Join([]string{
		s.key.Render("enter") + " select",
		s.key.Render("y") + " type-first",
		s.key.Render("n") + " classic",
		s.key.Render("q") + " quit",
	}, s.muted.Render(" · ")))
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	box := s.paneFocus.Width(width - 2).Height(innerH).Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderTabs() string {
	s := m.styles
	parts := []string{s.emphasis.Render("seshagy")}
	for _, tab := range m.sourceTabs() {
		label := fmt.Sprintf("[%s] %s", tab.key, tab.name)
		if tab.mode == m.source {
			parts = append(parts, s.tabActive.Render(label))
		} else {
			parts = append(parts, s.tabInactive.Render(label))
		}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, "  "))
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
			name: tabNameForMode(mode),
			mode: mode,
		})
	}
	return tabs
}

func tabNameForMode(mode sessionmgr.SourceMode) string {
	switch mode {
	case sessionmgr.ModeSessions:
		return "Sessions"
	case sessionmgr.ModeAgents:
		return "Agents"
	case sessionmgr.ModeCurrentAgents:
		return "Current agents"
	case sessionmgr.ModeZoxide:
		return "Zoxide"
	case sessionmgr.ModeFD:
		return "fd"
	default:
		return "All"
	}
}

func (m Model) renderIntegrationPrompt(height int) string {
	s := m.styles
	width := max(40, m.width-4)
	innerW := max(36, width-4)
	innerH := max(8, height-6)
	lines := []string{
		s.title.Render("Install agent state hooks?"),
		s.muted.Render("seshagy now uses hook/plugin reports for agent state instead of pane text or process inspection."),
		s.muted.Render("Toggle the detected integrations you want to install, then press enter."),
		"",
	}
	if len(m.integrationRows) == 0 {
		lines = append(lines, s.muted.Render("No missing hook integrations found for installed agents."))
	} else {
		for i, rec := range m.integrationRows {
			lines = append(lines, m.renderIntegrationRow(rec, i == m.integrationCursor, innerW))
		}
	}
	if len(m.integrationMessages) > 0 {
		lines = append(lines, "")
		for _, message := range m.integrationMessages {
			lines = append(lines, s.muted.Render(clampText(message, innerW)))
		}
	}
	lines = append(lines, "", clampText(strings.Join([]string{
		s.key.Render("space") + " toggle",
		s.key.Render("enter") + " install selected",
		s.key.Render("s/esc") + " skip",
		s.key.Render("r") + " rescan",
		s.key.Render("q") + " quit",
	}, s.muted.Render(" · ")), innerW))
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	box := s.paneFocus.Width(width - 2).Height(innerH).Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderIntegrationRow(rec integrations.Recommendation, selected bool, width int) string {
	s := m.styles
	prefix := "  "
	if selected {
		prefix = s.bar.Render("▌") + " "
	}
	box := "[ ]"
	if m.integrationSelected[rec.Target] {
		box = s.success.Render("[x]")
	}
	if !rec.AgentAvailable || !rec.Installable || rec.State == integrations.StatusCurrent {
		box = s.muted.Render("[-]")
	}
	state := string(rec.State)
	if rec.State == integrations.StatusCurrent {
		state = "current"
	} else if rec.Reason != "" {
		state = rec.Reason
	}
	left := fmt.Sprintf("%s %-18s", box, rec.Label)
	line := prefix + composeLine(left, state, max(1, width-2), s.muted)
	if selected {
		line = s.selectedBG.Render(pad(line, width))
	}
	return line
}

func (m Model) renderBody(height int) string {
	gap := 2
	if m.width < 80 || !m.showPreview {
		gap = 0
	}
	leftW := m.width
	rightW := 0
	if m.showPreview && m.width >= 80 {
		leftW = max(34, (m.width-gap)/2)
		if leftW > 72 {
			leftW = 72
		}
		rightW = m.width - leftW - gap
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
	counts := sortedCounts(m.items)
	titleName := titleForMode(m.source)
	if m.agentStateFilteringActive() {
		titleName += " · " + agentStateFilterLabel(m.agentStateFilter)
	}
	title := fmt.Sprintf("%s (%d", titleName, len(items))
	if m.query != "" {
		title += fmt.Sprintf("/%d match", len(m.items))
		if len(items) != 1 {
			title += "es"
		}
	}
	title += ")"
	if m.source == sessionmgr.ModeAll {
		title = fmt.Sprintf("All (%d · %d sessions · %d agents · %d dirs)", len(items), counts[sessionmgr.KindSession], counts[sessionmgr.KindAgent], counts[sessionmgr.KindZoxide]+counts[sessionmgr.KindFD])
	}
	lines := []string{s.title.Render(title)}
	if m.loading {
		lines = append(lines, s.muted.Render("refreshing…"))
	}
	if len(items) == 0 {
		empty := "no items"
		if m.query != "" {
			empty = "no matches for " + m.query
		} else if m.agentStateFilteringActive() {
			empty = "no agent panes with state " + agentStateFilterLabel(m.agentStateFilter)
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
		state := s.info.Render("◌")
		if item.Attached {
			state = s.success.Render("●")
		}
		name := s.tabActive.Render(item.Name)
		return rowText(m.iconFor(item.Kind), state, name), ago(item.Activity)
	case sessionmgr.KindAgent:
		icons := m.config.IconSet()
		state := renderAgentState(s, item.AgentState)
		if !icons.Enabled {
			state = renderAgentStateLabel(s, item.AgentState)
		}
		message := item.AgentMessage
		if message == "" {
			message = item.AgentSource
		}
		secondary := item.Location
		if item.Path != "" {
			secondary += " · " + item.Path
		}
		if message != "" {
			secondary += " · " + message
		}
		return rowText(m.iconFor(item.Kind), state, s.tabActive.Render(item.AgentName)), secondary
	case sessionmgr.KindZoxide:
		return rowText(m.iconFor(item.Kind), item.Path), "zoxide"
	case sessionmgr.KindFD:
		return rowText(m.iconFor(item.Kind), item.Path), "fd"
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
	return strings.Join(kept, " ")
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
	innerW := max(10, width-4)
	innerH := max(4, height-2)
	item, ok := m.selectedItem()
	var lines []string
	if !ok {
		lines = []string{s.title.Render("Details"), "", s.muted.Render("select an item")}
	} else {
		lines = m.detailLines(item, innerW)
	}
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	return s.pane.Width(width - 2).Height(height - 2).Render(content)
}

func (m Model) detailLines(item sessionmgr.Item, width int) []string {
	s := m.styles
	switch item.Kind {
	case sessionmgr.KindSession:
		attached := "no"
		if item.Attached {
			attached = s.success.Render("yes")
		}
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
	case sessionmgr.KindAgent:
		suffix := item.AgentMessage
		if suffix == "" {
			suffix = item.AgentSource
		}
		lines := []string{
			s.title.Render(item.AgentName),
			s.muted.Render("agent pane"),
			"",
			kv(s, "state", renderAgentState(s, item.AgentState)+" "+string(item.AgentState)),
			kv(s, "pane", item.PaneID),
			kv(s, "where", item.Location),
			kv(s, "path", item.Path),
		}
		if suffix != "" {
			lines = append(lines, kv(s, "note", clampText(suffix, width-8)))
		}
		return lines
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return []string{
			s.title.Render(sessionmgr.SessionNameFromDir(item.Path)),
			s.muted.Render(string(item.Kind) + " directory"),
			"",
			kv(s, "path", item.Path),
			kv(s, "enter", "create/switch tmux session"),
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
	for _, line := range strings.Split(content, "\n") {
		lines = append(lines, clampText(line, innerW))
	}
	return s.pane.Width(width - 2).Height(height - 2).Render(trimHeight(strings.Join(lines, "\n"), innerH))
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
	statusLeft = append(statusLeft, s.info.Render(modeName(m.source)))
	if m.config.TypeFirst.Enabled {
		statusLeft = append(statusLeft, s.emphasis.Render("type-first"))
		if m.prefixArmed {
			statusLeft = append(statusLeft, s.warning.Render("prefix"))
		}
	}
	if m.agentStateFilteringActive() {
		statusLeft = append(statusLeft, s.emphasis.Render("state:"+agentStateFilterLabel(m.agentStateFilter)))
	}
	if m.query != "" {
		statusLeft = append(statusLeft, s.emphasis.Render("/"+m.query))
	}
	left := strings.Join(statusLeft, "  ")
	help := ""
	if m.showHelp {
		if m.config.TypeFirst.Enabled && !m.prefixArmed {
			help = strings.Join([]string{
				s.key.Render("type") + " filter",
				s.key.Render(m.config.PrefixKey()) + " actions",
				s.key.Render("backspace") + " edit",
			}, s.muted.Render(" · "))
		} else {
			helpParts := []string{
				s.key.Render("?") + " help",
				s.key.Render("q") + " quit",
				s.key.Render("enter") + " attach/create/focus",
				s.key.Render("/") + " filter",
			}
			if isAgentSource(m.source) {
				helpParts = append(helpParts,
					s.key.Render("s")+" state",
					s.key.Render("S")+" all",
				)
			}
			helpParts = append(helpParts,
				s.key.Render("r")+" refresh",
				s.key.Render("R")+" rename",
				s.key.Render("x")+" kill",
				s.key.Render("y")+" yazi",
				s.key.Render("i")+" hooks",
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
	footerW := max(1, m.width-1)
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
		"rename cancelled",
		"yazi closed without a directory",
		"nothing selected",
		"delete only applies to sessions and agents",
		"rename only applies to sessions",
		"state filter only applies to agent panes":
		return true
	default:
		return false
	}
}

func titleForMode(mode sessionmgr.SourceMode) string {
	switch mode {
	case sessionmgr.ModeSessions:
		return "Sessions"
	case sessionmgr.ModeAgents:
		return "Agents"
	case sessionmgr.ModeCurrentAgents:
		return "Current session agents"
	case sessionmgr.ModeZoxide:
		return "Zoxide"
	case sessionmgr.ModeFD:
		return "fd"
	default:
		return "All"
	}
}

func renderAgentState(s styles, state sessionmgr.AgentState) string {
	switch state {
	case sessionmgr.AgentWorking:
		return s.success.Render("▶")
	case sessionmgr.AgentBlocked:
		return s.warning.Render("◆")
	case sessionmgr.AgentAborted:
		return s.danger.Render("■")
	case sessionmgr.AgentDone:
		return s.success.Render("✓")
	case sessionmgr.AgentIdle:
		return s.info.Render("◌")
	default:
		return s.muted.Render("?")
	}
}

func renderAgentStateLabel(s styles, state sessionmgr.AgentState) string {
	label := "[" + agentStateText(state) + "]"
	switch state {
	case sessionmgr.AgentWorking, sessionmgr.AgentDone:
		return s.success.Render(label)
	case sessionmgr.AgentBlocked:
		return s.warning.Render(label)
	case sessionmgr.AgentAborted:
		return s.danger.Render(label)
	case sessionmgr.AgentIdle:
		return s.info.Render(label)
	default:
		return s.muted.Render(label)
	}
}

func agentStateText(state sessionmgr.AgentState) string {
	if state == "" {
		return string(sessionmgr.AgentUnknown)
	}
	return string(state)
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
