package tui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestParseQueryReadsPrefixTokens(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  queryTokens
	}{
		{"empty query", "", queryTokens{}},
		{"plain text is lowercased", "Api SVC", queryTokens{plain: "api svc"}},
		{"whitespace collapses", "  Api   svc  ", queryTokens{plain: "api svc"}},
		{"agent token", "!claude", queryTokens{agent: []string{"claude"}}},
		{"bare at asks for every agent", "@", queryTokens{allAgents: true}},
		{
			"workspace or state token",
			"@frontend",
			queryTokens{workspaceState: []string{"frontend"}},
		},
		{"state token", "#blocked", queryTokens{state: []string{"blocked"}}},
		{"path token", "/tui", queryTokens{path: []string{"tui"}}},
		{
			"tokens and plain text side by side",
			"fix @seshagy #idle",
			queryTokens{plain: "fix", workspaceState: []string{"seshagy"}, state: []string{"idle"}},
		},
		{
			"several tokens of one kind",
			"!claude !pi /tui /internal",
			queryTokens{agent: []string{"claude", "pi"}, path: []string{"tui", "internal"}},
		},
		{"bare prefix is not plain text", "!", queryTokens{}},
		{"bare state prefix is not plain text", "#", queryTokens{}},
		{"bare path prefix is not plain text", "/", queryTokens{}},
		{
			"a bare prefix beside text works the same",
			"! fix",
			queryTokens{plain: "fix"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseQuery(tt.query); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseQuery(%q) = %#v, want %#v", tt.query, got, tt.want)
			}
		})
	}
}

// tokenItems is the fixture the token matcher and filterRows tables share: two
// claude/pi agents in one workspace, a blocked codex in another, a matching
// session, and a matching zoxide directory.
func tokenItems() []sessionmgr.Item {
	return []sessionmgr.Item{
		{
			Kind:             sessionmgr.KindAgent,
			AgentName:        "claude",
			AgentDisplayName: "fix api",
			PaneID:           "%1",
			Location:         "seshagy",
			Path:             "/home/milo/tui-hybrid",
			AgentState:       sessionmgr.AgentIdle,
		},
		{
			Kind:             sessionmgr.KindAgent,
			AgentName:        "pi",
			AgentDisplayName: "fix skills",
			PaneID:           "%2",
			Location:         "seshagy",
			Path:             "/home/milo/skills",
			AgentState:       sessionmgr.AgentWorking,
		},
		{
			Kind:             sessionmgr.KindAgent,
			AgentName:        "codex",
			AgentDisplayName: "ship it",
			PaneID:           "%3",
			Location:         "work",
			Path:             "/home/milo/work",
			AgentState:       sessionmgr.AgentBlocked,
		},
		{Kind: sessionmgr.KindSession, Name: "tui-hybrid", Path: "/home/milo/tui-hybrid"},
		{
			Kind: sessionmgr.KindZoxide,
			Name: "/home/milo/tui-hybrid",
			Path: "/home/milo/tui-hybrid",
		},
	}
}

func TestQueryTokensMatchItems(t *testing.T) {
	items := tokenItems()
	claude, pi, codex, session, dir := items[0], items[1], items[2], items[3], items[4]
	host := sessionmgr.Item{Kind: sessionmgr.KindHost, Name: "box", Target: "box"}
	tests := []struct {
		name  string
		query string
		item  sessionmgr.Item
		want  bool
	}{
		{"agent token matches the shown name", "!fix api", claude, true},
		{"agent token matches a renamed agent", "!fix", claude, true},
		{"agent token matches the agent kind", "!cla", claude, true},
		{"agent token misses another kind", "!codex", claude, false},
		{"agent token drops non-agents", "!claude", session, false},
		{"agent token drops hosts", "!claude", host, false},
		{"every agent token must match", "!claude !codex", claude, false},
		{"bare at keeps any agent", "@", codex, true},
		{"bare at drops sessions", "@", session, false},
		{"workspace token matches the workspace label", "@seshagy", claude, true},
		{"workspace token misses another workspace", "@work", claude, false},
		{"workspace token also matches the state", "@idle", claude, true},
		{"workspace token drops non-agents", "@seshagy", session, false},
		{"state token matches the normalized state", "#idle", claude, true},
		{"state token matches a substring", "#ork", pi, true},
		{"state token misses another state", "#idle", codex, false},
		{"state token drops non-agents", "#idle", dir, false},
		{"path token matches a path substring", "/tui", claude, true},
		{"path token misses another path", "/tui", codex, false},
		{"path token never matches an empty path", "/tui", host, false},
		{"path token keeps non-agents", "/tui", session, true},
		{"tokens are ANDed", "@seshagy #idle", claude, true},
		{"tokens are ANDed across axes", "@seshagy #idle", pi, false},
		{"tokens are ANDed with paths", "@seshagy /tui", claude, true},
		{"tokens and plain text still match", "fix @seshagy", claude, true},
		{"bare agent prefix is ignored", "!", session, true},
		{"bare state prefix is ignored", "#", host, true},
		{"bare path prefix is ignored", "/", host, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := parseQuery(tt.query)
			if got := tokens.matches(tt.item); got != tt.want {
				t.Fatalf("parseQuery(%q).matches(%q) = %v, want %v",
					tt.query, itemTitle(tt.item), got, tt.want)
			}
		})
	}
}

func TestFilterRowsNarrowsOnQueryTokens(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"agent tokens and plain text", "fix @seshagy #idle", []string{"fix api"}},
		{
			"bare at lists every agent",
			"@",
			[]string{"ship it", "fix skills", "fix api"},
		},
		{"agent kind token", "!claude", []string{"fix api"}},
		{"workspace token", "@seshagy", []string{"fix skills", "fix api"}},
		{"state token", "#idle", []string{"fix api"}},
		{"plain text still fuzzy-matches", "fix", []string{"fix api", "fix skills"}},
		{
			"path token keeps every item with that path",
			"/tui",
			[]string{"fix api", "tui-hybrid", "/home/milo/tui-hybrid"},
		},
		{
			"plain text matches paths and titles the same way",
			"tui-hybrid",
			[]string{"fix api", "tui-hybrid", "/home/milo/tui-hybrid"},
		},
		{
			"bare prefix narrows nothing",
			"!",
			[]string{"ship it", "fix skills", "fix api", "tui-hybrid", "/home/milo/tui-hybrid"},
		},
		{"tokens of two axes with no overlap", "#blocked @seshagy", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := filterRows(tokenItems(), scopeAll, stateFilterNone, tt.query)
			got := make([]string, 0, len(rows))
			for _, r := range rows {
				got = append(got, itemTitle(r.item))
			}
			want := slices.Clone(tt.want)
			if len(got) != len(want) {
				t.Fatalf("filterRows(%q) = %v, want %v", tt.query, got, want)
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("filterRows(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

// TestFilterRowsTokenQueryRanksLikeAnEmptyQuery pins the order of a token-only
// query: no plain text means no fuzzy scores, so the flat list follows the
// section order and itemRank the empty prompt uses (needs-you first).
func TestFilterRowsTokenQueryRanksLikeAnEmptyQuery(t *testing.T) {
	rows := filterRows(tokenItems(), scopeAll, stateFilterNone, "@")
	want := []string{"ship it", "fix skills", "fix api"}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, title := range want {
		if got := itemTitle(rows[i].item); got != title {
			t.Fatalf("row %d = %q, want %q", i, got, title)
		}
		if len(rows[i].matches) != 0 {
			t.Fatalf("row %d highlighted a token: %v", i, rows[i].matches)
		}
	}
}

func TestFilterRowsTokensCombineWithScopeAndStateFilter(t *testing.T) {
	tests := []struct {
		name          string
		sc            scope
		state         stateFilter
		query         string
		wantMatched   int
		wantHighlight bool
	}{
		{
			"an agent token cannot escape a session scope",
			scopeSessions,
			stateFilterNone,
			"@",
			0,
			false,
		},
		{
			"the state filter still applies beside a token",
			scopeAll,
			stateFilterBlocked,
			"#blocked",
			1,
			false,
		},
		{
			"the state filter and a token can disagree",
			scopeAll,
			stateFilterWorking,
			"#blocked",
			0,
			false,
		},
		{
			"a plain token narrows inside the scope",
			scopeAgents,
			stateFilterNone,
			"fix",
			2,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := filterRows(tokenItems(), tt.sc, tt.state, tt.query)
			if len(rows) != tt.wantMatched {
				t.Fatalf("filterRows(%q) = %d rows, want %d", tt.query, len(rows), tt.wantMatched)
			}
			if tt.wantMatched == 0 {
				return
			}
			highlighted := rows[0].matches
			if (len(highlighted) > 0) != tt.wantHighlight {
				t.Fatalf("highlight = %v, want highlight %v", highlighted, tt.wantHighlight)
			}
		})
	}
}

// TestFilterRowsHighlightingComesFromThePlainText guards the split: the fuzzy
// pattern is the plain tokens only, so a query that is mostly tokens still
// highlights the title, and the token text itself is never fuzzed.
func TestFilterRowsHighlightingComesFromThePlainText(t *testing.T) {
	rows := filterRows(tokenItems(), scopeAll, stateFilterNone, "#idle !claude fix")
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the single idle claude", len(rows))
	}
	if title := itemTitle(rows[0].item); title != "fix api" {
		t.Fatalf("matched title = %q, want the claude pane", title)
	}
	if !slices.Equal(rows[0].matches, []int{0, 1, 2}) {
		t.Fatalf("highlight positions = %v, want the plain token's title match", rows[0].matches)
	}
}
