package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// ephemeralTickMsg drives the --ephemeral focus-loss poll. It uses a faster
// cadence (150ms) than the regular data-refresh tick (1-5s) so dismissal feels
// instant when the user switches focus away from the dashboard.
type ephemeralTickMsg time.Time

const ephemeralTickInterval = 150 * time.Millisecond

func ephemeralTickCmd() tea.Cmd {
	return tea.Tick(ephemeralTickInterval, func(t time.Time) tea.Msg {
		return ephemeralTickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.searchInput.Width = max(20, msg.Width/2)
		m.renameInput.Width = max(20, msg.Width/2)
		return m, m.previewForSelection()
	case tickMsg:
		interval := tickIntervalFor(m.source)
		if m.setup.active {
			return m, tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		if m.installMenu.active {
			return m, tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		if m.cacheFresh(m.source) {
			// Even when the active source is fresh, keep the ModeAll cache warm
			// so the overview hero counts stay current on other tabs.
			cmds := []tea.Cmd{tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })}
			if !m.cacheFresh(sessionmgr.ModeAll) {
				if _, mc := m.beginRefresh(sessionmgr.ModeAll, false); mc != nil {
					cmds = append(cmds, mc)
				}
			}
			return m, tea.Batch(cmds...)
		}
		var cmd tea.Cmd
		m, cmd = m.beginRefresh(m.source, false)
		cmds := []tea.Cmd{
			cmd,
			tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) }),
		}
		if !m.cacheFresh(sessionmgr.ModeAll) {
			if _, mc := m.beginRefresh(sessionmgr.ModeAll, false); mc != nil {
				cmds = append(cmds, mc)
			}
		}
		return m, tea.Batch(cmds...)
	case ephemeralTickMsg:
		if !m.ephemeral {
			return m, nil
		}
		// A kill is in flight: the close refocuses the client, but KillSession
		// restores focus right after. Don't treat that transient focus-loss as
		// a dismissal — keep the dashboard alive until the action completes.
		if m.killInFlight {
			return m, ephemeralTickCmd()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		switch m.mux.Kind() {
		case sessionmgr.BackendNone:
			// No multiplexer: focus-loss is undefined; no-op the poll.
			return m, ephemeralTickCmd()
		case sessionmgr.BackendHerdr:
			if m.herdrPaneID == "" || m.herdrWorkspaceID == "" {
				if paneID, wsID, ok := sessionmgr.ResolveHerdrEphemeralTarget(ctx); ok {
					m.herdrPaneID = paneID
					m.herdrWorkspaceID = wsID
				}
			}
			if m.herdrPaneID != "" &&
				sessionmgr.HerdrFocusLost(ctx, m.herdrPaneID, m.herdrWorkspaceID) {
				return m, tea.Quit
			}
			return m, ephemeralTickCmd()
		default: // BackendTmux
			if sessionmgr.TmuxFocusLost(ctx, "", "") {
				return m, tea.Quit
			}
			return m, ephemeralTickCmd()
		}
	case setupMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		if msg.prompt {
			m.openInputModePrompt(false)
			m.status = "choose startup input mode"
			return m, nil
		}
		return m, nil
	case installMenuMsg:
		if msg.show {
			if m.setup.active {
				m.pendingInstall = true
				return m, nil
			}
			m.openInstallMenu(true)
			return m, nil
		}
		return m, nil
	case installResultMsg:
		if msg.err != nil {
			m.installMenu.statuses[msg.name] = "failed"
			m.installMenu.message = msg.name + " " + msg.action + " failed: " + msg.err.Error()
		} else {
			if msg.action == "install" {
				m.installMenu.statuses[msg.name] = "installed"
			} else {
				m.installMenu.statuses[msg.name] = "uninstalled"
			}
			m.installMenu.message = msg.name + " " + msg.action + " ok"
		}
		return m, nil
	case catalogRefreshMsg:
		if msg.err != nil {
			m.status = "catalog refresh skipped"
			return m, nil
		}
		sessionmgr.ReloadManifests()
		updated := len(msg.result.Fetched)
		if updated > 0 {
			m.status = fmt.Sprintf("agent rules updated (%d)", updated)
			m = m.invalidateAllCaches()
			var refresh tea.Cmd
			m, refresh = m.beginRefresh(m.source, true)
			return m, tea.Batch(refresh, m.previewForSelection())
		}
		return m, nil
	case refreshMsg:
		return m.handleRefreshMsg(msg)
	case previewMsg:
		if msg.key != m.selectedKey() {
			return m, nil
		}
		m.previewKey = msg.key
		if msg.err != nil {
			m.preview = m.styles.danger.Render(msg.err.Error())
		} else {
			m.preview = msg.preview
		}
		return m, nil
	case createDoneMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.err = msg.err
			return m, nil
		}
		verb := "using"
		if msg.created {
			verb = "created"
		}
		m.status = fmt.Sprintf("%s %s %s", verb, m.terms.SessionNoun, msg.item.Name)
		return m, attachCmd(m.mux, msg.item)
	case attachDoneMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.err = msg.err
		} else {
			m.status = "returned from " + m.terms.BackendName
		}
		m = m.invalidateAllCaches()
		var refresh tea.Cmd
		m, refresh = m.beginRefresh(m.source, true)
		return m, tea.Batch(refresh, m.previewForSelection())
	case actionDoneMsg:
		if msg.kind == actionKill {
			m.killInFlight = false
		}
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			m = m.invalidateAllCaches()
			var refresh tea.Cmd
			m, refresh = m.beginRefresh(m.source, true)
			return m, tea.Batch(refresh, m.previewForSelection())
		}
		m.err = nil
		m.status = msg.status
		m = m.invalidateAllCaches()
		var refresh tea.Cmd
		m, refresh = m.beginRefresh(m.source, true)
		return m, tea.Batch(refresh, m.previewForSelection())
	case yaziDoneMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.err = msg.err
			return m, nil
		}
		if msg.path == "" {
			m.status = "yazi closed without a directory"
			return m, nil
		}
		return m, createSessionCmd(m.mux, msg.path)
	case tea.KeyMsg:
		if m.setup.active {
			model, cmd := m.handleSetupKey(msg)
			if mm, ok := model.(Model); ok && !mm.setup.active && mm.pendingInstall {
				mm.pendingInstall = false
				mm.openInstallMenu(true)
				return mm, nil
			}
			return model, cmd
		}
		if m.installMenu.active {
			return m.handleInstallMenuKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}
