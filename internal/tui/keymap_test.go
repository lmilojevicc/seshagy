package tui

import (
	"sort"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// pickerKeys is the key set the picker's dispatch answers, which the table must
// cover exactly: the dispatch reads the table, so a key added or dropped without
// a table change shows up as a failure here.
var pickerKeys = []string{
	"up", "down", "ctrl+p", "ctrl+k", "ctrl+n", "ctrl+j",
	"pgup", "pgdown", "home", "end",
	"tab", "shift+tab", "enter", "esc", "f1", "backspace", "delete",
	"ctrl+a", "ctrl+s", "ctrl+t", "ctrl+z", "ctrl+f", "ctrl+e",
	"ctrl+r", "ctrl+x", "ctrl+o", "ctrl+y", "ctrl+g", "ctrl+l", "ctrl+u", "ctrl+w",
	"ctrl+c",
}

// TestKeyBindingsCoverEveryPickerKey pins the table against the picker's key set
// in both directions: every key the dispatch answers is bound, and every key the
// table binds is dispatchable.
func TestKeyBindingsCoverEveryPickerKey(t *testing.T) {
	bound := make([]string, 0, len(pickerKeys))
	for _, binding := range pickerKeyBindings {
		if binding.run == nil {
			continue
		}
		bound = append(bound, binding.keys...)
	}
	sort.Strings(bound)
	want := append([]string(nil), pickerKeys...)
	sort.Strings(want)
	if len(bound) != len(want) {
		t.Fatalf("table binds %d keys %v, want %d %v", len(bound), bound, len(want), want)
	}
	for i := range want {
		if bound[i] != want[i] {
			t.Fatalf("table binds %v, want %v", bound, want)
		}
	}
	for _, key := range pickerKeys {
		if keyBindingFor(key) == nil {
			t.Fatalf("key %q does not resolve to a binding", key)
		}
	}
	if binding := keyBindingFor("x"); binding != nil {
		t.Fatalf("a printable rune resolved to a binding: %+v", binding)
	}
}

// TestKeyBindingsNameEachKeyOncePerList pins the two lists the table feeds: the
// footer never draws the same key twice, and neither does the help overlay.
func TestKeyBindingsNameEachKeyOncePerList(t *testing.T) {
	for _, list := range []struct {
		name     string
		bindings []keyBinding
	}{
		{"footer", footerBindings()},
		{"help", helpBindings()},
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

// TestFooterAndHelpBindingsAreDisjoint pins the footer/help split: an entry is a
// footer hint or a help row, never both. esc is the one key both lists name —
// the footer calls it the quit key, the help spells out what it clears first.
func TestFooterAndHelpBindingsAreDisjoint(t *testing.T) {
	footer := map[string]bool{}
	for _, binding := range footerBindings() {
		for _, key := range binding.keys {
			footer[key] = true
		}
	}
	shared := make([]string, 0, 1)
	for _, binding := range helpBindings() {
		for _, key := range binding.keys {
			if footer[key] {
				shared = append(shared, key)
			}
		}
	}
	sort.Strings(shared)
	if len(shared) != 1 || shared[0] != "esc" {
		t.Fatalf("footer and help share %v, want only esc", shared)
	}
	// The help row that names esc documents it rather than binding it: the
	// footer's row is the one the dispatch answers.
	if binding := keyBindingFor("esc"); binding == nil || !binding.footer {
		t.Fatalf("esc dispatches to %+v, want the footer's quit binding", binding)
	}
}

// TestFooterHintsAreTheCoreActions pins the footer's contents and order: the
// core actions only, with the help and quit hints last.
func TestFooterHintsAreTheCoreActions(t *testing.T) {
	m := newTestModel(t)
	want := []footerHint{
		{"↑↓", "move"},
		{"⏎", "open"},
		{"^r", "rename"},
		{"^x", "kill"},
		{"^o", "preview"},
		{"^y", "yazi"},
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
	for _, binding := range helpBindings() {
		if binding.terms {
			if got := m.bindingLabel(binding); got != "workspaces" {
				t.Fatalf("herdr label = %q, want workspaces", got)
			}
		}
	}
	m.terms = sessionmgr.TmuxTerms()
	if got := m.bindingLabel(pickerKeyBindings[0]); got != "move" {
		t.Fatalf("label = %q, want move", got)
	}
}

// TestKeyHintDrawsGlyphsAndChords pins how a binding's keys draw.
func TestKeyHintDrawsGlyphsAndChords(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"arrows", []string{"up", "down"}, "↑↓"},
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
		if len(binding.keys) != 1 || binding.keys[0] != "esc" {
			t.Fatalf("binding %v has no run and is not a query-token row", binding.keys)
		}
	}
}
