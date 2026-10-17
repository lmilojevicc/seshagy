package tui

import (
	"slices"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestScopeForSigilMapsEveryChip(t *testing.T) {
	tests := []struct {
		sigil rune
		want  scope
	}{
		{'@', scopeAgents},
		{'!', scopeAttention},
		{'$', scopeSessions},
		{'*', scopeDirs},
		{'~', scopeZoxide},
		{'/', scopeFind},
		{'#', scopeHosts},
	}
	for _, tt := range tests {
		got, ok := scopeForSigil(tt.sigil)
		if !ok || got != tt.want {
			t.Fatalf("scopeForSigil(%q) = %v, %v; want %v", tt.sigil, got, ok, tt.want)
		}
	}
	if _, ok := scopeForSigil('x'); ok {
		t.Fatal("scopeForSigil('x') matched a scope")
	}
}

func TestScopeChipsAndCycling(t *testing.T) {
	if got := scopeAll.chip(); got != "all" {
		t.Fatalf("scopeAll.chip() = %q", got)
	}
	if got := scopeAgents.chip(); got != "@ agents" {
		t.Fatalf("scopeAgents.chip() = %q", got)
	}
	if got := scopeAttention.label(); got != "needs you" {
		t.Fatalf("scopeAttention.label() = %q", got)
	}
	// Cycling walks every scope in order and wraps at both ends.
	seen := make(map[scope]bool, scopeCount)
	current := scopeAll
	for i := 0; i < int(scopeCount); i++ {
		if seen[current] {
			t.Fatalf("scope %v visited twice", current)
		}
		seen[current] = true
		current = current.next(1)
	}
	if current != scopeAll {
		t.Fatalf("forward cycle ended on %v, want scopeAll", current)
	}
	if got := scopeAll.next(-1); got != scopeHosts {
		t.Fatalf("scopeAll.next(-1) = %v, want scopeHosts", got)
	}
}

func TestScopeIncludesKinds(t *testing.T) {
	blocked := sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentState: sessionmgr.AgentBlocked}
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
		{"attention keeps blocked", scopeAttention, blocked, true},
		{"attention drops working", scopeAttention, working, false},
		{
			"sessions keeps sessions",
			scopeSessions,
			sessionmgr.Item{Kind: sessionmgr.KindSession},
			true,
		},
		{"dirs keeps zoxide", scopeDirs, sessionmgr.Item{Kind: sessionmgr.KindZoxide}, true},
		{"dirs keeps fd", scopeDirs, sessionmgr.Item{Kind: sessionmgr.KindFD}, true},
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
	rows := filterRows(items, scopeAll, "")
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
	rows := filterRows([]sessionmgr.Item{idle, done}, scopeAll, "")
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
	rows := filterRows(items, scopeAgents, "")
	if len(rows) != 1 || rows[0].item.Kind != sessionmgr.KindAgent {
		t.Fatalf("scopeAgents rows = %#v", rows)
	}
}

func TestFilterRowsFuzzyRanksTitleMatchesAboveMetaMatches(t *testing.T) {
	items := []sessionmgr.Item{
		agentItem("pi", "%1", "api-service"),
		{Kind: sessionmgr.KindSession, Name: "api"},
	}
	rows := filterRows(items, scopeAll, "api")
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
	if rows := filterRows(items, scopeAll, "frontend"); len(rows) != 1 {
		t.Fatalf("location query rows = %d, want 1", len(rows))
	}
	if rows := filterRows(items, scopeAll, "claud"); len(rows) != 1 {
		t.Fatalf("agent kind query rows = %d, want 1", len(rows))
	}
	if rows := filterRows(items, scopeAll, "zzz"); len(rows) != 0 {
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
