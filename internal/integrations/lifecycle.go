package integrations

type lifecycleHook struct {
	event  string
	action string
}

// claudeLifecycleHooks follows Claude Code hook events and Kimi-style state mapping.
var claudeLifecycleHooks = []lifecycleHook{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"PostToolUseFailure", "working"},
	{"SubagentStop", "working"},
	{"PreCompact", "working"},
	{"PermissionRequest", "blocked"},
	{"Stop", "idle"},
	{"SessionEnd", "release"},
}

// droidLifecycleHooks matches Factory Droid settings hook events.
var droidLifecycleHooks = []lifecycleHook{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"Notification", "blocked"},
	{"SubagentStop", "working"},
	{"PreCompact", "working"},
	{"Stop", "idle"},
	{"SessionEnd", "release"},
}

// qodercliLifecycleHooks matches Qoder CLI settings hook events.
var qodercliLifecycleHooks = []lifecycleHook{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"PostToolUseFailure", "working"},
	{"SubagentStart", "working"},
	{"SubagentStop", "working"},
	{"PreCompact", "working"},
	{"Notification", "blocked"},
	{"PermissionRequest", "blocked"},
	{"Stop", "idle"},
	{"SessionEnd", "release"},
}

// codexLifecycleHooks matches Codex hooks.json events.
var codexLifecycleHooks = []lifecycleHook{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PermissionRequest", "blocked"},
	{"Stop", "idle"},
	{"SessionEnd", "release"},
}

// copilotLifecycleHooks matches GitHub Copilot CLI settings hook events.
var copilotLifecycleHooks = []lifecycleHook{
	{"SessionStart", "session"},
	{"UserPromptSubmit", "working"},
	{"PreToolUse", "working"},
	{"PostToolUse", "working"},
	{"PostToolUseFailure", "working"},
	{"Stop", "idle"},
	{"SessionEnd", "release"},
}

// copilotStaleLifecycleHooks removes legacy Herdr-style Copilot lifecycle entries.
var copilotStaleLifecycleHooks = []string{
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"Stop",
	"agentStop",
	"SessionEnd",
	"notification",
	"sessionStart",
}

// cursorLifecycleHooks matches Cursor Agent hooks.json events.
var cursorLifecycleHooks = []lifecycleHook{
	{"sessionStart", "session"},
	{"beforeSubmitPrompt", "working"},
	{"beforeShellExecution", "working"},
	{"beforeMCPExecution", "working"},
	{"stop", "idle"},
	{"sessionEnd", "release"},
}

// cursorStaleLifecycleHooks removes legacy session-only Cursor hook entries.
var cursorStaleLifecycleHooks = []lifecycleHook{
	{"beforeSubmitPrompt", "session"},
	{"beforeShellExecution", "session"},
	{"beforeMCPExecution", "session"},
	{"stop", "session"},
	{"sessionEnd", "session"},
}

func nestedLifecycleMatcher(event string, matcherStar bool) string {
	if matcherStar && event == "SessionStart" {
		return "*"
	}
	return ""
}
