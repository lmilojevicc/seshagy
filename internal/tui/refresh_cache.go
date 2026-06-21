package tui

import (
	"fmt"
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

func (m Model) applyCacheEntry(mode sessionmgr.SourceMode) Model {
	entry, ok := m.cacheEntry(mode)
	if !ok {
		return m
	}
	m.items = entry.items
	m.err = entry.err
	if entry.err != nil {
		m.status = entry.err.Error()
	} else if entry.warning != "" {
		m.status = entry.warning
	} else {
		m.status = fmt.Sprintf("loaded %d item%s", len(entry.items), plural(len(entry.items)))
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
	return m, refreshCmd(source, gen, m.config.LoadOptions())
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
		m.err = msg.err
		m.status = msg.err.Error()
		return m, nil
	}
	m.err = nil
	m.items = msg.items
	m.clampCursor()
	if msg.warning != "" {
		m.status = msg.warning
	} else {
		m.status = fmt.Sprintf("loaded %d item%s", len(msg.items), plural(len(msg.items)))
	}
	return m, m.previewForSelection()
}
