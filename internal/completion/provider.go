package completion

import (
	"context"
	"sort"
	"strings"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type Candidate struct {
	Value       string
	Description string
}

type Provider interface {
	Values(context.Context, ValueKind) ([]Candidate, error)
}

type RuntimeProvider struct {
	mux sessionmgr.Multiplexer
}

func NewRuntimeProvider() RuntimeProvider { return RuntimeProvider{mux: sessionmgr.Detect()} }

func NewRuntimeProviderWithMux(mux sessionmgr.Multiplexer) RuntimeProvider {
	return RuntimeProvider{mux: mux}
}

func (p RuntimeProvider) Values(ctx context.Context, kind ValueKind) ([]Candidate, error) {
	if kind == ValueSessionLine {
		cfg, err := appconfig.Load()
		if err != nil {
			return nil, err
		}
		items, err := sessionmgr.CompletionSessions(ctx, p.mux)
		if err != nil {
			return nil, err
		}
		return sessionLineCandidates(items, cfg.IconSet(), p.mux.Terms().SessionNoun), nil
	}

	var panes []sessionmgr.CompletionPane
	var err error
	if kind == ValueAgent || kind == ValueSource {
		panes, err = sessionmgr.CompletionAgentPanes(ctx, p.mux)
	} else {
		panes, err = sessionmgr.CompletionPanes(ctx, p.mux)
	}
	if err != nil {
		return nil, err
	}
	values := make([]Candidate, 0, len(panes)+len(integrations.Available()))
	for _, pane := range panes {
		var value, description string
		switch kind {
		case ValuePane:
			value, description = pane.ID, pane.Cwd
			if description == "" {
				description = "pane"
			}
		case ValueDirectory:
			value, description = pane.Cwd, "pane directory"
		case ValueAgent:
			value, description = pane.Agent, "detected agent"
		case ValueSource:
			value, description = pane.Source, "observed source"
		default:
			return nil, nil
		}
		if value != "" {
			values = append(values, Candidate{Value: value, Description: description})
		}
	}
	if kind == ValueAgent || kind == ValueSource {
		for _, name := range integrations.Available() {
			value := name
			if kind == ValueSource {
				value = "seshagy:" + name
			}
			values = append(values, Candidate{Value: value, Description: "integration"})
		}
	}
	return uniqueCandidates(values), nil
}

func sessionLineCandidates(
	items []sessionmgr.Item,
	icons sessionmgr.IconSet,
	sessionNoun string,
) []Candidate {
	renderedCounts := make(map[string]int, len(items))
	targetCounts := make(map[string]int, len(items))
	for _, item := range items {
		renderedCounts[renderedSessionLine(item, icons)]++
		targetCounts[item.ActionTarget()]++
	}

	values := make([]Candidate, 0, len(items))
	for _, item := range items {
		description := item.Name
		if description == "" {
			description = sessionNoun
		}
		rendered := renderedSessionLine(item, icons)
		// Prefer the familiar fzf/rendered value only when the resolver can map
		// it to exactly this session. A rendered label that is also another
		// session's opaque target would resolve by raw-target precedence.
		renderedIsSafe := renderedCounts[rendered] == 1 &&
			(targetCounts[rendered] == 0 ||
				(targetCounts[rendered] == 1 && item.ActionTarget() == rendered))
		if renderedIsSafe {
			values = append(values, Candidate{Value: rendered, Description: description})
			continue
		}
		if target := item.ActionTarget(); target != "" && targetCounts[target] == 1 {
			values = append(values, Candidate{Value: target, Description: description})
		}
	}
	return uniqueCandidates(values)
}

func renderedSessionLine(item sessionmgr.Item, icons sessionmgr.IconSet) string {
	return strings.TrimSpace(sessionmgr.StripANSI(sessionmgr.FormatLineWithIcons(item, icons)))
}

func uniqueCandidates(values []Candidate) []Candidate {
	byValue := make(map[string]Candidate, len(values))
	for _, candidate := range values {
		if candidate.Value == "" {
			continue
		}
		if old, exists := byValue[candidate.Value]; !exists || old.Description == "" {
			byValue[candidate.Value] = candidate
		}
	}
	values = values[:0]
	for _, candidate := range byValue {
		values = append(values, candidate)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Value < values[j].Value })
	return values
}

func filterCandidates(values []Candidate, prefix string) []Candidate {
	filtered := values[:0]
	for _, candidate := range values {
		if strings.HasPrefix(candidate.Value, prefix) {
			filtered = append(filtered, candidate)
		}
	}
	return uniqueCandidates(filtered)
}
