package tui

import (
	"fmt"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func (m Model) projectLayout(needs layoutNeeds) layoutView {
	return m.projectLayoutAtWithNeeds(time.Now(), needs)
}

func (m Model) projectLayoutAt(now time.Time) layoutView {
	return m.projectLayoutAtWithNeeds(now, layoutNeeds{})
}

func (m Model) projectLayoutAtWithNeeds(now time.Time, needs layoutNeeds) layoutView {
	visible := m.visibleItems()
	view := layoutView{
		Frame:   frameView{Width: m.width, Height: m.height},
		Sources: m.projectSources(visible),
		Collection: m.projectCollection(
			visible,
			now,
		),
		Search:  m.projectSearch(),
		Actions: m.projectActions(),
	}
	if needs.Overview {
		view.Overview = m.projectOverview()
	}
	if needs.Details {
		view.Details = m.projectDetails(now)
	}
	if needs.Preview && m.showPreview {
		view.Preview = m.projectPreview()
	}
	return view
}

func (m Model) projectSources(visible []sessionmgr.Item) sourcesView {
	order := m.config.SourceOrder()
	entries := make([]sourceEntryView, 0, len(order))
	for i, mode := range order {
		items, known := m.projectionItemsForSource(mode)
		entries = append(entries, sourceEntryView{
			ID:         projectionSourceID(mode),
			Key:        fmt.Sprintf("%d", i+1),
			Label:      mode.DisplayNames(m.terms).Tab,
			Selected:   mode == m.source,
			Count:      len(items),
			CountKnown: known,
			Refreshing: m.refreshInflight(mode),
		})
	}
	return sourcesView{
		Entries:      entries,
		VisibleCount: len(visible),
		TotalCount:   len(m.items),
		Loading:      m.loading,
		Refreshing:   m.refreshInflight(m.source),
		SpinnerFrame: m.spinnerFrame,
	}
}

func (m Model) projectionItemsForSource(mode sessionmgr.SourceMode) ([]sessionmgr.Item, bool) {
	if mode == m.source {
		return m.items, true
	}
	entry, ok := m.cacheEntry(mode)
	if !ok {
		return nil, false
	}
	return entry.items, true
}

func projectionSourceID(mode sessionmgr.SourceMode) sourceID {
	return sourceID(appconfig.SourceModeName(mode))
}

func (m Model) projectCollection(visible []sessionmgr.Item, now time.Time) collectionView {
	rows := make([]rowView, len(visible))
	for i, item := range visible {
		rows[i] = m.projectRow(item, i == m.cursor, now)
	}
	counts := sortedCounts(visible)
	view := collectionView{
		Source:         projectionSourceID(m.source),
		Title:          m.source.DisplayNames(m.terms).Title,
		Rows:           rows,
		State:          collectionReady,
		VisibleCount:   len(visible),
		TotalCount:     len(m.items),
		SessionCount:   counts[sessionmgr.KindSession],
		AgentCount:     counts[sessionmgr.KindAgent],
		DirectoryCount: counts[sessionmgr.KindZoxide] + counts[sessionmgr.KindFD],
	}
	if m.source == sessionmgr.ModeAgents {
		view.ScopeLabel = "all"
		if m.agentsCurrentOnly {
			if m.currentSession == "" {
				view.ScopeLabel = "current " + m.terms.SessionNoun
			} else {
				view.ScopeLabel = m.currentSessionLabel()
			}
		}
		if m.agentsStateFilter != "" {
			view.StateFilterLabel = "state: " + string(m.agentsStateFilter)
		}
	}

	entry, cached := m.cacheEntry(m.source)
	if m.loading && len(visible) == 0 {
		view.State = collectionLoading
		return view
	}
	if cached {
		view.Warning = entry.warning
	}
	switch {
	case cached && entry.err != nil:
		view.State = collectionError
		view.Error = entry.err.Error()
	case len(visible) == 0 && m.collectionFilterActive():
		view.State = collectionFilteredEmpty
		view.EmptyMessage = "no items"
		if m.query != "" {
			view.EmptyMessage = "no matches for " + m.query
		}
	case len(visible) == 0:
		view.State = collectionEmpty
		view.EmptyMessage = "no items"
	}
	return view
}

func (m Model) projectOverview() *overviewView {
	view := &overviewView{State: overviewLoading}
	var (
		items []sessionmgr.Item
		entry modeCache
		known bool
	)
	if m.source == sessionmgr.ModeAll {
		items = m.items
		entry, known = m.cacheEntry(sessionmgr.ModeAll)
		if m.loading && len(items) == 0 {
			return view
		}
	} else {
		entry, known = m.cacheEntry(sessionmgr.ModeAll)
		if !known {
			return view
		}
		items = entry.items
	}

	stats := aggregateOverviewStats(items)
	view.Sessions = stats.sessions
	view.Agents = overviewAgentCountsView{
		Working: stats.agents[sessionmgr.AgentWorking],
		Blocked: stats.agents[sessionmgr.AgentBlocked],
		Done:    stats.agents[sessionmgr.AgentDone],
		Idle:    stats.agents[sessionmgr.AgentIdle],
		Unknown: stats.agents[sessionmgr.AgentUnknown],
	}
	switch {
	case known && entry.err != nil:
		view.State = overviewError
		view.Error = entry.err.Error()
	case known && entry.warning != "":
		view.State = overviewWarning
		view.Warning = entry.warning
	case len(items) == 0:
		view.State = overviewEmpty
	default:
		view.State = overviewReady
	}
	return view
}

func (m Model) projectDetails(now time.Time) *detailsView {
	view := &detailsView{State: detailsNoSelection, Title: "Details"}
	item, ok := m.selectedItem()
	if !ok {
		return view
	}
	view.State = detailsReady
	view.Title = m.projectDetailsTitle(item)
	icons := m.config.IconSet()
	switch item.Kind {
	case sessionmgr.KindSession:
		attached, attachedIndicator := projectAttachedDetail(icons, item.Attached)
		view.Fields = []detailFieldView{
			{Label: "path", Value: sessionmgr.ContractHome(item.Path)},
			{
				Label: "attached", Value: attached,
				Indicator: attachedIndicator,
			},
			{Label: m.terms.WindowPlural, Value: fmt.Sprint(item.Windows)},
		}
		if item.Panes > 0 {
			view.Fields = append(view.Fields, detailFieldView{
				Label: m.terms.PanePlural, Value: fmt.Sprint(item.Panes),
			})
		}
		if !item.Activity.IsZero() {
			view.Fields = append(view.Fields, detailFieldView{
				Label: "activity", Value: agoAt(item.Activity, now),
			})
		}
		if !item.Created.IsZero() {
			view.Fields = append(view.Fields, detailFieldView{
				Label: "created", Value: agoAt(item.Created, now),
			})
		}
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		view.Fields = []detailFieldView{
			{Label: "path", Value: item.Path},
			{
				Label: "enter",
				Value: "create/switch " + m.terms.BackendName + " " + m.terms.SessionNoun,
			},
		}
	case sessionmgr.KindAgent:
		view.Fields = []detailFieldView{
			{
				Label: "state", Value: agentStateText(item.AgentState),
				Indicator: projectAgentStateIndicator(icons, item.AgentState),
			},
			{Label: "location", Value: item.Location},
		}
		if m.terms.BackendName != "herdr" {
			view.Fields = append(view.Fields, detailFieldView{
				Label: m.terms.SessionNoun, Value: item.Session,
			})
		}
		if item.TabLabel != "" {
			view.Fields = append(view.Fields, detailFieldView{
				Label: m.terms.WindowNoun, Value: item.TabLabel,
			})
		}
		view.Fields = append(view.Fields, detailFieldView{
			Label: "path", Value: sessionmgr.ContractHome(item.Path),
		})
	}
	return view
}

func projectAttachedDetail(icons sessionmgr.IconSet, attached bool) (string, indicatorView) {
	indicator := projectTmuxStateIndicator(icons, attached)
	if indicator.Mode == displayHidden {
		if attached {
			return "yes", indicator
		}
		return "no", indicator
	}
	label := indicator.Label
	if label == "" {
		if attached {
			label = "attached"
		} else {
			label = "detached"
		}
	}
	return label, indicator
}

func (m Model) projectDetailsTitle(item sessionmgr.Item) string {
	switch item.Kind {
	case sessionmgr.KindSession:
		return item.Name + " · " + m.terms.BackendName + " " + m.terms.SessionNoun
	case sessionmgr.KindAgent:
		return item.DisplayName() + " · agent"
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return sessionmgr.SessionNameFromDir(item.Path) + " · " +
			string(item.Kind) + " directory"
	default:
		return item.DisplayName()
	}
}

func (m Model) projectPreview() *previewView {
	view := &previewView{
		State:   previewLoading,
		Title:   "Preview",
		Content: m.preview,
		Error:   m.previewError,
		Anchor:  previewAnchorTop,
	}
	item, selected := m.selectedItem()
	if selected {
		view.Title = "Preview · " + item.DisplayName()
		if isTailPreviewKind(item.Kind) {
			view.Anchor = previewAnchorBottom
		}
	}
	switch {
	case selected && m.previewKey != "" && m.previewKey != item.Key():
		view.State = previewLoading
	case m.previewError != "":
		view.State = previewError
		view.Content = ""
	case m.preview == noPreviewAvailableText:
		view.State = previewEmpty
	case m.preview == "":
		view.State = previewLoading
	default:
		view.State = previewReady
	}
	return view
}

func (m Model) collectionFilterActive() bool {
	return m.query != "" ||
		(m.source == sessionmgr.ModeAgents &&
			(m.agentsCurrentOnly || m.agentsStateFilter != ""))
}

func (m Model) projectRow(item sessionmgr.Item, selected bool, now time.Time) rowView {
	icons := m.config.IconSet()
	row := rowView{
		Kind:     rowKind(item.Kind),
		Label:    item.DisplayName(),
		Icon:     projectKindIndicator(icons, item.Kind),
		Selected: selected,
	}

	switch item.Kind {
	case sessionmgr.KindSession:
		row.Session = sessionRowView{
			Attached: item.Attached,
			State:    projectTmuxStateIndicator(icons, item.Attached),
			Activity: agoAt(item.Activity, now),
		}
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		row.Directory = directoryRowView{
			Source: projectionSourceID(sourceForDirectoryKind(item.Kind)),
			Path:   item.Path,
		}
	case sessionmgr.KindAgent:
		row.Agent = agentRowView{
			Name:        item.AgentName,
			DisplayName: item.DisplayName(),
			State:       string(item.AgentState),
			Indicator:   projectAgentStateIndicator(icons, item.AgentState),
			Activity:    agoAt(item.AgentUpdated, now),
			Location:    item.Location,
		}
	}
	return row
}

func sourceForDirectoryKind(kind sessionmgr.Kind) sessionmgr.SourceMode {
	if kind == sessionmgr.KindZoxide {
		return sessionmgr.ModeZoxide
	}
	return sessionmgr.ModeFD
}

func projectKindIndicator(icons sessionmgr.IconSet, kind sessionmgr.Kind) indicatorView {
	style := icons.For(kind)
	if style.Text == "" {
		return indicatorView{Mode: displayHidden}
	}
	view := indicatorView{Color: style.Color}
	if icons.ASCII {
		view.Mode = displayLabel
		view.Label = style.Text
	} else {
		view.Mode = displayIcon
		view.Icon = style.Text
	}
	return view
}

func projectTmuxStateIndicator(icons sessionmgr.IconSet, attached bool) indicatorView {
	if icons.TmuxStateHidden() {
		return indicatorView{Mode: displayHidden}
	}
	style := icons.ForTmuxState(attached)
	view := indicatorView{Icon: style.Icon, Label: style.ASCII, Color: style.Color}
	if icons.TmuxStateUsesLabels() {
		view.Mode = displayLabel
	} else {
		view.Mode = displayIcon
	}
	return view
}

func projectAgentStateIndicator(
	icons sessionmgr.IconSet,
	state sessionmgr.AgentState,
) indicatorView {
	if icons.AgentStateHidden() {
		return indicatorView{Mode: displayHidden}
	}
	style := icons.ForAgentState(state)
	view := indicatorView{Icon: style.Icon, Label: style.ASCII, Color: style.Color}
	if icons.AgentStateUsesIcons() {
		view.Mode = displayIcon
	} else {
		view.Mode = displayLabel
	}
	return view
}

func agoAt(value, now time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	d := now.Sub(value)
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

func (m Model) projectSearch() searchView {
	mode := searchClassic
	if m.config.TypeFirst.Enabled {
		mode = searchTypeFirst
	}
	return searchView{
		Mode:        mode,
		Query:       m.query,
		Editing:     m.inputMode == modeSearch,
		Prompt:      m.searchInput.Prompt,
		Placeholder: m.searchInput.Placeholder,
		Prefix:      m.config.PrefixKey(),
	}
}

func (m Model) projectActions() actionsView {
	view := actionsView{
		Expanded:    m.showHelp,
		PrefixArmed: m.prefixArmed,
		Prefix:      m.config.PrefixKey(),
	}
	if !m.showHelp {
		view.Hints = []actionHintView{{Key: "?", Label: "help", Available: true}}
		return view
	}
	if m.config.TypeFirst.Enabled && !m.prefixArmed {
		view.Hints = []actionHintView{
			{Key: "type", Label: "filter", Available: true},
			{Key: m.config.PrefixKey(), Label: "actions", Available: true},
			{Key: m.config.PrefixKey() + " m", Label: "mode", Available: true},
			{Key: "backspace", Label: "edit", Available: true},
		}
		return view
	}

	selected, selectedOK := m.selectedItem()
	canActivate := selectedOK
	if selectedOK && selected.Kind == sessionmgr.KindAgent {
		canActivate = selected.Session != "" && selected.Window != "" && selected.PaneID != ""
	}
	canRename := selectedOK &&
		(selected.Kind == sessionmgr.KindSession || selected.Kind == sessionmgr.KindAgent)
	canDelete := selectedOK && selected.Kind == sessionmgr.KindSession && !m.killInFlight

	view.Hints = []actionHintView{
		{Key: "?", Label: "help", Available: true},
		{Key: "tab/⇧+tab", Label: "sections", Available: true},
		{Key: "q", Label: "quit", Available: true},
		{Key: "enter", Label: "attach/create/focus", Available: canActivate},
		{Key: "/", Label: "filter", Available: true},
		{Key: "r", Label: "refresh", Available: true},
		{Key: "p", Label: "preview", Available: true},
		{Key: "m", Label: "mode", Available: true},
		{Key: "h", Label: "install", Available: true},
	}
	if m.source == sessionmgr.ModeAgents {
		view.Hints = append(view.Hints,
			actionHintView{Key: "o", Label: "this session", Available: true},
			actionHintView{Key: "s", Label: "filter state", Available: true},
			actionHintView{Key: "R", Label: "rename", Available: canRename},
		)
	} else {
		view.Hints = append(view.Hints,
			actionHintView{Key: "R", Label: "rename", Available: canRename},
			actionHintView{Key: "x", Label: "kill", Available: canDelete},
			actionHintView{Key: "y", Label: "yazi", Available: true},
		)
	}
	return view
}
