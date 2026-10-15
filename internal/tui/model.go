package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
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

	mux    sessionmgr.Multiplexer
	terms  sessionmgr.Terms
	logger *slog.Logger

	width  int
	height int

	scope scope
	query string

	items     []sessionmgr.Item
	cursor    int
	cursorKey string

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

	// currentSession is the multiplexer session hosting seshagy, resolved on
	// every agent refresh; session rows for it are marked "current".
	currentSession string

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

	installMenu installMenuState
}

// installMenuState holds the state of the agent-integration install menu
// overlay (first-run popup + manual ctrl+a reopen).
type installMenuState struct {
	active   bool
	cursor   int
	statuses map[string]string
	message  string
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
	cfg, cfgErr := appconfig.Config{}, settings.startupErr
	if settings.config != nil {
		cfg = *settings.config
	} else {
		var err error
		cfg, err = appconfig.Load()
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
		mux:              mux,
		terms:            mux.Terms(),
		logger:           logger,
		showPreview:      showPreview,
		cache:            make(map[sessionmgr.SourceMode]modeCache),
		refreshGen:       make(map[sessionmgr.SourceMode]uint64),
		inflightRefresh:  make(map[sessionmgr.SourceMode]uint64),
		reportedWarnings: make(map[sessionmgr.SourceMode]string),
		checkPopup:       mux.InMultiplexerPopup,
		ephemeral:        settings.ephemeral,
	}
	if cfgErr != nil {
		m.notify(cfgErr.Error(), sevError)
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
	return filterRows(m.items, m.scope, m.query)
}

// syncCursor re-anchors the cursor on the selected item's key so a refresh that
// reorders or shortens the list cannot move the selection; it clamps when the
// selected item is gone.
func (m *Model) syncCursor() {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		m.cursorKey = ""
		return
	}
	if m.cursorKey != "" {
		for i := range rows {
			if rows[i].item.Key() == m.cursorKey {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = clampVal(m.cursor, 0, len(rows)-1)
	m.cursorKey = rows[m.cursor].item.Key()
}

// setCursor moves the cursor to index (clamped) and records the selected key.
func (m Model) setCursor(index int) Model {
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

// currentSessionLabel returns a human-facing label for the current session id.
// Under herdr the id is opaque (e.g. "wB"); agent items in the same session
// carry the resolved workspace label in Location, so it is looked up from the
// loaded items and falls back to the raw id. Under tmux the id IS the name.
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

// previewVisible reports whether the preview column is drawn: enabled and wide
// enough to keep a usable list beside it.
func (m Model) previewVisible() bool {
	return m.showPreview && m.width >= previewMinWidth
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
	return m, previewCmd(m.mux, item)
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

// windowCountLabel abbreviates the multiplexer's window noun to a 3-rune meta
// cell ("win" under tmux, "tab" under herdr).
func windowCountLabel(terms sessionmgr.Terms, count int) string {
	noun := []rune(terms.WindowNoun)
	if len(noun) > 3 {
		noun = noun[:3]
	}
	return fmt.Sprintf("%d %s", count, string(noun))
}
