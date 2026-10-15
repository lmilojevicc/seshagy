package tui

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// scope is the inline filter the list is narrowed to. Scopes replace the old
// source tabs: typing a scope's sigil on an empty query selects it, and tab /
// shift+tab cycle through them in order.
type scope int

const (
	scopeAll scope = iota
	scopeAgents
	scopeAttention
	scopeSessions
	scopeDirs
	scopeZoxide
	scopeFind
	scopeHosts
	scopeCount
)

type scopeInfo struct {
	sigil rune
	label string
}

var scopeInfos = [scopeCount]scopeInfo{
	scopeAll:       {sigil: 0, label: "all"},
	scopeAgents:    {sigil: '@', label: "agents"},
	scopeAttention: {sigil: '!', label: "needs you"},
	scopeSessions:  {sigil: '$', label: "sessions"},
	scopeDirs:      {sigil: '*', label: "dirs"},
	scopeZoxide:    {sigil: '~', label: "zoxide"},
	scopeFind:      {sigil: '/', label: "find"},
	scopeHosts:     {sigil: '#', label: "hosts"},
}

func (s scope) sigil() rune   { return scopeInfos[s].sigil }
func (s scope) label() string { return scopeInfos[s].label }

// chip is the "<sigil> <label>" text used by the filter strip and empty state.
func (s scope) chip() string {
	if s.sigil() == 0 {
		return s.label()
	}
	return string(s.sigil()) + " " + s.label()
}

// scopeForSigil maps a typed rune to the scope it selects.
func scopeForSigil(r rune) (scope, bool) {
	for s := scopeAgents; s < scopeCount; s++ {
		if s.sigil() == r {
			return s, true
		}
	}
	return scopeAll, false
}

func (s scope) next(delta int) scope {
	n := int(scopeCount)
	return scope(((int(s)+delta)%n + n) % n)
}

// includes reports whether an item belongs to the scope.
func (s scope) includes(item sessionmgr.Item) bool {
	switch s {
	case scopeAgents:
		return item.Kind == sessionmgr.KindAgent
	case scopeAttention:
		return needsAttention(item)
	case scopeSessions:
		return item.Kind == sessionmgr.KindSession
	case scopeDirs:
		return item.Kind == sessionmgr.KindZoxide || item.Kind == sessionmgr.KindFD
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

// needsAttention reports agents waiting on the user: blocked on a prompt, or
// finished a turn the user has not looked at yet.
func needsAttention(item sessionmgr.Item) bool {
	return item.Kind == sessionmgr.KindAgent &&
		(item.AgentState == sessionmgr.AgentBlocked || item.AgentState == sessionmgr.AgentDone)
}

// itemRank orders the unfiltered list: agents by urgency, then sessions, then
// directories (zoxide by frecency before fd discoveries).
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

// filterRows applies the scope and fuzzy query to items. Without a query the
// rows follow itemRank (stable within a rank, so backend order such as zoxide
// frecency survives); with a query the best fuzzy score wins and itemRank
// breaks ties.
func filterRows(items []sessionmgr.Item, sc scope, query string) []row {
	eligible := make([]sessionmgr.Item, 0, len(items))
	for _, item := range items {
		if sc.includes(item) {
			eligible = append(eligible, item)
		}
	}
	query = strings.TrimSpace(query)
	if query == "" {
		sort.SliceStable(eligible, func(i, j int) bool {
			return itemRank(eligible[i]) < itemRank(eligible[j])
		})
		rows := make([]row, len(eligible))
		for i, item := range eligible {
			rows[i] = row{item: item}
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
	for _, match := range fuzzy.FindNoSort(query, candidates) {
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
			matched = append(matched, scored{row: row{item: eligible[index]}, score: score})
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
