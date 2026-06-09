package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	width  int
	height int

	source sessionmgr.SourceMode
	items  []sessionmgr.Item
	cursor int

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

func New() Model {
	search := textinput.New()
	search.Placeholder = "filter sessions, agents, directories"
	search.Prompt = "/ "
	search.CharLimit = 256
	rename := textinput.New()
	rename.Placeholder = "new session name"
	rename.Prompt = "rename > "
	rename.CharLimit = 128
	return Model{
		styles:      defaultStyles(),
		source:      sessionmgr.ModeAll,
		showPreview: true,
		showHelp:    true,
		searchInput: search,
		renameInput: rename,
	}
}

func Run() error {
	m := New()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.source), tickCmd())
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
		return m, tea.Batch(refreshCmd(m.source), tickCmd())
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
		return m, tea.Batch(refreshCmd(m.source), m.previewForSelection())
	case actionDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		m.err = nil
		m.status = msg.status
		return m, tea.Batch(refreshCmd(m.source), m.previewForSelection())
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
		return m, refreshCmd(m.source)
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
	case "?", "h", "alt+h":
		m.showHelp = !m.showHelp
		return m, nil
	case "a", "1", "ctrl+a":
		return m.switchSource(sessionmgr.ModeAll)
	case "t", "2", "ctrl+t":
		return m.switchSource(sessionmgr.ModeSessions)
	case "g", "3", "ctrl+g":
		return m.switchSource(sessionmgr.ModeAgents)
	case "o", "ctrl+o":
		return m.switchSource(sessionmgr.ModeCurrentAgents)
	case "z", "4", "ctrl+z":
		return m.switchSource(sessionmgr.ModeZoxide)
	case "f", "5", "ctrl+f":
		return m.switchSource(sessionmgr.ModeFD)
	case "y", "ctrl+y":
		return m.startYazi()
	}
	return m, nil
}

func (m Model) switchSource(source sessionmgr.SourceMode) (tea.Model, tea.Cmd) {
	m.source = source
	m.cursor = 0
	m.loading = true
	m.status = "loading " + modeName(source)
	return m, refreshCmd(source)
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
	if m.query == "" {
		return m.items
	}
	query := strings.ToLower(m.query)
	out := make([]sessionmgr.Item, 0, len(m.items))
	for _, item := range m.items {
		haystack := strings.ToLower(strings.Join([]string{string(item.Kind), item.Name, item.Path, item.AgentName, string(item.AgentState), item.Location, item.AgentMessage, item.AgentSource}, " "))
		if strings.Contains(haystack, query) {
			out = append(out, item)
		}
	}
	return out
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

func refreshCmd(source sessionmgr.SourceMode) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		items, err := sessionmgr.Load(ctx, source)
		return refreshMsg{items: items, err: err}
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

func cwdLabel() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(wd)
}
