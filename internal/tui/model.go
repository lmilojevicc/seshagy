package tui

import (
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

	cache           map[sessionmgr.SourceMode]modeCache
	refreshGen      map[sessionmgr.SourceMode]uint64
	inflightRefresh map[sessionmgr.SourceMode]uint64

	integration integrationPrompt
	setup       setupPrompt
}

// integrationPrompt holds the state of the hook-integration selection prompt.
type integrationPrompt struct {
	active        bool
	startupPrompt bool
	rows          []integrations.Recommendation
	selected      map[integrations.Target]bool
	cursor        int
	messages      []string
}

// setupPrompt holds the state of the first-launch / manual input-mode prompt.
type setupPrompt struct {
	active bool
	manual bool
	cursor int
}

type refreshMsg struct {
	source sessionmgr.SourceMode
	gen    uint64
	items  []sessionmgr.Item
	err    error
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

type (
	attachDoneMsg struct{ err error }
	yaziDoneMsg   struct {
		path string
		err  error
	}
)

type tickMsg time.Time

type integrationsMsg struct {
	recs    []integrations.Recommendation
	startup bool
	err     error
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
		styles:          stylesFromConfig(cfg),
		config:          cfg,
		source:          cfg.DefaultSource(),
		showPreview:     true,
		showHelp:        true,
		searchInput:     search,
		renameInput:     rename,
		cache:           make(map[sessionmgr.SourceMode]modeCache),
		refreshGen:      make(map[sessionmgr.SourceMode]uint64),
		inflightRefresh: make(map[sessionmgr.SourceMode]uint64),
		integration:     integrationPrompt{selected: map[integrations.Target]bool{}},
		setup:           setupPrompt{cursor: 1},
		loading:         true,
	}
	m.refreshGen[m.source] = 1
	m.inflightRefresh[m.source] = 1
	if cfgErr != nil {
		m.err = cfgErr
		m.status = cfgErr.Error()
	}
	return m
}

func Run() error {
	m := New()
	sessionmgr.StartManifestAutoUpdate(
		m.config.Agents.ManifestCatalogURL,
		m.config.Agents.ManifestAutoUpdate,
	)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		refreshCmd(m.source, m.inflightRefresh[m.source], m.config.LoadOptions()),
		startupSetupCmd(m.config),
		tickCmd(),
	)
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

func wrapCursorUp(cursor, count int) int {
	if count < 2 {
		if count == 0 {
			return 0
		}
		return min(cursor, count-1)
	}
	if cursor <= 0 {
		return count - 1
	}
	return cursor - 1
}

func wrapCursorDown(cursor, count int) int {
	if count < 2 {
		if count == 0 {
			return 0
		}
		return min(cursor, count-1)
	}
	if cursor >= count-1 {
		return 0
	}
	return cursor + 1
}

func (m Model) visibleItems() []sessionmgr.Item {
	if m.query == "" && !m.agentStateFilteringActive() {
		return m.items
	}
	query := strings.ToLower(m.query)
	out := make([]sessionmgr.Item, 0, len(m.items))
	for _, item := range m.items {
		if m.agentStateFilteringActive() &&
			(item.Kind != sessionmgr.KindAgent || item.AgentState != m.agentStateFilter) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(
				strings.Join(
					[]string{
						string(item.Kind),
						item.Name,
						item.Path,
						item.AgentName,
						string(item.AgentState),
						item.Location,
						item.AgentMessage,
						item.AgentSource,
						item.AgentSessionID,
					},
					" ",
				),
			)
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
