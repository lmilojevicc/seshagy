package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestProjectLayoutSourcesPreservesConfiguredMembershipOrderAndLabels(t *testing.T) {
	cfg := appconfig.Default()
	cfg.Sources.Order = []string{"agents", "sessions", "current-agents"}
	m := New(
		WithConfig(cfg),
		WithMultiplexer(sessionmgr.NewTmuxBackend()),
	)
	m.source = sessionmgr.ModeAgents
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindAgent}, {Kind: sessionmgr.KindAgent}}
	m.loading = false

	got := m.projectLayout(layoutNeeds{}).Sources
	wantIDs := []sourceID{"agents", "sessions", "all", "zoxide", "fd"}
	wantLabels := []string{"Agents", "Sessions", "All", "Zoxide", "fd"}
	if len(got.Entries) != len(wantIDs) {
		t.Fatalf("source entries = %d, want %d: %#v", len(got.Entries), len(wantIDs), got.Entries)
	}
	for i, entry := range got.Entries {
		if entry.ID != wantIDs[i] || entry.Label != wantLabels[i] {
			t.Errorf(
				"source[%d] = (%q, %q), want (%q, %q)",
				i,
				entry.ID,
				entry.Label,
				wantIDs[i],
				wantLabels[i],
			)
		}
		if entry.Key != string(rune('1'+i)) {
			t.Errorf("source[%d].Key = %q, want %q", i, entry.Key, string(rune('1'+i)))
		}
		if entry.Selected != (i == 0) {
			t.Errorf("source[%d].Selected = %v, want %v", i, entry.Selected, i == 0)
		}
	}
	if got.VisibleCount != 2 || got.TotalCount != 2 {
		t.Fatalf("source counts = %d/%d, want 2/2", got.VisibleCount, got.TotalCount)
	}
	if got.ShowTotalCount {
		t.Fatal("source projection shows total count without a text query")
	}
	m.query = "agent"
	if queried := m.projectLayout(layoutNeeds{}).Sources; !queried.ShowTotalCount {
		t.Fatal("source projection hides total count with an active text query")
	}
	if got.Entries[3].ID != "zoxide" || got.Entries[4].ID != "fd" {
		t.Fatalf("unavailable directory sources removed: %#v", got.Entries)
	}
	if !got.Entries[0].CountKnown || got.Entries[0].Count != 2 {
		t.Fatalf("active source availability facts = %#v, want known count 2", got.Entries[0])
	}
	if got.Entries[3].CountKnown || got.Entries[4].CountKnown {
		t.Fatalf("uncached directory source counts marked known: %#v", got.Entries)
	}
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeZoxide: {items: []sessionmgr.Item{{Kind: sessionmgr.KindZoxide}}},
	}
	m.inflightRefresh[sessionmgr.ModeFD] = 7
	available := m.projectLayout(layoutNeeds{}).Sources
	if !available.Entries[3].CountKnown || available.Entries[3].Count != 1 {
		t.Fatalf("cached zoxide availability facts = %#v", available.Entries[3])
	}
	if !available.Entries[4].Refreshing {
		t.Fatalf("fd refresh fact = %#v, want refreshing", available.Entries[4])
	}
	for _, entry := range got.Entries {
		if entry.ID == "current-agents" {
			t.Fatal("CLI-only current-agents source crossed into layout projection")
		}
	}

	herdr := New(
		WithConfig(appconfig.Default()),
		WithMultiplexer(sessionmgr.NewHerdrBackend()),
	).projectLayout(layoutNeeds{}).Sources
	var sessionLabel string
	for _, entry := range herdr.Entries {
		if entry.ID == "sessions" {
			sessionLabel = entry.Label
			break
		}
	}
	if sessionLabel != "Workspaces" {
		t.Fatalf("herdr sessions label = %q, want Workspaces", sessionLabel)
	}
}

func TestProjectRowsExposeSemanticFieldsAcrossDisplayModes(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	items := []sessionmgr.Item{
		{
			Kind: sessionmgr.KindSession, Name: "attached", Attached: true,
			Activity: now.Add(-2 * time.Hour),
		},
		{
			Kind: sessionmgr.KindSession, Name: "detached", Attached: false,
			Activity: now.Add(-3 * time.Minute),
		},
		{
			Kind: sessionmgr.KindAgent, AgentName: "pi", AgentDisplayName: "reviewer",
			AgentState: sessionmgr.AgentWorking, Location: "workspace:1",
		},
		{Kind: sessionmgr.KindZoxide, Path: "/src/zoxide"},
		{Kind: sessionmgr.KindFD, Path: "/src/fd"},
	}

	for _, tt := range []struct {
		name           string
		configure      func(*appconfig.Config)
		wantKindMode   displayModeView
		wantKindValue  string
		wantStateMode  displayModeView
		wantTmuxValue  string
		wantAgentValue string
		wantColor      string
	}{
		{
			name: "icons",
			configure: func(cfg *appconfig.Config) {
				cfg.Icons.Mode = appconfig.IconModeIcons
				cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeIcons
				cfg.Icons.AgentStateMode = appconfig.StateDisplayModeIcons
				cfg.Icons.Session.Icon = "◆"
				cfg.Icons.Session.Color = "13"
				cfg.Icons.TmuxState.Attached.Icon = "▲"
				cfg.Icons.TmuxState.Attached.Color = "12"
				cfg.Icons.AgentState.Working.Icon = "★"
				cfg.Icons.AgentState.Working.Color = "14"
			},
			wantKindMode: displayIcon, wantKindValue: "◆",
			wantStateMode: displayIcon, wantTmuxValue: "▲", wantAgentValue: "★",
			wantColor: "13",
		},
		{
			name: "text",
			configure: func(cfg *appconfig.Config) {
				cfg.Icons.Mode = appconfig.IconModeText
				cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeText
				cfg.Icons.AgentStateMode = appconfig.StateDisplayModeText
				cfg.Icons.Session.Label = "sess"
				cfg.Icons.Session.Color = "13"
				cfg.Icons.TmuxState.Attached.Label = "linked"
				cfg.Icons.TmuxState.Attached.Color = "12"
				cfg.Icons.AgentState.Working.Label = "busy"
				cfg.Icons.AgentState.Working.Color = "14"
			},
			wantKindMode: displayLabel, wantKindValue: "sess",
			wantStateMode: displayLabel, wantTmuxValue: "linked", wantAgentValue: "busy",
			wantColor: "13",
		},
		{
			name: "hidden states",
			configure: func(cfg *appconfig.Config) {
				cfg.Icons.Mode = appconfig.IconModeNone
				cfg.Icons.TmuxStateMode = appconfig.StateDisplayModeNone
				cfg.Icons.AgentStateMode = appconfig.StateDisplayModeNone
			},
			wantKindMode: displayHidden, wantStateMode: displayHidden,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := appconfig.Default()
			tt.configure(&cfg)
			cfg.Normalize()
			m := New(WithConfig(cfg), WithMultiplexer(sessionmgr.NewTmuxBackend()))
			m.loading = false
			m.source = sessionmgr.ModeAll
			m.items = append([]sessionmgr.Item(nil), items...)
			m.cursor = 1

			rows := m.projectLayoutAt(now).Collection.Rows
			if len(rows) != len(items) {
				t.Fatalf("rows = %d, want %d", len(rows), len(items))
			}

			attached := rows[0]
			if attached.Kind != rowKind(sessionmgr.KindSession) || attached.Label != "attached" ||
				attached.Selected || !attached.Session.Attached || attached.Session.Activity != "2h" {
				t.Fatalf("attached session semantics = %#v", attached)
			}
			if attached.Icon.Mode != tt.wantKindMode || attached.Icon.Color != tt.wantColor {
				t.Fatalf("session kind indicator = %#v", attached.Icon)
			}
			assertIndicatorValue(t, attached.Icon, tt.wantKindValue)
			if attached.Session.State.Mode != tt.wantStateMode ||
				attached.Session.State.Color != map[displayModeView]string{
					displayHidden: "", displayIcon: "12", displayLabel: "12",
				}[tt.wantStateMode] {
				t.Fatalf("attached state indicator = %#v", attached.Session.State)
			}
			assertIndicatorValue(t, attached.Session.State, tt.wantTmuxValue)

			detached := rows[1]
			if !detached.Selected || detached.Session.Attached ||
				detached.Session.Activity != "3m" {
				t.Fatalf("detached session semantics = %#v", detached)
			}

			agent := rows[2]
			if agent.Label != "reviewer" || agent.Agent.Name != "pi" ||
				agent.Agent.DisplayName != "reviewer" || agent.Agent.State != "working" ||
				agent.Agent.Location != "workspace:1" {
				t.Fatalf("agent semantics = %#v", agent)
			}
			if agent.Agent.Indicator.Mode != tt.wantStateMode {
				t.Fatalf("agent state mode = %#v", agent.Agent.Indicator)
			}
			assertIndicatorValue(t, agent.Agent.Indicator, tt.wantAgentValue)

			if rows[3].Directory != (directoryRowView{Path: "/src/zoxide"}) ||
				rows[4].Directory != (directoryRowView{Path: "/src/fd"}) {
				t.Fatalf("directory semantics = %#v / %#v", rows[3], rows[4])
			}
		})
	}
}

func TestProjectAgentRowsPreserveEveryState(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeAgents
	for _, tt := range []struct {
		state sessionmgr.AgentState
		icon  string
		color string
	}{
		{state: sessionmgr.AgentIdle, icon: "○", color: "8"},
		{state: sessionmgr.AgentWorking, icon: "●", color: "10"},
		{state: sessionmgr.AgentBlocked, icon: "◐", color: "11"},
		{state: sessionmgr.AgentDone, icon: "◉", color: "14"},
		{state: sessionmgr.AgentUnknown, icon: "?", color: "8"},
	} {
		m.items = []sessionmgr.Item{{
			Kind: sessionmgr.KindAgent, AgentName: "pi", AgentState: tt.state,
			Location: "workspace:1",
		}}
		row := m.projectLayoutAt(now).Collection.Rows[0]
		if row.Agent.State != string(tt.state) || row.Agent.Location != "workspace:1" {
			t.Fatalf("state %q semantics = %#v", tt.state, row.Agent)
		}
		if row.Agent.Indicator != (indicatorView{
			Mode: displayIcon, Icon: tt.icon, Label: string(tt.state), Color: tt.color,
		}) {
			t.Fatalf("state %q indicator = %#v", tt.state, row.Agent.Indicator)
		}
	}
}

func assertIndicatorValue(t *testing.T, got indicatorView, want string) {
	t.Helper()
	switch got.Mode {
	case displayHidden:
		if got.Icon != "" || got.Label != "" {
			t.Fatalf("hidden indicator contains values: %#v", got)
		}
	case displayIcon:
		if got.Icon != want {
			t.Fatalf("indicator icon = %q, want %q: %#v", got.Icon, want, got)
		}
	case displayLabel:
		if got.Label != want {
			t.Fatalf("indicator label = %q, want %q: %#v", got.Label, want, got)
		}
	default:
		t.Fatalf("unknown indicator mode: %#v", got)
	}
}

func TestProjectLayoutCollectionPreservesCanonicalOrderCountsAndPrivacy(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.source = sessionmgr.ModeAll
	m.cursor = 1
	m.items = []sessionmgr.Item{
		{
			Kind: sessionmgr.KindFD, Path: "/src/zeta", Name: "RAW_FD_NAME",
			Target: "RAW_FD_TARGET", PaneID: "RAW_FD_PANE_ID", Session: "RAW_FD_SESSION",
			Window: "RAW_FD_WINDOW", Pane: "RAW_FD_PANE", TabLabel: "RAW_FD_TAB",
			AgentSource: "RAW_FD_AGENT_SOURCE",
		},
		{
			Kind: sessionmgr.KindSession, Name: "alpha", Target: "RAW_SESSION_TARGET",
			Path: "RAW_SESSION_PATH", Attached: true, Activity: now.Add(-2 * time.Hour),
			PaneID: "RAW_SESSION_PANE_ID", Session: "RAW_SESSION_SESSION",
			Window: "RAW_SESSION_WINDOW", Pane: "RAW_SESSION_PANE", TabLabel: "RAW_SESSION_TAB",
			AgentSource: "RAW_SESSION_AGENT_SOURCE",
		},
		{
			Kind: sessionmgr.KindAgent, Name: "RAW_AGENT_ITEM_NAME", Target: "RAW_AGENT_TARGET",
			Path: "RAW_AGENT_PATH", AgentName: "pi", AgentDisplayName: "reviewer",
			AgentState: sessionmgr.AgentWorking, AgentUpdated: now.Add(-time.Minute),
			Location: "alpha:1", PaneID: "RAW_AGENT_PANE_ID", Session: "RAW_AGENT_SESSION",
			Window: "RAW_AGENT_WINDOW", Pane: "RAW_AGENT_PANE", TabLabel: "RAW_AGENT_TAB",
			AgentSource: "RAW_AGENT_SOURCE",
		},
	}

	got := m.projectLayoutAt(now)
	if got.Collection.SessionCount != 1 || got.Collection.AgentCount != 1 ||
		got.Collection.DirectoryCount != 1 {
		t.Fatalf("kind counts = %#v", got.Collection)
	}
	if len(got.Collection.Rows) != 3 || !got.Collection.Rows[1].Selected {
		t.Fatalf("canonical rows/selection = %#v", got.Collection.Rows)
	}
	if got.Collection.Rows[0].Label != "/src/zeta" ||
		got.Collection.Rows[1].Label != "alpha" ||
		got.Collection.Rows[2].Label != "reviewer" {
		t.Fatalf("canonical row labels = %#v", got.Collection.Rows)
	}

	projected := allProjectedStrings(got)
	for _, forbidden := range []string{
		"RAW_FD_NAME", "RAW_FD_TARGET", "RAW_FD_PANE_ID", "RAW_FD_SESSION",
		"RAW_FD_WINDOW", "RAW_FD_PANE", "RAW_FD_TAB", "RAW_FD_AGENT_SOURCE",
		"RAW_SESSION_TARGET", "RAW_SESSION_PATH", "RAW_SESSION_PANE_ID",
		"RAW_SESSION_SESSION", "RAW_SESSION_WINDOW", "RAW_SESSION_PANE",
		"RAW_SESSION_TAB", "RAW_SESSION_AGENT_SOURCE", "RAW_AGENT_ITEM_NAME",
		"RAW_AGENT_TARGET", "RAW_AGENT_PATH", "RAW_AGENT_PANE_ID", "RAW_AGENT_SESSION",
		"RAW_AGENT_WINDOW", "RAW_AGENT_PANE", "RAW_AGENT_TAB", "RAW_AGENT_SOURCE",
	} {
		if strings.Contains(projected, forbidden) {
			t.Errorf("layout projection leaks raw value %q in %q", forbidden, projected)
		}
	}

	m.items[0].Path = "mutated-controller-path"
	m.items[1].Name = "mutated-controller-name"
	m.items[2].AgentDisplayName = "mutated-controller-agent"
	if got.Collection.Rows[0].Label != "/src/zeta" ||
		got.Collection.Rows[1].Label != "alpha" ||
		got.Collection.Rows[2].Agent.DisplayName != "reviewer" {
		t.Fatalf("projection aliases controller rows: %#v", got.Collection.Rows)
	}

	got.Sources.Entries[0].Label = "mutated-projection-source"
	got.Collection.Rows[0].Label = "mutated-projection-row"
	got.Actions.Hints[0].Label = "mutated-projection-hint"
	fresh := m.projectLayoutAt(now)
	if fresh.Sources.Entries[0].Label == "mutated-projection-source" ||
		fresh.Collection.Rows[0].Label == "mutated-projection-row" ||
		fresh.Actions.Hints[0].Label == "mutated-projection-hint" {
		t.Fatalf("projection slices alias later projections: %#v", fresh)
	}
}

func allProjectedStrings(value any) string {
	var b strings.Builder
	appendProjectedStrings(reflect.ValueOf(value), &b)
	return b.String()
}

func appendProjectedStrings(value reflect.Value, b *strings.Builder) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.String:
		b.WriteString(value.String())
		b.WriteByte('\n')
	case reflect.Struct:
		for i := range value.NumField() {
			appendProjectedStrings(value.Field(i), b)
		}
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			appendProjectedStrings(value.Index(i), b)
		}
	case reflect.Pointer:
		if !value.IsNil() {
			appendProjectedStrings(value.Elem(), b)
		}
	}
}

func TestProjectLayoutCollectionDiagnosticsAndLoadingPrecedence(t *testing.T) {
	base := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	base.source = sessionmgr.ModeFD
	base.loading = false
	base.items = nil
	base.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeFD: {
			warning: "partial fd warning",
			err:     testProjectionError("fd failed"),
		},
	}

	failed := base.projectLayout(layoutNeeds{}).Collection
	if failed.State != collectionError || failed.Warning != "partial fd warning" ||
		failed.Error != "fd failed" {
		t.Fatalf("collection diagnostics = %#v", failed)
	}

	base.loading = true
	loading := base.projectLayout(layoutNeeds{}).Collection
	if loading.State != collectionLoading || loading.Warning != "" || loading.Error != "" {
		t.Fatalf("loading leaked stale diagnostics = %#v", loading)
	}
}

type testProjectionError string

func (e testProjectionError) Error() string { return string(e) }

func TestProjectLayoutCollectionStatesAndAgentScope(t *testing.T) {
	base := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewHerdrBackend()))
	base.source = sessionmgr.ModeAgents
	base.loading = false
	base.agentsCurrentOnly = true
	base.currentSession = "workspace-opaque-id"
	base.items = []sessionmgr.Item{{
		Kind: sessionmgr.KindAgent, Session: "workspace-opaque-id", Location: "Frontend",
	}}

	resolved := base.projectLayout(layoutNeeds{}).Collection
	if resolved.ScopeLabel != "Frontend" {
		t.Fatalf("resolved scope = %q, want Frontend", resolved.ScopeLabel)
	}
	base.items[0].Location = ""
	fallback := base.projectLayout(layoutNeeds{}).Collection
	if fallback.ScopeLabel != "workspace-opaque-id" {
		t.Fatalf("scope fallback = %q, want opaque current workspace id", fallback.ScopeLabel)
	}

	for _, tt := range []struct {
		name      string
		mutate    func(*Model)
		wantState collectionState
		wantEmpty string
	}{
		{
			name: "zero-item loading", wantState: collectionLoading, wantEmpty: "no items",
			mutate: func(m *Model) { m.loading = true; m.items = nil },
		},
		{
			name: "empty", wantState: collectionEmpty, wantEmpty: "no items",
			mutate: func(m *Model) { m.items = nil; m.agentsCurrentOnly = false },
		},
		{
			name: "filtered empty", wantState: collectionFilteredEmpty,
			wantEmpty: "no matches for missing",
			mutate:    func(m *Model) { m.query = "missing" },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			m.cache = nil
			tt.mutate(&m)
			got := m.projectLayout(layoutNeeds{}).Collection
			if got.State != tt.wantState || got.EmptyMessage != tt.wantEmpty {
				t.Fatalf(
					"collection state = (%v, %q), want (%v, %q)",
					got.State,
					got.EmptyMessage,
					tt.wantState,
					tt.wantEmpty,
				)
			}
		})
	}
}

func TestProjectLayoutSearchAndActionsPreserveSharedSemantics(t *testing.T) {
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(sessionmgr.NewTmuxBackend()))
	m.loading = false
	m.query = "needle"
	m.inputMode = modeSearch
	m.source = sessionmgr.ModeAll
	m.items = []sessionmgr.Item{{Kind: sessionmgr.KindFD, Path: "/src/needle"}}

	classic := m.projectLayout(layoutNeeds{})
	if classic.Search != (searchView{
		Mode: searchClassic, Query: "needle", Editing: true,
	}) {
		t.Fatalf("classic search projection = %#v", classic.Search)
	}
	wantClassic := commonActionHints(true)
	wantClassic = append(wantClassic,
		actionHintView{Key: "R", Label: "rename"},
		actionHintView{Key: "x", Label: "kill"},
		actionHintView{Key: "y", Label: "yazi", Available: true},
	)
	if classic.Actions.Prefix != m.config.PrefixKey() {
		t.Fatalf(
			"classic Actions prefix = %q, want %q",
			classic.Actions.Prefix,
			m.config.PrefixKey(),
		)
	}
	if !reflect.DeepEqual(classic.Actions.Hints, wantClassic) {
		t.Fatalf("classic Actions = %#v, want %#v", classic.Actions.Hints, wantClassic)
	}
	if !hintAvailable(classic.Actions.Hints, "enter") ||
		hintAvailable(classic.Actions.Hints, "R") ||
		hintAvailable(classic.Actions.Hints, "x") {
		t.Fatalf("fd-selected action eligibility = %#v", classic.Actions.Hints)
	}

	m.source = sessionmgr.ModeAgents
	m.query = ""
	m.items = []sessionmgr.Item{{
		Kind: sessionmgr.KindAgent, AgentName: "pi", PaneID: "%1",
		Session: "s", Window: "w",
	}}
	agents := m.projectLayout(layoutNeeds{}).Actions
	wantAgents := append(commonActionHints(true),
		actionHintView{Key: "o", Label: "this session", Available: true},
		actionHintView{Key: "s", Label: "filter state", Available: true},
		actionHintView{Key: "R", Label: "rename", Available: true},
	)
	if !reflect.DeepEqual(agents.Hints, wantAgents) {
		t.Fatalf("Agents Actions = %#v, want %#v", agents.Hints, wantAgents)
	}
	if !hintAvailable(agents.Hints, "enter") || !hintAvailable(agents.Hints, "R") {
		t.Fatalf("valid agent action eligibility = %#v", agents.Hints)
	}

	m.showHelp = false
	collapsed := m.projectLayout(layoutNeeds{}).Actions
	wantCollapsed := actionsView{
		Expanded: false, Prefix: m.config.PrefixKey(), PrefixArmed: false,
		Hints: []actionHintView{{Key: "?", Label: "help", Available: true}},
	}
	if !reflect.DeepEqual(collapsed, wantCollapsed) {
		t.Fatalf("collapsed Actions = %#v, want %#v", collapsed, wantCollapsed)
	}

	m.showHelp = true
	m.config.TypeFirst.Enabled = true
	m.config.TypeFirst.Prefix = "ctrl+g"
	m.prefixArmed = false
	typeFirst := m.projectLayout(layoutNeeds{})
	if typeFirst.Search.Mode != searchTypeFirst {
		t.Fatalf("type-first search = %#v", typeFirst.Search)
	}
	wantTypeFirst := actionsView{
		Expanded: true, Prefix: "ctrl+g", PrefixArmed: false,
		Hints: []actionHintView{
			{Key: "type", Label: "filter", Available: true},
			{Key: "ctrl+g", Label: "actions", Available: true},
			{Key: "ctrl+g m", Label: "mode", Available: true},
			{Key: "backspace", Label: "edit", Available: true},
		},
	}
	if !reflect.DeepEqual(typeFirst.Actions, wantTypeFirst) {
		t.Fatalf("type-first Actions = %#v, want %#v", typeFirst.Actions, wantTypeFirst)
	}

	m.prefixArmed = true
	armed := m.projectLayout(layoutNeeds{}).Actions
	wantArmed := actionsView{
		Expanded: true, Prefix: "ctrl+g", PrefixArmed: true,
		Hints: append(commonActionHints(true),
			actionHintView{Key: "o", Label: "this session", Available: true},
			actionHintView{Key: "s", Label: "filter state", Available: true},
			actionHintView{Key: "R", Label: "rename", Available: true},
		),
	}
	if !reflect.DeepEqual(armed, wantArmed) {
		t.Fatalf("prefix-armed Actions = %#v, want %#v", armed, wantArmed)
	}
}

func hintAvailable(hints []actionHintView, key string) bool {
	for _, hint := range hints {
		if hint.Key == key {
			return hint.Available
		}
	}
	return false
}

func commonActionHints(activate bool) []actionHintView {
	return []actionHintView{
		{Key: "?", Label: "help", Available: true},
		{Key: "tab/⇧+tab", Label: "sections", Available: true},
		{Key: "q", Label: "quit", Available: true},
		{Key: "enter", Label: "attach/create/focus", Available: activate},
		{Key: "/", Label: "filter", Available: true},
		{Key: "r", Label: "refresh", Available: true},
		{Key: "p", Label: "preview", Available: true},
		{Key: "m", Label: "mode", Available: true},
		{Key: "h", Label: "install", Available: true},
	}
}

func TestLayoutProjectionTypesExcludeControllerBackendAndMutableSurfaces(t *testing.T) {
	forbidden := map[reflect.Type]string{
		reflect.TypeOf(Model{}):              "Model",
		reflect.TypeOf(textinput.Model{}):    "textinput.Model",
		reflect.TypeOf(appconfig.Config{}):   "config.Config",
		reflect.TypeOf(sessionmgr.Item{}):    "sessionmgr.Item",
		reflect.TypeOf((*error)(nil)).Elem(): "error interface",
	}
	if path, why := forbiddenProjectionType(
		reflect.TypeOf(layoutView{}),
		forbidden,
		nil,
	); why != "" {
		t.Fatalf("layout projection exposes %s at %s", why, path)
	}
}

func forbiddenProjectionType(
	typ reflect.Type,
	forbidden map[reflect.Type]string,
	seen map[reflect.Type]bool,
) (string, string) {
	if seen == nil {
		seen = make(map[reflect.Type]bool)
	}
	if why, ok := forbidden[typ]; ok {
		return typ.String(), why
	}
	if seen[typ] {
		return "", ""
	}
	seen[typ] = true
	switch typ.Kind() {
	case reflect.Map, reflect.Func, reflect.Chan, reflect.Interface, reflect.UnsafePointer:
		return typ.String(), typ.Kind().String()
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return forbiddenProjectionType(typ.Elem(), forbidden, seen)
	case reflect.Struct:
		for i := range typ.NumField() {
			field := typ.Field(i)
			if path, why := forbiddenProjectionType(field.Type, forbidden, seen); why != "" {
				return typ.String() + "." + field.Name + "." + path, why
			}
		}
	}
	return "", ""
}
