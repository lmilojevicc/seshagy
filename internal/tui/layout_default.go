package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// layoutRenderTheme is the immutable presentation input used by layout
// compositions. It contains no controller or backend behavior.
type layoutRenderTheme struct {
	styles styles
	icons  sessionmgr.IconSet
}

type renderedDashboard struct {
	Header string
	Body   string
}

// renderDefault composes the current default dashboard from value-only state.
// The shared shell renders Actions/TextEntry and joins the final frame.
var _ renderLayout = renderDefault

func renderDefault(view layoutView, theme layoutRenderTheme) renderedDashboard {
	header := renderDefaultHeader(view, theme)
	bodyHeight := max(1, view.Frame.ContentHeight-lipgloss.Height(header))
	return renderedDashboard{
		Header: header,
		Body:   trimHeight(renderDefaultBody(view, theme, bodyHeight), bodyHeight),
	}
}

func renderDefaultHeader(view layoutView, theme layoutRenderTheme) string {
	usableW := safeWidth(view.Frame.Width)
	if view.Overview == nil || view.Overview.Items == 0 || view.Frame.Height < 14 {
		return renderDefaultSourcesTile(view.Sources, theme.styles, usableW)
	}

	stats := defaultOverviewStats(*view.Overview)
	gap := 1
	sourcesW, agentW, workspaceW, ok := defaultTopRowWidths(
		view.Sources,
		usableW,
		stats,
		theme,
	)
	if !ok {
		return renderDefaultSourcesTile(view.Sources, theme.styles, usableW)
	}

	workspaceTitle := strings.ToUpper(defaultSourceLabel(view.Sources, sourceID("sessions")))
	workspaceContent := theme.styles.emphasis.Render(fmt.Sprintf("%d", stats.sessions))
	workspaceTile := paneWithTitle(
		theme.styles.tileWorkspace,
		theme.styles.workspaceTileTitle,
		workspaceContent,
		workspaceTitle,
		workspaceW,
		0,
	)

	agentContent := defaultAgentChips(
		theme.styles,
		theme.icons,
		stats,
		max(1, agentW-4),
	)
	agentTile := paneWithTitle(
		theme.styles.tileAgent,
		theme.styles.agentTileTitle,
		agentContent,
		strings.ToUpper(defaultSourceLabel(view.Sources, sourceID("agents"))),
		agentW,
		0,
	)

	sourcesTile := renderDefaultSourcesTile(view.Sources, theme.styles, sourcesW)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sourcesTile, strings.Repeat(" ", gap),
		agentTile, strings.Repeat(" ", gap),
		workspaceTile,
	)
}

func renderDefaultSourcesTile(view sourcesView, s styles, width int) string {
	inner := max(1, width-4)
	chips := renderDefaultSourceChips(view, s, inner)
	return paneWithTitle(s.tileSources, s.sourcesTileTitle, chips, "SOURCES", width, 0)
}

func renderDefaultSourceChips(view sourcesView, s styles, width int) string {
	count := defaultSourceCountBadge(view)
	countW := lipgloss.Width(count)
	if countW >= width {
		return s.muted.Render(clampText(count, width))
	}
	chipsW := width - countW - 1
	line := renderDefaultSourceChipLabels(view, s, "key-name")
	if lipgloss.Width(line) > chipsW {
		line = renderDefaultSourceChipLabels(view, s, "name")
	}
	if lipgloss.Width(line) > chipsW {
		line = renderDefaultSourceChipLabels(view, s, "key")
	}
	return composeLine(clampText(line, chipsW), count, width, s.muted)
}

func renderDefaultSourceChipLabels(view sourcesView, s styles, format string) string {
	parts := make([]string, 0, len(view.Entries))
	for _, entry := range view.Entries {
		var label string
		switch format {
		case "key-name":
			label = entry.Key + " " + entry.Label
		case "name":
			label = entry.Label
		default:
			label = entry.Key
		}
		if entry.Selected {
			parts = append(parts, s.chipActive.Render(label))
		} else {
			parts = append(parts, s.chipIdle.Render(label))
		}
	}
	return strings.Join(parts, s.muted.Render(" | "))
}

func defaultSourceCountBadge(view sourcesView) string {
	count := fmt.Sprintf("%d", view.VisibleCount)
	if view.ShowTotalCount {
		count = fmt.Sprintf("%d/%d", view.VisibleCount, view.TotalCount)
	}
	if view.Loading || view.Refreshing {
		frames := []rune(spinnerFrames)
		count += " " + string(frames[view.SpinnerFrame%len(frames)])
	}
	return count
}

func defaultCompleteSourceChipRowFits(view sourcesView, s styles, width int) bool {
	return lipgloss.Width(renderDefaultSourceChipLabels(view, s, "key"))+
		1+lipgloss.Width(defaultSourceCountBadge(view)) <= width
}

func defaultSourceLabel(view sourcesView, id sourceID) string {
	for _, entry := range view.Entries {
		if entry.ID == id {
			return entry.Label
		}
	}
	return ""
}

func defaultOverviewStats(view overviewView) overviewStats {
	return overviewStats{
		sessions: view.Sessions,
		agents: map[sessionmgr.AgentState]int{
			sessionmgr.AgentWorking: view.Agents.Working,
			sessionmgr.AgentBlocked: view.Agents.Blocked,
			sessionmgr.AgentDone:    view.Agents.Done,
			sessionmgr.AgentIdle:    view.Agents.Idle,
			sessionmgr.AgentUnknown: view.Agents.Unknown,
		},
	}
}

func defaultTopRowWidths(
	sources sourcesView,
	usableW int,
	stats overviewStats,
	theme layoutRenderTheme,
) (sourcesW, agentW, workspaceW int, ok bool) {
	const gap = 1
	workspaceCompact := clampVal(22, 16, usableW/6)
	compactAgent := clampVal(34, 26, usableW/3)
	sourcesW = usableW - workspaceCompact - compactAgent - 2*gap
	if sourcesW < 20 || !defaultCompleteSourceChipRowFits(
		sources,
		theme.styles,
		sourcesW-4,
	) {
		return 0, 0, 0, false
	}
	workspaceW = 16
	agentW = compactAgent + (workspaceCompact - workspaceW)
	if !theme.icons.AgentStateUsesIcons() {
		sourceFloor := max(
			20,
			lipgloss.Width(renderDefaultSourceChipLabels(sources, theme.styles, "key"))+
				1+lipgloss.Width(defaultSourceCountBadge(sources))+4,
		)
		natural := lipgloss.Width(defaultAgentChipRow(
			theme.styles,
			theme.icons,
			stats,
			0,
			" ",
		)) + 4
		if natural > agentW {
			if maxAgent := usableW - workspaceW - 2*gap - sourceFloor; maxAgent > agentW {
				agentW = clampVal(natural, agentW, maxAgent)
			}
		}
	}
	sourcesW = usableW - workspaceW - agentW - 2*gap
	return sourcesW, agentW, workspaceW, true
}

func defaultAgentChips(
	s styles,
	icons sessionmgr.IconSet,
	stats overviewStats,
	innerW int,
) string {
	innerW = max(1, innerW)
	for _, spec := range []struct {
		maxLabel int
		maxCount int
		sep      string
	}{
		{0, 0, "  "},
		{0, 0, " "},
		{4, 0, " "},
		{3, 0, " "},
		{2, 0, " "},
		{1, 0, " "},
		{1, 3, " "},
		{1, 2, " "},
		{1, 1, " "},
	} {
		row := defaultAgentChipRowFitted(
			s,
			icons,
			stats,
			spec.maxLabel,
			spec.maxCount,
			spec.sep,
		)
		if lipgloss.Width(row) <= innerW {
			return row
		}
	}
	return clampText(defaultAgentChipRowFitted(s, icons, stats, 1, 1, " "), innerW)
}

func defaultAgentChipRow(
	s styles,
	icons sessionmgr.IconSet,
	stats overviewStats,
	maxLabel int,
	sep string,
) string {
	return defaultAgentChipRowFitted(s, icons, stats, maxLabel, 0, sep)
}

func defaultAgentChipRowFitted(
	s styles,
	icons sessionmgr.IconSet,
	stats overviewStats,
	maxLabel, maxCount int,
	sep string,
) string {
	parts := make([]string, 0, len(agentStateOrder))
	for _, state := range agentStateOrder {
		count := stats.agents[state]
		iconStyle := icons.ForAgentState(state)
		label := iconStyle.Text
		if maxLabel > 0 && lipgloss.Width(label) > maxLabel {
			label = clampText(label, maxLabel)
		}
		countText := fmt.Sprintf("%d", count)
		if maxCount > 0 && lipgloss.Width(countText) > maxCount {
			countText = compactAgentCount(count, maxCount)
		}
		glyph := renderAgentStateStyled(s, state, label, iconStyle.Color)
		cnt := renderAgentStateStyled(s, state, countText, iconStyle.Color)
		parts = append(parts, glyph+" "+cnt)
	}
	return strings.Join(parts, sep)
}

func renderDefaultBody(view layoutView, theme layoutRenderTheme, height int) string {
	usableW := safeWidth(view.Frame.Width)
	gap := 2
	if view.Preview == nil || view.Frame.Width < previewMinWidth {
		gap = 0
	}
	leftW := usableW
	rightW := 0
	if view.Preview != nil && view.Frame.Width >= previewMinWidth {
		leftW = max(34, (usableW-gap)/2)
		if leftW > 72 {
			leftW = 72
		}
		rightW = usableW - leftW - gap
	}
	left := renderDefaultCollection(view, theme, leftW, height)
	if rightW <= 0 {
		return left
	}
	right := renderDefaultRightPane(view, theme, rightW, height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

func renderDefaultCollection(
	view layoutView,
	theme layoutRenderTheme,
	width, height int,
) string {
	s := theme.styles
	collection := view.Collection
	innerW := max(10, width-4)
	innerH := max(3, height-2)
	title := defaultCollectionTitle(view)
	lines := []string{}
	if collection.State == collectionLoading {
		lines = append(lines, s.muted.Render("refreshing…"))
	}
	if len(collection.Rows) == 0 {
		empty := collection.EmptyMessage
		if empty == "" {
			empty = "no items"
		}
		lines = append(lines, "", s.muted.Render(empty))
	} else {
		cursor := defaultSelectedRow(collection.Rows)
		start, end := visibleWindow(len(collection.Rows), cursor, max(1, innerH-1))
		if start > 0 {
			lines = append(lines, s.muted.Render(fmt.Sprintf("  ↑ %d more", start)))
		}
		for index := start; index < end; index++ {
			lines = append(lines, renderDefaultRow(collection.Rows[index], innerW, theme))
		}
		if end < len(collection.Rows) {
			lines = append(
				lines,
				s.muted.Render(fmt.Sprintf("  ↓ %d more", len(collection.Rows)-end)),
			)
		}
	}
	content := trimHeight(strings.Join(lines, "\n"), innerH)
	box := paneWithTitle(s.paneList, s.listTitle, content, title, width, height)
	if view.Search.Query != "" && view.Search.Mode == searchTypeFirst && !view.Search.Editing {
		boxLines := strings.Split(box, "\n")
		if len(boxLines) > 0 {
			w := lipgloss.Width(boxLines[0])
			if w >= 7 {
				boxLines[len(boxLines)-1] = titledBottomEdge(
					clampText(view.Search.Query, w-6),
					w,
					s.paneList.GetBorderBottomForeground(),
					s.listTitle,
				)
				box = strings.Join(boxLines, "\n")
			}
		}
	}
	return box
}

func defaultCollectionTitle(view layoutView) string {
	collection := view.Collection
	title := fmt.Sprintf("%s (%d", collection.Title, collection.VisibleCount)
	if view.Search.Query != "" {
		title += fmt.Sprintf("/%d match", collection.TotalCount)
		if collection.VisibleCount != 1 {
			title += "es"
		}
	}
	title += ")"
	if collection.Source == sourceID("all") {
		return fmt.Sprintf(
			"%s (%d · %d %s · %d agents · %d dirs)",
			collection.Title,
			collection.VisibleCount,
			collection.SessionCount,
			collection.SessionPlural,
			collection.AgentCount,
			collection.DirectoryCount,
		)
	}
	if collection.Source == sourceID("agents") {
		title = fmt.Sprintf(
			"%s (%d · %s)",
			collection.Title,
			collection.VisibleCount,
			collection.ScopeLabel,
		)
		if collection.StateFilterLabel != "" {
			title += " · " + collection.StateFilterLabel
		}
	}
	return title
}

func defaultSelectedRow(rows []rowView) int {
	for index, row := range rows {
		if row.Selected {
			return index
		}
	}
	return 0
}

func renderDefaultRow(row rowView, width int, theme layoutRenderTheme) string {
	s := theme.styles
	prefix := "  "
	if row.Selected {
		prefix = s.bar.Render("▌") + " "
	}
	primary, trailing := defaultRowParts(row, theme)
	bodyW := max(1, width-2)
	line := prefix + composeLine(primary, trailing, bodyW, s.muted)
	if row.Selected {
		line = s.selectedBG.Render(pad(line, width))
	}
	return line
}

func defaultRowParts(row rowView, theme layoutRenderTheme) (string, string) {
	s := theme.styles
	switch row.Kind {
	case rowKind(sessionmgr.KindSession):
		return rowText(
			renderDefaultKindIndicator(row.Icon),
			renderDefaultTmuxIndicator(s, row.Session.State, row.Session.Attached, true),
			s.itemName.Render(row.Label),
		), row.Session.Activity
	case rowKind(sessionmgr.KindZoxide):
		return rowText(
			renderDefaultKindIndicator(row.Icon),
			s.itemName.Render(row.Directory.Path),
		), "zoxide"
	case rowKind(sessionmgr.KindFD):
		return rowText(
			renderDefaultKindIndicator(row.Icon),
			s.itemName.Render(row.Directory.Path),
		), "fd"
	case rowKind(sessionmgr.KindAgent):
		return rowText(
			renderDefaultAgentIndicator(s, row.Agent.Indicator, row.Agent.State, true),
			s.itemName.Render(row.Agent.DisplayName),
		), row.Agent.Location
	default:
		return row.Label, ""
	}
}

func renderDefaultKindIndicator(view indicatorView) string {
	text := view.Icon
	if view.Mode == displayLabel {
		text = view.Label
	}
	if view.Mode == displayHidden || text == "" {
		return ""
	}
	style := lipgloss.NewStyle().Bold(true)
	if view.Color != "" {
		style = style.Foreground(lipgloss.Color(view.Color))
	}
	return style.Render(text)
}

func renderDefaultTmuxIndicator(
	s styles,
	view indicatorView,
	attached bool,
	bracketLabel bool,
) string {
	if view.Mode == displayHidden {
		return ""
	}
	text := view.Icon
	if view.Mode == displayLabel {
		text = view.Label
		if bracketLabel {
			text = "[" + text + "]"
		}
	}
	return renderTmuxStateStyled(s, attached, text, view.Color)
}

func renderDefaultAgentIndicator(
	s styles,
	view indicatorView,
	stateText string,
	bracketLabel bool,
) string {
	if view.Mode == displayHidden {
		return ""
	}
	text := view.Icon
	if view.Mode == displayLabel {
		text = view.Label
		if bracketLabel {
			text = "[" + text + "]"
		}
	}
	return renderAgentStateStyled(s, sessionmgr.AgentState(stateText), text, view.Color)
}

func renderDefaultRightPane(
	view layoutView,
	theme layoutRenderTheme,
	width, height int,
) string {
	const (
		paneGapH    = 1
		previewMinH = 3
	)
	detail := renderDefaultDetails(view.Details, theme, width, 0)
	detailH := lipgloss.Height(detail)
	maxDetailH := height - paneGapH - previewMinH
	if detailH > maxDetailH {
		detail = renderDefaultDetails(view.Details, theme, width, maxDetailH)
		detailH = lipgloss.Height(detail)
	}
	previewH := height - detailH - paneGapH
	preview := renderDefaultPreview(view.Preview, theme, width, previewH)
	return lipgloss.JoinVertical(lipgloss.Left, detail, "", preview)
}

func renderDefaultDetails(
	view *detailsView,
	theme layoutRenderTheme,
	width, height int,
) string {
	title := "Details"
	if view != nil && view.State == detailsReady {
		title = view.Title
	}
	content := strings.Join(defaultDetailLines(view, theme), "\n")
	if height > 0 {
		content = trimHeight(content, max(1, height-2))
	}
	return paneWithTitle(
		theme.styles.paneDetail,
		theme.styles.metadataTitle,
		content,
		title,
		width,
		height,
	)
}

func defaultDetailLines(view *detailsView, theme layoutRenderTheme) []string {
	s := theme.styles
	if view == nil || view.State != detailsReady {
		return []string{s.muted.Render("select an item")}
	}
	lines := make([]string, 0, len(view.Fields))
	for _, field := range view.Fields {
		value := field.Value
		switch {
		case field.Indicator.Mode == displayIcon:
			value = rowText(renderDefaultDetailIndicator(s, field), field.Value)
		case field.Indicator.Mode == displayLabel &&
			field.IndicatorKind == detailIndicatorAttached:
			value = renderDefaultTmuxIndicator(
				s,
				field.Indicator,
				field.Attached,
				false,
			)
		}
		lines = append(lines, kv(s, field.Label, value))
	}
	return lines
}

func renderDefaultDetailIndicator(s styles, field detailFieldView) string {
	if field.Indicator.Mode != displayIcon {
		return ""
	}
	switch field.IndicatorKind {
	case detailIndicatorAttached:
		return renderTmuxStateStyled(
			s,
			field.Attached,
			field.Indicator.Icon,
			field.Indicator.Color,
		)
	case detailIndicatorAgentState:
		return renderAgentStateStyled(
			s,
			sessionmgr.AgentState(field.AgentState),
			field.Indicator.Icon,
			field.Indicator.Color,
		)
	default:
		return ""
	}
}

func renderDefaultPreview(
	view *previewView,
	theme layoutRenderTheme,
	width, height int,
) string {
	s := theme.styles
	innerW := max(10, width-4)
	innerH := max(1, height-2)
	title := "Preview"
	content := ""
	anchor := previewAnchorTop
	if view != nil {
		title = view.Title
		content = view.Content
		anchor = view.Anchor
		switch view.State {
		case previewError:
			content = s.danger.Render(view.Error)
		case previewLoading:
			content = s.muted.Render("preview loading…")
		}
	}
	if content == "" {
		content = s.muted.Render("preview loading…")
	}
	previewLines := strings.Split(content, "\n")
	start := 0
	if anchor == previewAnchorBottom && len(previewLines) > innerH {
		start = len(previewLines) - innerH
	}
	lines := make([]string, 0, innerH)
	for index := start; index-start < innerH && index < len(previewLines); index++ {
		lines = append(lines, clampText(previewLines[index], innerW))
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
