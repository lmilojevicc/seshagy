package tui

import (
	"slices"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// keys builds a [tui.keys] override map for one action.
func keys(id string, names ...string) map[string]appconfig.KeyList {
	return map[string]appconfig.KeyList{id: appconfig.KeyList(names)}
}

// mappingKeys reads an action's effective keys out of the table.
func mappingKeys(t *testing.T, table KeyTable, id string) []string {
	t.Helper()
	for _, mapping := range table.Mappings() {
		if mapping.ID == id {
			return mapping.Keys
		}
	}
	t.Fatalf("the table has no action %q", id)
	return nil
}

// TestResolveKeysReplacesTheDefaultKeys pins the one rule an override follows:
// the keys it lists are the action's keys, and the ones it does not are gone.
func TestResolveKeysReplacesTheDefaultKeys(t *testing.T) {
	tests := []struct {
		name   string
		names  appconfig.KeyList
		action string
		want   string
	}{
		{name: "string form", names: appconfig.KeyList{"f2"}, action: "agents", want: "f2"},
		{
			name:  "list form",
			names: appconfig.KeyList{"f9", "ctrl+x", "delete"}, action: "kill",
			want: "f9,ctrl+x,delete",
		},
		{
			name:  "a filter chord the picker never had",
			names: appconfig.KeyList{"f5"}, action: "remotes", want: "f5",
		},
		{
			name:  "one key of a merged action",
			names: appconfig.KeyList{"f7"}, action: "up", want: "f7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := ResolveKeys(keys(tt.action, tt.names...))
			if got := strings.Join(mappingKeys(t, table, tt.action), ","); got != tt.want {
				t.Fatalf("keys = %q, want %q", got, tt.want)
			}
			if issues := table.Issues(); len(issues) != 0 {
				t.Fatalf("issues = %v, want none", issues)
			}
		})
	}
}

// TestResolveKeysNormalizesKeyNames pins the spellings a config may use: case is
// ignored, and ctrl-a, C-a and ^a all name ctrl+a.
func TestResolveKeysNormalizesKeyNames(t *testing.T) {
	tests := []struct{ name, action, want string }{
		{name: "F2", action: "agents", want: "f2"},
		{name: "ctrl-q", action: "agents", want: "ctrl+q"},
		{name: "CTRL-Q", action: "agents", want: "ctrl+q"},
		{name: "C-Q", action: "agents", want: "ctrl+q"},
		{name: "c-q", action: "agents", want: "ctrl+q"},
		{name: "^q", action: "agents", want: "ctrl+q"},
		{name: "ctrL-b", action: "agents", want: "ctrl+b"},
		{name: "ALT+X", action: "agents", want: "alt+x"},
		{name: "PGUP", action: "page_up", want: "pgup"},
		{name: "Shift+Tab", action: "previous_filter", want: "shift+tab"},
		{name: "Delete", action: "kill", want: "delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := ResolveKeys(keys(tt.action, tt.name))
			if got := strings.Join(mappingKeys(t, table, tt.action), ","); got != tt.want {
				t.Fatalf("key %q = %q, want %q", tt.name, got, tt.want)
			}
			if issues := table.Issues(); len(issues) != 0 {
				t.Fatalf("issues = %v, want none", issues)
			}
		})
	}
}

// TestResolveKeysRejectsOverridesItCannotRead pins every override the picker
// drops: an unknown or fixed action id, a value that is not a key name or a list
// of them, and a key it cannot bind. A rejected key leaves the rest of its
// entry's keys in place.
func TestResolveKeysRejectsOverridesItCannotRead(t *testing.T) {
	tests := []struct {
		name    string
		keys    map[string]appconfig.KeyList
		want    string
		action  string
		keep    string
		keptKey string
	}{
		{
			name: "unknown action", keys: keys("bogus", "f2"),
			want: "tui.keys.bogus: unknown action", action: "agents", keep: "ctrl+a",
		},
		{
			name: "fixed key", keys: keys("esc", "f2"),
			want: "tui.keys.esc: esc is not remappable", action: "agents", keep: "ctrl+a",
		},
		{
			name: "enter is fixed", keys: keys("enter", "f2"),
			want: "tui.keys.enter: enter is not remappable", action: "agents", keep: "ctrl+a",
		},
		{
			name: "unreadable value", keys: map[string]appconfig.KeyList{"agents": nil},
			want: "tui.keys.agents: want a key name or a list of keys", action: "agents",
			keep: "ctrl+a",
		},
		{
			name: "reserved key", keys: keys("agents", "enter"),
			want: `"enter" is reserved for enter`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "ctrl+m is enter", keys: keys("agents", "C-M"),
			want: `"ctrl+m" is reserved for enter`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "ctrl+i is tab", keys: keys("agents", "^i"),
			want: `"ctrl+i" is reserved for tab`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "ctrl+[ is esc", keys: keys("agents", "ctrl+["),
			want: `"ctrl+[" is reserved for esc`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "ctrl+? is backspace", keys: keys("agents", "^?"),
			want: `"ctrl+?" is reserved for backspace`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "backspace is fixed", keys: keys("agents", "backspace"),
			want: `"backspace" is reserved for backspace`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "ctrl+c is fixed", keys: keys("agents", "ctrl+c"),
			want: `"ctrl+c" is reserved for ctrl+c`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "printable rune", keys: keys("agents", "a"),
			want: `"a" would type into the query`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "question mark", keys: keys("agents", "?"),
			want: `"?" would type into the query`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "slash", keys: keys("agents", "/"),
			want: `"/" would type into the query`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "space", keys: keys("agents", " "),
			want: `" " would type into the query`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "unknown name", keys: keys("agents", "foo"),
			want: `"foo" is not a key name`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "unknown chord", keys: keys("agents", "ctrl+alt+x"),
			want: `"ctrl+alt+x" is not a key name`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "shifted rune", keys: keys("agents", "shift+a"),
			want: `"shift+a" is not a key name`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "no such function key", keys: keys("agents", "f21"),
			want: `"f21" is not a key name`, action: "agents", keep: "ctrl+a",
		},
		{
			name: "one bad key of a list", keys: keys("kill", "f9", "a", "delete"),
			want: `"a" would type into the query`, action: "kill", keep: "f9,delete",
			keptKey: "f9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := ResolveKeys(tt.keys)
			issues := strings.Join(table.Issues(), "\n")
			if !strings.Contains(issues, tt.want) {
				t.Fatalf("issues = %q, want %q", issues, tt.want)
			}
			// An override whose every key was rejected keeps the action's
			// defaults instead of unbinding it, and says so.
			if tt.keep == "ctrl+a" && !strings.Contains(issues, "ignored") &&
				!strings.Contains(issues, "no usable key; keeping the default") {
				t.Fatalf("issues = %q, want the keep-the-default note", issues)
			}
			got := strings.Join(mappingKeys(t, table, tt.action), ",")
			if got != tt.keep {
				t.Fatalf("action %q keys = %q, want %q", tt.action, got, tt.keep)
			}
			// A dropped key leaves the rest of the table alone.
			if tt.keptKey != "" {
				if _, ok := table.bindingFor(tt.keptKey); !ok {
					t.Fatalf("key %q was dropped with the bad one", tt.keptKey)
				}
			}
		})
	}
}

// TestResolveKeysMovesAKeyAnOverrideTookFromADefault pins the conflict rule that
// makes a remap visible: the override keeps the key, the action that had it
// loses it and says so, and an action left with none says it is unbound.
func TestResolveKeysMovesAKeyAnOverrideTookFromADefault(t *testing.T) {
	table := ResolveKeys(keys("agents", "ctrl+e"))
	want := []string{
		"ctrl+e moved from remotes to agents",
		"remotes is now unbound",
	}
	if got := table.Issues(); !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	if got := strings.Join(mappingKeys(t, table, "agents"), ","); got != "ctrl+e" {
		t.Fatalf("agents keys = %q, want ctrl+e", got)
	}
	if got := mappingKeys(t, table, "remotes"); len(got) != 0 {
		t.Fatalf("remotes keys = %v, want none", got)
	}
	// The action that lost its only key lost its help row with it.
	for _, binding := range table.helpBindings() {
		if binding.id == "remotes" {
			t.Fatalf("the unbound remotes action still has a help row: %+v", binding)
		}
	}

	m := remappedModel(t, keys("agents", "ctrl+e"))
	m = withItems(m, pickerItems())
	m, _ = press(t, m, "ctrl+e")
	if m.scope != scopeAgents {
		t.Fatalf("ctrl+e left the scope %v, want agents", m.scope)
	}
}

// TestResolveKeysLeavesTheOtherKeysOfTheActionItTookFrom pins a partial move: a
// key taken from an action that answers several leaves the rest in place, and
// the footer and the help row draw what is left.
func TestResolveKeysLeavesTheOtherKeysOfTheActionItTookFrom(t *testing.T) {
	table := ResolveKeys(keys("page_up", "ctrl+p"))
	want := []string{"ctrl+p moved from up to page_up"}
	if got := table.Issues(); !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	if got := strings.Join(mappingKeys(t, table, "up"), ","); got != "up,ctrl+k" {
		t.Fatalf("up keys = %q, want up,ctrl+k", got)
	}
	m := remappedModel(t, keys("page_up", "ctrl+p"))
	hints := m.footerHints()
	if hints[0].key != "↑↓" || hints[0].label != "move" {
		t.Fatalf("the move hint = %+v, want the arrows", hints[0])
	}
	row := helpRowFor(t, openHelp(t, m), "ctrl+k")
	if row.label != "move up" {
		t.Fatalf("the up help row = %+v, want the remaining chord", row)
	}
	// The moved key page-ups instead of moving the cursor up.
	loaded := withItems(m, pickerItems())
	loaded.cursor = 3
	moved, _ := press(t, loaded, "ctrl+p")
	if moved.cursor != max(0, 3-loaded.pageSize()) {
		t.Fatalf("ctrl+p left the cursor at %d, want a page up", moved.cursor)
	}
}

// TestResolveKeysSettlesTwoOverridesClaimingAKey pins the second conflict rule:
// of two overrides claiming one key, the id that sorts first keeps it.
func TestResolveKeysSettlesTwoOverridesClaimingAKey(t *testing.T) {
	tests := []struct {
		name   string
		keys   map[string]appconfig.KeyList
		want   []string
		winner []string
		losers map[string][]string
	}{
		{
			name: "agents sorts before remotes",
			keys: map[string]appconfig.KeyList{
				"agents":  {"f2"},
				"remotes": {"f2"},
			},
			want:   []string{"f2 moved from remotes to agents", "remotes is now unbound"},
			winner: []string{"agents", "f2"},
			losers: map[string][]string{"remotes": nil},
		},
		{
			name: "remotes sorts before zoxide",
			keys: map[string]appconfig.KeyList{
				"zoxide":  {"f2"},
				"remotes": {"f2"},
			},
			want: []string{
				"f2 moved from zoxide to remotes",
				"zoxide is now unbound",
			},
			winner: []string{"remotes", "f2"},
			losers: map[string][]string{"zoxide": nil},
		},
		{
			name: "the loser keeps its other keys",
			keys: map[string]appconfig.KeyList{
				"remotes": {"f2"},
				"zoxide":  {"f2", "ctrl+z"},
			},
			want:   []string{"f2 moved from zoxide to remotes"},
			winner: []string{"remotes", "f2"},
			losers: map[string][]string{"zoxide": {"ctrl+z"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := ResolveKeys(tt.keys)
			if got := table.Issues(); !slices.Equal(got, tt.want) {
				t.Fatalf("issues = %v, want %v", got, tt.want)
			}
			if got := strings.Join(mappingKeys(t, table, tt.winner[0]), ","); got != tt.winner[1] {
				t.Fatalf("%s keys = %q, want %q", tt.winner[0], got, tt.winner[1])
			}
			for id, want := range tt.losers {
				if got := strings.Join(
					mappingKeys(t, table, id),
					",",
				); got != strings.Join(
					want,
					",",
				) {
					t.Fatalf("%s keys = %q, want %v", id, got, want)
				}
			}
		})
	}
}

// TestResolveKeysBindsEveryKeyOnce pins the table's invariant: no key is bound
// twice, whatever the config asks for. The fixed esc pair is the one key both
// the footer hint and a help row name.
func TestResolveKeysBindsEveryKeyOnce(t *testing.T) {
	tests := []struct {
		name string
		keys map[string]appconfig.KeyList
	}{
		{"defaults", nil},
		{"one remap", keys("agents", "f2")},
		{"a chord taken from a filter", keys("agents", "ctrl+e")},
		{"two overrides on one key", map[string]appconfig.KeyList{
			"agents": {"f2"}, "remotes": {"f2"}, "zoxide": {"F2"},
		}},
		{
			"an override on every filter key",
			map[string]appconfig.KeyList{
				"agents": {"ctrl+s"}, "state": {"ctrl+t"}, "sessions": {"ctrl+z"},
				"zoxide": {"ctrl+f"}, "find": {"ctrl+e"}, "remotes": {"ctrl+a"},
			},
		},
		{"an override repeated in its own list", keys("kill", "f9", "f9")},
		{"fixed keys claimed", map[string]appconfig.KeyList{
			"agents": {"esc"}, "remotes": {"enter"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := ResolveKeys(tt.keys)
			owners := map[string][]string{}
			for _, binding := range table.bindings {
				if binding.run == nil {
					continue
				}
				for _, key := range binding.keys {
					owners[key] = append(owners[key], binding.id)
				}
			}
			for key, ids := range owners {
				if len(ids) != 1 {
					t.Fatalf("key %q is bound by %v, want one action", key, ids)
				}
			}
		})
	}
}

// TestResolveKeysLeavesTheDefaultsAlone pins that overrides the picker cannot
// read change nothing: the rejected entry is reported and the default table
// stands.
func TestResolveKeysLeavesTheDefaultsAlone(t *testing.T) {
	table := ResolveKeys(map[string]appconfig.KeyList{
		"bogus":  {"f2"},
		"esc":    {"f2"},
		"agents": nil,
	})
	if len(table.Issues()) != 3 {
		t.Fatalf("issues = %v, want one per rejected entry", table.Issues())
	}
	def := defaultKeyTable()
	for _, action := range remappableKeys {
		got := strings.Join(mappingKeys(t, table, action), ",")
		want := strings.Join(mappingKeys(t, def, action), ",")
		if got != want {
			t.Fatalf("action %q keys = %q, want the default %q", action, got, want)
		}
	}
	m := remappedModel(t, map[string]appconfig.KeyList{"bogus": {"f2"}, "agents": nil})
	m, _ = press(t, m, "ctrl+a")
	if m.scope != scopeAgents {
		t.Fatalf("the rejected override remapped ctrl+a: scope = %v", m.scope)
	}
}

// TestUnboundActionsLeaveTheFooterAndHelp pins the [] form: an action with no
// keys answers nothing, and neither the footer nor the help overlay draws it.
func TestUnboundActionsLeaveTheFooterAndHelp(t *testing.T) {
	m := remappedModel(t, map[string]appconfig.KeyList{
		"yazi":    {},
		"preview": {},
	})
	loaded := withItems(m, pickerItems())

	if _, ok := m.keys.bindingFor("ctrl+y"); ok {
		t.Fatal("the unbound yazi action still dispatches on ctrl+y")
	}
	if _, ok := m.keys.bindingFor("ctrl+o"); ok {
		t.Fatal("the unbound preview action still dispatches on ctrl+o")
	}
	// ctrl+o is a ctrl chord, so nothing types it into the query either.
	pressed, _ := press(t, loaded, "ctrl+o")
	if pressed.showPreview != loaded.showPreview {
		t.Fatal("the unbound preview action still toggled the preview")
	}
	if pressed.query != "" {
		t.Fatalf("the unbound key typed %q into the query", pressed.query)
	}

	footer := sessionmgr.StripANSI(m.renderFooter(160))
	if strings.Contains(footer, "yazi") || strings.Contains(footer, "preview") {
		t.Fatalf("the footer still draws an unbound action: %q", footer)
	}
	open := openHelp(t, m)
	for _, block := range open.helpBlocks() {
		for _, row := range block.rows {
			if row.label == "yazi" || row.label == "preview" {
				t.Fatalf("the help overlay still draws the unbound %q row", row.label)
			}
		}
	}
}

// TestFooterFallsBackToTheKeysThatRemain pins the footer when a config unbinds
// every hint but the fixed quit key: the last hints are still the ones that
// survive.
func TestFooterFallsBackToTheKeysThatRemain(t *testing.T) {
	tests := []struct {
		name   string
		unbind []string
		want   string
	}{
		{
			"defaults",
			nil,
			"↑↓ move  ⏎ open  ^r rename  ^x kill  ^o preview  ^y yazi  F1 help  esc quit",
		},
		{
			name:   "no yazi",
			unbind: []string{"yazi"},
			want:   "↑↓ move  ⏎ open  ^r rename  ^x kill  ^o preview  F1 help  esc quit",
		},
		{
			name:   "the droppable hints only",
			unbind: []string{"up", "down", "rename", "kill", "preview", "yazi"},
			want:   "⏎ open  F1 help  esc quit",
		},
		{
			name:   "help too",
			unbind: []string{"up", "down", "rename", "kill", "preview", "yazi", "help"},
			want:   "⏎ open  esc quit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrides := map[string]appconfig.KeyList{}
			for _, id := range tt.unbind {
				overrides[id] = appconfig.KeyList{}
			}
			m := remappedModel(t, overrides)
			if got := sessionmgr.StripANSI(m.renderFooter(200)); got != tt.want {
				t.Fatalf("footer = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRemappedKeyDispatchesAndTheDefaultKeyDoesNot pins the remap on the wire:
// the new key runs the action and the key it replaced runs nothing.
func TestRemappedKeyDispatchesAndTheDefaultKeyDoesNot(t *testing.T) {
	m := remappedModel(t, keys("agents", "f2"))
	m = withItems(m, pickerItems())

	old, _ := press(t, m, "ctrl+a")
	if old.scope != scopeAll {
		t.Fatalf("the replaced ctrl+a still switched the scope to %v", old.scope)
	}
	if old.query != "" {
		t.Fatalf("the replaced ctrl+a typed %q into the query", old.query)
	}
	fresh, _ := press(t, m, "f2")
	if fresh.scope != scopeAgents {
		t.Fatalf("f2 left the scope %v, want agents", fresh.scope)
	}
	// The new key toggles the way the default did: the active scope's key
	// returns to all.
	back, _ := press(t, fresh, "f2")
	if back.scope != scopeAll {
		t.Fatalf("a second f2 left the scope %v, want all", back.scope)
	}
}

// TestRemappedKeysReachTheFooterAndTheHelp pins that the effective table is what
// both draw: a remapped footer key and a remapped help key show up where the
// default did.
func TestRemappedKeysReachTheFooterAndTheHelp(t *testing.T) {
	m := remappedModel(t, map[string]appconfig.KeyList{
		"preview": {"f4"},
		"agents":  {"f2"},
		"kill":    {"f9", "delete"},
	})
	footer := sessionmgr.StripANSI(m.renderFooter(160))
	if !strings.Contains(footer, "f4 preview") {
		t.Fatalf("footer = %q, want the remapped preview key", footer)
	}
	if strings.Contains(footer, "^o preview") {
		t.Fatalf("footer = %q, want no ^o", footer)
	}
	rendered := helpBoxText(t, m)
	if !strings.Contains(rendered, "f2") || !strings.Contains(rendered, "agents") {
		t.Fatalf("the help overlay does not draw the remapped key:\n%s", rendered)
	}
	if strings.Contains(rendered, "^a") {
		t.Fatalf("the help overlay still draws the replaced key:\n%s", rendered)
	}
	// kill keeps the footer's key and the help row draws the one it left it.
	if !strings.Contains(footer, "f9 kill") {
		t.Fatalf("footer = %q, want the remapped first key", footer)
	}
	if row := helpRowFor(t, openHelp(t, m), "delete"); row.label != "kill" {
		t.Fatalf("the kill help row = %+v, want the key the footer left it", row)
	}
}

// helpBoxText is the help overlay's box as text, without its styling.
func helpBoxText(t *testing.T, m Model) string {
	t.Helper()
	_, _, _, box := helpBox(t, openHelp(t, m))
	return sessionmgr.StripANSI(strings.Join(box, "\n"))
}

// TestNewToastsTheKeyBindingIssues pins the start-up notice: one toast naming
// the command that prints the issues, and nothing when the config is clean.
func TestNewToastsTheKeyBindingIssues(t *testing.T) {
	tests := []struct {
		name  string
		keys  map[string]appconfig.KeyList
		count int
		want  string
	}{
		{"clean", nil, 0, ""},
		{"one issue", keys("bogus", "f2"), 1, "1 key binding issue — see `seshagy config keys`"},
		{
			name: "two issues",
			keys: map[string]appconfig.KeyList{"bogus": {"f2"}, "agents": {"a", "f3"}},
			want: "2 key binding issues — see `seshagy config keys`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := remappedModel(t, tt.keys)
			if got := latestNotificationText(m); got != tt.want {
				t.Fatalf("toast = %q, want %q", got, tt.want)
			}
			if tt.want != "" && latestNotificationSeverity(m) != sevWarning {
				t.Fatalf("toast severity = %v, want a warning", latestNotificationSeverity(m))
			}
		})
	}
}

// TestResolveKeysKeepsTheDefaultsWhenEveryKeyIsRejected pins the edge of the
// drop rule: an entry whose every key is rejected is a typo, not a request to
// unbind, so the action keeps its default keys and the issues say so. Only an
// explicit empty list unbinds.
func TestResolveKeysKeepsTheDefaultsWhenEveryKeyIsRejected(t *testing.T) {
	table := ResolveKeys(keys("agents", "a", "?"))
	want := []string{
		`tui.keys.agents: "a" would type into the query`,
		`tui.keys.agents: "?" would type into the query`,
		"tui.keys.agents: no usable key; keeping the default",
	}
	if got := table.Issues(); !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	if got := strings.Join(mappingKeys(t, table, "agents"), ","); got != "ctrl+a" {
		t.Fatalf("agents keys = %q, want the default ctrl+a", got)
	}
	unbound := ResolveKeys(map[string]appconfig.KeyList{"agents": {}})
	if got := mappingKeys(t, unbound, "agents"); len(got) != 0 {
		t.Fatalf("an explicit [] left agents with %v, want none", got)
	}
}

// TestResolveKeysCannotRemapTheQueryTokenRows pins that the rows documenting the
// query grammar are fixed like the picker's own keys: an override on one is
// reported and the token text the overlay draws is left alone.
func TestResolveKeysCannotRemapTheQueryTokenRows(t *testing.T) {
	table := ResolveKeys(map[string]appconfig.KeyList{
		"!name":   {"f2"},
		"example": {"f2"},
	})
	want := []string{
		"tui.keys.!name: !name is not remappable; ignored",
		"tui.keys.example: example is not remappable; ignored",
	}
	if got := table.Issues(); !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	for _, binding := range table.helpBindings() {
		if binding.group != groupQueryTokens {
			continue
		}
		if binding.id == "!name" && keyHint(binding.keys) != "!name" {
			t.Fatalf("the token row draws %q, want !name", keyHint(binding.keys))
		}
		if binding.id == "example" && len(binding.keys) != 0 {
			t.Fatalf("the example row draws %v, want no keys", binding.keys)
		}
	}
}
