package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputMode == modeSearch {
		switch msg.String() {
		case "esc":
			m.inputMode = modeNormal
			m.searchInput.Blur()
			return m, nil
		case "enter":
			m.inputMode = modeNormal
			m.searchInput.Blur()
			m.query = m.searchInput.Value()
			m.clampCursor()
			return m, m.previewForSelection()
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, m.previewForSelection()
		case "down":
			if m.cursor < len(m.visibleItems())-1 {
				m.cursor++
			}
			return m, m.previewForSelection()
		case "ctrl+c":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.query = m.searchInput.Value()
		m.clampCursor()
		return m, tea.Batch(cmd, m.previewForSelection())
	}

	if m.inputMode == modeRename {
		switch msg.String() {
		case "esc":
			m.inputMode = modeNormal
			m.renameInput.Blur()
			m.status = "rename cancelled"
			return m, nil
		case "enter":
			newName := strings.TrimSpace(m.renameInput.Value())
			oldName := m.renameFrom
			m.inputMode = modeNormal
			m.renameInput.Blur()
			m.renameFrom = ""
			if newName == "" || oldName == "" || newName == oldName {
				m.status = "rename cancelled"
				return m, nil
			}
			return m, renameCmd(oldName, newName)
		case "ctrl+c":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}

	if m.config.TypeFirst.Enabled {
		return m.handleTypeFirstKey(msg)
	}
	return m.handleActionKey(msg)
}

func (m Model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if source, ok := m.sourceForNumberKey(msg.String()); ok {
		return m.switchSource(source)
	}
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, m.previewForSelection()
	case "down", "j":
		if m.cursor < len(m.visibleItems())-1 {
			m.cursor++
		}
		return m, m.previewForSelection()
	case "pgup", "ctrl+u":
		m.cursor = max(0, m.cursor-10)
		return m, m.previewForSelection()
	case "pgdown", "ctrl+d":
		m.cursor = min(max(0, len(m.visibleItems())-1), m.cursor+10)
		return m, m.previewForSelection()
	case "home":
		m.cursor = 0
		return m, m.previewForSelection()
	case "end":
		m.cursor = max(0, len(m.visibleItems())-1)
		return m, m.previewForSelection()
	case "enter":
		return m.activateSelected()
	case "r", "ctrl+r":
		m.loading = true
		m.status = "refreshing"
		return m, refreshCmd(m.source, m.config.LoadOptions())
	case "x", "ctrl+x":
		return m.deleteSelected()
	case "R":
		return m.startRename()
	case "/":
		m.inputMode = modeSearch
		m.searchInput.SetValue(m.query)
		m.searchInput.Focus()
		return m, textinput.Blink
	case "backspace":
		if m.query != "" {
			m.query = ""
			m.searchInput.SetValue("")
			m.cursor = 0
			return m, m.previewForSelection()
		}
	case "p", "alt+p":
		m.showPreview = !m.showPreview
		return m, m.previewForSelection()
	case "s":
		return m.cycleAgentStateFilter()
	case "S":
		return m.clearAgentStateFilter()
	case "i":
		m.integration.active = true
		m.status = "scanning hook integrations"
		return m, integrationsCmd()
	case "m":
		m.openInputModePrompt(true)
		m.status = "change input mode"
		return m, nil
	case "?", "h", "alt+h":
		m.showHelp = !m.showHelp
		return m, nil
	case "a", "ctrl+a":
		return m.switchSource(sessionmgr.ModeAll)
	case "t", "ctrl+t":
		return m.switchSource(sessionmgr.ModeSessions)
	case "g", "ctrl+g":
		return m.switchSource(sessionmgr.ModeAgents)
	case "o", "ctrl+o":
		return m.switchSource(sessionmgr.ModeCurrentAgents)
	case "z", "ctrl+z":
		return m.switchSource(sessionmgr.ModeZoxide)
	case "f", "ctrl+f":
		return m.switchSource(sessionmgr.ModeFD)
	case "y", "ctrl+y":
		return m.startYazi()
	}
	return m, nil
}

func (m Model) sourceForNumberKey(key string) (sessionmgr.SourceMode, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return sessionmgr.ModeAll, false
	}
	idx := int(key[0] - '1')
	order := m.config.SourceOrder()
	if idx < 0 || idx >= len(order) {
		return sessionmgr.ModeAll, false
	}
	return order[idx], true
}

func (m Model) handleTypeFirstKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.prefixArmed {
		m.prefixArmed = false
		if m.isPrefixKey(msg) {
			m.status = "prefix cancelled"
			return m, nil
		}
		return m.handleActionKey(msg)
	}
	if m.isPrefixKey(msg) {
		m.prefixArmed = true
		m.status = "prefix active: next key is an action"
		return m, nil
	}
	if isUnprefixedNavigationKey(msg) {
		return m.handleActionKey(msg)
	}
	switch msg.String() {
	case "backspace":
		return m.deleteFilterRune()
	case "esc":
		return m.clearFilterText()
	}
	if isPrintableKey(msg) {
		return m.appendFilterText(string(msg.Runes))
	}
	m.status = "press " + m.config.PrefixKey() + " before actions"
	return m, nil
}

func (m Model) isPrefixKey(msg tea.KeyMsg) bool {
	return msg.String() == m.config.PrefixKey()
}

func isPrintableKey(msg tea.KeyMsg) bool {
	return len(msg.Runes) > 0 && !msg.Alt
}

func isUnprefixedNavigationKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "enter", "up", "down", "pgup", "ctrl+u", "pgdown", "ctrl+d", "home", "end":
		return true
	default:
		return false
	}
}

func (m Model) appendFilterText(text string) (tea.Model, tea.Cmd) {
	m.query += text
	m.searchInput.SetValue(m.query)
	m.clampCursor()
	m.status = "filter: " + m.query
	return m, m.previewForSelection()
}

func (m Model) deleteFilterRune() (tea.Model, tea.Cmd) {
	if m.query == "" {
		return m, nil
	}
	runes := []rune(m.query)
	m.query = string(runes[:len(runes)-1])
	m.searchInput.SetValue(m.query)
	m.clampCursor()
	if m.query == "" {
		m.status = "filter cleared"
	} else {
		m.status = "filter: " + m.query
	}
	return m, m.previewForSelection()
}

func (m Model) clearFilterText() (tea.Model, tea.Cmd) {
	if m.query == "" {
		return m, nil
	}
	m.query = ""
	m.searchInput.SetValue("")
	m.cursor = 0
	m.status = "filter cleared"
	return m, m.previewForSelection()
}

func (m Model) handleIntegrationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc", "s":
		m.integration.active = false
		m.status = "hook installation skipped"
		if m.integration.startupPrompt {
			if err := recordIntegrationPromptDismissed(); err != nil {
				m.status = err.Error()
				return m, nil
			}
			m.integration.startupPrompt = false
		}
		return m, nil
	case "up", "k":
		if m.integration.cursor > 0 {
			m.integration.cursor--
		}
		return m, nil
	case "down", "j":
		if m.integration.cursor < len(m.integration.rows)-1 {
			m.integration.cursor++
		}
		return m, nil
	case " ":
		if len(m.integration.rows) == 0 {
			return m, nil
		}
		rec := m.integration.rows[m.integration.cursor]
		if rec.AgentAvailable && rec.Installable && rec.State != integrations.StatusCurrent {
			m.integration.selected[rec.Target] = !m.integration.selected[rec.Target]
		}
		return m, nil
	case "enter":
		var targets []integrations.Target
		for _, rec := range m.integration.rows {
			if m.integration.selected[rec.Target] {
				targets = append(targets, rec.Target)
			}
		}
		m.status = "installing selected hook integrations"
		return m, installIntegrationsCmd(targets)
	case "r":
		m.status = "rescanning hook integrations"
		return m, integrationsCmd()
	}
	return m, nil
}

func (m *Model) openInputModePrompt(manual bool) {
	m.setup.active = true
	m.setup.manual = manual
	if m.config.TypeFirst.Enabled {
		m.setup.cursor = 0
	} else {
		m.setup.cursor = 1
	}
}

func (m Model) handleSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "down", "j", "k", "tab":
		if m.setup.cursor == 0 {
			m.setup.cursor = 1
		} else {
			m.setup.cursor = 0
		}
		return m, nil
	case "y", "Y":
		return m.applyTypeFirstSetup(true)
	case "n", "N":
		return m.applyTypeFirstSetup(false)
	case "esc":
		if m.setup.manual {
			return m.cancelInputModePrompt()
		}
		return m.applyTypeFirstSetup(false)
	case "enter":
		return m.applyTypeFirstSetup(m.setup.cursor == 0)
	}
	return m, nil
}

func (m Model) cancelInputModePrompt() (tea.Model, tea.Cmd) {
	m.setup.active = false
	m.setup.manual = false
	m.status = "input mode change cancelled"
	return m, nil
}

func (m Model) applyTypeFirstSetup(enabled bool) (tea.Model, tea.Cmd) {
	manual := m.setup.manual
	cfg := m.config
	cfg.TypeFirst.Enabled = enabled
	if strings.TrimSpace(cfg.TypeFirst.Prefix) == "" {
		cfg.TypeFirst.Prefix = appconfig.DefaultPrefix
	}
	cfg.Setup.TypeFirstPromptSeen = true
	if err := appconfig.Save(cfg); err != nil {
		m.err = err
		m.status = err.Error()
		return m, nil
	}
	m.config = cfg
	m.setup.active = false
	m.setup.manual = false
	m.err = nil
	if enabled {
		m.status = "type-first mode enabled"
	} else {
		m.status = "classic input mode selected"
	}
	if manual {
		return m, nil
	}
	return m, startupIntegrationsCmd()
}

func (m Model) switchSource(source sessionmgr.SourceMode) (tea.Model, tea.Cmd) {
	m.source = source
	m.cursor = 0
	m.loading = true
	m.status = "loading " + modeName(source)
	return m, refreshCmd(source, m.config.LoadOptions())
}

func (m Model) cycleAgentStateFilter() (tea.Model, tea.Cmd) {
	if !isAgentSource(m.source) {
		m.status = "state filter only applies to agent panes"
		return m, nil
	}
	m.agentStateFilter = nextAgentStateFilter(m.agentStateFilter)
	m.cursor = 0
	m.clampCursor()
	m.status = "agent state filter: " + agentStateFilterLabel(m.agentStateFilter)
	return m, m.previewForSelection()
}

func (m Model) clearAgentStateFilter() (tea.Model, tea.Cmd) {
	if !isAgentSource(m.source) {
		m.status = "state filter only applies to agent panes"
		return m, nil
	}
	m.agentStateFilter = ""
	m.cursor = 0
	m.clampCursor()
	m.status = "agent state filter: all"
	return m, m.previewForSelection()
}

func (m Model) activateSelected() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		m.status = "nothing selected"
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		m.status = "attaching " + item.Name
		return m, attachCmd(item.Name)
	case sessionmgr.KindAgent:
		m.status = "focusing " + item.Location
		return m, focusAgentCmd(item.PaneID)
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		m.status = "creating session from " + item.Path
		return m, createSessionCmd(item.Path)
	default:
		return m, nil
	}
}

func (m Model) deleteSelected() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		m.status = "nothing selected"
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		m.status = "killing session " + item.Name
		return m, deleteSessionCmd(item.Name)
	case sessionmgr.KindAgent:
		m.status = "killing pane " + item.PaneID
		return m, deleteAgentCmd(item.PaneID)
	default:
		m.status = "delete only applies to sessions and agents"
		return m, nil
	}
}

func (m Model) startRename() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || item.Kind != sessionmgr.KindSession {
		m.status = "rename only applies to sessions"
		return m, nil
	}
	m.inputMode = modeRename
	m.renameFrom = item.Name
	m.renameInput.SetValue(item.Name)
	m.renameInput.Focus()
	m.status = "renaming " + item.Name
	return m, textinput.Blink
}

func (m Model) startYazi() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inPopup, err := checkTmuxPopup(ctx)
	if err != nil {
		m.status = fmt.Sprintf("checking tmux popup: %v", err)
		m.err = err
		return m, nil
	}
	if inPopup {
		err := fmt.Errorf("cannot open yazi inside a tmux popup")
		m.status = err.Error()
		m.err = err
		return m, nil
	}
	file, err := os.CreateTemp("", "seshagy-yazi-*")
	if err != nil {
		m.status = err.Error()
		m.err = err
		return m, nil
	}
	path := file.Name()
	_ = file.Close()
	m.status = "opening yazi"
	return m, tea.ExecProcess(sessionmgr.RunYaziCommand(path), func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return yaziDoneMsg{err: err}
		}
		data, _ := os.ReadFile(path)
		return yaziDoneMsg{path: strings.TrimSpace(string(data))}
	})
}
