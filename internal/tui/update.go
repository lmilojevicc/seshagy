package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.searchInput.Width = max(20, msg.Width/3)
		m.renameInput.Width = max(20, msg.Width/3)
		return m, m.previewForSelection()
	case tickMsg:
		if m.integration.active || m.setup.active {
			return m, tickCmd()
		}
		if m.cacheFresh(m.source) {
			return m, tickCmd()
		}
		var cmd tea.Cmd
		m, cmd = m.beginRefresh(m.source, false)
		return m, tea.Batch(cmd, tickCmd())
	case integrationsMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.integration.rows = msg.recs
		m.integration.selected = map[integrations.Target]bool{}
		for _, rec := range msg.recs {
			m.integration.selected[rec.Target] = rec.AgentAvailable && rec.Installable &&
				rec.State != integrations.StatusCurrent
		}
		if m.integration.cursor >= len(m.integration.rows) {
			m.integration.cursor = max(0, len(m.integration.rows)-1)
		}
		if len(msg.recs) > 0 {
			m.integration.active = true
			if msg.startup {
				m.integration.startupPrompt = true
			}
			m.status = "install hook integrations for detected agents"
		} else if m.integration.active {
			m.integration.active = false
			m.status = "no missing hook integrations for detected agents"
		}
		return m, nil
	case integrationsInstalledMsg:
		m.integration.messages = msg.messages
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, integrationsCmd()
		}
		m.err = nil
		if len(msg.messages) == 0 {
			m.status = "no integrations selected"
		} else {
			m.status = strings.Join(msg.messages, " · ")
		}
		if m.integration.startupPrompt {
			if err := recordIntegrationPromptDismissed(); err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.integration.startupPrompt = false
		}
		m.integration.active = false
		m = m.invalidateAllCaches()
		var refresh tea.Cmd
		m, refresh = m.beginRefresh(m.source, true)
		return m, tea.Batch(integrationsCmd(), refresh)
	case setupMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, startupIntegrationsCmd()
		}
		if msg.prompt {
			m.openInputModePrompt(false)
			m.status = "choose startup input mode"
			return m, nil
		}
		return m, startupIntegrationsCmd()
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
		m.status = fmt.Sprintf("%s session %s", verb, msg.name)
		return m, attachCmd(msg.name)
	case attachDoneMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.err = msg.err
		} else {
			m.status = "returned from tmux"
		}
		m = m.invalidateAllCaches()
		var refresh tea.Cmd
		m, refresh = m.beginRefresh(m.source, true)
		return m, tea.Batch(refresh, m.previewForSelection())
	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
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
		return m, createSessionCmd(msg.path)
	case tea.KeyMsg:
		if m.setup.active {
			return m.handleSetupKey(msg)
		}
		if m.integration.active {
			return m.handleIntegrationKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}
