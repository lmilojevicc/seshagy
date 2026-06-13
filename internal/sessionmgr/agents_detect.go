package sessionmgr

import (
	"path/filepath"
	"strings"
)

// detectAgentName is the legacy fallback used by ParseAgents when a pane has no
// hook-reported @agent_name. It infers an agent identity from the pane's
// foreground command and title. Hook-reported metadata always takes priority;
// this only runs when that metadata is absent.
func detectAgentName(command, title string) string {
	rawCommand := command
	basename := normalizeAgentLookupName(filepath.Base(command))
	titleLower := strings.ToLower(title)

	// Match bash's is_shell_command: only actual interactive shells, not runtimes.
	// This gates the π/pi title fallback — node/bun/python are NOT shells here.
	isShell := func(name string) bool {
		switch name {
		case "sh", "bash", "zsh", "fish", "tmux":
			return true
		}
		return false
	}

	// Match bash behavior: π title check is gated by !isShell, and comes before
	// command-based matching. A pane running "node" with title "π - foo" IS detected
	// as pi, but a pane running "zsh" with title "π - foo" is NOT.
	if strings.HasPrefix(titleLower, "π") || strings.HasPrefix(titleLower, "pi ") {
		if !isShell(basename) {
			return "pi"
		}
	}

	if name := agentNameFromBasename(basename); name != "" {
		return name
	}
	if name := agentNameFromBasenamePrefix(basename); name != "" {
		return name
	}

	// Runtimes and unrecognized binaries may wrap the real agent in argv or path.
	if name := agentNameFromWrappedCommand(rawCommand); name != "" {
		return name
	}

	if isRuntime(basename) {
		return ""
	}

	if strings.Contains(titleLower, "claude code") {
		return "claude"
	}
	if strings.Contains(titleLower, "opencode") {
		return "opencode"
	}
	if strings.HasPrefix(titleLower, "codex") || strings.HasPrefix(titleLower, "codex -") {
		return "codex"
	}
	if strings.HasPrefix(titleLower, "droid") || strings.HasPrefix(titleLower, "droid -") {
		return "droid"
	}
	if strings.HasPrefix(titleLower, "gemini") || strings.HasPrefix(titleLower, "gemini -") {
		return "gemini"
	}
	if strings.Contains(titleLower, "cursor") {
		return "cursor"
	}
	if strings.Contains(titleLower, "grok") {
		return "grok"
	}

	return ""
}

func agentNameFromBasename(basename string) string {
	switch basename {
	case "pi":
		return "pi"
	case "claude", "claude-code":
		return "claude"
	case "opencode", "open-code":
		return "opencode"
	case "codex":
		return "codex"
	case "droid":
		return "droid"
	case "gemini":
		return "gemini"
	case "cursor", "cursor-agent":
		return "cursor"
	case "agy", "antigravity", "antigravity-cli":
		return "agy"
	case "cline":
		return "cline"
	case "copilot", "github-copilot", "ghcs":
		return "copilot"
	case "kimi", "kimi-code":
		return "kimi"
	case "kiro", "kiro-cli":
		return "kiro"
	case "amp", "amp-local":
		return "amp"
	case "grok", "grok-build":
		return "grok"
	case "hermes", "hermes-agent":
		return "hermes"
	case "kilo", "kilo-code":
		return "kilo"
	case "qodercli", "qoderclicn", "qoder", "qodercn":
		return "qodercli"
	}
	return ""
}

func agentNameFromBasenamePrefix(basename string) string {
	if strings.HasPrefix(basename, "codex-") || strings.HasPrefix(basename, "codex_") {
		return "codex"
	}
	if strings.HasPrefix(basename, "droid-") || strings.HasPrefix(basename, "droid_") {
		return "droid"
	}
	if strings.HasPrefix(basename, "grok-") || strings.HasPrefix(basename, "grok_") {
		return "grok"
	}
	return ""
}

func agentNameFromWrappedCommand(command string) string {
	tokens := commandTokens(command)
	if len(tokens) == 0 {
		return ""
	}

	runtime := normalizeAgentLookupName(filepath.Base(tokens[0]))
	if isRuntime(runtime) {
		if name := unwrapRuntimeArgv(runtime, tokens); name != "" {
			return name
		}
	}

	for _, token := range tokens {
		if !looksLikeAgentPath(token) {
			continue
		}
		if name := agentNameFromPathToken(token); name != "" {
			return name
		}
	}
	return ""
}

func looksLikeAgentPath(token string) bool {
	token = strings.Trim(token, `"'`)
	return strings.Contains(token, "/") || strings.Contains(token, "\\")
}

func unwrapRuntimeArgv(runtime string, tokens []string) string {
	switch runtime {
	case "node", "bun":
		return scriptArgAgentName(tokens, []string{"-e", "--eval", "-p", "--print"}, nil)
	case "python", "python3":
		return scriptArgAgentName(tokens, []string{"-c"}, []string{"-m"})
	case "sh", "bash", "zsh", "fish":
		return scriptArgAgentName(tokens, []string{"-c"}, nil)
	}
	return ""
}

func scriptArgAgentName(tokens []string, evalFlags, moduleFlags []string) string {
	for i := 1; i < len(tokens); i++ {
		arg := tokens[i]
		if arg == "--" {
			if i+1 < len(tokens) {
				return agentNameFromPathToken(tokens[i+1])
			}
			return ""
		}
		if flagMatches(arg, evalFlags) || flagMatches(arg, moduleFlags) {
			return ""
		}
		if strings.HasPrefix(arg, "-") {
			if optionTakesValue(arg) && i+1 < len(tokens) {
				i++
			}
			continue
		}
		return agentNameFromPathToken(arg)
	}
	return ""
}

func flagMatches(arg string, flags []string) bool {
	for _, flag := range flags {
		if arg == flag {
			return true
		}
		if strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") &&
			strings.HasPrefix(arg, flag) && len(arg) > len(flag) {
			return true
		}
		if strings.HasPrefix(flag, "--") {
			if rest, ok := strings.CutPrefix(arg, flag); ok && strings.HasPrefix(rest, "=") {
				return true
			}
		}
	}
	return false
}

func optionTakesValue(arg string) bool {
	switch arg {
	case "-r", "--require", "--loader", "--import", "--experimental-loader",
		"--inspect-port", "-W", "-X", "-S", "-L", "-o":
		return true
	}
	return false
}

func agentNameFromPathToken(token string) string {
	token = strings.Trim(token, `"'`)
	if token == "" || strings.HasPrefix(token, "-") {
		return ""
	}
	if !looksLikeAgentPath(token) {
		return ""
	}
	basename := normalizeAgentLookupName(filepath.Base(token))
	if name := agentNameFromBasename(basename); name != "" {
		return name
	}
	return agentNameFromBasenamePrefix(basename)
}

func commandTokens(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	var tokens []string
	for len(command) > 0 {
		command = strings.TrimLeft(command, " \t")
		if command == "" {
			break
		}
		if token, rest, ok := commandTextToken(command); ok {
			tokens = append(tokens, token)
			command = rest
		} else {
			break
		}
	}
	return tokens
}

func commandTextToken(input string) (token, rest string, ok bool) {
	if input == "" {
		return "", "", false
	}
	first := input[0]
	if first == '"' || first == '\'' {
		if end := strings.IndexByte(input[1:], first); end >= 0 {
			return input[1 : 1+end], input[1+end+1:], true
		}
		return input[1:], "", true
	}
	if idx := strings.IndexAny(input, " \t"); idx >= 0 {
		return input[:idx], input[idx:], true
	}
	return input, "", true
}

func normalizeAgentLookupName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".ps1", ".js"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func isRuntime(name string) bool {
	switch name {
	case "sh", "bash", "zsh", "fish", "tmux",
		"node", "bun", "python", "python3",
		"cmd", "powershell", "pwsh":
		return true
	}
	return false
}
