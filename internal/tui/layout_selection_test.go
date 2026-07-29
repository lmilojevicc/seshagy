package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestResolveLayoutUsesClosedCanonicalSet(t *testing.T) {
	if string(layoutDefault) != appconfig.LayoutDefault ||
		string(layoutZen) != appconfig.LayoutZen {
		t.Fatalf(
			"layout IDs drifted from config tokens: default=%q Zen=%q",
			layoutDefault,
			layoutZen,
		)
	}

	for _, tt := range []struct {
		name  string
		input string
		want  layoutID
		known bool
	}{
		{name: "missing", input: "", want: layoutDefault, known: true},
		{name: "default", input: appconfig.LayoutDefault, want: layoutDefault, known: true},
		{name: "Zen", input: appconfig.LayoutZen, want: layoutZen, known: true},
		{name: "unknown", input: "focus", want: layoutDefault, known: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, known := resolveLayout(tt.input)
			if got.id != tt.want || known != tt.known || got.render == nil ||
				got.renderActions == nil || got.renderInput == nil || got.inputContentWidth == nil {
				t.Fatalf(
					"resolveLayout(%q) = id:%q known:%v render:%v actions:%v input:%v width:%v, want id:%q known:%v",
					tt.input,
					got.id,
					known,
					got.render != nil,
					got.renderActions != nil,
					got.renderInput != nil,
					got.inputContentWidth != nil,
					tt.want,
					tt.known,
				)
			}
		})
	}
}

func TestNewSelectsLayoutWithoutChangingControllerDefaults(t *testing.T) {
	newConfigured := func(layout string) Model {
		cfg := appconfig.Default()
		cfg.TUI.Layout = layout
		return New(
			WithConfig(cfg),
			WithMultiplexer(sessionmgr.NewNoopBackend()),
		)
	}

	missing := newConfigured("")
	defaults := newConfigured(appconfig.LayoutDefault)
	zen := newConfigured(appconfig.LayoutZen)

	if missing.layoutSpec().id != layoutDefault || defaults.layoutSpec().id != layoutDefault {
		t.Fatalf(
			"missing/default layouts = %q/%q, want default",
			missing.layout.id,
			defaults.layout.id,
		)
	}
	if zen.layoutSpec().id != layoutZen {
		t.Fatalf("Zen layout = %q, want %q", zen.layout.id, layoutZen)
	}
	if len(missing.notifications) != 0 || len(defaults.notifications) != 0 ||
		len(zen.notifications) != 0 {
		t.Fatalf(
			"known layout notifications = missing:%#v default:%#v Zen:%#v",
			missing.notifications,
			defaults.notifications,
			zen.notifications,
		)
	}

	if zen.showPreview != defaults.showPreview || zen.showHelp != defaults.showHelp ||
		zen.source != defaults.source || zen.inputMode != defaults.inputMode ||
		zen.agentsCurrentOnly != defaults.agentsCurrentOnly ||
		zen.agentsStateFilter != defaults.agentsStateFilter ||
		zen.loading != defaults.loading || zen.spinnerActive != defaults.spinnerActive {
		t.Fatalf("Zen changed shared controller defaults: default=%#v Zen=%#v", defaults, zen)
	}
}

func TestUnknownLayoutWarnsOnceAndFallsBackWithoutReplacingStartupError(t *testing.T) {
	cfg := appconfig.Default()
	cfg.TUI.Layout = "  Focus  "

	fallback := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
	)
	if fallback.layoutSpec().id != layoutDefault {
		t.Fatalf("unknown layout selected %q, want default", fallback.layoutSpec().id)
	}
	if len(fallback.notifications) != 1 ||
		fallback.notifications[0].text != `unknown tui layout "focus"; using default` ||
		fallback.notifications[0].sev != sevWarning {
		t.Fatalf("unknown layout notifications = %#v", fallback.notifications)
	}

	startupErr := errors.New("invalid source configuration")
	withError := New(
		WithConfig(cfg),
		WithStartupError(startupErr),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
	)
	if len(withError.notifications) != 2 {
		t.Fatalf(
			"startup notifications = %#v, want error plus layout warning",
			withError.notifications,
		)
	}
	if withError.notifications[0].text != startupErr.Error() ||
		withError.notifications[0].sev != sevError {
		t.Fatalf("startup error was replaced: %#v", withError.notifications)
	}
	if withError.notifications[1].text != `unknown tui layout "focus"; using default` ||
		withError.notifications[1].sev != sevWarning {
		t.Fatalf("layout warning = %#v", withError.notifications[1])
	}
}

func TestConfiguredLayoutDispatchAndInputStyleRemainShared(t *testing.T) {
	newSurface := func(layout, inputStyle string) (Model, string) {
		cfg := appconfig.Default()
		cfg.TUI.Layout = layout
		cfg.TUI.InputStyle = inputStyle
		m := New(
			WithConfig(cfg),
			WithMultiplexer(sessionmgr.NewNoopBackend()),
		)
		m.width, m.height = 100, 24
		m.loading = false
		m.source = sessionmgr.ModeSessions
		m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}
		m.showHelp = false
		return m, sessionmgr.StripANSI(m.renderNormalSurface())
	}

	missing, missingFrame := newSurface("", appconfig.InputStylePopup)
	defaults, defaultFrame := newSurface(appconfig.LayoutDefault, appconfig.InputStylePopup)
	zen, zenFrame := newSurface(appconfig.LayoutZen, appconfig.InputStylePopup)

	if missingFrame != defaultFrame {
		t.Fatal("missing layout did not dispatch the exact Default renderer")
	}
	if missing.layoutSpec().id != layoutDefault || defaults.layoutSpec().id != layoutDefault {
		t.Fatal("missing/explicit Default did not resolve to Default")
	}
	if zen.layoutSpec().id != layoutZen || !strings.Contains(zenFrame, "seshagy") ||
		strings.Contains(zenFrame, "SOURCES") || zenFrame == defaultFrame {
		t.Fatalf("configured Zen did not dispatch Zen renderer\n%s", zenFrame)
	}

	zen.inputMode = modeSearch
	if !zen.inputPopupActive() || zen.inlineInputActive() {
		t.Fatal("Zen changed shared popup input behavior")
	}
	cmdline, _ := newSurface(appconfig.LayoutZen, appconfig.InputStyleCmdline)
	cmdline.inputMode = modeSearch
	if cmdline.inputPopupActive() || !cmdline.inlineInputActive() {
		t.Fatal("Zen changed shared cmdline input behavior")
	}
}

func TestPreviewActionAvailabilityFollowsSelectedLayout(t *testing.T) {
	for _, tt := range []struct {
		layout string
		want   bool
	}{
		{layout: appconfig.LayoutDefault, want: true},
		{layout: appconfig.LayoutZen, want: false},
	} {
		t.Run(tt.layout, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.TUI.Layout = tt.layout
			m := New(
				WithConfig(cfg),
				WithMultiplexer(sessionmgr.NewNoopBackend()),
			)
			m.showHelp = true
			got := false
			for _, hint := range m.projectActions().Hints {
				if hint.Key == "p" && hint.Label == "preview" {
					got = true
				}
			}
			if got != tt.want {
				t.Fatalf("Preview action present = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypeFirstAltPUsesPreviewActionPath(t *testing.T) {
	altP := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}, Alt: true}

	for _, preview := range []bool{false, true} {
		state := "disabled"
		if preview {
			state = "enabled"
		}
		t.Run("default/"+state, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.TUI.Layout = appconfig.LayoutDefault
			cfg.TUI.Preview = &preview
			cfg.TypeFirst.Enabled = true
			mux := &countingPreviewMux{Multiplexer: sessionmgr.NewNoopBackend()}
			m := New(WithConfig(cfg), WithMultiplexer(mux))
			m.loading = false
			m.source = sessionmgr.ModeSessions
			m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}

			model, cmd := m.handleKey(altP)
			got := model.(Model)
			if got.showPreview == preview {
				t.Fatalf("Default Alt+P left showPreview = %v", got.showPreview)
			}
			executeImmediateCommands(t, cmd)
			wantCaptures := 0
			if !preview {
				wantCaptures = 1
			}
			if mux.captures != wantCaptures || len(got.notifications) != 0 {
				t.Fatalf(
					"Default Alt+P = captures:%d notifications:%#v, want captures:%d",
					mux.captures,
					got.notifications,
					wantCaptures,
				)
			}
		})

		t.Run("zen/"+state, func(t *testing.T) {
			cfg := appconfig.Default()
			cfg.TUI.Layout = appconfig.LayoutZen
			cfg.TUI.Preview = &preview
			cfg.TypeFirst.Enabled = true
			mux := &countingPreviewMux{Multiplexer: sessionmgr.NewNoopBackend()}
			m := New(WithConfig(cfg), WithMultiplexer(mux))
			m.loading = false
			m.source = sessionmgr.ModeSessions
			m.items = []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}}

			model, cmd := m.handleKey(altP)
			got := model.(Model)
			if got.showPreview != preview || cmd != nil || mux.captures != 0 {
				t.Fatalf(
					"Zen Alt+P = show:%v cmd:%v captures:%d, want show:%v",
					got.showPreview,
					cmd,
					mux.captures,
					preview,
				)
			}
			if len(got.notifications) != 1 ||
				got.notifications[0].text != "preview is not available in the zen layout" ||
				got.notifications[0].sev != sevInfo {
				t.Fatalf("Zen Alt+P notifications = %#v", got.notifications)
			}
		})
	}
}

func TestTypeFirstPlainPRemainsFilterText(t *testing.T) {
	preview := false
	cfg := appconfig.Default()
	cfg.TUI.Layout = appconfig.LayoutDefault
	cfg.TUI.Preview = &preview
	cfg.TypeFirst.Enabled = true
	m := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
	)

	model, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	got := model.(Model)
	if got.query != "p" || got.searchInput.Value() != "p" || got.showPreview || cmd != nil ||
		len(got.notifications) != 0 {
		t.Fatalf(
			"plain p = query:%q input:%q preview:%v cmd:%v notifications:%#v",
			got.query,
			got.searchInput.Value(),
			got.showPreview,
			cmd,
			got.notifications,
		)
	}
}

func TestZenPreviewEffectsStayDisabledAcrossSharedUpdatePaths(t *testing.T) {
	preview := true
	cfg := appconfig.Default()
	cfg.TUI.Layout = appconfig.LayoutZen
	cfg.TUI.Preview = &preview
	mux := &countingPreviewMux{Multiplexer: sessionmgr.NewNoopBackend()}
	m := New(WithConfig(cfg), WithMultiplexer(mux))
	m.loading = false
	m.source = sessionmgr.ModeSessions
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "one"},
		{Kind: sessionmgr.KindSession, Name: "two"},
	}
	m.cursor = 0
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{}
	m.cache = map[sessionmgr.SourceMode]modeCache{}

	model, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = model.(Model)
	executeImmediateCommands(t, cmd)

	model, cmd = m.handleActionKey(keyMsg("down"))
	m = model.(Model)
	executeImmediateCommands(t, cmd)
	if m.cursor != 1 {
		t.Fatalf("selection cursor = %d, want 1", m.cursor)
	}

	model, cmd = m.handleActionKey(keyMsg("r"))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("shared refresh command was gated under Zen")
	}
	m = applyImmediateMessages(t, m, executeImmediateCommands(t, cmd))

	m.source = sessionmgr.ModeAll
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{}
	m.cache[sessionmgr.ModeSessions] = modeCache{
		items:     []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "cached"}},
		fetchedAt: time.Now(),
	}
	model, cmd = m.handleActionKey(keyMsg("2"))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("shared source navigation refresh was gated under Zen")
	}
	if item, ok := m.selectedItem(); !ok || item.Name != "cached" {
		t.Fatalf("source navigation did not select cached session: %#v, ok=%v", item, ok)
	}
	applyImmediateMessages(t, m, executeImmediateCommands(t, cmd))

	if mux.captures != 0 {
		t.Fatalf("Zen shared update paths captured Preview %d times", mux.captures)
	}
}

func TestZenRetainsSharedOverviewWarm(t *testing.T) {
	cfg := appconfig.Default()
	cfg.TUI.Layout = appconfig.LayoutZen
	m := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewNoopBackend()),
	)
	m.source = sessionmgr.ModeSessions
	m.inflightRefresh = map[sessionmgr.SourceMode]uint64{sessionmgr.ModeSessions: 1}
	m.refreshGen = map[sessionmgr.SourceMode]uint64{sessionmgr.ModeSessions: 1}

	if !m.needsOverviewWarm() {
		t.Fatal("Zen did not request the shared Overview warm")
	}
	msg := m.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) != 8 {
		t.Fatalf("Zen Init = %T with %d commands, want 8-command shared warm", msg, len(batch))
	}
}

func applyImmediateMessages(t *testing.T, m Model, messages []tea.Msg) Model {
	t.Helper()
	for _, msg := range messages {
		model, cmd := m.Update(msg)
		m = model.(Model)
		m = applyImmediateMessages(t, m, executeImmediateCommands(t, cmd))
	}
	return m
}

func executeImmediateCommands(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var messages []tea.Msg
	for _, child := range batch {
		messages = append(messages, executeImmediateCommands(t, child)...)
	}
	return messages
}
