package tui

import (
	"fmt"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func (m Model) projectLayout(_ layoutNeeds) layoutView {
	return m.projectLayoutAt(time.Now())
}

func (m Model) projectLayoutAt(now time.Time) layoutView {
	visible := m.visibleItems()
	return layoutView{
		Frame:   frameView{Width: m.width, Height: m.height},
		Sources: m.projectSources(visible),
		Collection: m.projectCollection(
			visible,
			now,
		),
		Search:  m.projectSearch(),
		Actions: m.projectActions(),
	}
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
