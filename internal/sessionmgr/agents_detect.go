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
	command = filepath.Base(command)
	command = strings.TrimSuffix(command, ".exe")
	command = strings.TrimSuffix(command, ".cmd")
	command = strings.TrimSuffix(command, ".bat")
	command = strings.TrimSuffix(command, ".ps1")
	command = strings.TrimSuffix(command, ".js")
	commandLower := strings.ToLower(command)
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
		if !isShell(commandLower) {
			return "pi"
		}
	}

	// isRuntime gates the other title fallbacks (claude, opencode, codex, etc.).
	// Includes interpreters/runtimes that could appear as pane_current_command
	// without being agents themselves.
	isRuntime := func(name string) bool {
		switch name {
		case "sh", "bash", "zsh", "fish", "tmux",
			"node", "bun", "python", "python3",
			"cmd", "powershell", "pwsh":
			return true
		}
		return false
	}

	switch commandLower {
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

	// Wildcard matching for agents with variant binaries (e.g. codex-local, droid-agent).
	if strings.HasPrefix(commandLower, "codex-") || strings.HasPrefix(commandLower, "codex_") {
		return "codex"
	}
	if strings.HasPrefix(commandLower, "droid-") || strings.HasPrefix(commandLower, "droid_") {
		return "droid"
	}

	if isRuntime(commandLower) {
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
