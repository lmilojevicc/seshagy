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
			m.cursor = wrapCursorUp(m.cursor, len(m.visibleItems()))
			return m, m.previewForSelection()
		case "down":
			m.cursor = wrapCursorDown(m.cursor, len(m.visibleItems()))
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
			m.notify("rename cancelled", sevInfo)
			return m, nil
		case "enter":
			newName := strings.TrimSpace(m.renameInput.Value())
			oldName := m.renameFrom
			kind := m.renameKind
			session := m.renameSession
			target := m.renameTarget
			m.inputMode = modeNormal
			m.renameInput.Blur()
			m.renameFrom = ""
			m.renameTarget = ""
			m.renameSession = ""
			m.renameKind = ""
			if newName == "" && kind == sessionmgr.KindAgent {
				return m, renameAgentCmd(m.mux, target, oldName, session, "")
			}
			if newName == "" || oldName == "" || newName == oldName {
				m.notify("rename cancelled", sevInfo)
				return m, nil
			}
			switch kind {
			case sessionmgr.KindSession:
				return m, renameCmd(m.mux, target, oldName, newName)
			case sessionmgr.KindAgent:
				return m, renameAgentCmd(m.mux, target, oldName, session, newName)
			default:
				m.notify("rename only applies to "+m.terms.SessionPlural+" and agents", sevInfo)
				return m, nil
			}
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
		m.cursor = wrapCursorUp(m.cursor, len(m.visibleItems()))
		return m, m.previewForSelection()
	case "down", "j":
		m.cursor = wrapCursorDown(m.cursor, len(m.visibleItems()))
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
	case "tab":
		return m.switchSource(m.nextSource(+1))
	case "shift+tab":
		return m.switchSource(m.nextSource(-1))
	case "enter":
		return m.activateSelected()
	case "r", "ctrl+r":
		m.notify("refreshing", sevInfo)
		if len(m.items) == 0 {
			m.loading = true
		}
		var cmd tea.Cmd
		m, cmd = m.beginRefresh(m.source, true)
		return m, cmd
	case "x", "ctrl+x":
		return m.deleteSelected()
	case "R":
		return m.startRename()
	case "o":
		if m.source != sessionmgr.ModeAgents {
			return m, nil
		}
		m.agentsCurrentOnly = !m.agentsCurrentOnly
		switch {
		case m.agentsCurrentOnly && m.currentSession == "":
			m.notify("agents: not in a "+m.terms.BackendName+" "+m.terms.SessionNoun, sevWarning)
		case m.agentsCurrentOnly:
			m.notify("agents: "+m.currentSessionLabel(), sevInfo)
		default:
			m.notify("agents: all "+m.terms.SessionPlural, sevInfo)
		}
		m.clampCursor()
		return m, m.previewForSelection()
	case "s":
		if m.source != sessionmgr.ModeAgents {
			return m, nil
		}
		m.agentsStateFilter = nextAgentStateFilter(m.agentsStateFilter)
		if m.agentsStateFilter == "" {
			m.notify("agents: all states", sevInfo)
		} else {
			m.notify("agents: "+string(m.agentsStateFilter), sevInfo)
		}
		m.clampCursor()
		return m, m.previewForSelection()
	case "/":
		m.inputMode = modeSearch
		m.query = ""
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		m.clampCursor()
		return m, tea.Batch(textinput.Blink, m.previewForSelection())
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
	case "m":
		m.openInputModePrompt(true)
		m.notify("change input mode", sevInfo)
		return m, nil
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "h":
		m.openInstallMenu(false)
		m.notify("install menu", sevInfo)
		return m, nil
	case "a", "ctrl+a":
		return m.switchSource(sessionmgr.ModeAll)
	case "t", "ctrl+t":
		return m.switchSource(sessionmgr.ModeSessions)
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

// nextSource returns the source mode offset by delta from the current source
// (e.g. +1 next tab, -1 previous tab), wrapping around the configured tab
// order. When the current source is not part of the tab order (e.g. a
// CLI-only mode) or the order is empty, the first tab (or the current source
// when there are no tabs) is returned.
func (m Model) nextSource(delta int) sessionmgr.SourceMode {
	order := m.config.SourceOrder()
	if len(order) == 0 {
		return m.source
	}
	idx := -1
	for i, mode := range order {
		if mode == m.source {
			idx = i
			break
		}
	}
	if idx < 0 {
		return order[0]
	}
	n := len(order)
	next := (idx + delta) % n
	if next < 0 {
		next += n
	}
	return order[next]
}

func (m Model) handleTypeFirstKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.prefixArmed {
		m.prefixArmed = false
		if m.isPrefixKey(msg) {
			m.notify("prefix cancelled", sevInfo)
			return m, nil
		}
		return m.handleActionKey(msg)
	}
	if m.isPrefixKey(msg) {
		m.prefixArmed = true
		m.notify("prefix active: next key is an action", sevInfo)
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
	m.notify("press "+m.config.PrefixKey()+" before actions", sevInfo)
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
	case "enter",
		"up",
		"down",
		"pgup",
		"ctrl+u",
		"pgdown",
		"ctrl+d",
		"home",
		"end",
		"tab",
		"shift+tab":
		return true
	default:
		return false
	}
}

func (m Model) appendFilterText(text string) (tea.Model, tea.Cmd) {
	m.query += text
	m.searchInput.SetValue(m.query)
	m.clampCursor()
	m.notify("filter: "+m.query, sevInfo)
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
		m.notify("filter cleared", sevInfo)
	} else {
		m.notify("filter: "+m.query, sevInfo)
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
	m.notify("filter cleared", sevInfo)
	return m, m.previewForSelection()
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

func (m *Model) openInstallMenu(firstRun bool) {
	m.installMenu.active = true
	m.installMenu.cursor = 0
	names := integrations.Available()
	if m.installMenu.statuses == nil {
		m.installMenu.statuses = make(map[string]string, len(names))
	}
	for _, n := range names {
		if m.installMenu.statuses[n] == "" {
			m.installMenu.statuses[n] = "idle"
		}
	}
	if firstRun {
		m.installMenu.message = "first run — choose integrations to install"
	} else {
		m.installMenu.message = ""
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
	m.notify("input mode change cancelled", sevInfo)
	return m, nil
}

func (m Model) handleInstallMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	names := integrations.Available()
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		return m.closeInstallMenu()
	case "up", "k":
		m.installMenu.cursor = wrapCursorUp(m.installMenu.cursor, len(names))
		return m, nil
	case "down", "j":
		m.installMenu.cursor = wrapCursorDown(m.installMenu.cursor, len(names))
		return m, nil
	case "enter", "i":
		if len(names) == 0 {
			return m, nil
		}
		name := names[m.installMenu.cursor]
		m.installMenu.statuses[name] = "installing"
		m.installMenu.message = "installing " + name + "…"
		return m, installIntegrationCmd(name, "install")
	case "u":
		if len(names) == 0 {
			return m, nil
		}
		name := names[m.installMenu.cursor]
		m.installMenu.statuses[name] = "uninstalling"
		m.installMenu.message = "uninstalling " + name + "…"
		return m, installIntegrationCmd(name, "uninstall")
	case "a":
		cmds := make([]tea.Cmd, 0, len(names))
		for _, n := range names {
			m.installMenu.statuses[n] = "installing"
			cmds = append(cmds, installIntegrationCmd(n, "install"))
		}
		m.installMenu.message = "installing all…"
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) closeInstallMenu() (tea.Model, tea.Cmd) {
	m.installMenu.active = false
	m.installMenu.message = ""
	if !m.config.Setup.InstallMenuSeen {
		cfg := m.config
		cfg.Setup.InstallMenuSeen = true
		if err := appconfig.Save(cfg); err != nil {
			m.notify(err.Error(), sevError)
			return m, nil
		}
		m.config = cfg
	}
	m.notify("install menu closed", sevInfo)
	var refresh tea.Cmd
	m, refresh = m.beginRefresh(m.source, true)
	return m, refresh
}

func (m Model) applyTypeFirstSetup(enabled bool) (tea.Model, tea.Cmd) {
	cfg := m.config
	cfg.TypeFirst.Enabled = enabled
	if strings.TrimSpace(cfg.TypeFirst.Prefix) == "" {
		cfg.TypeFirst.Prefix = appconfig.DefaultPrefix
	}
	cfg.Setup.TypeFirstPromptSeen = true
	if err := appconfig.Save(cfg); err != nil {
		m.notify(err.Error(), sevError)
		return m, nil
	}
	m.config = cfg
	m.setup.active = false
	m.setup.manual = false
	if enabled {
		m.notify("type-first mode enabled", sevInfo)
	} else {
		m.notify("classic input mode selected", sevInfo)
	}
	return m, nil
}

func (m Model) switchSource(source sessionmgr.SourceMode) (tea.Model, tea.Cmd) {
	m.source = source
	m.cursor = 0
	if m.cacheFresh(source) {
		m = m.applyCacheEntry(source)
		m.loading = false
	} else {
		m.items = nil
		m.loading = true
		m.notify("loading "+source.DisplayNames(m.terms).List, sevInfo)
	}
	var refresh tea.Cmd
	m, refresh = m.beginRefresh(source, false)
	return m, tea.Batch(refresh, m.previewForSelection())
}

func (m Model) activateSelected() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		m.notify("nothing selected", sevInfo)
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		m.notify("attaching "+item.Name, sevInfo)
		return m, attachCmd(m.mux, item)
	case sessionmgr.KindAgent:
		if item.Session == "" || item.Window == "" || item.PaneID == "" {
			m.notify("cannot focus agent (missing pane info)", sevInfo)
			return m, nil
		}
		// Flip done→idle before focusing: the user is visiting the pane. Run
		// synchronously because focusAgentCmd suspends the TUI (tea.ExecProcess),
		// so the flip must persist first. Errors are non-fatal.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = m.mux.MarkAgentVisited(ctx, item.PaneID)
		m.notify(fmt.Sprintf("focusing %s on %s", item.DisplayName(), item.Location), sevInfo)
		return m, focusAgentCmd(m.mux, item)
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		m.notify("creating "+m.terms.SessionNoun+" from "+item.Path, sevInfo)
		return m, createSessionCmd(m.mux, item.Path)
	default:
		return m, nil
	}
}

func (m Model) deleteSelected() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		m.notify("nothing selected", sevInfo)
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		if m.killInFlight {
			return m, nil
		}
		m.notify(m.terms.KillVerb+" "+m.terms.SessionNoun+" "+item.Name, sevInfo)
		m.killInFlight = true
		return m, deleteSessionCmd(m.mux, item)
	default:
		m.notify("delete only applies to "+m.terms.SessionPlural, sevInfo)
		return m, nil
	}
}

func (m Model) startRename() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		m.notify("nothing selected", sevInfo)
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		m.inputMode = modeRename
		m.renameKind = item.Kind
		m.renameFrom = item.Name
		m.renameTarget = item.ActionTarget()
		m.renameInput.SetValue("")
		m.renameInput.Focus()
		m.notify("renaming "+item.Name, sevInfo)
		return m, textinput.Blink
	case sessionmgr.KindAgent:
		m.inputMode = modeRename
		m.renameKind = item.Kind
		m.renameFrom = item.AgentName
		m.renameSession = item.Session
		m.renameTarget = item.ActionTarget() // herdr rename targets the pane id
		m.renameInput.SetValue(item.DisplayName())
		m.renameInput.Focus()
		m.notify("renaming agent "+item.AgentName, sevInfo)
		return m, textinput.Blink
	default:
		m.notify("rename only applies to "+m.terms.SessionPlural+" and agents", sevInfo)
		return m, nil
	}
}

func (m Model) startYazi() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inPopup, err := m.checkPopup(ctx)
	if err != nil {
		m.notify(fmt.Sprintf("checking %s popup: %v", m.terms.BackendName, err), sevError)
		return m, nil
	}
	if inPopup {
		err := fmt.Errorf("cannot open yazi inside a %s popup", m.terms.BackendName)
		m.notify(err.Error(), sevError)
		return m, nil
	}
	file, err := os.CreateTemp("", "seshagy-yazi-*")
	if err != nil {
		m.notify(err.Error(), sevError)
		return m, nil
	}
	path := file.Name()
	_ = file.Close()
	m.notify("opening yazi", sevInfo)
	return m, tea.ExecProcess(sessionmgr.RunYaziCommand(path), func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return yaziDoneMsg{err: err}
		}
		data, _ := os.ReadFile(path)
		return yaziDoneMsg{path: strings.TrimSpace(string(data))}
	})
}

// agentStateFilterOrder is the full cycle order for the 's' Agents-tab state
// filter, including the leading "" (no filter / all states) so the wrap lands
// back on all states rather than skipping it.
var agentStateFilterOrder = []sessionmgr.AgentState{
	"", // all states
	sessionmgr.AgentWorking,
	sessionmgr.AgentBlocked,
	sessionmgr.AgentIdle,
	sessionmgr.AgentDone,
	sessionmgr.AgentUnknown,
}

// nextAgentStateFilter advances the 's' state filter through its cycle:
// "" (all) -> working -> blocked -> idle -> done -> unknown -> "" (all).
func nextAgentStateFilter(cur sessionmgr.AgentState) sessionmgr.AgentState {
	for i, st := range agentStateFilterOrder {
		if st == cur {
			return agentStateFilterOrder[(i+1)%len(agentStateFilterOrder)]
		}
	}
	// Unrecognized value -> reset to all states.
	return agentStateFilterOrder[0]
}
