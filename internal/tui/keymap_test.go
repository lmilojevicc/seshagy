package tui

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// pickerKeys is the picker's hand-maintained key set, kept beside the table it
// mirrors. Because the list is maintained by hand, it pins only what it names:
// every key listed here must still resolve to a binding, and the table must not
// bind a run-capable key the list omits. It says nothing about the table's
// run-less query-token rows.
var pickerKeys = []string{
	"up", "down", "ctrl+p", "ctrl+k", "ctrl+n", "ctrl+j",
	"pgup", "pgdown", "home", "end",
	"tab", "shift+tab", "enter", "esc", "f1", "backspace", "delete",
	"ctrl+a", "ctrl+s", "ctrl+t", "ctrl+z", "ctrl+f", "ctrl+e", "alt+m",
	"ctrl+r", "ctrl+x", "ctrl+o", "ctrl+y", "ctrl+g", "ctrl+l", "ctrl+u", "ctrl+w",
	"ctrl+c",
}

// remappableKeys is the id of every action a [tui.keys] entry can override, in
// table order.
var remappableKeys = []string{
	"up", "down", "rename", "kill", "preview", "yazi", "machine", "help",
	"agents", "state", "sessions", "zoxide", "find", "remotes",
	"next_filter", "previous_filter",
	"page_up", "page_down", "first", "last",
	"integrations", "refresh", "clear_query", "delete_word",
}

// TestKeyBindingsCoverEveryPickerKey pins the table against the hand-maintained
// pickerKeys list: the table's run-capable keys are exactly the ones listed, and
// every listed key resolves to a binding.
func TestKeyBindingsCoverEveryPickerKey(t *testing.T) {
	table := defaultKeyTable()
	bound := make([]string, 0, len(pickerKeys))
	for _, binding := range table.bindings {
		if binding.run == nil {
			continue
		}
		bound = append(bound, binding.keys...)
	}
	sort.Strings(bound)
	want := append([]string(nil), pickerKeys...)
	sort.Strings(want)
	if !slices.Equal(bound, want) {
		t.Fatalf("table binds %d keys %v, want %d %v", len(bound), bound, len(want), want)
	}
	for _, key := range pickerKeys {
		if _, ok := table.bindingFor(key); !ok {
			t.Fatalf("key %q does not resolve to a binding", key)
		}
	}
	if binding, ok := table.bindingFor("x"); ok {
		t.Fatalf("a printable rune resolved to a binding: %+v", binding)
	}
}

// TestEveryActionHasAStableID pins the table's ids: every row carries one, the
// remappable ids are exactly the actions a [tui.keys] entry can override, and
// esc is the one id two rows share.
func TestEveryActionHasAStableID(t *testing.T) {
	ids := map[string]int{}
	for _, binding := range pickerKeyBindings {
		if binding.id == "" {
			t.Fatalf("binding %v has no id", binding.keys)
		}
		ids[binding.id]++
	}
	for id, rows := range ids {
		if rows > 1 && id != "esc" {
			t.Fatalf("id %q names %d rows, want one", id, rows)
		}
	}
	if ids["esc"] != 2 {
		t.Fatalf("esc names %d rows, want the footer hint and the help row", ids["esc"])
	}

	got := make([]string, 0, len(remappableKeys))
	for _, mapping := range defaultKeyTable().Mappings() {
		got = append(got, mapping.ID)
	}
	if !slices.Equal(got, remappableKeys) {
		t.Fatalf("remappable actions = %v, want %v", got, remappableKeys)
	}
}

// TestKeyBindingsNameEachKeyOncePerList pins the two lists the table feeds: the
// footer never draws the same key twice, and neither does the help overlay.
func TestKeyBindingsNameEachKeyOncePerList(t *testing.T) {
	table := defaultKeyTable()
	for _, list := range []struct {
		name     string
		bindings []keyBinding
	}{
		{"footer", table.footerBindings()},
		{"help", table.helpBindings()},
	} {
		seen := map[string]bool{}
		for _, binding := range list.bindings {
			for _, key := range binding.keys {
				if seen[key] {
					t.Fatalf("%s names %q twice", list.name, key)
				}
				seen[key] = true
			}
		}
	}
}

// TestFooterAndHelpBindingsAreDisjoint pins the footer/help split: every key an
// action answers is drawn once, either by the action's footer hint or by its
// help row. Two keys are named by both lists: esc, which the footer calls the
// quit key while the help spells out what it clears first, and alt+m, whose
// footer hint cannot say which machine is selected (the help row can).
func TestFooterAndHelpBindingsAreDisjoint(t *testing.T) {
	table := defaultKeyTable()
	footer := map[string]bool{}
	for _, binding := range table.footerBindings() {
		for _, key := range binding.keys {
			footer[key] = true
		}
	}
	shared := make([]string, 0, 2)
	for _, binding := range table.helpBindings() {
		for _, key := range binding.keys {
			if footer[key] {
				shared = append(shared, key)
			}
		}
	}
	sort.Strings(shared)
	if want := []string{"alt+m", "esc"}; !slices.Equal(shared, want) {
		t.Fatalf("footer and help share %v, want %v", shared, want)
	}
	// The help row that names esc documents it rather than binding it: the
	// footer's row is the one the dispatch answers.
	if binding, ok := table.bindingFor("esc"); !ok || !binding.footer {
		t.Fatalf("esc dispatches to %+v, want the footer's quit binding", binding)
	}
	// alt+m is deliberately named in both lists, and the help row is the one
	// carrying the selected machine's name.
	row := helpRowFor(t, openHelp(t, newTestModelWith(t, newFakeMachineMux())), "alt+m")
	if row.label != "machine" {
		t.Fatalf("the alt+m help row = %+v, want the machine filter", row)
	}
}

// TestFooterHintsAreTheCoreActions pins the footer's contents and order: the
// core actions only, each drawn with the first key it answers, with the help and
// quit hints last.
func TestFooterHintsAreTheCoreActions(t *testing.T) {
	// A herdr model: the machine hint exists only where saved machines do.
	m := newTestModelWith(t, newFakeMachineMux())
	want := []footerHint{
		{"↑↓", "move"},
		{"⏎", "open"},
		{"^r", "rename"},
		{"^x", "kill"},
		{"^o", "preview"},
		{"^y", "yazi"},
		{"alt+m", "machine"},
		{"F1", "help"},
		{"esc", "quit"},
	}
	got := m.footerHints()
	if len(got) != len(want) {
		t.Fatalf("footer hints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("footer hint %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestBindingLabelsCarryTheBackendSessionWord pins the one label that follows
// the active backend: the sessions filter reads "workspaces" under herdr.
func TestBindingLabelsCarryTheBackendSessionWord(t *testing.T) {
	m := newTestModel(t)
	m.terms = sessionmgr.HerdrTerms()
	for _, binding := range m.keys.helpBindings() {
		if binding.terms {
			if got := m.bindingLabel(binding); got != "workspaces" {
				t.Fatalf("herdr label = %q, want workspaces", got)
			}
		}
	}
	m.terms = sessionmgr.TmuxTerms()
	if got := m.bindingLabel(pickerKeyBindings[0]); got != "move up" {
		t.Fatalf("label = %q, want move up", got)
	}
}

// TestKeyHintDrawsGlyphsAndChords pins how a run of keys draws.
func TestKeyHintDrawsGlyphsAndChords(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"arrow", []string{"up"}, "↑"},
		{"chord pair", []string{"ctrl+p", "ctrl+k"}, "^p/^k"},
		{"enter", []string{"enter"}, "⏎"},
		{"help", []string{"f1"}, "F1"},
		{"plain name", []string{"shift+tab"}, "shift+tab"},
		{"no keys", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyHint(tt.keys); got != tt.want {
				t.Fatalf("keyHint(%v) = %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

// TestEveryBindingHasALabelAndGroup pins the table's shape: every row is filed
// under a group the overlay draws and carries a label, and the rows without a
// run are the query-grammar rows plus the esc row the footer already binds.
func TestEveryBindingHasALabelAndGroup(t *testing.T) {
	groups := map[keyGroup]bool{}
	for _, group := range keyGroups {
		groups[group] = true
	}
	for _, binding := range pickerKeyBindings {
		if !groups[binding.group] {
			t.Fatalf(
				"binding %+v has group %q, which the overlay does not draw",
				binding,
				binding.group,
			)
		}
		if binding.label == "" && len(binding.keys) > 0 {
			t.Fatalf("binding %v has no label", binding.keys)
		}
		if binding.run != nil || binding.group == groupQueryTokens {
			continue
		}
		if binding.id != "esc" {
			t.Fatalf("binding %v has no run and is not a query-token row", binding.keys)
		}
	}
}

// TestDefaultKeyTableRowsAreUnchanged pins the help overlay's default rows — the
// groups in draw order, and each row's keys and label — against the table the
// picker had before the keys were remappable: a footer action contributes the
// keys the footer does not draw, so the overlay reads the same as it always has.
func TestDefaultKeyTableRowsAreUnchanged(t *testing.T) {
	m := newTestModel(t)
	want := []string{
		"Filters|^a|agents",
		"Filters|^s|agent state",
		"Filters|^t|sessions",
		"Filters|^z|zoxide",
		"Filters|^f|find",
		"Filters|^e|remotes",
		"Filters|tab|next filter",
		"Filters|shift+tab|previous filter",
		"Filters|backspace|delete a character; clear filters",
		"Filters|esc|clear query → filters → quit",
		"Query tokens|!name|agent name",
		"Query tokens|@|all agents",
		"Query tokens|@ws|agent workspace or state",
		"Query tokens|#state|agent state",
		"Query tokens|/path|path contains",
		"Query tokens||fix @seshagy #idle",
		"Navigation|^p/^k|move up",
		"Navigation|^n/^j|move down",
		"Navigation|pgup|page up",
		"Navigation|pgdown|page down",
		"Navigation|home|first item",
		"Navigation|end|last item",
		"Actions|delete|kill",
		"Actions|^g|integrations",
		"Actions|^l|refresh",
		"Actions|^u|clear the query",
		"Actions|^w|delete a word",
		"Actions|^c|quit",
	}
	got := make([]string, 0, len(want))
	for _, block := range m.helpBlocks() {
		for _, row := range block.rows {
			got = append(got, strings.Join([]string{string(block.group), row.key, row.label}, "|"))
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("help rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestMachineRowsAppearOnlyWithHerdrMachines pins the one backend-dependent part
// of the frame: the machine filter is drawn by a herdr picker and nowhere else,
// so tmux and noop keep the footer and the help overlay they always had.
func TestMachineRowsAppearOnlyWithHerdrMachines(t *testing.T) {
	shows := func(m Model) (footer, help bool) {
		for _, hint := range m.footerHints() {
			if hint.label == "machine" {
				footer = true
			}
		}
		for _, block := range m.helpBlocks() {
			for _, row := range block.rows {
				if row.label == "machine" {
					help = true
				}
			}
		}
		return footer, help
	}

	tmux := newTestModel(t)
	if footer, help := shows(tmux); footer || help {
		t.Fatalf("a tmux picker draws the machine filter: footer %v help %v", footer, help)
	}
	herdr := newTestModelWith(t, newFakeMachineMux())
	footer, help := shows(herdr)
	if !footer || !help {
		t.Fatalf("a herdr picker hides the machine filter: footer %v help %v", footer, help)
	}
	// The help row names the machine in effect once one is saved.
	withCatalog, _ := machineModel(t)
	withCatalog.machine = "dev-id"
	if row := helpRowFor(t, openHelp(t, withCatalog), "alt+m"); row.value != "@devbox" {
		t.Fatalf("the machine help row = %+v, want the selected machine", row)
	}
}
