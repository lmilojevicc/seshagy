package sessionmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const maxAgentDisplayLabelLen = 128

type AgentLabelEntry struct {
	Label    string `json:"label"`
	ForAgent string `json:"for_agent"`
}

type AgentLabelsStore struct {
	entries map[string]AgentLabelEntry
}

func agentLabelsPath() string {
	return filepath.Join(xdg.StateHome(), "seshagy", "agent-labels.json")
}

func LoadAgentLabels() (*AgentLabelsStore, error) {
	path := agentLabelsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentLabelsStore{entries: map[string]AgentLabelEntry{}}, nil
		}
		return nil, err
	}
	var raw map[string]AgentLabelEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("agent labels: %w", err)
	}
	if raw == nil {
		raw = map[string]AgentLabelEntry{}
	}
	return &AgentLabelsStore{entries: raw}, nil
}

func (s *AgentLabelsStore) Save() error {
	path := agentLabelsPath()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data)
}

func (s *AgentLabelsStore) Set(paneID, label, forAgent string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return s.Clear(paneID)
	}
	if len([]rune(label)) > maxAgentDisplayLabelLen {
		return fmt.Errorf("agent label must be at most %d characters", maxAgentDisplayLabelLen)
	}
	paneID = strings.TrimSpace(paneID)
	forAgent = strings.TrimSpace(forAgent)
	if paneID == "" || forAgent == "" {
		return fmt.Errorf("pane id and agent name are required")
	}
	if s.entries == nil {
		s.entries = map[string]AgentLabelEntry{}
	}
	s.entries[paneID] = AgentLabelEntry{Label: label, ForAgent: forAgent}
	return s.Save()
}

func (s *AgentLabelsStore) Clear(paneID string) error {
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return fmt.Errorf("pane id is required")
	}
	if len(s.entries) == 0 {
		return nil
	}
	delete(s.entries, paneID)
	return s.Save()
}

func (s *AgentLabelsStore) Get(paneID, agentName string) string {
	entry, ok := s.entries[paneID]
	if !ok || entry.ForAgent != agentName {
		return ""
	}
	return entry.Label
}

func (s *AgentLabelsStore) Prune(knownPaneIDs []string) error {
	if len(s.entries) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(knownPaneIDs))
	for _, paneID := range knownPaneIDs {
		known[paneID] = struct{}{}
	}
	changed := false
	for paneID := range s.entries {
		if _, ok := known[paneID]; !ok {
			delete(s.entries, paneID)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.Save()
}

func SetAgentDisplayName(paneID, label, forAgent string) error {
	store, err := LoadAgentLabels()
	if err != nil {
		return err
	}
	return store.Set(paneID, label, forAgent)
}

func ClearAgentDisplayName(paneID string) error {
	store, err := LoadAgentLabels()
	if err != nil {
		return err
	}
	return store.Clear(paneID)
}

func ApplyAgentLabels(items []Item, sessionFilter string) {
	store, err := LoadAgentLabels()
	if err != nil || store == nil {
		return
	}
	paneIDs := make([]string, 0, len(items))
	for i := range items {
		if items[i].Kind != KindAgent {
			continue
		}
		paneIDs = append(paneIDs, items[i].PaneID)
		if label := store.Get(items[i].PaneID, items[i].AgentName); label != "" {
			items[i].AgentDisplayName = label
		}
	}
	if sessionFilter == "" && len(paneIDs) > 0 {
		_ = store.Prune(paneIDs)
	}
}
