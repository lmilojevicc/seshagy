package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

const (
	zenContentMinWidth   = 40
	zenContentIdealWidth = 72
	zenContentMaxWidth   = 88
)

// renderZen composes the centered, borderless Zen dashboard from value-only
// state. Actions and live input remain owned by the shared shell.
var _ renderLayout = renderZen

func renderZen(view layoutView, theme layoutRenderTheme) renderedDashboard {
	contentWidth := zenContentWidth(view.Frame.Width)
	contentHeight := max(1, view.Frame.ContentHeight)
	headerBlock := renderZenHeader(view, theme, contentWidth)
	headerHeight := min(lipgloss.Height(headerBlock), max(0, contentHeight-1))
	headerBlock = trimZenBlock(headerBlock, headerHeight)
	header := ""
	if headerBlock != "" {
		header = zenPlaceContent(headerBlock, view.Frame.Width, contentWidth)
	}
	bodyHeight := contentHeight - headerHeight
	body := renderZenCollection(view.Collection, theme, contentWidth, bodyHeight)
	return renderedDashboard{
		Header: header,
		Body:   zenPlaceContent(body, view.Frame.Width, contentWidth),
	}
}

func trimZenBlock(block string, height int) string {
	if block == "" || height <= 0 {
		return ""
	}
	lines := strings.Split(block, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func zenContentWidth(frameWidth int) int {
	if frameWidth <= 0 {
		return zenContentIdealWidth
	}
	return zenContentWidthForUsable(safeWidth(frameWidth))
}

func zenContentWidthForUsable(usable int) int {
	if usable >= tabWidthUnset/2 {
		return zenContentIdealWidth
	}
	maxContent := usable - 4
	if maxContent < zenContentMinWidth {
		return max(1, usable)
	}
	return min(maxContent, zenContentMaxWidth)
}

func zenPlaceContent(block string, frameWidth, contentWidth int) string {
	if block == "" {
		return ""
	}
	left := 0
	if frameWidth > 0 {
		left = max(0, (safeWidth(frameWidth)-contentWidth)/2)
	}
	prefix := strings.Repeat(" ", left)
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		lines[index] = prefix + clampStyledText(line, contentWidth)
	}
	return strings.Join(lines, "\n")
}

func renderZenHeader(view layoutView, theme layoutRenderTheme, width int) string {
	s := theme.styles
	lines := []string{
		zenCenterText(s.emphasis.Render("seshagy"), width),
		zenCenterText(s.title.Render(strings.ToLower(view.Collection.Title)), width),
	}
	if dots := renderZenSourceDots(view.Sources, s); dots != "" {
		lines = append(lines, zenCenterText(dots, width))
	}
	if overview := renderZenOverview(view, theme, width); overview != "" {
		lines = append(lines, strings.Split(overview, "\n")...)
	}
	if subtitle := renderZenSubtitle(view.Collection); subtitle != "" {
		lines = append(lines, zenCenterText(s.muted.Render(subtitle), width))
	}
	if view.Search.Query != "" && !view.Search.Editing {
		lines = append(lines, zenCenterText(s.muted.Render("/"+view.Search.Query), width))
	}
	return strings.Join(append(lines, ""), "\n")
}

func renderZenSourceDots(view sourcesView, s styles) string {
	parts := make([]string, 0, len(view.Entries))
	for _, entry := range view.Entries {
		if entry.Selected {
			parts = append(parts, s.emphasis.Render("●"))
		} else {
			parts = append(parts, s.muted.Render("○"))
		}
	}
	return strings.Join(parts, " ")
}

func renderZenOverview(view layoutView, theme layoutRenderTheme, width int) string {
	if view.Overview == nil || view.Overview.State == overviewLoading {
		return ""
	}
	overview := *view.Overview
	s := theme.styles
	segments := []string{s.muted.Render(fmt.Sprintf(
		"%d %s",
		overview.Sessions,
		view.Collection.SessionPlural,
	))}
	total := overview.Agents.Working + overview.Agents.Blocked + overview.Agents.Done +
		overview.Agents.Idle + overview.Agents.Unknown
	if total > 0 {
		counts := map[sessionmgr.AgentState]int{
			sessionmgr.AgentWorking: overview.Agents.Working,
			sessionmgr.AgentBlocked: overview.Agents.Blocked,
			sessionmgr.AgentDone:    overview.Agents.Done,
			sessionmgr.AgentIdle:    overview.Agents.Idle,
			sessionmgr.AgentUnknown: overview.Agents.Unknown,
		}
		for _, state := range agentStateOrder {
			segments = append(segments, renderZenAgentStateChip(
				s,
				theme.icons,
				state,
				counts[state],
				view.Collection.AgentStateFilter == string(state),
			))
		}
		all := fmt.Sprintf("%d agents", total)
		if view.Collection.AgentStateFilter == "" {
			all = s.emphasis.Render(all)
		} else {
			all = s.muted.Render(all)
		}
		segments = append(segments, all)
	}
	return strings.Join(packZenOverviewLines(segments, s.muted.Render("  "), width), "\n")
}

func packZenOverviewLines(segments []string, separator string, width int) []string {
	width = max(1, width)
	lines := make([]string, 0, len(segments))
	current := ""
	for _, segment := range segments {
		if current == "" {
			current = segment
			continue
		}
		candidate := current + separator + segment
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, zenCenterText(current, width))
		current = segment
	}
	if current != "" {
		lines = append(lines, zenCenterText(current, width))
	}
	return lines
}

func renderZenAgentStateChip(
	s styles,
	icons sessionmgr.IconSet,
	state sessionmgr.AgentState,
	count int,
	active bool,
) string {
	text := fmt.Sprintf("%d %s", count, agentStateText(state))
	style := icons.ForAgentState(state)
	if icons.AgentStateUsesIcons() && style.Text != "" {
		text = fmt.Sprintf("%s %s", style.Text, text)
	}
	if active {
		return s.emphasis.Render(sessionmgr.StripANSI(text))
	}
	return renderAgentStateStyled(s, state, text, style.Color)
}

func renderZenSubtitle(view collectionView) string {
	if view.Source != sourceID("agents") {
		return ""
	}
	parts := make([]string, 0, 2)
	if view.ScopeLabel != "" && view.ScopeLabel != "all" {
		parts = append(parts, view.ScopeLabel)
	}
	if view.StateFilterLabel != "" {
		parts = append(parts, view.StateFilterLabel)
	}
	if len(parts) == 0 {
		return ""
	}
	return "· " + strings.Join(parts, " · ")
}

func renderZenCollection(
	view collectionView,
	theme layoutRenderTheme,
	width, height int,
) string {
	s := theme.styles
	lines := make([]string, 0, max(1, height))
	if height > 2 {
		lines = append(lines, "")
	}

	if len(view.Rows) == 0 {
		message := view.EmptyMessage
		switch view.State {
		case collectionLoading:
			message = "refreshing…"
		case collectionError:
			if view.Error != "" {
				message = view.Error
			}
		}
		if message == "" {
			message = "no items"
		}
		style := s.muted
		if view.State == collectionError {
			style = s.danger
		}
		lines = append(lines, zenCenterText(style.Render(message), width))
		return trimHeight(strings.Join(lines, "\n"), height)
	}

	available := max(1, height-len(lines))
	selected := defaultSelectedRow(view.Rows)
	windowHeight := available
	for range 2 {
		start, end := visibleWindow(len(view.Rows), selected, windowHeight)
		hints := 0
		if start > 0 {
			hints++
		}
		if end < len(view.Rows) {
			hints++
		}
		hints = min(hints, max(0, available-1))
		if next := max(1, available-hints); next != windowHeight {
			windowHeight = next
			continue
		}
		break
	}
	start, end := visibleWindow(len(view.Rows), selected, windowHeight)
	hintSlots := max(0, available-(end-start))
	showTop := start > 0 && hintSlots > 0
	if showTop {
		hintSlots--
		lines = append(lines, s.muted.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for index := start; index < end; index++ {
		lines = append(lines, renderZenRow(view.Rows[index], width, theme))
	}
	if end < len(view.Rows) && hintSlots > 0 {
		lines = append(lines, s.muted.Render(fmt.Sprintf("  ↓ %d more", len(view.Rows)-end)))
	}
	return trimHeight(strings.Join(lines, "\n"), height)
}

func renderZenRow(row rowView, width int, theme layoutRenderTheme) string {
	s := theme.styles
	prefix := "  "
	if row.Selected {
		prefix = s.bar.Render("›") + " "
	}
	primary, trailing := defaultRowParts(row, theme)
	if row.Selected {
		primary = s.emphasis.Render(sessionmgr.StripANSI(primary))
	}
	return prefix + composeZenLine(primary, trailing, max(1, width-2), s.muted)
}

func composeZenLine(left, right string, width int, rightStyle lipgloss.Style) string {
	if right == "" {
		return clampStyledText(left, width)
	}
	right = rightStyle.Render(right)
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if leftWidth+rightWidth+1 > width {
		return clampStyledText(left, width)
	}
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
}

func zenCenterText(text string, width int) string {
	width = max(1, width)
	text = clampStyledText(text, width)
	return strings.Repeat(" ", max(0, (width-lipgloss.Width(text))/2)) + text
}
