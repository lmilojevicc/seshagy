package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// handleKey routes a key press in the picker. The picker is always type-first:
// printable runes extend the query, and every action is a ctrl/arrow/tab/enter
// chord.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.renameActive {
		return m.handleRenameKey(msg)
	}
	if m.killArmed {
		return m.handleKillConfirmKey(msg)
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch msg.String() {
	case "esc":
		if m.query != "" || m.scope != scopeAll {
			m.query = ""
			m.scope = scopeAll
			return m.afterSelectionChanged()
		}
		return m, tea.Quit
	case "up", "ctrl+p", "ctrl+k":
		return m.moveCursor(wrapCursorUp(m.cursor, len(m.rows())))
	case "down", "ctrl+n", "ctrl+j":
		return m.moveCursor(wrapCursorDown(m.cursor, len(m.rows())))
	case "pgup":
		return m.moveCursor(m.cursor - m.pageSize())
	case "pgdown":
		return m.moveCursor(m.cursor + m.pageSize())
	case "home":
		return m.moveCursor(0)
	case "end":
		return m.moveCursor(len(m.rows()) - 1)
	case "tab":
		m.scope = m.scope.next(1)
		return m.afterSelectionChanged()
	case "shift+tab":
		m.scope = m.scope.next(-1)
		return m.afterSelectionChanged()
	case "enter":
		return m.activateSelected()
	case "ctrl+r":
		return m.startRename()
	case "ctrl+x", "delete":
		return m.armKill()
	case "ctrl+o":
		m.showPreview = !m.showPreview
		if !m.previewVisible() {
			return m, nil
		}
		// Toggling the preview keeps the selection where it is.
		var preview tea.Cmd
		m, preview = m.requestPreview()
		return m, preview
	case "ctrl+y":
		return m.startYazi()
	case "ctrl+l":
		m = m.invalidateAllCaches()
		var reload tea.Cmd
		m, reload = m.reloadCmd()
		return m, reload
	case "ctrl+a":
		m.openInstallMenu(false)
		return m, nil
	case "ctrl+u":
		if m.query == "" {
			return m, nil
		}
		m.query = ""
		return m.afterSelectionChanged()
	case "ctrl+w":
		return m.deleteQueryWord()
	case "backspace":
		if runes := []rune(m.query); len(runes) > 0 {
			m.query = string(runes[:len(runes)-1])
			return m.afterSelectionChanged()
		}
		if m.scope != scopeAll {
			m.scope = scopeAll
			return m.afterSelectionChanged()
		}
		return m, nil
	}
	if !isPrintableKey(msg) {
		return m, nil
	}
	// A sigil typed on an empty query selects that scope instead of being
	// inserted; the same keystroke switches scope while one is active.
	if m.query == "" && len(msg.Runes) == 1 {
		if sc, ok := scopeForSigil(msg.Runes[0]); ok {
			m.scope = sc
			return m.afterSelectionChanged()
		}
	}
	m.query += string(msg.Runes)
	return m.afterSelectionChanged()
}

// moveCursor moves the selection and schedules the preview capture for it.
func (m Model) moveCursor(index int) (tea.Model, tea.Cmd) {
	m = m.setCursor(index)
	var cmd tea.Cmd
	m, cmd = m.requestPreview()
	return m, cmd
}

// afterSelectionChanged lands the cursor on the first row after a scope or
// query change and schedules a fresh preview capture. Narrowing the list is not
// a cursor move: only an explicit move makes refreshes follow the selected item.
func (m Model) afterSelectionChanged() (tea.Model, tea.Cmd) {
	m.cursorMoved = false
	m.syncCursor()
	var cmd tea.Cmd
	m, cmd = m.requestPreview()
	return m, cmd
}

func (m Model) deleteQueryWord() (tea.Model, tea.Cmd) {
	trimmed := strings.TrimRight(m.query, " ")
	cut := strings.LastIndexAny(trimmed, " \t")
	next := ""
	if cut >= 0 {
		next = trimmed[:cut+1]
	}
	if next == m.query {
		return m, nil
	}
	m.query = next
	return m.afterSelectionChanged()
}

// isPrintableKey reports whether the key carries text to insert. Space is a
// printable rune in bubbletea (KeySpace with Runes == [' ']).
func isPrintableKey(msg tea.KeyMsg) bool {
	return len(msg.Runes) > 0 && !msg.Alt
}

// pageSize is the items a page jump covers: the items the results window
// currently draws, since an item is one or two lines tall.
func (m Model) pageSize() int {
	rows := m.rows()
	if len(rows) == 0 {
		return 1
	}
	win := newItemFlow(rows).window(m.cursor, m.bodyHeight())
	return max(1, win.end-win.start)
}

func (m Model) cancelRename() Model {
	m.renameActive = false
	m.renameText = ""
	m.renameFrom = ""
	m.renameTarget = ""
	m.renameSession = ""
	m.renameAgentType = ""
	m.renameKind = ""
	return m
}

func (m Model) startRename() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		m.renameActive = true
		m.renameKind = item.Kind
		m.renameFrom = item.Name
		m.renameTarget = item.ActionTarget()
		m.renameText = item.Name
		return m, nil
	case sessionmgr.KindAgent:
		m.renameActive = true
		m.renameKind = item.Kind
		m.renameFrom = item.DisplayName()
		m.renameAgentType = item.AgentName
		m.renameSession = item.Session
		m.renameTarget = item.ActionTarget() // herdr rename targets the pane id
		m.renameText = item.DisplayName()
		return m, nil
	default:
		m.notify("rename only applies to "+m.terms.SessionPlural+" and agents", sevWarning)
		return m, nil
	}
}

func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.cancelRename(), nil
	case "enter":
		return m.applyRename()
	case "backspace":
		runes := []rune(m.renameText)
		if len(runes) > 0 {
			m.renameText = string(runes[:len(runes)-1])
		}
		return m, nil
	case "ctrl+u":
		m.renameText = ""
		return m, nil
	}
	if isPrintableKey(msg) {
		m.renameText += string(msg.Runes)
	}
	return m, nil
}

func (m Model) applyRename() (tea.Model, tea.Cmd) {
	newName := strings.TrimSpace(m.renameText)
	kind := m.renameKind
	oldName := m.renameFrom
	target := m.renameTarget
	session := m.renameSession
	agentType := m.renameAgentType
	m = m.cancelRename()
	if newName == oldName {
		return m, nil
	}
	switch kind {
	case sessionmgr.KindSession:
		if newName == "" || oldName == "" {
			return m, nil
		}
		return m, renameCmd(m.mux, target, oldName, newName)
	case sessionmgr.KindAgent:
		// An empty alias clears the agent's display name.
		return m, renameAgentCmd(m.mux, target, agentType, session, newName)
	default:
		return m, nil
	}
}

// armKill asks for confirmation before killing the selected session or agent
// pane. Unsupported kinds only warn.
func (m Model) armKill() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
	case sessionmgr.KindAgent:
		if item.PaneID == "" {
			m.notify("cannot kill agent (missing pane ID)", sevWarning)
			return m, nil
		}
	default:
		m.notify("kill only applies to "+m.terms.SessionPlural+" and agents", sevWarning)
		return m, nil
	}
	if m.killInFlight {
		return m, nil
	}
	m.killArmed = true
	m.killPending = item
	return m, nil
}

func (m Model) handleKillConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	item := m.killPending
	m.killArmed = false
	m.killPending = sessionmgr.Item{}
	switch msg.String() {
	case "enter", "y":
		if m.killInFlight {
			return m, nil
		}
		m.killInFlight = true
		if item.Kind == sessionmgr.KindAgent {
			return m, deleteAgentCmd(m.mux, item)
		}
		return m, deleteSessionCmd(m.mux, item)
	}
	return m, nil
}

func (m Model) activateSelected() (tea.Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
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
		return m, createSessionCmd(m.mux, item.Path)
	case sessionmgr.KindHost:
		return m.activateHost(item)
	default:
		return m, nil
	}
}

// activateHost opens a remote host. Under herdr the running `seshagy run`
// launcher switches the attached client to the host's server; under tmux there
// is no launcher, so the host is reached the way any ssh host would be.
func (m Model) activateHost(item sessionmgr.Item) (tea.Model, tea.Cmd) {
	host, ok := m.hostByName(item.Name)
	if !ok {
		m.notify("unknown host "+item.Name, sevError)
		return m, nil
	}
	switch m.mux.Kind() {
	case sessionmgr.BackendHerdr:
		return m, switchHostCmd(host)
	case sessionmgr.BackendTmux:
		return m, openSSHWindowCmd(host)
	default:
		m.notify("cannot open remote hosts without tmux or herdr", sevError)
		return m, nil
	}
}

// reportHostFailure toasts a host that could not be opened, translating the
// missing-launcher case into the fix the user needs.
func (m Model) reportHostFailure(err error) Model {
	if errors.Is(err, remote.ErrNoLauncher) {
		m.notify("start herdr with `seshagy run` to switch servers", sevError)
		return m
	}
	m.notify(err.Error(), sevError)
	return m
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
		m.notify("cannot open yazi inside a "+m.terms.BackendName+" popup", sevError)
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
	m = m.invalidateAllCaches()
	var reload tea.Cmd
	m, reload = m.reloadCmd()
	return m, reload
}
