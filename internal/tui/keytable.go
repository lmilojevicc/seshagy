package tui

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
)

// KeyMapping is one remappable action of the picker's key table: the id a
// [tui.keys] entry names, the keys the action answers, and the label the footer
// and the help overlay draw. `seshagy config keys` prints them.
type KeyMapping struct {
	ID    string   `json:"id"`
	Keys  []string `json:"keys"`
	Label string   `json:"label"`
}

// KeyTable is the picker's effective key table: the default bindings with the
// configured [tui.keys] overrides applied and the keys the overrides and the
// defaults take from one another settled. The picker builds it once, in New,
// and the key dispatch, the footer and the help overlay all read it, so a
// remapped action cannot reach one of them and not the others.
type KeyTable struct {
	bindings []keyBinding
	issues   []string
}

// ResolveKeys builds the picker's key table from the configured [tui.keys]
// overrides. An override replaces the action's default keys, an empty list
// unbinds it, and a key an override takes from another action moves to it. Every
// problem with the overrides — an unknown or fixed id, a value that is neither a
// key name nor a list of them, a key the picker cannot bind, a key two actions
// claim — is reported in KeyTable.Issues.
func ResolveKeys(overrides map[string]appconfig.KeyList) KeyTable {
	table := KeyTable{bindings: append([]keyBinding(nil), pickerKeyBindings...)}
	// The copies share their key slices with the defaults, so every edit below
	// rebuilds the slice it touches.
	overridden := make(map[string]bool, len(overrides))
	for _, id := range sortedKeyIDs(overrides) {
		list := overrides[id]
		binding := table.action(id)
		switch {
		case binding == nil:
			table.issues = append(table.issues, fmt.Sprintf(
				"tui.keys.%s: unknown action; ignored", id,
			))
			continue
		case binding.fixed || binding.run == nil:
			table.issues = append(table.issues, fmt.Sprintf(
				"tui.keys.%s: %s is not remappable; ignored", id, id,
			))
			continue
		case list == nil:
			table.issues = append(table.issues, fmt.Sprintf(
				"tui.keys.%s: want a key name or a list of keys; ignored", id,
			))
			continue
		}
		keys := table.bindKeys(id, list)
		if len(keys) == 0 && len(list) > 0 {
			// Every key was rejected: a typo must not unbind the action, so it
			// keeps its defaults. Only an explicit empty list unbinds.
			table.issues = append(table.issues, fmt.Sprintf(
				"tui.keys.%s: no usable key; keeping the default", id,
			))
			continue
		}
		binding.keys = keys
		overridden[id] = true
	}
	table.resolveKeyConflicts(overridden)
	return table
}

// Issues lists the problems the configured overrides had, in the order the table
// resolved them: the entries it ignored first, in id order, then the keys one
// action took from another.
func (t KeyTable) Issues() []string {
	return append([]string(nil), t.issues...)
}

// Mappings lists the table's remappable actions in table order, with the keys
// each answers once the overrides are applied. The fixed keys and the
// query-token rows are not in it: the config cannot remap them.
func (t KeyTable) Mappings() []KeyMapping {
	mappings := make([]KeyMapping, 0, len(t.bindings))
	for _, binding := range t.bindings {
		if binding.fixed || binding.run == nil {
			continue
		}
		keys := make([]string, len(binding.keys))
		copy(keys, binding.keys)
		mappings = append(mappings, KeyMapping{
			ID:    binding.id,
			Keys:  keys,
			Label: binding.label,
		})
	}
	return mappings
}

// bindingFor returns the binding a key triggers; ok is false when no binding
// answers it, and a printable rune then extends the query instead. The table
// binds every key once, so the first match is the only one.
func (t KeyTable) bindingFor(key string) (keyBinding, bool) {
	for _, binding := range t.bindings {
		if binding.run == nil {
			continue
		}
		for _, candidate := range binding.keys {
			if candidate == key {
				return binding, true
			}
		}
	}
	return keyBinding{}, false
}

// footerBindings are the footer's hints in draw order: every action that draws
// one, carrying the single key the hint shows — the action's first. An action
// the config unbound draws no hint.
func (t KeyTable) footerBindings() []keyBinding {
	hints := make([]keyBinding, 0, len(t.bindings))
	for _, binding := range t.bindings {
		if !binding.footer || len(binding.keys) == 0 {
			continue
		}
		hint := binding
		hint.keys = binding.keys[:1]
		hints = append(hints, hint)
	}
	return hints
}

// helpBindings are the help overlay's rows in table order: for an action the
// footer draws, the keys the footer leaves it — and no row at all when the
// footer's one key is all it answers — and for the rest, every key the action
// answers. An action the config unbound draws no row. The query-token rows carry
// the token they document rather than a key, so they always draw, as does the
// esc row that spells out what the footer's quit hint only names.
func (t KeyTable) helpBindings() []keyBinding {
	rows := make([]keyBinding, 0, len(t.bindings))
	for _, binding := range t.bindings {
		row := binding
		row.footer = false
		if binding.footer {
			if len(binding.keys) < 2 {
				continue
			}
			row.keys = binding.keys[1:]
		} else if binding.run != nil && len(binding.keys) == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// action returns the binding a [tui.keys] entry names, or nil when the table has
// no action with that id. The fixed keys carry ids too, so the caller can tell
// an unknown action from one the config cannot remap.
func (t KeyTable) action(id string) *keyBinding {
	for i := range t.bindings {
		if t.bindings[i].id == id {
			return &t.bindings[i]
		}
	}
	return nil
}

// bindKeys normalises and validates one override's key names, reporting every
// key it drops. A name the picker can dispatch is kept in the order the config
// listed it; a repetition of a name already kept is dropped, so the action never
// answers one key twice.
func (t *KeyTable) bindKeys(id string, list appconfig.KeyList) []string {
	keys := make([]string, 0, len(list))
	for _, name := range list {
		key, problem := resolveKeyName(name)
		if problem != "" {
			t.issues = append(t.issues, fmt.Sprintf("tui.keys.%s: %s", id, problem))
			continue
		}
		if slices.Contains(keys, key) {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// resolveKeyName turns one configured key name into the bubbletea key name the
// table dispatches, or reports why the picker cannot bind it.
func resolveKeyName(name string) (string, string) {
	key := normalizeKeyName(name)
	switch {
	case reservedKeys[key] != "":
		return "", fmt.Sprintf("%q is reserved for %s", key, reservedKeys[key])
	case isBindingKeyName(key):
		return key, ""
	case isPrintableKeyName(key):
		return "", fmt.Sprintf("%q would type into the query", key)
	default:
		return "", fmt.Sprintf("%q is not a key name", key)
	}
}

// normalizeKeyName maps the spellings a config may use onto the bubbletea key
// name the table dispatches on. Case is ignored, and the three ways of spelling
// a chord — ctrl+a, C-a, ^a — all normalise to ctrl+a.
func normalizeKeyName(name string) string {
	key := strings.ToLower(name)
	for _, prefix := range []string{"ctrl-", "c-", "^"} {
		if rest, ok := strings.CutPrefix(key, prefix); ok {
			return "ctrl+" + rest
		}
	}
	return key
}

// reservedKeys are the keys a [tui.keys] entry may not claim, and the key each
// one stands for: the picker's own fixed keys, plus the terminal aliases that
// send the byte of one of them.
var reservedKeys = map[string]string{
	"enter":     "enter",
	"esc":       "esc",
	"backspace": "backspace",
	"ctrl+c":    "ctrl+c",
	"ctrl+m":    "enter",
	"ctrl+i":    "tab",
	"ctrl+[":    "esc",
	"ctrl+?":    "backspace",
}

// namedKeys are the key names bubbletea gives a key that is not a rune or a
// chord; the function keys are checked separately.
var namedKeys = map[string]bool{
	"up": true, "down": true, "left": true, "right": true,
	"pgup": true, "pgdown": true, "home": true, "end": true,
	"tab": true, "shift+tab": true, "insert": true, "delete": true,
	"ctrl+pgup": true, "ctrl+pgdown": true,
	"ctrl+up": true, "ctrl+down": true, "ctrl+left": true, "ctrl+right": true,
	"ctrl+home": true, "ctrl+end": true,
	"shift+up": true, "shift+down": true, "shift+left": true, "shift+right": true,
	"shift+home": true, "shift+end": true,
	"ctrl+shift+up": true, "ctrl+shift+down": true,
	"ctrl+shift+left": true, "ctrl+shift+right": true,
	"ctrl+shift+home": true, "ctrl+shift+end": true,
}

// ctrlChordRunes are the characters bubbletea names a ctrl chord after. The
// rest of the control bytes reach the picker as one of the fixed keys or as
// ctrl+a to ctrl+z.
const ctrlChordRunes = "abcdefghijklmnopqrstuvwxyz@\\]^_"

// isBindingKeyName reports whether a normalised name is a key the picker can
// dispatch: a name bubbletea gives a key, or a chord it names.
func isBindingKeyName(key string) bool {
	if namedKeys[key] {
		return true
	}
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok {
		return len(rest) == 1 && strings.ContainsRune(ctrlChordRunes, rune(rest[0]))
	}
	if rest, ok := strings.CutPrefix(key, "alt+"); ok {
		return isPrintableKeyName(rest)
	}
	return isFunctionKey(key)
}

// isFunctionKey reports whether a name is one of the function keys bubbletea
// names: f1 to f20, spelled with no leading zero.
func isFunctionKey(name string) bool {
	digits, ok := strings.CutPrefix(name, "f")
	if !ok {
		return false
	}
	number, err := strconv.Atoi(digits)
	return err == nil && number >= 1 && number <= 20 && strconv.Itoa(number) == digits
}

// isPrintableKeyName reports whether a name is one printable rune: the keys that
// type into the query rather than dispatch.
func isPrintableKeyName(name string) bool {
	runes := []rune(name)
	return len(runes) == 1 && unicode.IsPrint(runes[0])
}

// resolveKeyConflicts settles the keys more than one action answers, leaving
// every key bound once. An override beats a default — the default action loses
// the key, and then reports it, which is what makes a chord taken from a filter
// visible — and two overrides are settled in favour of the id that sorts first.
// overridden names the actions whose keys came from the config.
func (t *KeyTable) resolveKeyConflicts(overridden map[string]bool) {
	owners := make(map[string][]int)
	for i, binding := range t.bindings {
		if binding.run == nil || binding.fixed {
			continue
		}
		for _, key := range binding.keys {
			owners[key] = append(owners[key], i)
		}
	}
	keys := make([]string, 0, len(owners))
	for key := range owners {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		claimants := owners[key]
		if len(claimants) < 2 {
			continue
		}
		winner := conflictWinner(t.bindings, claimants, overridden)
		for _, i := range claimants {
			if i == winner {
				continue
			}
			binding := &t.bindings[i]
			binding.keys = withoutKey(binding.keys, key)
			t.issues = append(t.issues, fmt.Sprintf(
				"%s moved from %s to %s", key, binding.id, t.bindings[winner].id,
			))
			if len(binding.keys) == 0 {
				t.issues = append(t.issues, fmt.Sprintf("%s is now unbound", binding.id))
			}
		}
	}
}

// conflictWinner picks the action that keeps a contested key.
func conflictWinner(bindings []keyBinding, claimants []int, overridden map[string]bool) int {
	winner := claimants[0]
	for _, i := range claimants[1:] {
		switch {
		case overridden[bindings[i].id] && !overridden[bindings[winner].id]:
			winner = i
		case overridden[bindings[i].id] == overridden[bindings[winner].id] &&
			bindings[i].id < bindings[winner].id:
			winner = i
		}
	}
	return winner
}

// withoutKey drops one key from a binding's keys, keeping the rest in place.
func withoutKey(keys []string, key string) []string {
	kept := make([]string, 0, len(keys))
	for _, candidate := range keys {
		if candidate != key {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// sortedKeyIDs lists a [tui.keys] map's action ids in order, so the table
// resolves the same overrides the same way every time.
func sortedKeyIDs(overrides map[string]appconfig.KeyList) []string {
	ids := make([]string, 0, len(overrides))
	for id := range overrides {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// keyIssueToast is the start-up notice a config whose [tui.keys] entries had
// problems earns: how many there were, and where to read them.
func keyIssueToast(issues int) string {
	if issues == 1 {
		return "1 key binding issue — see `seshagy config keys`"
	}
	return fmt.Sprintf("%d key binding issues — see `seshagy config keys`", issues)
}
