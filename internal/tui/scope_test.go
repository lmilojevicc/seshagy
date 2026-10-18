package tui

import (
	"slices"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestScopeOrderCyclesInDrawOrder(t *testing.T) {
	want := []scope{scopeAll, scopeAgents, scopeSessions, scopeHosts, scopeZoxide, scopeFind}
	if !slices.Equal(scopeOrder, want) {
		t.Fatalf("scopeOrder = %v, want %v", scopeOrder, want)
	}
	// Cycling walks every scope in order and wraps at both ends.
	always := func(scope) bool { return true }
	seen := make(map[scope]bool, scopeCount)
	current := scopeAll
	for i := 0; i < len(scopeOrder); i++ {
		if seen[current] {
			t.Fatalf("scope %v visited twice", current)
		}
		seen[current] = true
		current = nextScope(current, 1, always)
	}
	if current != scopeAll {
		t.Fatalf("forward cycle ended on %v, want scopeAll", current)
	}
	if got := nextScope(scopeAll, -1, always); got != scopeFind {
		t.Fatalf("nextScope(scopeAll, -1) = %v, want scopeFind", got)
	}
}

func TestNextScopeStepsOverScopesWithoutASource(t *testing.T) {
	noHosts := func(sc scope) bool { return sc != scopeHosts }
	if got := nextScope(scopeSessions, 1, noHosts); got != scopeZoxide {
		t.Fatalf("forward past a missing hosts scope = %v, want scopeZoxide", got)
	}
	if got := nextScope(scopeZoxide, -1, noHosts); got != scopeSessions {
		t.Fatalf("backward past a missing hosts scope = %v, want scopeSessions", got)
	}
	// Nothing available but the scope itself: the cycle stays put.
	none := func(scope) bool { return false }
	if got := nextScope(scopeAgents, 1, none); got != scopeAgents {
		t.Fatalf("nextScope with no available scope = %v, want scopeAgents", got)
	}
}

func TestScopeLabels(t *testing.T) {
	want := map[scope]string{
		scopeAll:      "all",
		scopeAgents:   "agents",
		scopeSessions: "sessions",
		scopeHosts:    "remotes",
		scopeZoxide:   "zoxide",
		scopeFind:     "find",
	}
	for sc, label := range want {
		if got := scopeLabels[sc]; got != label {
			t.Fatalf("scopeLabels[%v] = %q, want %q", sc, got, label)
		}
	}
}

func TestScopeIncludesKinds(t *testing.T) {
	working := sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking}
	tests := []struct {
		name string
		sc   scope
		item sessionmgr.Item
		want bool
	}{
		{"all keeps sessions", scopeAll, sessionmgr.Item{Kind: sessionmgr.KindSession}, true},
		{"agents keeps agents", scopeAgents, working, true},
		{
			"agents drops sessions",
			scopeAgents,
			sessionmgr.Item{Kind: sessionmgr.KindSession},
			false,
		},
		{
			"sessions keeps sessions",
			scopeSessions,
			sessionmgr.Item{Kind: sessionmgr.KindSession},
			true,
		},
		{"zoxide keeps zoxide", scopeZoxide, sessionmgr.Item{Kind: sessionmgr.KindZoxide}, true},
		{"zoxide drops fd", scopeZoxide, sessionmgr.Item{Kind: sessionmgr.KindFD}, false},
		{"find keeps fd", scopeFind, sessionmgr.Item{Kind: sessionmgr.KindFD}, true},
		{"find drops zoxide", scopeFind, sessionmgr.Item{Kind: sessionmgr.KindZoxide}, false},
		{"hosts keeps hosts", scopeHosts, sessionmgr.Item{Kind: sessionmgr.KindHost}, true},
		{"hosts drops agents", scopeHosts, working, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sc.includes(tt.item); got != tt.want {
				t.Fatalf("%v.includes() = %v, want %v", tt.sc, got, tt.want)
			}
		})
	}
}

func TestStateFilterCycle(t *testing.T) {
	// The ctrl+s cycle: blocked → done → working → idle → none → blocked.
	cycle := []stateFilter{
		stateFilterBlocked,
		stateFilterDone,
		stateFilterWorking,
		stateFilterIdle,
		stateFilterNone,
	}
	current := stateFilterNone
	for i := 0; i < 2*len(cycle); i++ {
		current = current.next()
		if want := cycle[i%len(cycle)]; current != want {
			t.Fatalf("step %d = %v, want %v", i, current, want)
		}
	}
}

func TestStateFilterLabelsAndMatching(t *testing.T) {
	if got := stateFilterNone.label(); got != "" {
		t.Fatalf("stateFilterNone.label() = %q, want empty", got)
	}
	for filter, label := range map[stateFilter]string{
		stateFilterBlocked: "blocked",
		stateFilterDone:    "done",
		stateFilterWorking: "working",
		stateFilterIdle:    "idle",
	} {
		if got := filter.label(); got != label {
			t.Fatalf("stateFilter(%d).label() = %q, want %q", filter, got, label)
		}
	}

	idle := sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentIdle}
	session := sessionmgr.Item{Kind: sessionmgr.KindSession}
	tests := []struct {
		name   string
		filter stateFilter
		item   sessionmgr.Item
		want   bool
	}{
		{"none keeps agents", stateFilterNone, idle, true},
		{"none keeps sessions", stateFilterNone, session, true},
		{"idle keeps idle", stateFilterIdle, idle, true},
		{"idle drops sessions", stateFilterIdle, session, false},
		{
			"blocked drops working",
			stateFilterBlocked,
			sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentWorking},
			false,
		},
		{
			"done keeps done",
			stateFilterDone,
			sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentDone},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.matches(tt.item); got != tt.want {
				t.Fatalf("%v.matches(%v) = %v, want %v", tt.filter, tt.item.Kind, got, tt.want)
			}
		})
	}
}

func TestFilterRowsFilesItemsUnderSectionsInOrder(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindFD, Path: "/tmp/fd", Name: "/tmp/fd"},
		{Kind: sessionmgr.KindSession, Name: "dev"},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentIdle,
			PaneID:     "%1",
		},
		{Kind: sessionmgr.KindZoxide, Path: "/tmp/z", Name: "/tmp/z"},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "claude",
			AgentState: sessionmgr.AgentBlocked,
			PaneID:     "%2",
		},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "codex",
			AgentState: sessionmgr.AgentWorking,
			PaneID:     "%3",
		},
		{Kind: sessionmgr.KindHost, Name: "box", Target: "box"},
	}
	rows := filterRows(items, scopeAll, stateFilterNone, "")
	// Sections draw in group order — needs you, agents, sessions, hosts, zoxide,
	// find — with the established rank inside each one: a blocked agent before a
	// working one, and the dir sources in frecency order.
	want := []string{"claude", "codex", "pi", "dev", "box", "/tmp/z", "/tmp/fd"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, title := range want {
		if got := itemTitle(rows[i].item); got != title {
			t.Fatalf("row %d = %q, want %q", i, got, title)
		}
	}
	// The sections the rows file under, in draw order.
	var groups []group
	for _, r := range rows {
		if len(groups) == 0 || groups[len(groups)-1] != r.group {
			groups = append(groups, r.group)
		}
	}
	wantGroups := []group{
		groupNeedsYou, groupAgents, groupSessions, groupHosts, groupZoxide, groupFind,
	}
	if !slices.Equal(groups, wantGroups) {
		t.Fatalf("sections = %v, want %v", groups, wantGroups)
	}
}

func TestFilterRowsFilesFinishedAgentsUnderNeedsYou(t *testing.T) {
	done := agentItem("codex", "%1", "api")
	done.AgentState = sessionmgr.AgentDone
	idle := agentItem("pi", "%2", "skills")
	rows := filterRows([]sessionmgr.Item{idle, done}, scopeAll, stateFilterNone, "")
	if got := rows[0].group; got != groupNeedsYou {
		t.Fatalf("finished agent section = %v, want needs you", got)
	}
	if got := rows[1].group; got != groupAgents {
		t.Fatalf("idle agent section = %v, want agents", got)
	}
}

func TestFilterRowsAppliesScope(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "dev"},
		{
			Kind:       sessionmgr.KindAgent,
			AgentName:  "pi",
			AgentState: sessionmgr.AgentBlocked,
			PaneID:     "%1",
		},
	}
	rows := filterRows(items, scopeAgents, stateFilterNone, "")
	if len(rows) != 1 || rows[0].item.Kind != sessionmgr.KindAgent {
		t.Fatalf("scopeAgents rows = %#v", rows)
	}

	// The state filter narrows the same scope to one state.
	rows = filterRows(items, scopeAgents, stateFilterBlocked, "")
	if len(rows) != 1 || rows[0].item.AgentState != sessionmgr.AgentBlocked {
		t.Fatalf("scopeAgents+blocked rows = %#v", rows)
	}
	if rows := filterRows(items, scopeAll, stateFilterBlocked, ""); len(rows) != 1 {
		t.Fatalf("a state filter drops every session: %#v", rows)
	}
}

func TestFilterRowsFuzzyRanksTitleMatchesAboveMetaMatches(t *testing.T) {
	items := []sessionmgr.Item{
		agentItem("pi", "%1", "api-service"),
		{Kind: sessionmgr.KindSession, Name: "api"},
	}
	rows := filterRows(items, scopeAll, stateFilterNone, "api")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].item.Name != "api" {
		t.Fatalf("top row = %q, want the title match %q", rows[0].item.Name, "api")
	}
	if len(rows[0].matches) == 0 {
		t.Fatal("title match positions not recorded")
	}
	if len(rows[1].matches) != 0 {
		t.Fatalf("location-only match recorded title positions: %v", rows[1].matches)
	}
}

func TestFilterRowsMatchesLocationAndAgentKind(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindAgent, AgentName: "claude", PaneID: "%1", Location: "frontend"},
	}
	if rows := filterRows(items, scopeAll, stateFilterNone, "frontend"); len(rows) != 1 {
		t.Fatalf("location query rows = %d, want 1", len(rows))
	}
	if rows := filterRows(items, scopeAll, stateFilterNone, "claud"); len(rows) != 1 {
		t.Fatalf("agent kind query rows = %d, want 1", len(rows))
	}
	if rows := filterRows(items, scopeAll, stateFilterNone, "zzz"); len(rows) != 0 {
		t.Fatalf("non-matching query rows = %d, want 0", len(rows))
	}
}

func TestDedupeDirsPrefersZoxide(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindZoxide, Path: "~/code", Name: "~/code"},
		{Kind: sessionmgr.KindFD, Path: "~/code", Name: "~/code"},
		{Kind: sessionmgr.KindFD, Path: "~/other", Name: "~/other"},
	}
	out := dedupeDirs(items)
	if len(out) != 2 {
		t.Fatalf("dedupeDirs = %#v, want 2 items", out)
	}
	if out[1].Path != "~/other" {
		t.Fatalf("second item = %#v, want the fd-only path", out[1])
	}
}
