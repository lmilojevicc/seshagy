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

// keyBinding is one row of the picker's key table. The key dispatch, the footer
// and the help overlay all read it, so a key cannot exist in one of them and be
// missing from another. Every entry is either a footer hint or a help row, never
// both, and the entries carrying a run are the picker's dispatchable keys.
type keyBinding struct {
	// keys are the bubbletea key names the binding answers, in the order the
	// hints draw them. They are dispatch keys, not display text: keyHint turns
	// them into the glyphs the footer and the help overlay draw.
	keys []string
	// label names the binding in the help overlay.
	label string
	// group is the help overlay's section for the binding.
	group keyGroup
	// footer marks the entry as a footer hint, which the help overlay skips.
	footer bool
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
	// query grammar rather than a key, and those rows never dispatch.
	run func(Model, string) (tea.Model, tea.Cmd)
}

// pickerKeyBindings is the picker's single key table. The footer hints come
// first, in the order the footer draws them; the help rows follow, in table
// order within each group.
var pickerKeyBindings = []keyBinding{
	// Footer hints: the core actions, drawn in the frame's bottom line.
	{
		keys:   []string{"up", "down"},
		label:  "move",
		group:  groupNavigation,
		footer: true,
		run: func(m Model, key string) (tea.Model, tea.Cmd) {
			if key == "up" {
				return m.moveUp()
			}
			return m.moveDown()
		},
	},
	{
		keys:   []string{"enter"},
		label:  "open",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.activateSelected),
	},
	{
		keys:   []string{"ctrl+r"},
		label:  "rename",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.startRename),
	},
	{
		keys:   []string{"ctrl+x"},
		label:  "kill",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.armKill),
	},
	{
		keys:   []string{"ctrl+o"},
		label:  "preview",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.togglePreview),
	},
	{
		keys:   []string{"ctrl+y"},
		label:  "yazi",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.startYazi),
	},
	{
		keys:   []string{"f1"},
		label:  "help",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.openHelp),
	},
	{
		keys:   []string{"esc"},
		label:  "quit",
		group:  groupActions,
		footer: true,
		run:    keyRun(Model.escapeKey),
	},

	// Filters: the scope keys and the agent-state cycle, narrowed by the query.
	{
		keys:   []string{"ctrl+a"},
		label:  "agents",
		group:  groupFilters,
		active: activeScope(scopeAgents),
		run:    keyRun(Model.toggleAgentsKey),
	},
	{
		keys:   []string{"ctrl+s"},
		label:  "agent state",
		group:  groupFilters,
		active: func(m Model) bool { return m.stateFilter != stateFilterNone },
		value:  stateFilterValue,
		run:    keyRun(Model.cycleStateFilterKey),
	},
	{
		keys:   []string{"ctrl+t"},
		label:  "sessions",
		group:  groupFilters,
		terms:  true,
		active: activeScope(scopeSessions),
		run:    keyRun(Model.sessionsScopeKey),
	},
	{
		keys:   []string{"ctrl+z"},
		label:  "zoxide",
		group:  groupFilters,
		active: activeScope(scopeZoxide),
		run:    keyRun(Model.zoxideScopeKey),
	},
	{
		keys:   []string{"ctrl+f"},
		label:  "find",
		group:  groupFilters,
		active: activeScope(scopeFind),
		run:    keyRun(Model.findScopeKey),
	},
	{
		keys:   []string{"ctrl+e"},
		label:  "remotes",
		group:  groupFilters,
		active: activeScope(scopeHosts),
		run:    keyRun(Model.hostsScopeKey),
	},
	{
		keys:  []string{"tab"},
		label: "next filter",
		group: groupFilters,
		run:   keyRun(Model.nextScopeKey),
	},
	{
		keys:  []string{"shift+tab"},
		label: "previous filter",
		group: groupFilters,
		run:   keyRun(Model.previousScopeKey),
	},
	{
		keys:  []string{"backspace"},
		label: "delete a character; clear filters",
		group: groupFilters,
		run:   keyRun(Model.backspaceKey),
	},
	{
		keys:  []string{"esc"},
		label: "clear query → filters → quit",
		group: groupFilters,
	},

	// Query tokens: the prompt's prefix grammar, ANDed with the filters above.
	{keys: []string{"!name"}, label: "agent name", group: groupQueryTokens},
	{keys: []string{"@"}, label: "all agents", group: groupQueryTokens},
	{keys: []string{"@ws"}, label: "agent workspace or state", group: groupQueryTokens},
	{keys: []string{"#state"}, label: "agent state", group: groupQueryTokens},
	{keys: []string{"/path"}, label: "path contains", group: groupQueryTokens},
	{label: "fix @seshagy #idle", group: groupQueryTokens},

	// Navigation: the movement chords the footer's arrow hint does not name.
	{
		keys:  []string{"ctrl+p", "ctrl+k"},
		label: "move up",
		group: groupNavigation,
		run:   keyRun(Model.moveUp),
	},
	{
		keys:  []string{"ctrl+n", "ctrl+j"},
		label: "move down",
		group: groupNavigation,
		run:   keyRun(Model.moveDown),
	},
	{keys: []string{"pgup"}, label: "page up", group: groupNavigation, run: keyRun(Model.pageUp)},
	{
		keys:  []string{"pgdown"},
		label: "page down",
		group: groupNavigation,
		run:   keyRun(Model.pageDown),
	},
	{
		keys:  []string{"home"},
		label: "first item",
		group: groupNavigation,
		run:   keyRun(Model.firstRow),
	},
	{keys: []string{"end"}, label: "last item", group: groupNavigation, run: keyRun(Model.lastRow)},

	// Actions: everything the footer's hints do not name.
	{
		keys:  []string{"delete"},
		label: "kill",
		group: groupActions,
		run:   keyRun(Model.armKill),
	},
	{
		keys:  []string{"ctrl+g"},
		label: "integrations",
		group: groupActions,
		run:   keyRun(Model.openIntegrations),
	},
	{
		keys:  []string{"ctrl+l"},
		label: "refresh",
		group: groupActions,
		run:   keyRun(Model.refreshAll),
	},
	{
		keys:  []string{"ctrl+u"},
		label: "clear the query",
		group: groupActions,
		run:   keyRun(Model.clearQuery),
	},
	{
		keys:  []string{"ctrl+w"},
		label: "delete a word",
		group: groupActions,
		run:   keyRun(Model.deleteQueryWord),
	},
	{keys: []string{"ctrl+c"}, label: "quit", group: groupActions, run: keyRun(Model.quitKey)},
}

// keyRun adapts a handler that answers the same way whatever key triggered it to
// the table's handler signature.
func keyRun(run func(Model) (tea.Model, tea.Cmd)) func(Model, string) (tea.Model, tea.Cmd) {
	return func(m Model, _ string) (tea.Model, tea.Cmd) { return run(m) }
}

// keyBindingFor returns the binding a key triggers, or nil when no binding
// answers it — a printable rune extends the query instead. Help rows that
// document the query grammar carry no run and never dispatch.
func keyBindingFor(key string) *keyBinding {
	for i := range pickerKeyBindings {
		binding := &pickerKeyBindings[i]
		if binding.run == nil {
			continue
		}
		for _, candidate := range binding.keys {
			if candidate == key {
				return binding
			}
		}
	}
	return nil
}

// footerBindings are the table's footer hints, in draw order.
func footerBindings() []keyBinding {
	hints := make([]keyBinding, 0, len(pickerKeyBindings))
	for _, binding := range pickerKeyBindings {
		if binding.footer {
			hints = append(hints, binding)
		}
	}
	return hints
}

// helpBindings are the table's help rows, in table order: every binding the
// footer does not draw, plus the query-token rows.
func helpBindings() []keyBinding {
	rows := make([]keyBinding, 0, len(pickerKeyBindings))
	for _, binding := range pickerKeyBindings {
		if !binding.footer {
			rows = append(rows, binding)
		}
	}
	return rows
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

// keyHint draws a binding's keys as one hint: the arrow pair as the single ↑↓
// run it reads as, a chord pair as ^n/^j, a lone key as itself. It is empty for
// a row with no keys.
func keyHint(keys []string) string {
	if len(keys) == 2 && keys[0] == "up" && keys[1] == "down" {
		return "↑↓"
	}
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = keyGlyph(key)
	}
	return strings.Join(parts, "/")
}
