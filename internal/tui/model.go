package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type notifSev int

const (
	sevInfo notifSev = iota
	sevWarning
	sevError
)

type notification struct {
	text string
	sev  notifSev
	at   time.Time
}

const spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

// tuiSources are the independent sources the picker loads and merges, each with
// its own cache entry, TTL, and generation. ModeAll stays for the CLI's
// --get-* output; the TUI never uses it.
var tuiSources = []sessionmgr.SourceMode{
	sessionmgr.ModeAgents,
	sessionmgr.ModeSessions,
	sessionmgr.ModeZoxide,
	sessionmgr.ModeFD,
}

// Model is the type-first picker. It keeps one merged item list and narrows it
// with the inline scope filter and the query typed into the prompt line.
type Model struct {
	styles styles
	config appconfig.Config
	// icons is the configured icon set, resolved once per model instead of once
	// per rendered row.
	icons sessionmgr.IconSet

	mux    sessionmgr.Multiplexer
	terms  sessionmgr.Terms
	logger *slog.Logger

	width  int
	height int

	// layout is the configured [tui] layout: horizontal, vertical, or dropdown.
	layout string

	// keys is the effective key table: the default bindings with the configured
	// [tui.keys] overrides applied. The dispatch, the footer and the help overlay
	// all read it.
	keys KeyTable

	scope scope
	// stateFilter narrows the agents scope to one agent state. It is the second
	// filter axis ctrl+s cycles, and a switch to any other scope clears it.
	stateFilter stateFilter
	query       string

	// hosts are the remote machines offered by the `# hosts` scope. They come
	// from config.toml and ~/.ssh/config, so they are loaded once at startup and
	// on an explicit reload instead of by the source poll.
	hosts []remote.Host

	items     []sessionmgr.Item
	cursor    int
	cursorKey string
	// cursorMoved records an explicit up/down/page/home/end since the last scope
	// or query change. Only then does a refresh follow the selected key; until
	// then the top row stays selected, so sources that stream in after startup
	// cannot drag the cursor down the list.
	cursorMoved bool
	// listTop is the flow line the results window opened on in the last frame.
	// The window keeps it and scrolls only when the cursor's item leaves it.
	listTop int

	loadedOnce bool

	showPreview       bool
	preview           string
	previewError      string
	previewKey        string // item key whose payload is loaded
	previewPendingKey string // item key of an in-flight capture

	renameActive    bool
	renameText      string
	renameFrom      string
	renameTarget    string
	renameKind      sessionmgr.Kind
	renameSession   string
	renameAgentType string

	killArmed   bool
	killPending sessionmgr.Item

	spinnerFrame  int
	spinnerActive bool

	notifications []notification

	// agentStarted caches when each agent pane's process started, keyed by pane
	// id. The pid it was resolved from is cached with it, so a pane hosting a
	// new agent process is resolved again. Ages render from these values.
	agentStarted map[string]agentStart
	// agentStartPending guards the in-flight start-time resolution so the 1s
	// tick cannot queue a second ps call while one is running.
	agentStartPending bool

	cache           map[sessionmgr.SourceMode]modeCache
	refreshGen      map[sessionmgr.SourceMode]uint64
	inflightRefresh map[sessionmgr.SourceMode]uint64
	// reportedWarnings remembers the last non-fatal message shown per source so
	// a repeated source failure is not toasted on every refresh.
	reportedWarnings map[sessionmgr.SourceMode]string

	checkPopup func(context.Context) (bool, error)

	// ephemeral enables the focus-loss dismissal poll (--ephemeral). The
	// herdr pane/workspace ids are resolved once and cached so discovery
	// (via the focused pane) only runs before the first focus change.
	ephemeral        bool
	herdrPaneID      string
	herdrWorkspaceID string

	// killInFlight suppresses the ephemeral focus-loss dismissal while a
	// session/workspace or agent-pane kill is in flight, so the close's
	// refocus doesn't quit seshagy before the action completes.
	killInFlight bool

	// helpOpen is the F1 help overlay. It keeps the picker's own state — query,
	// filters, cursor, scroll — untouched behind it, and swallows every key but
	// its scroll and close chords.
	helpOpen bool
	// helpTop is the help row the overlay's window opens on.
	helpTop int

	installMenu installMenuState
}

// installMenuState holds the state of the agent-integration install menu
// overlay (first-run popup + manual ctrl+g reopen).
type installMenuState struct {
	active   bool
	cursor   int
	statuses map[string]string
	message  string
}

type refreshMsg struct {
	source  sessionmgr.SourceMode
	gen     uint64
	items   []sessionmgr.Item
	warning string
	err     error
}

// agentStart is a resolved agent process start time plus the pid it came from.
type agentStart struct {
	pid     int
	started time.Time
}

// agentStartedMsg carries the agent start times resolved in the background,
// keyed by pane id. A pane whose process could not be resolved is absent from
// the map, which renders as no age cell.
type agentStartedMsg struct{ starts map[string]agentStart }

type previewMsg struct {
	key     string
	preview string
	err     error
}

// hostsMsg carries the remote hosts the picker offers.
type hostsMsg struct{ hosts []remote.Host }

// hostSwitchMsg reports the outcome of asking the `seshagy run` launcher to
// switch its client; a nil error means the launcher is detaching this client.
type hostSwitchMsg struct{ err error }

// hostWindowMsg reports the outcome of opening a tmux window on a host.
type hostWindowMsg struct{ err error }

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

type options struct {
	config     *appconfig.Config
	startupErr error
	mux        sessionmgr.Multiplexer
	logger     *slog.Logger
	ephemeral  bool
}

// Option configures dependencies before a Model is constructed.
type Option func(*options)

func WithConfig(cfg appconfig.Config) Option {
	return func(opts *options) { opts.config = &cfg }
}

func WithStartupError(err error) Option {
	return func(opts *options) { opts.startupErr = err }
}

func WithMultiplexer(mux sessionmgr.Multiplexer) Option {
	return func(opts *options) { opts.mux = mux }
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) { opts.logger = logger }
}

// WithEphemeral enables the focus-loss dismissal poll so the dashboard exits
// when its hosting pane/window/session loses focus (--ephemeral).
func WithEphemeral(b bool) Option {
	return func(opts *options) { opts.ephemeral = b }
}

func New(optionFns ...Option) Model {
	settings := options{}
	for _, option := range optionFns {
		option(&settings)
	}
	var cfg appconfig.Config
	cfgErr := settings.startupErr
	if settings.config != nil {
		cfg = *settings.config
	} else {
		loaded, err := appconfig.Load()
		cfg = loaded
		if cfgErr == nil {
			cfgErr = err
		}
	}
	mux := settings.mux
	if mux == nil {
		mux = sessionmgr.Detect()
	}
	logger := settings.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	showPreview := true
	if cfg.TUI.Preview != nil {
		showPreview = *cfg.TUI.Preview
	}
	m := Model{
		styles:           stylesFromConfig(cfg),
		config:           cfg,
		layout:           cfg.TUILayout(),
		keys:             ResolveKeys(cfg.TUI.Keys),
		icons:            cfg.IconSet(),
		mux:              mux,
		terms:            mux.Terms(),
		logger:           logger,
		showPreview:      showPreview,
		cache:            make(map[sessionmgr.SourceMode]modeCache),
		refreshGen:       make(map[sessionmgr.SourceMode]uint64),
		inflightRefresh:  make(map[sessionmgr.SourceMode]uint64),
		reportedWarnings: make(map[sessionmgr.SourceMode]string),
		agentStarted:     make(map[string]agentStart),
		checkPopup:       mux.InMultiplexerPopup,
		ephemeral:        settings.ephemeral,
	}
	if cfgErr != nil {
		m.notify(cfgErr.Error(), sevError)
	}
	if issues := len(m.keys.Issues()); issues > 0 {
		m.notify(keyIssueToast(issues), sevWarning)
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
		tickCmd(),
		notificationTickCmd(),
		refreshCatalogsCmd(m.config),
		hostsCmd(m.config),
		startupInstallMenuCmd(m.config),
	}
	for _, source := range tuiSources {
		var cmd tea.Cmd
		m, cmd = m.beginRefresh(source, false)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if m.ephemeral {
		cmds = append(cmds, ephemeralTickCmd())
	}
	return tea.Batch(cmds...)
}

func (m *Model) notify(text string, sev notifSev) {
	m.notifications = append(m.notifications, notification{text: text, sev: sev, at: time.Now()})
	if len(m.notifications) > 3 {
		m.notifications = m.notifications[len(m.notifications)-3:]
	}
}

// rows is the visible, scope-filtered, query-ranked list.
func (m Model) rows() []row {
	return filterRows(m.items, m.scope, m.stateFilter, m.query)
}

// grouped reports whether the results window draws its section headers. An empty
// query files the list under its sections, and any query collapses it into one
// ranked list. A list whose rows all fall in one section draws no header either:
// a lone header would only repeat what the one scope already says.
func (m Model) grouped(rows []row) bool {
	if strings.TrimSpace(m.query) != "" {
		return false
	}
	for _, r := range rows {
		if r.group != rows[0].group {
			return true
		}
	}
	return false
}

// syncCursor keeps the selection stable across a refresh that reorders or
// shortens the list. Until the user has moved the cursor it holds row 0, the
// most urgent item, because the key seen so far came from a partially loaded
// list rather than from a choice. Afterwards the cursor follows the selected key
// and clamps by index when that item is gone.
func (m *Model) syncCursor() {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		m.cursorKey = ""
		return
	}
	if !m.cursorMoved {
		m.cursor = 0
		m.cursorKey = rows[0].item.Key()
		return
	}
	for i := range rows {
		if rows[i].item.Key() == m.cursorKey {
			m.cursor = i
			return
		}
	}
	m.cursor = clampVal(m.cursor, 0, len(rows)-1)
	m.cursorKey = rows[m.cursor].item.Key()
}

// setCursor moves the cursor to index (clamped) and records the selected key.
// The move is explicit, so later refreshes follow that key.
func (m Model) setCursor(index int) Model {
	m.cursorMoved = true
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		m.cursorKey = ""
		return m
	}
	m.cursor = clampVal(index, 0, len(rows)-1)
	m.cursorKey = rows[m.cursor].item.Key()
	return m
}

func (m Model) selectedItem() (sessionmgr.Item, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return sessionmgr.Item{}, false
	}
	return rows[m.cursor].item, true
}

func (m Model) selectedKey() string {
	item, ok := m.selectedItem()
	if !ok {
		return ""
	}
	return item.Key()
}

// hostByName resolves a host row back to its registry entry, which carries the
// session and proxy details the row does not.
func (m Model) hostByName(name string) (remote.Host, bool) {
	return remote.Find(m.hosts, name)
}

// previewCmdFor picks the source of a row's preview: hosts are rendered from
// local config, every other kind is captured from the multiplexer.
func (m Model) previewCmdFor(item sessionmgr.Item) tea.Cmd {
	if item.Kind == sessionmgr.KindHost {
		if host, ok := m.hostByName(item.Name); ok {
			return hostPreviewCmd(host)
		}
	}
	return previewCmd(m.mux, item)
}

// resolveAgentStarts schedules the background resolution of the agent start
// times that are not cached yet. It is a no-op while a resolution is in flight.
func (m Model) resolveAgentStarts() (Model, tea.Cmd) {
	if m.agentStartPending {
		return m, nil
	}
	pending := m.pendingAgentStarts()
	if len(pending) == 0 {
		return m, nil
	}
	m.agentStartPending = true
	return m, agentStartTimesCmd(m.mux, pending)
}

// pendingAgentStarts lists the agent items whose process start time is missing
// from the cache, or was resolved from a pid the pane no longer hosts (a pane
// that restarted its agent).
func (m Model) pendingAgentStarts() []sessionmgr.Item {
	pending := make([]sessionmgr.Item, 0, len(m.items))
	for _, item := range m.items {
		if item.Kind != sessionmgr.KindAgent || item.PaneID == "" {
			continue
		}
		if cached, ok := m.agentStarted[item.PaneID]; ok &&
			(item.AgentPID == 0 || cached.pid == item.AgentPID) {
			continue
		}
		pending = append(pending, item)
	}
	return pending
}

// applyAgentStarts stores resolved start times and re-stamps the item list so
// the age column renders from them.
func (m Model) applyAgentStarts(msg agentStartedMsg) Model {
	m.agentStartPending = false
	if m.agentStarted == nil {
		m.agentStarted = make(map[string]agentStart)
	}
	for pane, start := range msg.starts {
		m.agentStarted[pane] = start
	}
	m.items = m.mergedItems()
	return m
}

// previewVisible reports whether the preview window is drawn: enabled by
// [tui] preview and not toggled off with ctrl+o. Where it goes is the layout's
// business — a frame too narrow for a preview column stacks it under the results
// window instead of dropping it.
func (m Model) previewVisible() bool {
	return m.showPreview && m.width > 0 && m.height > 0
}

// requestPreview schedules a capture for the selected item unless its payload
// is already loaded or already in flight, so a source refresh every second does
// not re-capture the same pane. It is a no-op when the preview column is hidden.
func (m Model) requestPreview() (Model, tea.Cmd) {
	if !m.previewVisible() {
		return m, nil
	}
	item, ok := m.selectedItem()
	if !ok {
		return m, nil
	}
	key := item.Key()
	if key == m.previewKey || key == m.previewPendingKey {
		return m, nil
	}
	m.previewPendingKey = key
	m.preview = ""
	m.previewError = ""
	return m, m.previewCmdFor(item)
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

// windowCountLabel names a session's window count in the multiplexer's own
// vocabulary: "2 tabs" under herdr, "3 windows" under tmux.
func windowCountLabel(terms sessionmgr.Terms, count int) string {
	noun := terms.WindowPlural
	if count == 1 {
		noun = terms.WindowNoun
	}
	return fmt.Sprintf("%d %s", count, noun)
}

// followCursor moves the remembered results-window top just enough to keep the
// cursor's item fully visible, after any update that moved the cursor, changed
// the rows, or resized the frame.
func (m Model) followCursor() Model {
	rows := m.rows()
	if len(rows) == 0 {
		m.listTop = 0
		return m
	}
	m.listTop = newItemFlow(rows, m.grouped(rows)).follow(m.listTop, m.cursor, m.bodyHeight())
	return m
}
