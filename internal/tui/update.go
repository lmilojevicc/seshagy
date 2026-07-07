package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

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
			return m, tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
		}
		var cmd tea.Cmd
		m, cmd = m.beginRefresh(m.source, false)
		return m, tea.Batch(
			cmd,
			tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) }),
		)
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
