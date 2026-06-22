package sessionmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const (
	appStateDir     = "seshagy"
	agentLabelsFile = "agent-labels.json"
)

// AgentLabelStore persists display-only aliases for agent panes, keyed by
// agentType:sessionName. Keying on the agent type (rather than the pane id)
// means restarting the same agent in the same session keeps its alias, while
// swapping one agent for another (e.g. pi -> claude) reads as a different
// identity and does not bleed the alias across.
type AgentLabelStore struct {
	Labels map[string]string `json:"labels"`
}

func agentLabelKey(agentType, session string) string {
	return agentType + ":" + session
}

func agentLabelsPath() string {
	return filepath.Join(xdg.StateHome(), appStateDir, agentLabelsFile)
}

// LoadAgentLabels reads the persisted alias store. A missing or unreadable
// file yields an empty store rather than an error so a first run is clean.
func LoadAgentLabels() AgentLabelStore {
	data, err := os.ReadFile(agentLabelsPath())
	if err != nil {
		return AgentLabelStore{Labels: map[string]string{}}
	}
	var store AgentLabelStore
	if err := json.Unmarshal(data, &store); err != nil {
		return AgentLabelStore{Labels: map[string]string{}}
	}
	if store.Labels == nil {
		store.Labels = map[string]string{}
	}
	return store
}

// SaveAgentLabel sets (or clears, when displayName is empty) the alias for the
// given agent type and session, persisting the whole store.
func SaveAgentLabel(agentType, session, displayName string) error {
	store := LoadAgentLabels()
	key := agentLabelKey(agentType, session)
	if strings.TrimSpace(displayName) == "" {
		delete(store.Labels, key)
	} else {
		store.Labels[key] = displayName
	}
	return saveAgentLabels(store)
}

func saveAgentLabels(store AgentLabelStore) error {
	path := agentLabelsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ApplyAgentLabels overlays persisted aliases onto agent items, setting
// AgentDisplayName where an alias exists.
func ApplyAgentLabels(items []Item) []Item {
	store := LoadAgentLabels()
	if len(store.Labels) == 0 {
		return items
	}
	for i := range items {
		if items[i].Kind != KindAgent {
			continue
		}
		if label, ok := store.Labels[agentLabelKey(items[i].AgentName, items[i].Session)]; ok {
			items[i].AgentDisplayName = label
		}
	}
	return items
}
