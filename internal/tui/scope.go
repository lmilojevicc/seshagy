package tui

import (
	"sort"

	"github.com/sahilm/fuzzy"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// scope is the inline filter the list is narrowed to. Scopes replace the old
// source tabs: a ctrl chord selects one (ctrl+a agents, ctrl+t sessions, ...),
// and tab / shift+tab cycle through them in order.
type scope int

const (
	scopeAll scope = iota
	scopeAgents
	scopeSessions
	scopeHosts
	scopeZoxide
	scopeFind
	scopeCount
)

// scopeOrder is the cycle tab and shift+tab walk: all → agents → sessions →
// hosts → zoxide → find → all. A scope with no source is stepped over.
var scopeOrder = []scope{
	scopeAll,
	scopeAgents,
	scopeSessions,
	scopeHosts,
	scopeZoxide,
	scopeFind,
}

// scopeLabels are the scope names the prompt window title and the empty state
// draw, lowercase. The sessions scope is the exception: its label is the
// backend's own word for a session (see Model.scopeLabel).
var scopeLabels = [scopeCount]string{
	scopeAll:      "all",
	scopeAgents:   "agents",
	scopeSessions: "sessions",
	scopeHosts:    "remotes",
	scopeZoxide:   "zoxide",
	scopeFind:     "find",
}

// nextScope returns the scope delta steps along scopeOrder from sc, stepping
// over the scopes the picker has no source for. It is the tab / shift+tab cycle.
func nextScope(from scope, delta int, available func(scope) bool) scope {
	n := len(scopeOrder)
	at := 0
	for i, sc := range scopeOrder {
		if sc == from {
			at = i
		}
	}
	for step := 1; step <= n; step++ {
		next := scopeOrder[((at+delta*step)%n+n)%n]
		if available(next) {
			return next
		}
	}
	return from
}

// includes reports whether an item belongs to the scope.
func (s scope) includes(item sessionmgr.Item) bool {
	switch s {
	case scopeAgents:
		return item.Kind == sessionmgr.KindAgent
	case scopeSessions:
		return item.Kind == sessionmgr.KindSession
	case scopeZoxide:
		return item.Kind == sessionmgr.KindZoxide
	case scopeFind:
		return item.Kind == sessionmgr.KindFD
	case scopeHosts:
		return item.Kind == sessionmgr.KindHost
	default:
		return true
	}
}

// stateFilter is the picker's second narrowing axis: the agent state ctrl+s
// cycles through. It only ever applies inside the agents scope, so a switch to
// another scope clears it.
type stateFilter int

const (
	stateFilterNone stateFilter = iota
	stateFilterBlocked
	stateFilterDone
	stateFilterWorking
	stateFilterIdle
	stateFilterCount
)

// stateFilterStates maps a filter to the normalized agent state it keeps. The
// none filter names no state: it is the absence of the filter.
var stateFilterStates = [stateFilterCount]sessionmgr.AgentState{
	stateFilterNone:    "",
	stateFilterBlocked: sessionmgr.AgentBlocked,
	stateFilterDone:    sessionmgr.AgentDone,
	stateFilterWorking: sessionmgr.AgentWorking,
	stateFilterIdle:    sessionmgr.AgentIdle,
}

// next is the ctrl+s cycle: blocked → done → working → idle → none → blocked.
func (f stateFilter) next() stateFilter { return (f + 1) % stateFilterCount }

// label names the filter for the prompt window title and the empty state. The
// none filter has no name.
func (f stateFilter) label() string { return string(stateFilterStates[f]) }

// matches reports whether an item survives the filter. The none filter keeps
// every item, so the agents scope can still show its whole list.
func (f stateFilter) matches(item sessionmgr.Item) bool {
	state := stateFilterStates[f]
	if state == "" {
		return true
	}
	return item.Kind == sessionmgr.KindAgent && item.AgentState == state
}

// needsAttention reports agents waiting on the user: blocked on a prompt, or
// finished a turn the user has not looked at yet.
func needsAttention(item sessionmgr.Item) bool {
	return item.Kind == sessionmgr.KindAgent &&
		(item.AgentState == sessionmgr.AgentBlocked || item.AgentState == sessionmgr.AgentDone)
}

// group is the section an empty-query list files a row under. Sections draw in
// this order: what the user owes an answer to first, then the agents, then the
// places to go (sessions, hosts, and the directory sources). A query drops the
// sections for one ranked list.
type group int

const (
	groupNeedsYou group = iota
	groupAgents
	groupSessions
	groupHosts
	groupZoxide
	groupFind
	groupCount
)

// groupLabels are the headers the sections draw, lowercase like the scope
// labels. The sessions section is the exception: its header uses the backend's
// own word for a session (see Model.groupLabel).
var groupLabels = [groupCount]string{
	groupNeedsYou: "needs you",
	groupAgents:   "agents",
	groupSessions: "sessions",
	groupHosts:    "hosts",
	groupZoxide:   "zoxide",
	groupFind:     "find",
}

func (g group) label() string { return groupLabels[g] }

// itemGroup is the section an item files under: the agents the user owes an
// answer to (blocked, or a finished turn they have not looked at) lead in their
// own section, the remaining agents follow, then sessions, hosts, and the
// directory sources.
func itemGroup(item sessionmgr.Item) group {
	switch item.Kind {
	case sessionmgr.KindAgent:
		if needsAttention(item) {
			return groupNeedsYou
		}
		return groupAgents
	case sessionmgr.KindSession:
		return groupSessions
	case sessionmgr.KindHost:
		return groupHosts
	case sessionmgr.KindZoxide:
		return groupZoxide
	default:
		return groupFind
	}
}

// itemRank orders the items inside a section: agents by urgency, then sessions,
// then directories (zoxide by frecency before fd discoveries).
func itemRank(item sessionmgr.Item) int {
	switch item.Kind {
	case sessionmgr.KindAgent:
		switch item.AgentState {
		case sessionmgr.AgentBlocked:
			return 0
		case sessionmgr.AgentDone:
			return 1
		case sessionmgr.AgentWorking:
			return 2
		default:
			return 3
		}
	case sessionmgr.KindSession:
		return 4
	case sessionmgr.KindZoxide:
		return 5
	case sessionmgr.KindFD:
		return 6
	case sessionmgr.KindHost:
		return 7
	default:
		return 8
	}
}

// row is a visible list entry plus the title rune positions the query matched.
type row struct {
	item    sessionmgr.Item
	matches []int
	// group is the section the row files under, which is where an empty-query
	// list draws its headers.
	group group
}

// searchFields are the per-item strings the query is matched against. The
// title comes first so its match positions can be highlighted.
const searchFieldCount = 5

// titleMatchBonus lifts title matches above equally scored matches in a
// path, location, or agent kind.
const titleMatchBonus = 40

func searchFields(item sessionmgr.Item) [searchFieldCount]string {
	return [searchFieldCount]string{
		itemTitle(item),
		item.Path,
		item.Location,
		item.AgentName,
		item.TabLabel,
	}
}

// filterRows applies the scope, the agent-state filter, and the query to items.
// The query is read as herdr-navigator's token grammar (see queryTokens): the
// prefix tokens narrow the list, and the plain text left over is fuzzy-matched.
// Without plain text the rows follow the section order and itemRank inside it,
// so the needs-you section leads whatever the items' own ranks say (stable
// within a rank, so backend order such as zoxide frecency survives); with plain
// text the best fuzzy score wins and itemRank breaks ties.
func filterRows(items []sessionmgr.Item, sc scope, state stateFilter, query string) []row {
	tokens := parseQuery(query)
	eligible := make([]sessionmgr.Item, 0, len(items))
	for _, item := range items {
		if sc.includes(item) && state.matches(item) && tokens.matches(item) {
			eligible = append(eligible, item)
		}
	}
	if tokens.plain == "" {
		sort.SliceStable(eligible, func(i, j int) bool {
			gi, gj := itemGroup(eligible[i]), itemGroup(eligible[j])
			if gi != gj {
				return gi < gj
			}
			return itemRank(eligible[i]) < itemRank(eligible[j])
		})
		rows := make([]row, len(eligible))
		for i, item := range eligible {
			rows[i] = row{item: item, group: itemGroup(item)}
		}
		return rows
	}

	candidates := make([]string, 0, len(eligible)*searchFieldCount)
	for _, item := range eligible {
		fields := searchFields(item)
		candidates = append(candidates, fields[:]...)
	}
	type scored struct {
		row
		score int
	}
	// An item can match on several fields; its best field decides its score
	// (title matches get a bonus so they outrank path/location matches), and
	// title positions are kept whenever the title matched so highlighting
	// works even when another field scored higher.
	slot := make(map[int]int, len(eligible))
	matched := make([]scored, 0, len(eligible))
	for _, match := range fuzzy.FindNoSort(tokens.plain, candidates) {
		index := match.Index / searchFieldCount
		isTitle := match.Index%searchFieldCount == 0
		score := match.Score
		if isTitle {
			score += titleMatchBonus
		}
		at, ok := slot[index]
		if !ok {
			at = len(matched)
			slot[index] = at
			matched = append(matched, scored{
				row:   row{item: eligible[index], group: itemGroup(eligible[index])},
				score: score,
			})
		} else if score > matched[at].score {
			matched[at].score = score
		}
		if isTitle {
			matched[at].matches = match.MatchedIndexes
		}
	}
	// Best score first; rank only breaks exact ties so equally good matches
	// still group sensibly.
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		return itemRank(matched[i].item) < itemRank(matched[j].item)
	})
	rows := make([]row, len(matched))
	for i, entry := range matched {
		rows[i] = entry.row
	}
	return rows
}

// itemTitle is the primary text a row shows and the query highlights.
func itemTitle(item sessionmgr.Item) string {
	switch item.Kind {
	case sessionmgr.KindAgent:
		return item.DisplayName()
	case sessionmgr.KindZoxide, sessionmgr.KindFD:
		return item.Path
	default:
		return item.Name
	}
}

// dedupeDirs drops fd directories already provided by zoxide so the same
// path never appears twice in the merged list.
func dedupeDirs(items []sessionmgr.Item) []sessionmgr.Item {
	zoxide := make(map[string]bool)
	for _, item := range items {
		if item.Kind == sessionmgr.KindZoxide {
			zoxide[item.Path] = true
		}
	}
	if len(zoxide) == 0 {
		return items
	}
	out := items[:0:0]
	for _, item := range items {
		if item.Kind == sessionmgr.KindFD && zoxide[item.Path] {
			continue
		}
		out = append(out, item)
	}
	return out
}
