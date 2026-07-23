package logging

import (
	"context"
	"log/slog"
)

type fieldKind uint8

const (
	fieldString fieldKind = iota
	fieldInt
	fieldDebugID
)

type fieldRule struct {
	kind   fieldKind
	values map[string]struct{}
}

type eventRule struct {
	component Component
	fields    map[string]fieldRule
}

func stringsOnly(values ...string) fieldRule {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return fieldRule{kind: fieldString, values: set}
}

var eventRules = map[Event]eventRule{
	EventAppStart: {
		ComponentApp,
		map[string]fieldRule{
			"backend": stringsOnly(
				"tmux",
				"herdr",
				"none",
			),
			"log_level": stringsOnly("debug", "info", "warn", "error"),
		},
	},
	EventAppStop: {
		ComponentApp,
		map[string]fieldRule{
			"backend": stringsOnly(
				"tmux",
				"herdr",
				"none",
			),
			"result":      stringsOnly("success", "failed"),
			"duration_ms": {kind: fieldInt},
		},
	},
	EventLogLimitReached: {ComponentLogging, map[string]fieldRule{"byte_limit": {kind: fieldInt}}},
	EventLogRetention: {
		ComponentLogging,
		map[string]fieldRule{
			"item_count": {
				kind: fieldInt,
			},
			"skipped_count": {kind: fieldInt},
			"warning_count": {kind: fieldInt},
			"error_class":   errorClassRule(),
		},
	},
	EventSourceLoad: {
		ComponentSession,
		map[string]fieldRule{
			"backend":       backendRule(),
			"source":        sourceRule(),
			"item_count":    {kind: fieldInt},
			"warning_count": {kind: fieldInt},
			"duration_ms":   {kind: fieldInt},
		},
	},
	EventSourceLoadFailed: {
		ComponentSession,
		map[string]fieldRule{
			"backend":     backendRule(),
			"source":      sourceRule(),
			"duration_ms": {kind: fieldInt},
			"error_class": errorClassRule(),
		},
	},
	EventSourceLoadDegraded: {
		ComponentSession,
		map[string]fieldRule{
			"backend":       backendRule(),
			"source":        stringsOnly("all"),
			"failed_source": sourceRule(),
			"error_class":   errorClassRule(),
		},
	},
	EventSessionKill: {
		ComponentSession,
		map[string]fieldRule{
			"backend":     backendRule(),
			"result":      stringsOnly("success", "failed"),
			"duration_ms": {kind: fieldInt},
			"error_class": errorClassRule(),
		},
	},
	EventSessionFocusRestore: {
		ComponentSession,
		map[string]fieldRule{
			"result": stringsOnly(
				"success",
				"failed",
				"skipped",
			),
			"duration_ms":  {kind: fieldInt},
			"workspace_id": {kind: fieldDebugID},
			"error_class":  errorClassRule(),
		},
	},
	EventAgentReport: {
		ComponentAgents,
		map[string]fieldRule{
			"backend": backendRule(),
			"pane_id": {kind: fieldDebugID},
			"state":   stateRule(),
			"seq":     {kind: fieldInt},
			"result":  stringsOnly("applied", "stale", "ignored_backend"),
		},
	},
	EventAgentReportFailed: {ComponentAgents, map[string]fieldRule{
		"backend": backendRule(), "result": stringsOnly("failed"), "error_class": errorClassRule(),
	}},
	EventAgentRelease: {
		ComponentAgents,
		map[string]fieldRule{
			"backend": backendRule(),
			"pane_id": {kind: fieldDebugID},
			"seq":     {kind: fieldInt},
			"result":  stringsOnly("applied", "stale", "cross_source", "ignored_backend"),
		},
	},
	EventAgentReleaseFailed: {ComponentAgents, map[string]fieldRule{
		"backend": backendRule(), "result": stringsOnly("failed"), "error_class": errorClassRule(),
	}},
	EventManifestSweep: {
		ComponentManifest,
		map[string]fieldRule{
			"item_count": {
				kind: fieldInt,
			},
			"skipped_count": {kind: fieldInt},
			"matched_count": {kind: fieldInt},
			"changed_count": {kind: fieldInt},
			"warning_count": {kind: fieldInt},
			"duration_ms":   {kind: fieldInt},
		},
	},
	EventManifestStateChange: {
		ComponentManifest,
		map[string]fieldRule{
			"pane_id": {
				kind: fieldDebugID,
			},
			"agent_type":     agentRule(),
			"previous_state": stateRule(),
			"state":          stateRule(),
		},
	},
	EventIntegrationInstall: {
		ComponentIntegrations,
		map[string]fieldRule{
			"integration": integrationRule(),
			"result":      stringsOnly("success", "failed"),
			"duration_ms": {kind: fieldInt},
			"error_class": errorClassRule(),
		},
	},
	EventIntegrationUninstall: {
		ComponentIntegrations,
		map[string]fieldRule{
			"integration": integrationRule(),
			"result":      stringsOnly("success", "failed"),
			"duration_ms": {kind: fieldInt},
			"error_class": errorClassRule(),
		},
	},
	EventTUIRefreshStale: {
		ComponentTUI,
		map[string]fieldRule{
			"source":             sourceRule(),
			"generation":         {kind: fieldInt},
			"current_generation": {kind: fieldInt},
		},
	},
}

func backendRule() fieldRule { return stringsOnly("tmux", "herdr", "none") }
func sourceRule() fieldRule {
	return stringsOnly("all", "sessions", "zoxide", "fd", "agents", "current-agents")
}

func errorClassRule() fieldRule {
	return stringsOnly(
		"canceled",
		"timeout",
		"permission",
		"not_found",
		"parse",
		"exec",
		"io",
		"invalid",
		"unknown",
	)
}

func stateRule() fieldRule { return stringsOnly("idle", "working", "blocked", "done", "unknown") }

func integrationRule() fieldRule { return stringsOnly("pi", "codex", "claude", "droid", "opencode") }
func agentRule() fieldRule {
	return stringsOnly(
		"pi",
		"codex",
		"claude",
		"droid",
		"opencode",
		"cursor",
		"agy",
		"grok",
		"amp",
		"cline",
		"devin",
		"gemini",
		"hermes",
		"kilo",
		"kimi",
		"kiro",
		"qodercli",
		"antigravity",
		"copilot",
		"unknown",
	)
}

type safetyHandler struct {
	next    slog.Handler
	attrs   []slog.Attr
	invalid bool
}

func newSafetyHandler(next slog.Handler) slog.Handler { return safetyHandler{next: next} }

func (h safetyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return !h.invalid && h.next.Enabled(ctx, level)
}

func (h safetyHandler) Handle(ctx context.Context, record slog.Record) error {
	if h.invalid {
		return nil
	}
	attrs := append([]slog.Attr(nil), h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	resolved := make([]slog.Attr, 0, len(attrs))
	values := make(map[string]slog.Value, len(attrs))
	for _, attr := range attrs {
		clean, ok := resolveSafeAttr(attr)
		if !ok {
			continue
		}
		resolved = append(resolved, clean)
		if _, exists := values[clean.Key]; !exists {
			values[clean.Key] = clean.Value
		}
	}
	eventValue, eventOK := stringValue(values["event"])
	componentValue, componentOK := stringValue(values["component"])
	event := Event(eventValue)
	rule, registered := eventRules[event]
	if !eventOK || !componentOK || !registered || Component(componentValue) != rule.component ||
		record.Message != eventValue {
		return nil
	}
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	seen := make(map[string]struct{}, len(resolved))
	for _, attr := range resolved {
		if _, duplicate := seen[attr.Key]; duplicate {
			continue
		}
		seen[attr.Key] = struct{}{}
		if validBaseAttr(attr) {
			clean.AddAttrs(attr)
			continue
		}
		if attr.Key == "event" || attr.Key == "component" {
			clean.AddAttrs(attr)
			continue
		}
		field, ok := rule.fields[attr.Key]
		if !ok || !validField(attr.Value, field, record.Level) {
			continue
		}
		clean.AddAttrs(attr)
	}
	return h.next.Handle(ctx, clean)
}

func resolveSafeAttr(attr slog.Attr) (slog.Attr, bool) {
	if attr.Key == "" {
		return slog.Attr{}, false
	}
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindAny || attr.Value.Kind() == slog.KindGroup {
		return slog.Attr{}, false
	}
	return attr, true
}

func (h safetyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	copyAttrs := append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return safetyHandler{next: h.next, attrs: copyAttrs, invalid: h.invalid}
}

func (h safetyHandler) WithGroup(_ string) slog.Handler {
	return safetyHandler{next: h.next, attrs: h.attrs, invalid: true}
}

func validBaseAttr(attr slog.Attr) bool {
	value := attr.Value
	switch attr.Key {
	case "schema_version":
		return value.Kind() == slog.KindInt64 && value.Int64() == SchemaVersion
	case "run_id", "app_version":
		return value.Kind() == slog.KindString && value.String() != ""
	default:
		return false
	}
}

func validField(value slog.Value, rule fieldRule, level slog.Level) bool {
	switch rule.kind {
	case fieldString:
		text, ok := stringValue(value)
		if !ok {
			return false
		}
		_, ok = rule.values[text]
		return ok
	case fieldInt:
		return value.Kind() == slog.KindInt64 && value.Int64() >= 0
	case fieldDebugID:
		return level == slog.LevelDebug && value.Kind() == slog.KindString && value.String() != ""
	default:
		return false
	}
}

func stringValue(value slog.Value) (string, bool) {
	return value.String(), value.Kind() == slog.KindString
}
