package completion

import (
	"strings"
	"testing"
)

func TestSpecIsCompleteAndWellFormed(t *testing.T) {
	// Entries here document targets for which an empty enum is intentionally
	// meaningful. New target-aware enums default to requiring at least one value.
	emptyEnumsExplicitlyValid := map[string]map[string]bool{}
	var walk func(string, *Command)
	walk = func(path string, command *Command) {
		seen := map[string]bool{}
		for _, flag := range command.Flags {
			if !strings.HasPrefix(flag.Name, "--") || flag.Description == "" || seen[flag.Name] {
				t.Errorf("%s invalid or duplicate flag %#v", path, flag)
			}
			seen[flag.Name] = true
			if flag.Value.Kind == "" {
				t.Errorf("%s %s has no value strategy", path, flag.Name)
			}
			for _, target := range flag.Targets {
				if len(command.Positionals) == 0 ||
					!containsValue(command.Positionals[0].Value.Values, target) {
					t.Errorf("%s %s owns unknown target %q", path, flag.Name, target)
				}
			}
			for target := range flag.Value.ValuesByTarget {
				if len(command.Positionals) == 0 ||
					!containsValue(command.Positionals[0].Value.Values, target) {
					t.Errorf("%s %s has values for unknown target %q", path, flag.Name, target)
				}
			}
			if flag.Value.ValuesByTarget != nil && len(command.Positionals) > 0 {
				for _, target := range command.Positionals[0].Value.Values {
					values, declared := flag.Value.ValuesByTarget[target]
					flagPath := path + " " + target + " " + flag.Name
					if !declared {
						t.Errorf("%s missing target enum metadata", flagPath)
					} else if len(values) == 0 &&
						!emptyEnumsExplicitlyValid[path+" "+flag.Name][target] {
						t.Errorf("%s has an empty enum without explicit validity", flagPath)
					}
				}
			}
		}
		for _, positional := range command.Positionals {
			if positional.Name == "" || positional.Description == "" ||
				positional.Value.Kind == "" {
				t.Errorf("%s invalid positional %#v", path, positional)
			}
		}
		seen = map[string]bool{}
		for i := range command.Children {
			child := &command.Children[i]
			if child.Name == "" || child.Description == "" || seen[child.Name] ||
				child.Name == "__complete" {
				t.Errorf("%s invalid or duplicate child %#v", path, child)
			}
			seen[child.Name] = true
			for _, alias := range child.Aliases {
				if alias == "" || seen[alias] {
					t.Errorf("%s duplicate alias %q", path, alias)
				}
				seen[alias] = true
			}
			walk(strings.TrimSpace(path+" "+child.Name), child)
		}
	}
	walk("seshagy", &Root)
}

func TestSpecContainsEveryIntentionalCommand(t *testing.T) {
	paths := []string{
		"--help",
		"--version",
		"config path",
		"config show",
		"config init",
		"diagnostics",
		"integration install",
		"integration uninstall",
		"keybind install",
		"keybind uninstall",
		"completion",
		"--get-all",
		"--get-sessions",
		"--get-zoxide",
		"--get-fd",
		"--get-agents",
		"--get-current-session-agents",
		"--delete-item",
		"--report-agent",
		"--release-agent",
	}
	for _, path := range paths {
		command := &Root
		for _, word := range strings.Fields(path) {
			command = FindChild(command, word)
			if command == nil {
				t.Fatalf("completion metadata missing command path %q", path)
			}
		}
	}
}
