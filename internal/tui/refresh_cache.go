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
		start, ok := m.agentStarted[agentStartKey(items[i])]
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

// switchMachine selects a saved machine ("" is the local server) and drops
// everything that was loaded for the previous one: the caches, the in-flight
// refreshes (the generation bump makes their results stale), the visible rows,
// the preview, the resolved agent start times and the error state.
func (m Model) switchMachine(id string) Model {
	if m.machine == id {
		return m
	}
	m.machine = id
	m = m.invalidateAllCaches()
	m.agentStarted = make(map[string]agentStart)
	m.agentStartPending = false
	m.preview, m.previewKey, m.previewPendingKey, m.previewError = "", "", "", ""
	m.loadError = ""
	m.loadedOnce = false
	m.cursor, m.cursorKey = 0, ""
	m.cursorMoved = false
	m.listTop = 0
	// The visible rows belong to the previous server too: its first refresh may
	// be seconds away, or never arrive on an unreachable profile, and enter or
	// ctrl+x on a leftover row would act on the server the user just left.
	m.items = m.mergedItems()
	m.syncCursor()
	m.killArmed = false
	m.killPending = sessionmgr.Item{}
	return m
}

// applyMachines stores the saved-machine catalog: the local server first, then
// every enabled profile in herdr's order. A disabled profile rejects every
// forwarded command, so it is not offered; a selection that vanished falls back
// to local. A catalog that cannot be read (a herdr without machine support, a
// corrupt catalog) leaves the picker exactly as it is today; once a catalog was
// read, a failed re-read keeps it and never moves the selection.
func (m Model) applyMachines(msg machinesMsg) (Model, tea.Cmd) {
	if m.selector == nil {
		return m, nil
	}
	if msg.err != nil {
		if m.machineErr == nil {
			m.notify("cannot list herdr machines", sevWarning)
		}
		m.machineErr = msg.err
		if len(m.machines) > 0 || m.machine == "" {
			// A catalog that was read once stays, and the selection never moves on
			// its own: this command also runs on every reload, so one slow or
			// unparseable read must not drop the saved profiles and retarget the
			// dashboard mid-work. Only a machine the picker has no catalog for
			// falls back to local.
			return m, nil
		}
		m = m.switchMachine("")
		var reload tea.Cmd
		m, reload = m.reloadCmd()
		return m, reload
	}
	m.machineErr = nil
	machines := make([]sessionmgr.Machine, 0, len(msg.machines)+1)
	machines = append(machines, sessionmgr.LocalMachine())
	for _, machine := range msg.machines {
		if machine.IsLocal() || !machine.Enabled {
			continue
		}
		machines = append(machines, machine)
	}
	m.machines = machines
	if m.machineIndex(m.machine) >= 0 {
		return m, nil
	}
	// The selected profile is gone (removed or disabled): fall back to local and
	// load it, so the picker never shows a server it can no longer reach.
	m.notify("machine removed — back to local", sevWarning)
	m = m.switchMachine("")
	var reload tea.Cmd
	m, reload = m.reloadCmd()
	return m, reload
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
	refresh := refreshCmd(m.activeMux(), source, gen, m.machine, m.config.LoadOptions())
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

// reportSourceFailure toasts a source failure once per distinct message and
// machine so a persistently broken source (missing fd, unreachable multiplexer, a
// dead ssh host) does not re-toast every second — while the same failure on
// another machine still speaks up when the user switches to it.
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
	reported := m.machine + "\x00" + message
	if m.reportedWarnings[source] == reported {
		return m
	}
	m.reportedWarnings[source] = reported
	m.notify(message, sev)
	return m
}

func (m Model) handleRefreshMsg(msg refreshMsg) (Model, tea.Cmd) {
	if msg.machine != m.machine {
		// The selection moved to another machine while this load ran: its items
		// belong to a server the picker is no longer showing.
		return m, nil
	}
	if m.inflightRefresh == nil || msg.gen != m.inflightRefresh[msg.source] {
		ctx := context.Background()
		if m.logger.Enabled(ctx, slog.LevelDebug) {
			logging.LogAttrs(ctx, m.logger, slog.LevelDebug,
				logging.EventTUIRefreshStale, logging.ComponentTUI,
				slog.String("source", msg.source.ConfigToken()),
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
	if m.machine != "" {
		// Only a saved machine gets a lasting error state: a dead ssh host would
		// otherwise leave an empty window with a toast that has already faded.
		// The local machine keeps today's toast-only behaviour.
		if msg.err != nil {
			m.loadError = msg.err.Error()
		} else {
			m.loadError = ""
		}
	}
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
