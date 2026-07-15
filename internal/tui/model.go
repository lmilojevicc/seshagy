package tui

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
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

	mux   sessionmgr.Multiplexer
	terms sessionmgr.Terms

	width  int
	height int

	source sessionmgr.SourceMode
	items  []sessionmgr.Item
	cursor int

	prefixArmed bool

	query         string
	searchInput   textinput.Model
	renameInput   textinput.Model
	renameFrom    string
	renameTarget  string
	renameKind    sessionmgr.Kind
	renameSession string
	inputMode     inputMode

	preview     string
	previewKey  string
	showPreview bool
	showHelp    bool
	loading     bool
	status      string
	err         error

	agentsCurrentOnly bool
	currentSession    string
	agentsStateFilter sessionmgr.AgentState

	cache           map[sessionmgr.SourceMode]modeCache
	refreshGen      map[sessionmgr.SourceMode]uint64
	inflightRefresh map[sessionmgr.SourceMode]uint64

	checkPopup func(context.Context) (bool, error)

	// ephemeral enables the focus-loss dismissal poll (--ephemeral). The
	// herdr pane/workspace ids are resolved once and cached so discovery
	// (via the focused pane) only runs before the first focus change.
	ephemeral        bool
	herdrPaneID      string
	herdrWorkspaceID string

	// killInFlight suppresses the ephemeral focus-loss dismissal while a
	// session/workspace kill (x) is in flight, so the close's refocus doesn't
	// quit seshagy before the focus-restore inside KillSession can land.
	killInFlight bool

	setup          setupPrompt
	installMenu    installMenuState
	pendingInstall bool
}

// installMenuState holds the state of the agent-integration install menu
// overlay (first-run popup + manual `h` reopen).
type installMenuState struct {
	active   bool
	cursor   int
	statuses map[string]string
	message  string
}

// setupPrompt holds the state of the first-launch / manual input-mode prompt.
type setupPrompt struct {
	active bool
	manual bool
	cursor int
}

type refreshMsg struct {
	source         sessionmgr.SourceMode
	gen            uint64
	items          []sessionmgr.Item
	currentSession string
	warning        string
	err            error
}

type previewMsg struct {
	key     string
	preview string
	err     error
}

type actionKind string

const (
	actionKill        actionKind = "kill"
	actionRename      actionKind = "rename"
	actionAgentRename actionKind = "agentRename"
)

type actionDoneMsg struct {
	kind   actionKind
	status string
	err    error
}

type createDoneMsg struct {
	item    sessionmgr.Item
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

type setupMsg struct {
	prompt bool
	err    error
}

// Option configures a Model at construction time.
type Option func(*Model)

// WithEphemeral enables the focus-loss dismissal poll so the dashboard exits
// when its hosting pane/window/session loses focus (--ephemeral).
func WithEphemeral(b bool) Option {
	return func(m *Model) { m.ephemeral = b }
}

func New(opts ...Option) Model {
	cfg, cfgErr := appconfig.Load()
	mux := sessionmgr.Detect()
	search := textinput.New()
	search.Placeholder = "filter sessions, directories"
	search.Prompt = "/ "
	search.CharLimit = 256
	rename := textinput.New()
	rename.Placeholder = "new name"
	rename.Prompt = "rename > "
	rename.CharLimit = 128
	showPreview := true
	if cfg.TUI.Preview != nil {
		showPreview = *cfg.TUI.Preview
	}
	m := Model{
		styles:          stylesFromConfig(cfg),
		config:          cfg,
		mux:             mux,
		terms:           mux.Terms(),
		source:          cfg.DefaultSource(),
		showPreview:     showPreview,
		showHelp:        true,
		searchInput:     search,
		renameInput:     rename,
		cache:           make(map[sessionmgr.SourceMode]modeCache),
		refreshGen:      make(map[sessionmgr.SourceMode]uint64),
		inflightRefresh: make(map[sessionmgr.SourceMode]uint64),
		checkPopup:      mux.InMultiplexerPopup,
		setup:           setupPrompt{cursor: 1},
		loading:         true,
	}
	m.refreshGen[m.source] = 1
	m.inflightRefresh[m.source] = 1
	if cfgErr != nil {
		m.err = cfgErr
		m.status = cfgErr.Error()
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

func Run(opts ...Option) error {
	m := New(opts...)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		refreshCmd(m.mux, m.source, m.inflightRefresh[m.source], m.config.LoadOptions()),
		startupSetupCmd(m.config),
		startupInstallMenuCmd(m.config),
		refreshCatalogsCmd(m.config),
		tickCmd(),
	}
	// Keep the ModeAll cache warm so the overview hero band shows correct
	// counts even when another source tab is active on launch.
	if m.source != sessionmgr.ModeAll {
		if _, cmd := m.beginRefresh(sessionmgr.ModeAll, false); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.ephemeral {
		cmds = append(cmds, ephemeralTickCmd())
	}
	return tea.Batch(cmds...)
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
	// The current-session scope filter ('o' in the Agents tab) applies
	// client-side here and composes with the search query. ModeAgents loads
	// every agent pane; this just hides those outside the current tmux
	// session.
	scope := m.source == sessionmgr.ModeAgents && m.agentsCurrentOnly &&
		m.currentSession != ""
	stateFilter := m.source == sessionmgr.ModeAgents && m.agentsStateFilter != ""
	if m.query == "" && !scope && !stateFilter {
		return m.items
	}
	query := strings.ToLower(m.query)
	out := make([]sessionmgr.Item, 0, len(m.items))
	for _, item := range m.items {
		if scope && item.Session != m.currentSession {
			continue
		}
		if stateFilter && item.AgentState != m.agentsStateFilter {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(
				strings.Join(
					[]string{
						string(item.Kind),
						item.Name,
						item.Path,
						item.Location,
						item.AgentName,
						item.AgentDisplayName,
						string(item.AgentState),
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

// currentSessionLabel returns a human-facing label for the current session id.
// Under herdr the id is opaque (e.g. "wB"); agent items in the same session
// carry the resolved workspace label in Location, so we look it up from the
// loaded items and fall back to the raw id when nothing better is available.
// Under tmux the id IS the name, so this returns it unchanged.
func (m Model) currentSessionLabel() string {
	if m.currentSession == "" {
		return ""
	}
	for _, item := range m.items {
		if item.Kind == sessionmgr.KindAgent && item.Session == m.currentSession &&
			item.Location != "" {
			return item.Location
		}
	}
	return m.currentSession
}

func (m Model) previewForSelection() tea.Cmd {
	if !m.showPreview {
		return nil
	}
	item, ok := m.selectedItem()
	if !ok {
		return nil
	}
	return previewCmd(m.mux, item)
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

// clampVal clamps v to the inclusive [lo, hi] range. If hi <= lo, only the
// lower bound is enforced (v is floored at lo but not capped).
func clampVal(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if hi > lo && v > hi {
		return hi
	}
	return v
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
