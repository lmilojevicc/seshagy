package integrations

import (
	"strings"
	"testing"
)

func TestYamlFlowSequenceItems(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
		items []string
	}{
		{name: "empty", value: "", ok: false},
		{name: "not flow", value: "enabled: true", ok: false},
		{name: "empty flow", value: "[]", ok: true, items: []string{}},
		{
			name:  "bare items",
			value: "[foo, bar]",
			ok:    true,
			items: []string{"foo", "bar"},
		},
		{
			name:  "quoted items",
			value: `["foo", 'bar']`,
			ok:    true,
			items: []string{"foo", "bar"},
		},
		{
			name:  "skips blanks",
			value: "[ foo , , bar ]",
			ok:    true,
			items: []string{"foo", "bar"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, ok := yamlFlowSequenceItems(tc.value)
			if ok != tc.ok {
				t.Fatalf("yamlFlowSequenceItems(%q) ok = %v, want %v", tc.value, ok, tc.ok)
			}
			if !ok {
				return
			}
			if len(items) != len(tc.items) {
				t.Fatalf("items = %#v, want %#v", items, tc.items)
			}
			for i := range items {
				if items[i] != tc.items[i] {
					t.Fatalf("items[%d] = %q, want %q", i, items[i], tc.items[i])
				}
			}
		})
	}
}

func TestHermesFlatPluginLines(t *testing.T) {
	empty := hermesFlatPluginLines(nil)
	if len(empty) != 1 || empty[0] != "plugins: []" {
		t.Fatalf("hermesFlatPluginLines(nil) = %#v, want [plugins: []]", empty)
	}

	items := hermesFlatPluginLines([]string{"alpha", "beta"})
	want := []string{"plugins:", "  - alpha", "  - beta"}
	if len(items) != len(want) {
		t.Fatalf("hermesFlatPluginLines() = %#v, want %#v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("line[%d] = %q, want %q", i, items[i], want[i])
		}
	}
}

func TestUpdateHermesInlinePluginListEnable(t *testing.T) {
	input := "plugins: [platforms/discord]\n"
	got := ensureHermesPluginEnabled(input)
	want := "plugins:\n  - " + hermesPluginName + "\n  - platforms/discord\n"
	if got != want {
		t.Fatalf("ensureHermesPluginEnabled():\n%s\nwant:\n%s", got, want)
	}
}

func TestUpdateHermesEnabledBlockEmptyList(t *testing.T) {
	lines := []string{
		"plugins:",
		"  enabled: []",
		"model:",
		"  provider: auto",
	}
	got := updateHermesEnabledBlock(lines, 4, 1, true)
	if len(got) < 3 || got[1] != "  enabled:" || got[2] != "    - "+hermesPluginName {
		t.Fatalf("updateHermesEnabledBlock() = %#v", got)
	}
}

func TestUpdateHermesEnabledBlockDisableExisting(t *testing.T) {
	input := strings.Join([]string{
		"plugins:",
		"  enabled:",
		"    - " + hermesPluginName,
		"    - platforms/discord",
	}, "\n") + "\n"
	got := removeHermesPluginEnabled(input)
	if strings.Contains(got, hermesPluginName) || !strings.Contains(got, "platforms/discord") {
		t.Fatalf("removeHermesPluginEnabled():\n%s", got)
	}
}

func TestUpdateHermesInlinePluginListDisable(t *testing.T) {
	input := strings.Join([]string{
		"plugins:",
		"  - " + hermesPluginName,
		"  - platforms/discord",
	}, "\n") + "\n"
	got := removeHermesPluginEnabled(input)
	want := "plugins:\n  - platforms/discord\n"
	if got != want {
		t.Fatalf("removeHermesPluginEnabled():\n%s\nwant:\n%s", got, want)
	}
}
