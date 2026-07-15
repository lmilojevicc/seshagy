package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type modeCache struct {
	items     []sessionmgr.Item
	fetchedAt time.Time
	err       error
	warning   string
}

func cacheTTL(mode sessionmgr.SourceMode) time.Duration {
	switch mode {
	case sessionmgr.ModeAll:
		return 2 * time.Second
	case sessionmgr.ModeAgents:
		return 2 * time.Second // fast poll for state changes
	default:
		return 15 * time.Second
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

// overviewItems returns the ModeAll item list used by the top overview hero
// band, so its counts stay correct regardless of the active source tab. The
// active tab's own list (m.items) is used directly when ModeAll is active;
// otherwise the warmed ModeAll cache is read. Returns nil before the first
// ModeAll load completes (the hero hides itself in that case).
func (m Model) overviewItems() []sessionmgr.Item {
	if m.source == sessionmgr.ModeAll {
		return m.items
	}
	entry, ok := m.cacheEntry(sessionmgr.ModeAll)
	if !ok {
		return nil
	}
	return entry.items
}

func (m Model) applyCacheEntry(mode sessionmgr.SourceMode) Model {
	entry, ok := m.cacheEntry(mode)
	if !ok {
		return m
	}
	m.items = entry.items
	if entry.err != nil {
		m.notify(entry.err.Error(), sevError)
	} else if entry.warning != "" {
		m.notify(entry.warning, sevWarning)
	}
	m.clampCursor()
	return m
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
	return m
}

func (m Model) storeCache(
	mode sessionmgr.SourceMode,
	items []sessionmgr.Item,
	warning string,
	err error,
) Model {
	if m.cache == nil {
		m.cache = make(map[sessionmgr.SourceMode]modeCache)
	}
	m.cache[mode] = modeCache{
		items:     items,
		fetchedAt: time.Now(),
		err:       err,
		warning:   warning,
	}
	return m
}

func (m Model) beginRefresh(source sessionmgr.SourceMode, force bool) (Model, tea.Cmd) {
	if m.inflightRefresh == nil {
		m.inflightRefresh = make(map[sessionmgr.SourceMode]uint64)
	}
	if m.refreshGen == nil {
		m.refreshGen = make(map[sessionmgr.SourceMode]uint64)
	}
	if force && m.inflightRefresh != nil {
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

func (m Model) backgroundRefreshing() bool {
	return m.refreshInflight(m.source) && !m.loading
}

func (m Model) handleRefreshMsg(msg refreshMsg) (Model, tea.Cmd) {
	if m.inflightRefresh == nil || msg.gen != m.inflightRefresh[msg.source] {
		return m, nil
	}
	m = m.finishRefresh(msg.source, msg.gen)
	m = m.storeCache(msg.source, msg.items, msg.warning, msg.err)

	if msg.source != m.source {
		return m, nil
	}

	m.loading = false
	if msg.err != nil {
		m.notify(msg.err.Error(), sevError)
		return m, nil
	}
	m.items = msg.items
	m.currentSession = msg.currentSession
	m.clampCursor()
	if msg.warning != "" {
		m.notify(msg.warning, sevWarning)
	}
	return m, m.previewForSelection()
}
