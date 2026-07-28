package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

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

	var frame string
	switch {
	case m.setup.active:
		frame = m.styles.app.Width(m.width).Height(m.height).
			Render(m.renderSetupPrompt(m.height))
	case m.installMenu.active:
		frame = m.styles.app.Width(m.width).Height(m.height).
			Render(m.renderInstallMenu(m.height))
	default:
		frame = m.renderNormalSurface()
	}
	return m.overlayNotifications(frame)
}

func (m Model) renderNormalSurface() string {
	view := m.projectLayout(layoutNeeds{Overview: true, Details: true, Preview: true})
	theme := defaultRenderTheme{styles: m.styles, icons: m.config.IconSet()}
	footer := m.renderShellFooter(view.Actions)
	view.Frame.ContentHeight = max(1, m.height-lipgloss.Height(footer))
	dashboard := renderDefault(view, theme)
	if m.inlineInputActive() {
		inputTile := m.renderInlineInputTile()
		if m.height == 1 {
			return renderInputOnly(m.projectInputChrome(safeWidth(m.width)))
		}
		if m.height < lipgloss.Height(dashboard.Header)+1+lipgloss.Height(inputTile) {
			return trimHeight(inputTile, m.height)
		}
	}

	frame := joinFrame(
		dashboard.Header,
		dashboard.Body,
		footer,
		m.width,
		m.height,
	)
	if !m.inputPopupActive() {
		return frame
	}
	background := frame
	if m.config.TUI.DimBackground != nil && *m.config.TUI.DimBackground {
		background = dimDashboard(frame)
	}
	popup := m.renderInputPopup()
	x := max(0, (m.width-lipgloss.Width(popup))/2)
	y := max(0, (m.height-lipgloss.Height(popup))/2)
	return overlay(background, popup, x, y)
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

func (m Model) inlineInputActive() bool {
	if m.inputMode != modeSearch && m.inputMode != modeRename {
		return false
	}
	return m.config.TUI.InputStyle == appconfig.InputStyleCmdline || !m.inputPopupActive()
}

// configuredInput returns the active text input sized to available, including
// its prompt and one cell for the cursor.
func (m Model) configuredInput(available int) (string, textinput.Model, string) {
	available = max(1, available)
	var title, help string
	var ti textinput.Model
	switch m.inputMode {
	case modeRename:
		title = "RENAME"
		ti = m.renameInput
		ti.Prompt = clampText(m.renameFrom, available/2) + " -> "
		help = "enter to rename · esc to cancel"
	default:
		title = "SEARCH"
		ti = m.searchInput
		help = "enter to filter · esc to cancel"
	}
	ti.Width = max(1, available-lipgloss.Width(ti.Prompt)-1)
	return title, ti, help
}

// renderInputPopup renders the bordered centered popup that hosts the
// search or rename text input plus a one-line help row.
func (m Model) renderInputPopup() string {
	boxWidth := min(60, m.width-4)
	if boxWidth < 30 {
		boxWidth = 30
	}
	contentWidth := max(1, boxWidth-4)
	return renderPopupInput(m.projectInputChrome(contentWidth), m.styles, boxWidth)
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

func compactAgentCount(count, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	raw := fmt.Sprintf("%d", count)
	if lipgloss.Width(raw) <= maxWidth {
		return raw
	}
	for _, unit := range []struct {
		value  int
		suffix string
	}{
		{1_000_000_000, "b"},
		{1_000_000, "m"},
		{1_000, "k"},
	} {
		if count >= unit.value {
			compact := fmt.Sprintf("%d%s", count/unit.value, unit.suffix)
			if lipgloss.Width(compact) <= maxWidth {
				return compact
			}
		}
	}
	return clampText(raw, maxWidth)
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

// titledBottomEdge is the bottom-border mirror of titledTopEdge: ╰─ title ──╯.
func titledBottomEdge(title string, w int, borderFG, titleFG lipgloss.TerminalColor) string {
	if title == "" || w < 7 {
		return lipgloss.NewStyle().Foreground(borderFG).
			Render("╰" + strings.Repeat("─", max(0, w-2)) + "╯")
	}
	clamped := clampText(title, w-6)
	dashes := w - 5 - lipgloss.Width(clamped)
	if dashes < 1 {
		dashes = 1
	}
	border := lipgloss.NewStyle().Foreground(borderFG)
	return border.Render("╰─ ") +
		lipgloss.NewStyle().Foreground(titleFG).Render(clamped) +
		border.Render(" "+strings.Repeat("─", dashes)+"╯")
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

// isTailPreviewKind reports whether a kind's preview is sourced from a
// multiplexer capture (tmux capture-pane or herdr pane read), both of which
// are bottom-anchored, rather than a top-down directory listing.
func isTailPreviewKind(kind sessionmgr.Kind) bool {
	return kind == sessionmgr.KindSession || kind == sessionmgr.KindAgent
}

func (m Model) renderShellFooter(actions actionsView) string {
	footerWidth := safeWidth(m.width)
	helpTile := renderDefaultActionsTile(actions, m.styles, footerWidth)
	popupSuppressed := m.config.TUI.InputStyle == appconfig.InputStylePopup &&
		(m.width < 34 || m.height < 5)
	if (m.inputMode == modeSearch || m.inputMode == modeRename) &&
		(m.config.TUI.InputStyle == appconfig.InputStyleCmdline || popupSuppressed) {
		inputTile := m.renderInlineInputTile()
		if m.height > 0 && m.height < 5 {
			return inputTile
		}
		return inputTile + "\n" + helpTile
	}
	return helpTile
}

func (m Model) renderInlineInputTile() string {
	footerWidth := safeWidth(m.width)
	contentWidth := max(1, footerWidth-4)
	return renderCmdlineInput(
		m.projectInputChrome(contentWidth),
		m.styles,
		footerWidth,
	)
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
