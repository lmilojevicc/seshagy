package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/completion"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestCompletionCommandEmitsScriptsAndValidatesArity(t *testing.T) {
	cliTestEnv(t)
	for _, shell := range []string{"bash", "zsh", "fish"} {
		out, err := captureStdout(t, func() error { return run([]string{"completion", shell}) })
		if err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if !strings.Contains(out, "seshagy") || strings.Contains(out, "eval") {
			t.Fatalf("completion %s output is unsafe or incomplete", shell)
		}
	}
	for _, args := range [][]string{{"completion"}, {"completion", "bash", "extra"}, {"completion", "powershell"}} {
		if err := run(
			args,
		); err == nil ||
			!strings.Contains(err.Error(), "usage: seshagy completion bash|zsh|fish") {
			t.Fatalf("run(%v) error = %v", args, err)
		}
	}
}

func TestHiddenCompletionRunsBeforeConfigLoggingAndIsNotAdvertised(t *testing.T) {
	dir := t.TempDir()
	configHome := filepath.Join(dir, "config")
	stateHome := filepath.Join(dir, "state")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("SESHAGY_LOG_LEVEL", "invalid")
	t.Setenv("SESHAGY_LOG_FILE", filepath.Join(stateHome, "should-not-exist.jsonl"))
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_ENV", "")
	out, err := captureStdout(t, func() error {
		return run([]string{"__complete", "--", "seshagy", "con"})
	})
	if err != nil || !strings.HasPrefix(out, "v1\tnofiles\n") ||
		!strings.Contains(out, "c\tconfig\t") {
		t.Fatalf("hidden completion output=%q err=%v", out, err)
	}
	if _, err := os.Stat(configHome); !os.IsNotExist(err) {
		t.Fatalf("hidden completion touched config home: %v", err)
	}
	if _, err := os.Stat(stateHome); !os.IsNotExist(err) {
		t.Fatalf("hidden completion touched state home: %v", err)
	}
	if strings.Contains(helpText(), "__complete") {
		t.Fatal("hidden endpoint is advertised in help")
	}
}

func TestHiddenDynamicCompletionHasNoApplicationSideEffects(t *testing.T) {
	dir := t.TempDir()
	configHome := filepath.Join(dir, "config")
	stateHome := filepath.Join(dir, "state")
	cacheHome := filepath.Join(dir, "cache")
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("SESHAGY_LOG_LEVEL", "invalid")
	t.Setenv("SESHAGY_LOG_FILE", filepath.Join(stateHome, "must-not-exist.jsonl"))
	t.Setenv("TMUX", "/tmp/fake,1,0")
	t.Setenv("HERDR_ENV", "")
	unexpected := make(chan string, 2)
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		want := []string{
			"list-panes",
			"-a",
			"-F",
			"#{pane_id}\x1f#{pane_current_path}\x1f#{pane_dead}",
		}
		if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
			unexpected <- strings.Join(args, " ")
			return nil, os.ErrInvalid
		}
		return []byte("%9\x1f/tmp/project\x1f0\n"), nil
	}, func(_ context.Context, args ...string) error {
		unexpected <- strings.Join(args, " ")
		return os.ErrInvalid
	})
	out, err := captureStdout(t, func() error {
		return run([]string{"__complete", "--", "seshagy", "--report-agent", "--pane", ""})
	})
	if err != nil || !strings.Contains(out, "c\t%9\t/tmp/project") {
		t.Fatalf("hidden dynamic completion output=%q err=%v", out, err)
	}
	select {
	case call := <-unexpected:
		t.Fatalf("dynamic completion made forbidden backend call: %s", call)
	default:
	}
	for _, path := range []string{configHome, stateHome, cacheHome} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("hidden dynamic completion wrote %s: %v", path, err)
		}
	}
}

func TestHelpCommandFormsExistInCompletionMetadata(t *testing.T) {
	lines := strings.Split(helpText(), "\n")
	for lineIndex, line := range lines {
		if !strings.HasPrefix(line, "  seshagy ") {
			continue
		}
		helpForm := line
		for i := lineIndex + 1; i < len(lines) && !strings.HasPrefix(lines[i], "  seshagy "); i++ {
			continuation := strings.TrimSpace(lines[i])
			if strings.HasPrefix(continuation, "--") || strings.HasPrefix(continuation, "[") {
				helpForm += " " + continuation
			}
		}
		usage := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "seshagy "))
		words := strings.Fields(usage)
		if len(words) == 0 || words[0] == "open" {
			continue
		}
		command := &completion.Root
		positionals := 0
		path := "seshagy"
		for _, raw := range words {
			if command.Terminal && positionals >= len(command.Positionals) {
				break
			}
			word := strings.Trim(raw, "[]()")
			if word == "" {
				continue
			}
			if strings.HasPrefix(word, "--") {
				next := completion.FindChild(command, word)
				if next == nil {
					break
				}
				command = next
				path += " " + command.Name
				positionals = 0
				continue
			}
			if positionals < len(command.Positionals) {
				positionals++
				continue
			}
			next := completion.FindChild(command, word)
			if next == nil {
				t.Errorf(
					"help path %s has nested token %q absent from completion metadata",
					path,
					word,
				)
				break
			}
			command = next
			path += " " + command.Name
			positionals = 0
		}
		for _, name := range regexp.MustCompile(`--[a-z][a-z-]*`).FindAllString(helpForm, -1) {
			if name != "--ephemeral" && name != command.Name &&
				!commandHasFlag(command, name) && completion.FindChild(command, name) == nil {
				t.Errorf("help path %s exposes flag %s absent from completion metadata", path, name)
			}
		}
	}
}

func commandHasFlag(command *completion.Command, name string) bool {
	for _, flag := range command.Flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func TestEveryEmittedKeybindFormHasParserParity(t *testing.T) {
	tests := []struct {
		name  string
		words []string
	}{
		{"install target", []string{"seshagy", "keybind", "install", ""}},
		{"uninstall target", []string{"seshagy", "keybind", "uninstall", ""}},
		{"tmux flags", []string{"seshagy", "keybind", "install", "tmux", "--"}},
		{"tmux modes", []string{"seshagy", "keybind", "install", "tmux", "--mode", ""}},
		{"herdr flags", []string{"seshagy", "keybind", "install", "herdr", "--"}},
		{"herdr modes", []string{"seshagy", "keybind", "install", "herdr", "--mode", ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := completion.Resolve(t.Context(), test.words, nil)
			if len(result.Candidates) == 0 {
				t.Fatal("completion emitted no candidates")
			}
			base := append([]string(nil), test.words[1:len(test.words)-1]...)
			for _, candidate := range result.Candidates {
				args := append(append([]string(nil), base...), candidate.Value)
				switch candidate.Value {
				case "--key":
					args = append(args, "s")
				case "--mode":
					if containsArg(args, "herdr") {
						args = append(args, "pane")
					} else {
						args = append(args, "popup")
					}
				case "--width", "--height":
					args = append(args, "80%")
				}
				args = removeExactArg(args, "--ephemeral")
				if len(args) == 0 || args[0] != "keybind" {
					t.Fatalf("unexpected completed argv: %v", args)
				}
				if _, err := parseKeybindCommand(args[1:]); err != nil {
					t.Errorf(
						"candidate %q produced parser-invalid argv %v: %v",
						candidate.Value,
						args,
						err,
					)
				}
			}
		})
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func removeExactArg(args []string, remove string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != remove {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func TestMalformedHiddenCompletionIsSilent(t *testing.T) {
	out, err := captureStdout(t, func() error { return run([]string{"__complete", "bad"}) })
	if err != nil || out != "" {
		t.Fatalf("malformed hidden request output=%q err=%v", out, err)
	}
}
