package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type inputMode int

const (
	modeNormal inputMode = iota
	modeSearch
	modeRename
)

type Model struct {
	styles styles
	config appconfig.Config

	width  int
	height int

	source sessionmgr.SourceMode
	items  []sessionmgr.Item
	cursor int

	agentStateFilter sessionmgr.AgentState
	prefixArmed      bool

	query       string
	searchInput textinput.Model
	renameInput textinput.Model
	renameFrom  string
	inputMode   inputMode

	preview     string
	previewKey  string
	showPreview bool
	showHelp    bool
	loading     bool
	status      string
	err         error

	integrationPrompt   bool
	integrationRows     []integrations.Recommendation
	integrationSelected map[integrations.Target]bool
	integrationCursor   int
	integrationMessages []string

	setupPrompt bool
	setupManual bool
	setupCursor int
}

type refreshMsg struct {
	items []sessionmgr.Item
	err   error
}

type previewMsg struct {
	key     string
	preview string
	err     error
}

type actionDoneMsg struct {
	status string
	err    error
}

type createDoneMsg struct {
	name    string
	created bool
	err     error
}

type attachDoneMsg struct{ err error }
type yaziDoneMsg struct {
	path string
	err  error
}

type tickMsg time.Time

type integrationsMsg struct {
	recs []integrations.Recommendation
	err  error
}

type integrationsInstalledMsg struct {
	messages []string
	err      error
}

type setupMsg struct {
	prompt bool
	err    error
}

var checkTmuxPopup = sessionmgr.InTmuxPopup

func New() Model {
	cfg, cfgErr := appconfig.Load()
	search := textinput.New()
	search.Placeholder = "filter sessions, agents, directories"
	search.Prompt = "/ "
	search.CharLimit = 256
	rename := textinput.New()
	rename.Placeholder = "new session name"
	rename.Prompt = "rename > "
	rename.CharLimit = 128
	m := Model{
		styles:              defaultStyles(),
		config:              cfg,
		source:              cfg.DefaultSource(),
		showPreview:         true,
		showHelp:            true,
		searchInput:         search,
		renameInput:         rename,
		integrationSelected: map[integrations.Target]bool{},
		setupCursor:         1,
	}
	if cfgErr != nil {
		m.err = cfgErr
		m.status = cfgErr.Error()
	}
	return m
}

func Run() error {
	m := New()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.source, m.config.LoadOptions()), startupSetupCmd(m.config), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.searchInput.Width = max(20, msg.Width/3)
		m.renameInput.Width = max(20, msg.Width/3)
		return m, m.previewForSelection()
	case tickMsg:
		return m, tea.Batch(refreshCmd(m.source, m.config.LoadOptions()), tickCmd())
	case integrationsMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.integrationRows = msg.recs
		m.integrationSelected = map[integrations.Target]bool{}
		for _, rec := range msg.recs {
			m.integrationSelected[rec.Target] = rec.AgentAvailable && rec.Installable && rec.State != integrations.StatusCurrent
		}
		if m.integrationCursor >= len(m.integrationRows) {
			m.integrationCursor = max(0, len(m.integrationRows)-1)
		}
		if len(msg.recs) > 0 {
			m.integrationPrompt = true
			m.status = "install hook integrations for detected agents"
		} else if m.integrationPrompt {
			m.integrationPrompt = false
			m.status = "no missing hook integrations for detected agents"
		}
		return m, nil
	case integrationsInstalledMsg:
		m.integrationMessages = msg.messages
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
		m.integrationPrompt = false
		return m, tea.Batch(integrationsCmd(), refreshCmd(m.source, m.config.LoadOptions()))
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
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		m.err = nil
		m.items = msg.items
		m.clampCursor()
		m.status = fmt.Sprintf("loaded %d item%s", len(msg.items), plural(len(msg.items)))
		return m, m.previewForSelection()
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
		return m, tea.Batch(refreshCmd(m.source, m.config.LoadOptions()), m.previewForSelection())
	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		m.err = nil
		m.status = msg.status
		return m, tea.Batch(refreshCmd(m.source, m.config.LoadOptions()), m.previewForSelection())
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
		if m.setupPrompt {
			return m.handleSetupKey(msg)
		}
		if m.integrationPrompt {
			return m.handleIntegrationKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

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
		m.integrationPrompt = true
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
		m.integrationPrompt = false
		m.status = "hook installation skipped"
		return m, nil
	case "up", "k":
		if m.integrationCursor > 0 {
			m.integrationCursor--
		}
		return m, nil
	case "down", "j":
		if m.integrationCursor < len(m.integrationRows)-1 {
			m.integrationCursor++
		}
		return m, nil
	case " ":
		if len(m.integrationRows) == 0 {
			return m, nil
		}
		rec := m.integrationRows[m.integrationCursor]
		if rec.AgentAvailable && rec.Installable && rec.State != integrations.StatusCurrent {
			m.integrationSelected[rec.Target] = !m.integrationSelected[rec.Target]
		}
		return m, nil
	case "enter":
		var targets []integrations.Target
		for _, rec := range m.integrationRows {
			if m.integrationSelected[rec.Target] {
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
	m.setupPrompt = true
	m.setupManual = manual
	if m.config.TypeFirst.Enabled {
		m.setupCursor = 0
	} else {
		m.setupCursor = 1
	}
}

func (m Model) handleSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "down", "j", "k", "tab":
		if m.setupCursor == 0 {
			m.setupCursor = 1
		} else {
			m.setupCursor = 0
		}
		return m, nil
	case "y", "Y":
		return m.applyTypeFirstSetup(true)
	case "n", "N":
		return m.applyTypeFirstSetup(false)
	case "esc":
		if m.setupManual {
			return m.cancelInputModePrompt()
		}
		return m.applyTypeFirstSetup(false)
	case "enter":
		return m.applyTypeFirstSetup(m.setupCursor == 0)
	}
	return m, nil
}

func (m Model) cancelInputModePrompt() (tea.Model, tea.Cmd) {
	m.setupPrompt = false
	m.setupManual = false
	m.status = "input mode change cancelled"
	return m, nil
}

func (m Model) applyTypeFirstSetup(enabled bool) (tea.Model, tea.Cmd) {
	manual := m.setupManual
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
	m.setupPrompt = false
	m.setupManual = false
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

func (m *Model) clampCursor() {
	vis := m.visibleItems()
	if len(vis) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(vis) {
		m.cursor = len(vis) - 1
	}
}

func (m Model) visibleItems() []sessionmgr.Item {
	if m.query == "" && !m.agentStateFilteringActive() {
		return m.items
	}
	query := strings.ToLower(m.query)
	out := make([]sessionmgr.Item, 0, len(m.items))
	for _, item := range m.items {
		if m.agentStateFilteringActive() && (item.Kind != sessionmgr.KindAgent || item.AgentState != m.agentStateFilter) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{string(item.Kind), item.Name, item.Path, item.AgentName, string(item.AgentState), item.Location, item.AgentMessage, item.AgentSource}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func (m Model) agentStateFilteringActive() bool {
	return isAgentSource(m.source) && m.agentStateFilter != ""
}

func isAgentSource(mode sessionmgr.SourceMode) bool {
	return mode == sessionmgr.ModeAgents || mode == sessionmgr.ModeCurrentAgents
}

func nextAgentStateFilter(current sessionmgr.AgentState) sessionmgr.AgentState {
	switch current {
	case "":
		return sessionmgr.AgentWorking
	case sessionmgr.AgentWorking:
		return sessionmgr.AgentBlocked
	case sessionmgr.AgentBlocked:
		return sessionmgr.AgentAborted
	case sessionmgr.AgentAborted:
		return sessionmgr.AgentDone
	case sessionmgr.AgentDone:
		return sessionmgr.AgentIdle
	case sessionmgr.AgentIdle:
		return sessionmgr.AgentUnknown
	default:
		return ""
	}
}

func agentStateFilterLabel(state sessionmgr.AgentState) string {
	if state == "" {
		return "all"
	}
	return string(state)
}

func (m Model) selectedItem() (sessionmgr.Item, bool) {
	vis := m.visibleItems()
	if len(vis) == 0 || m.cursor < 0 || m.cursor >= len(vis) {
		return sessionmgr.Item{}, false
	}
	return vis[m.cursor], true
}

func (m Model) selectedKey() string {
	item, ok := m.selectedItem()
	if !ok {
		return ""
	}
	return item.Key()
}

func (m Model) previewForSelection() tea.Cmd {
	if !m.showPreview {
		return nil
	}
	item, ok := m.selectedItem()
	if !ok {
		return nil
	}
	return previewCmd(item)
}

func refreshCmd(source sessionmgr.SourceMode, opts sessionmgr.LoadOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		items, err := sessionmgr.LoadWithOptions(ctx, source, opts)
		return refreshMsg{items: items, err: err}
	}
}

func integrationsCmd() tea.Cmd {
	return func() tea.Msg {
		return integrationsMsg{recs: integrations.RecommendedForPrompt()}
	}
}

func startupSetupCmd(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg {
		if cfg.Setup.TypeFirstPromptSeen {
			return setupMsg{}
		}
		return setupMsg{prompt: true}
	}
}

func startupIntegrationsCmd() tea.Cmd {
	return func() tea.Msg {
		shouldPrompt, err := claimStartupIntegrationPrompt()
		if err != nil {
			return integrationsMsg{err: fmt.Errorf("record first-launch hook prompt: %w", err)}
		}
		if !shouldPrompt {
			return integrationsMsg{}
		}
		return integrationsMsg{recs: integrations.RecommendedForPrompt()}
	}
}

func installIntegrationsCmd(targets []integrations.Target) tea.Cmd {
	return func() tea.Msg {
		var messages []string
		for _, target := range targets {
			installed, err := integrations.Install(target)
			if err != nil {
				return integrationsInstalledMsg{messages: messages, err: err}
			}
			messages = append(messages, installed...)
		}
		return integrationsInstalledMsg{messages: messages}
	}
}

func previewCmd(item sessionmgr.Item) tea.Cmd {
	key := item.Key()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var (
			preview string
			err     error
		)
		switch item.Kind {
		case sessionmgr.KindSession:
			preview, err = sessionmgr.CaptureSession(ctx, item.Name, 160)
		case sessionmgr.KindAgent:
			preview, err = sessionmgr.CaptureAgentPane(ctx, item.PaneID, 160)
		case sessionmgr.KindZoxide, sessionmgr.KindFD:
			preview, err = sessionmgr.ListDirectoryPreview(ctx, item.Path, 160)
		}
		if strings.TrimSpace(preview) == "" && err == nil {
			preview = "no preview available"
		}
		return previewMsg{key: key, preview: preview, err: err}
	}
}

func attachCmd(name string) tea.Cmd {
	return tea.ExecProcess(sessionmgr.AttachOrSwitchCommand(name), func(err error) tea.Msg {
		return attachDoneMsg{err: err}
	})
}

func focusAgentCmd(pane string) tea.Cmd {
	return tea.ExecProcess(sessionmgr.FocusAgentCommand(pane), func(err error) tea.Msg {
		return attachDoneMsg{err: err}
	})
}

func createSessionCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name, created, err := sessionmgr.CreateSessionFromDir(ctx, dir)
		return createDoneMsg{name: name, created: created, err: err}
	}
}

func deleteSessionCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.KillSession(ctx, name)
		return actionDoneMsg{status: "killed session " + name, err: err}
	}
}

func deleteAgentCmd(pane string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.KillAgentPane(ctx, pane)
		return actionDoneMsg{status: "killed pane " + pane, err: err}
	}
}

func renameCmd(oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.RenameSession(ctx, oldName, newName)
		return actionDoneMsg{status: fmt.Sprintf("renamed %s to %s", oldName, newName), err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func modeName(mode sessionmgr.SourceMode) string {
	switch mode {
	case sessionmgr.ModeSessions:
		return "sessions"
	case sessionmgr.ModeAgents:
		return "agents"
	case sessionmgr.ModeCurrentAgents:
		return "current agents"
	case sessionmgr.ModeZoxide:
		return "zoxide"
	case sessionmgr.ModeFD:
		return "fd"
	default:
		return "all"
	}
}

func sortedCounts(items []sessionmgr.Item) map[sessionmgr.Kind]int {
	counts := map[sessionmgr.Kind]int{}
	for _, item := range items {
		counts[item.Kind]++
	}
	return counts
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampText(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > w {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func modeRank(k sessionmgr.Kind) int {
	switch k {
	case sessionmgr.KindSession:
		return 0
	case sessionmgr.KindAgent:
		return 1
	case sessionmgr.KindZoxide:
		return 2
	case sessionmgr.KindFD:
		return 3
	default:
		return 9
	}
}

func SortItems(items []sessionmgr.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if modeRank(items[i].Kind) != modeRank(items[j].Kind) {
			return modeRank(items[i].Kind) < modeRank(items[j].Kind)
		}
		return strings.ToLower(items[i].DisplayName()) < strings.ToLower(items[j].DisplayName())
	})
}
