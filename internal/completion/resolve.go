package completion

import (
	"context"
	"strings"
	"time"
)

const dynamicTimeout = 200 * time.Millisecond

// Resolve completes words through the cursor. words[0] is argv0 and the final
// element is the current (possibly empty) token.
func Resolve(ctx context.Context, words []string, provider Provider) Result {
	if len(words) < 2 {
		return Result{Directive: NoFiles}
	}
	completed := make([]string, 0, len(words)-2)
	ephemeralUsed := false
	for _, word := range words[1 : len(words)-1] {
		// run removes the exact global token before any command parser sees argv.
		// Normalize it here too, especially while a value flag is awaiting its
		// separated value.
		if word == "--ephemeral" {
			ephemeralUsed = true
			continue
		}
		completed = append(completed, word)
	}
	prefix := words[len(words)-1]
	if len(completed) > 128 || totalBytes(words) > 32*1024 {
		return Result{Directive: NoFiles}
	}

	command := &Root
	used := map[string]bool{}
	positionals := []string{}
	var awaiting *Flag
	invalid := false

	for _, word := range completed {
		if awaiting != nil {
			awaiting = nil
			continue
		}
		if command == &Root && (word == "-h" || word == "help") {
			return Result{Directive: NoFiles}
		}
		if command == &Root && (word == "--version" || word == "version") {
			command = FindChild(&Root, "--version")
			continue
		}
		if child := FindChild(command, word); child != nil && len(positionals) == 0 {
			command = child
			positionals = nil
			continue
		}

		// Parser-required positionals precede command flags. Treat a completed
		// invalid enum as a dead end rather than suggesting a suffix that the
		// parser would reject (for example: keybind install --persistent tmux).
		if len(positionals) < len(command.Positionals) {
			value := command.Positionals[len(positionals)].Value
			if (value.Kind == ValueEnum && !containsValue(value.Values, word)) ||
				(value.Kind == ValueSessionLine && strings.HasPrefix(word, "--")) {
				invalid = true
				break
			}
			positionals = append(positionals, word)
			continue
		}

		if strings.HasPrefix(word, "--") {
			flag := findFlag(command, word)
			if flag == nil {
				invalid = true
				break
			}
			used[word] = true
			if flag.Value.Kind != ValueNone {
				copy := *flag
				awaiting = &copy
			}
			continue
		}

		// No command accepts additional free positionals. In particular, do not
		// recover from a parser-invalid flag/positional ordering by emitting flags.
		invalid = true
		break
	}

	if invalid {
		return Result{Directive: NoFiles}
	}
	if awaiting != nil {
		return resolveValue(
			ctx,
			adjustedValue(command, awaiting.Name, awaiting.Value, positionals),
			prefix,
			provider,
		)
	}

	if len(positionals) < len(command.Positionals) {
		value := command.Positionals[len(positionals)].Value
		return resolveValue(ctx, value, prefix, provider)
	}

	candidates := make([]Candidate, 0, len(command.Children)+len(command.Flags)+2)
	if len(positionals) == 0 {
		for i := range command.Children {
			child := command.Children[i]
			if child.Machine && (command != &Root || !strings.HasPrefix(prefix, "-")) {
				continue
			}
			candidates = append(
				candidates,
				Candidate{Value: child.Name, Description: child.Description},
			)
		}
	}
	for _, flag := range command.Flags {
		if !used[flag.Name] && flagAppliesToTarget(flag, positionals) {
			candidates = append(
				candidates,
				Candidate{Value: flag.Name, Description: flag.Description},
			)
		}
	}
	if command != &Root && !ephemeralUsed {
		candidates = append(
			candidates,
			Candidate{Value: "--ephemeral", Description: "Exit when the dashboard loses focus"},
		)
	}
	return Result{Directive: NoFiles, Candidates: filterCandidates(candidates, prefix)}
}

func resolveValue(ctx context.Context, value Value, prefix string, provider Provider) Result {
	directive := NoFiles
	if value.Kind == ValueDirectory {
		directive = Dirs
	}
	var candidates []Candidate
	switch value.Kind {
	case ValueEnum:
		for _, item := range value.Values {
			candidates = append(candidates, Candidate{Value: item, Description: item})
		}
	case ValuePane, ValueDirectory, ValueAgent, ValueSource, ValueSessionLine:
		if provider != nil {
			dynamicCtx, cancel := context.WithTimeout(ctx, dynamicTimeout)
			type response struct {
				values []Candidate
				err    error
			}
			result := make(chan response, 1)
			go func() {
				values, err := provider.Values(dynamicCtx, value.Kind)
				result <- response{values: values, err: err}
			}()
			select {
			case reply := <-result:
				if reply.err == nil {
					candidates = reply.values
				}
			case <-dynamicCtx.Done():
			}
			cancel()
		}
	}
	return Result{Directive: directive, Candidates: filterCandidates(candidates, prefix)}
}

func adjustedValue(_ *Command, _ string, value Value, positionals []string) Value {
	if len(positionals) > 0 && value.ValuesByTarget != nil {
		value.Values = value.ValuesByTarget[positionals[0]]
	}
	return value
}

func findFlag(command *Command, name string) *Flag {
	for i := range command.Flags {
		if command.Flags[i].Name == name {
			return &command.Flags[i]
		}
	}
	return nil
}

func flagAppliesToTarget(flag Flag, positionals []string) bool {
	if len(flag.Targets) == 0 || len(positionals) == 0 {
		return true
	}
	return containsValue(flag.Targets, positionals[0])
}

func containsValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func totalBytes(words []string) int {
	total := 0
	for _, word := range words {
		total += len(word)
	}
	return total
}
