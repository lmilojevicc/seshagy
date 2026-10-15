package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/logging"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type modeCache struct {
	items     []sessionmgr.Item
	fetchedAt time.Time
}

// cacheTTL is how long a source's results stay usable before the 1s tick
// refreshes it. Agents are polled fast for state changes, sessions are cheap,
// and directory listings barely move.
func cacheTTL(mode sessionmgr.SourceMode) time.Duration {
	switch mode {
	case sessionmgr.ModeAgents:
		return time.Second
	case sessionmgr.ModeSessions:
		return 2 * time.Second
	case sessionmgr.ModeZoxide:
		return 15 * time.Second
	case sessionmgr.ModeFD:
		return time.Minute
	default:
		return 5 * time.Second
	}
}

func (m Model) cacheEntry(mode sessionmgr.SourceMode) (modeCache, bool) {
	if m.cache == nil {
		return modeCache{}, false
	}
	entry, ok := m.cache[mode]
	return entry, ok
}

func (m Model) cacheFresh(mode sessionmgr.SourceMode) bool {
	entry, ok := m.cacheEntry(mode)
	if !ok {
		return false
	}
	return time.Since(entry.fetchedAt) < cacheTTL(mode)
}

// mergedItems rebuilds the visible list from every loaded source: sessions and
// agents first (itemRank then sorts by urgency), then the directory sources with
// fd discoveries already covered by zoxide dropped, then the remote hosts.
func (m Model) mergedItems() []sessionmgr.Item {
	merged := make([]sessionmgr.Item, 0, len(m.items))
	for _, mode := range tuiSources {
		entry, ok := m.cacheEntry(mode)
		if !ok {
			continue
		}
		merged = append(merged, entry.items...)
	}
	merged = append(merged, hostItems(m.hosts)...)
	merged = dedupeDirs(merged)
	return m.stampAgentStarts(merged)
}

// stampAgentStarts fills in the agent process start time each row's age column
// is computed from. The cache is keyed by pane id, and an entry resolved from a
// pid the pane no longer hosts is skipped so a replaced agent never inherits the
// previous process's age.
func (m Model) stampAgentStarts(items []sessionmgr.Item) []sessionmgr.Item {
	if len(m.agentStarted) == 0 {
		return items
	}
	for i := range items {
		if items[i].Kind != sessionmgr.KindAgent {
			continue
		}
		start, ok := m.agentStarted[items[i].PaneID]
		if !ok || (items[i].AgentPID > 0 && start.pid != items[i].AgentPID) {
			continue
		}
		items[i].StartedAt = start.started
	}
	return items
}

func (m Model) invalidateAllCaches() Model {
	if m.inflightRefresh != nil {
		if m.refreshGen == nil {
			m.refreshGen = make(map[sessionmgr.SourceMode]uint64)
		}
		for mode := range m.inflightRefresh {
			m.refreshGen[mode]++
		}
	}
	m.inflightRefresh = make(map[sessionmgr.SourceMode]uint64)
	m.cache = make(map[sessionmgr.SourceMode]modeCache)
	m.reportedWarnings = make(map[sessionmgr.SourceMode]string)
	return m
}

func (m Model) storeCache(mode sessionmgr.SourceMode, items []sessionmgr.Item) Model {
	if m.cache == nil {
		m.cache = make(map[sessionmgr.SourceMode]modeCache)
	}
	m.cache[mode] = modeCache{items: items, fetchedAt: time.Now()}
	return m
}

func (m Model) beginRefresh(source sessionmgr.SourceMode, force bool) (Model, tea.Cmd) {
	if m.inflightRefresh == nil {
		m.inflightRefresh = make(map[sessionmgr.SourceMode]uint64)
	}
	if m.refreshGen == nil {
		m.refreshGen = make(map[sessionmgr.SourceMode]uint64)
	}
	if force {
		delete(m.inflightRefresh, source)
	}
	if m.inflightRefresh[source] != 0 {
		return m, nil
	}
	m.refreshGen[source]++
	gen := m.refreshGen[source]
	m.inflightRefresh[source] = gen
	refresh := refreshCmd(m.mux, source, gen, m.config.LoadOptions())
	if m.spinnerActive {
		return m, refresh
	}
	m.spinnerActive = true
	return m, tea.Batch(refresh, spinnerTickCmd())
}

func (m Model) finishRefresh(source sessionmgr.SourceMode, gen uint64) Model {
	if m.inflightRefresh == nil || m.inflightRefresh[source] != gen {
		return m
	}
	delete(m.inflightRefresh, source)
	return m
}

func (m Model) refreshInflight(mode sessionmgr.SourceMode) bool {
	if m.inflightRefresh == nil {
		return false
	}
	return m.inflightRefresh[mode] != 0
}

func (m Model) anyRefreshInflight() bool {
	for _, gen := range m.inflightRefresh {
		if gen != 0 {
			return true
		}
	}
	return false
}

// reportSourceFailure toasts a source failure once per distinct message so a
// persistently broken source (missing fd, unreachable multiplexer) does not
// re-toast every second.
func (m Model) reportSourceFailure(
	source sessionmgr.SourceMode,
	message string,
	sev notifSev,
) Model {
	if message == "" {
		return m
	}
	if m.reportedWarnings == nil {
		m.reportedWarnings = make(map[sessionmgr.SourceMode]string)
	}
	if m.reportedWarnings[source] == message {
		return m
	}
	m.reportedWarnings[source] = message
	m.notify(message, sev)
	return m
}

func (m Model) handleRefreshMsg(msg refreshMsg) (Model, tea.Cmd) {
	if m.inflightRefresh == nil || msg.gen != m.inflightRefresh[msg.source] {
		ctx := context.Background()
		if m.logger.Enabled(ctx, slog.LevelDebug) {
			logging.LogAttrs(ctx, m.logger, slog.LevelDebug,
				logging.EventTUIRefreshStale, logging.ComponentTUI,
				slog.String("source", msg.source.Names().ConfigToken),
				slog.Int64("generation", int64(msg.gen)),
				slog.Int64("current_generation", int64(m.refreshGen[msg.source])),
			)
		}
		return m, nil
	}
	m = m.finishRefresh(msg.source, msg.gen)
	m = m.storeCache(msg.source, msg.items)
	m.items = m.mergedItems()
	m.loadedOnce = true
	m.syncCursor()
	switch {
	case msg.err != nil:
		m = m.reportSourceFailure(msg.source, msg.err.Error(), sevError)
	case msg.warning != "":
		m = m.reportSourceFailure(msg.source, msg.warning, sevWarning)
	}
	var preview tea.Cmd
	m, preview = m.requestPreview()
	cmds := make([]tea.Cmd, 0, 2)
	if preview != nil {
		cmds = append(cmds, preview)
	}
	// New agent panes (and panes whose agent was replaced) need their process
	// start time before the age column can show it.
	if msg.source == sessionmgr.ModeAgents && msg.err == nil {
		var starts tea.Cmd
		m, starts = m.resolveAgentStarts()
		if starts != nil {
			cmds = append(cmds, starts)
		}
	}
	return m, tea.Batch(cmds...)
}
