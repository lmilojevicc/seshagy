package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// keyGroup is the section a binding is listed under in the help overlay.
type keyGroup string

const (
	groupFilters     keyGroup = "Filters"
	groupQueryTokens keyGroup = "Query tokens"
	groupNavigation  keyGroup = "Navigation"
	groupActions     keyGroup = "Actions"
)

// keyGroups is the order the help overlay draws its sections in.
var keyGroups = []keyGroup{groupFilters, groupQueryTokens, groupNavigation, groupActions}

// keyBinding is one action of the picker's key table. The key dispatch, the
// footer and the help overlay all read the table, so a key cannot exist in one
// of them and be missing from another. Every action carries all of its keys: the
// footer draws an action's first key as its hint and the help overlay the keys
// the footer leaves it, and the entries carrying a run are what dispatches.
type keyBinding struct {
	// id is the action's stable name. The ids of the remappable actions are what
	// a [tui.keys] entry overrides (see ResolveKeys); the fixed keys carry an id
	// too, so every row of the table has one. esc is the only id two rows share:
	// the one key the footer's quit hint and the help row that spells out what
	// esc clears both name.
	id string
	// keys are the bubbletea key names the binding answers, in the order the
	// hints draw them. They are dispatch keys, not display text: keyHint turns
	// them into the glyphs the footer and the help overlay draw.
	keys []string
	// label names the binding in the help overlay and beside a footer hint.
	label string
	// group is the help overlay's section for the binding.
	group keyGroup
	// footer marks the entry as a footer hint, which draws the action's first
	// key. The help overlay draws the action's remaining keys.
	footer bool
	// fixed marks a key a [tui.keys] entry cannot remap. The fixed rows are the
	// picker's own keys and the query-token rows, whose keys are the token text
	// the help overlay documents rather than a key that dispatches.
	fixed bool
	// terms marks a label that carries the backend's own word for a session
	// ("sessions" under tmux, "workspaces" under herdr).
	terms bool
	// active reports whether the filter the binding selects is the one in
	// effect, so the help overlay can highlight its row.
	active func(Model) bool
	// value is the extra word the help overlay draws after the label: the ctrl+s
	// row names the agent state it is filtering on.
	value func(Model) string
	// run performs the binding. It is nil for a help row that documents the
	// query grammar or what the esc key clears rather than binding a key, and
	// those rows never dispatch.
	run func(Model) (tea.Model, tea.Cmd)
}

// pickerKeyBindings is the picker's default key table: the footer hints first,
// in the order the footer draws them, the help rows after them in table order
// within each group. ResolveKeys applies the configured [tui.keys] overrides to
// a copy of it.
var pickerKeyBindings = []keyBinding{
	// Footer hints: the core actions, drawn in the frame's bottom line.
	{
		id:     "up",
		keys:   []string{"up", "ctrl+p", "ctrl+k"},
		label:  "move up",
		group:  groupNavigation,
		footer: true,
		run:    Model.moveUp,
	},
	{
		id:     "down",
		keys:   []string{"down", "ctrl+n", "ctrl+j"},
		label:  "move down",
		group:  groupNavigation,
		footer: true,
		run:    Model.moveDown,
	},
	{
		id:     "enter",
		keys:   []string{"enter"},
		label:  "open",
		group:  groupActions,
		footer: true,
		fixed:  true,
		run:    Model.activateSelected,
	},
	{
		id:     "rename",
		keys:   []string{"ctrl+r"},
		label:  "rename",
		group:  groupActions,
		footer: true,
		run:    Model.startRename,
	},
	{
		id:     "kill",
		keys:   []string{"ctrl+x", "delete"},
		label:  "kill",
		group:  groupActions,
		footer: true,
		run:    Model.armKill,
	},
	{
		id:     "preview",
		keys:   []string{"ctrl+o"},
		label:  "preview",
		group:  groupActions,
		footer: true,
		run:    Model.togglePreview,
	},
	{
		id:     "yazi",
		keys:   []string{"ctrl+y"},
		label:  "yazi",
		group:  groupActions,
		footer: true,
		run:    Model.startYazi,
	},
	{
		id:     "help",
		keys:   []string{"f1"},
		label:  "help",
		group:  groupActions,
		footer: true,
		run:    Model.openHelp,
	},
	{
		id:     "esc",
		keys:   []string{"esc"},
		label:  "quit",
		group:  groupActions,
		footer: true,
		fixed:  true,
		run:    Model.escapeKey,
	},

	// Filters: the scope keys and the agent-state cycle, narrowed by the query.
	{
		id:     "agents",
		keys:   []string{"ctrl+a"},
		label:  "agents",
		group:  groupFilters,
		active: activeScope(scopeAgents),
		run:    Model.toggleAgentsKey,
	},
	{
		id:     "state",
		keys:   []string{"ctrl+s"},
		label:  "agent state",
		group:  groupFilters,
		active: func(m Model) bool { return m.stateFilter != stateFilterNone },
		value:  stateFilterValue,
		run:    Model.cycleStateFilterKey,
	},
	{
		id:     "sessions",
		keys:   []string{"ctrl+t"},
		label:  "sessions",
		group:  groupFilters,
		terms:  true,
		active: activeScope(scopeSessions),
		run:    Model.sessionsScopeKey,
	},
	{
		id:     "zoxide",
		keys:   []string{"ctrl+z"},
		label:  "zoxide",
		group:  groupFilters,
		active: activeScope(scopeZoxide),
		run:    Model.zoxideScopeKey,
	},
	{
		id:     "find",
		keys:   []string{"ctrl+f"},
		label:  "find",
		group:  groupFilters,
		active: activeScope(scopeFind),
		run:    Model.findScopeKey,
	},
	{
		id:     "remotes",
		keys:   []string{"ctrl+e"},
		label:  "remotes",
		group:  groupFilters,
		active: activeScope(scopeHosts),
		run:    Model.hostsScopeKey,
	},
	{
		id:    "next_filter",
		keys:  []string{"tab"},
		label: "next filter",
		group: groupFilters,
		run:   Model.nextScopeKey,
	},
	{
		id:    "previous_filter",
		keys:  []string{"shift+tab"},
		label: "previous filter",
		group: groupFilters,
		run:   Model.previousScopeKey,
	},
	{
		id:    "backspace",
		keys:  []string{"backspace"},
		label: "delete a character; clear filters",
		group: groupFilters,
		fixed: true,
		run:   Model.backspaceKey,
	},
	// The esc row documents what esc clears before it quits; the footer's esc
	// entry above is the one that binds the key.
	{
		id:    "esc",
		keys:  []string{"esc"},
		label: "clear query → filters → quit",
		group: groupFilters,
		fixed: true,
	},

	// Query tokens: the prompt's prefix grammar, ANDed with the filters above.
	// The token text is what these rows draw, so the config cannot remap them.
	{
		id:    "!name",
		keys:  []string{"!name"},
		label: "agent name",
		group: groupQueryTokens,
		fixed: true,
	},
	{id: "@", keys: []string{"@"}, label: "all agents", group: groupQueryTokens, fixed: true},
	{
		id:    "@ws",
		keys:  []string{"@ws"},
		label: "agent workspace or state",
		group: groupQueryTokens,
		fixed: true,
	},
	{
		id:    "#state",
		keys:  []string{"#state"},
		label: "agent state",
		group: groupQueryTokens,
		fixed: true,
	},
	{
		id:    "/path",
		keys:  []string{"/path"},
		label: "path contains",
		group: groupQueryTokens,
		fixed: true,
	},
	{id: "example", label: "fix @seshagy #idle", group: groupQueryTokens},

	// Navigation: the movement chords the footer's arrow hints do not name.
	{
		id:    "page_up",
		keys:  []string{"pgup"},
		label: "page up",
		group: groupNavigation,
		run:   Model.pageUp,
	},
	{
		id:    "page_down",
		keys:  []string{"pgdown"},
		label: "page down",
		group: groupNavigation,
		run:   Model.pageDown,
	},
	{
		id:    "first",
		keys:  []string{"home"},
		label: "first item",
		group: groupNavigation,
		run:   Model.firstRow,
	},
	{
		id:    "last",
		keys:  []string{"end"},
		label: "last item",
		group: groupNavigation,
		run:   Model.lastRow,
	},

	// Actions: everything the footer's hints do not name.
	{
		id:    "integrations",
		keys:  []string{"ctrl+g"},
		label: "integrations",
		group: groupActions,
		run:   Model.openIntegrations,
	},
	{
		id:    "refresh",
		keys:  []string{"ctrl+l"},
		label: "refresh",
		group: groupActions,
		run:   Model.refreshAll,
	},
	{
		id:    "clear_query",
		keys:  []string{"ctrl+u"},
		label: "clear the query",
		group: groupActions,
		run:   Model.clearQuery,
	},
	{
		id:    "delete_word",
		keys:  []string{"ctrl+w"},
		label: "delete a word",
		group: groupActions,
		run:   Model.deleteQueryWord,
	},
	{
		id:    "quit",
		keys:  []string{"ctrl+c"},
		label: "quit",
		group: groupActions,
		fixed: true,
		run:   Model.quitKey,
	},
}

// bindingLabel names a binding: the backend's own word for a session where the
// binding carries one (the sessions filter), its own label otherwise.
func (m Model) bindingLabel(binding keyBinding) string {
	if binding.terms {
		return m.terms.SessionPlural
	}
	return binding.label
}

// bindingValue is the word the help overlay draws after the binding's label.
func bindingValue(m Model, binding keyBinding) string {
	if binding.value == nil {
		return ""
	}
	return binding.value(m)
}

// bindingActive reports whether the help overlay highlights the binding's row.
func bindingActive(m Model, binding keyBinding) bool {
	if binding.active == nil {
		return false
	}
	return binding.active(m)
}

// activeScope is the filter highlight of a scope key.
func activeScope(sc scope) func(Model) bool {
	return func(m Model) bool { return m.scope == sc }
}

// stateFilterValue names the agent state ctrl+s is filtering on, "none" while
// the filter is off.
func stateFilterValue(m Model) string {
	if label := m.stateFilter.label(); label != "" {
		return label
	}
	return "none"
}

// keyGlyphs draw the keys that read better as a symbol than as their name.
var keyGlyphs = map[string]string{
	"up":    "↑",
	"down":  "↓",
	"enter": "⏎",
	"f1":    "F1",
}

// keyGlyph draws one key: the symbol where it has one, the short ^x form for a
// ctrl chord, the key's own name otherwise.
func keyGlyph(key string) string {
	if glyph, ok := keyGlyphs[key]; ok {
		return glyph
	}
	if name, ok := strings.CutPrefix(key, "ctrl+"); ok {
		return "^" + name
	}
	return key
}

// keyHint draws a run of keys as one hint: a chord pair as ^n/^j, a lone key as
// itself. It is empty for a row with no keys.
func keyHint(keys []string) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = keyGlyph(key)
	}
	return strings.Join(parts, "/")
}
