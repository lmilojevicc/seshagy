package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	kimiConfigBlockBegin = "# >>> seshagy kimi integration"
	kimiConfigBlockEnd   = "# <<< seshagy kimi integration"
)

var kimiHookEvents = []struct {
	event  string
	action string
}{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"SubagentStart", "working"},
	{"PreCompact", "working"},
	{"PermissionRequest", "blocked"},
	{"PermissionResult", "working"},
	{"Stop", "idle"},
	{"Interrupt", "idle"},
	{"SessionEnd", "release"},
}

func installKimi(binaryPath string) ([]string, error) {
	dir := kimiDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("kimi code config directory not found at %s", dir)
	}
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	if err := writeShellHook(TargetKimi, hookPath, binaryPath); err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "config.toml")
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	updated := buildKimiConfigWithHooks(string(existing), hookPath)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("installed Kimi Code hook to %s", hookPath),
		fmt.Sprintf("updated %s", configPath),
	}, nil
}

func uninstallKimi() ([]string, error) {
	dir := kimiDir()
	hookPath := filepath.Join(dir, "hooks", shellHookName)
	removed, err := removeFile(hookPath)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Kimi Code hook", hookPath, removed)
	configPath := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return messages, nil
		}
		return nil, err
	}
	updated := removeKimiConfigBlock(string(data))
	if updated != string(data) {
		if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("removed seshagy hook entries from %s", configPath))
	}
	return messages, nil
}

func buildKimiConfigWithHooks(content, hookPath string) string {
	result := strings.TrimRight(removeKimiConfigBlock(content), "\n")
	if result != "" {
		result += "\n\n"
	}
	result += kimiConfigBlockBegin + "\n"
	for _, hook := range kimiHookEvents {
		result += kimiHookTable(hook.event, hookPath, hook.action)
	}
	result += kimiConfigBlockEnd + "\n"
	return result
}

func kimiHookTable(event, hookPath, action string) string {
	command := shellHookCommand(hookPath, TargetKimi, action)
	return fmt.Sprintf(
		"[[hooks]]\nevent = %s\ncommand = %s\ntimeout = 10\n\n",
		tomlBasicString(event),
		tomlBasicString(command),
	)
}

func removeKimiConfigBlock(content string) string {
	trailingNewline := strings.HasSuffix(content, "\n")
	var lines []string
	inBlock := false
	removedBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == kimiConfigBlockBegin {
			inBlock = true
			removedBlock = true
			continue
		}
		if inBlock {
			if trimmed == kimiConfigBlockEnd {
				inBlock = false
			}
			continue
		}
		lines = append(lines, line)
	}
	if !removedBlock {
		return content
	}
	result := strings.Join(lines, "\n")
	if trailingNewline && result != "" {
		result += "\n"
	}
	return result
}

func tomlBasicString(value string) string {
	return strconv.Quote(value)
}

func shellHookCommand(hookPath string, target Target, action string) string {
	return "bash " + shellQuoteLiteral(hookPath) + " " + string(target) + " " + action
}
