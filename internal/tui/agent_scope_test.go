package tui

import (
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// agentItems builds KindAgent items across the given tmux sessions so the
// current-session scope filter has something to narrow.
func agentItems(sessions ...string) []sessionmgr.Item {
	items := make([]sessionmgr.Item, len(sessions))
	for i, s := range sessions {
		items[i] = sessionmgr.Item{
			Kind:      sessionmgr.KindAgent,
			Name:      "pi",
			AgentName: "pi",
			Session:   s,
			Location:  s + ":1.1",
			PaneID:    "%" + s,
		}
	}
	return items
}

// TestAgentsScopeToggleFiltersByCurrentSession proves the 'o' toggle narrows
// the Agents list to the current tmux session and composes with the search
// query.
func TestAgentsScopeToggleFiltersByCurrentSession(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.currentSession = "seshagy"
	m.items = agentItems("seshagy", "seshagy", "dotfiles", "monorepo")
	m.agentsCurrentOnly = false

	// No scope: all agents visible.
	if got := len(m.visibleItems()); got != 4 {
		t.Fatalf("visible without scope = %d, want 4", got)
	}

	// Scope on: only the two agents in the current session survive.
	m.agentsCurrentOnly = true
	vis := m.visibleItems()
	if len(vis) != 2 {
		t.Fatalf("visible with scope = %d, want 2", len(vis))
	}
	for _, it := range vis {
		if it.Session != "seshagy" {
			t.Fatalf("scoped item session = %q, want seshagy", it.Session)
		}
	}

	// Scope composes with a search query.
	m.query = "dotfiles"
	if got := len(m.visibleItems()); got != 0 {
		t.Fatalf("scoped + query = %d, want 0 (dotfiles not in current session)", got)
	}
	m.query = ""

	// Scope off restores all.
	m.agentsCurrentOnly = false
	if got := len(m.visibleItems()); got != 4 {
		t.Fatalf("visible after scope off = %d, want 4", got)
	}
}

// TestAgentsScopeEmptyCurrentSessionShowsAll confirms that toggling the scope
// on while not in a tmux session (currentSession empty) does not silently hide
// everything: the filter stays inert until a session is known.
func TestAgentsScopeEmptyCurrentSessionShowsAll(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.currentSession = ""
	m.items = agentItems("seshagy", "dotfiles")
	m.agentsCurrentOnly = true

	if got := len(m.visibleItems()); got != 2 {
		t.Fatalf("visible with empty currentSession = %d, want 2", got)
	}
}

// TestAgentsScopeDoesNotAffectModeAll is the critical non-regression guard:
// the current-session scope filter must only narrow ModeAgents. In ModeAll,
// where agents are mixed with sessions and directories, every item must stay
// visible regardless of the scope toggle.
func TestAgentsScopeDoesNotAffectModeAll(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAll
	m.currentSession = "seshagy"
	m.agentsCurrentOnly = true
	m.items = []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "seshagy", Session: "seshagy"},
		{Kind: sessionmgr.KindAgent, Name: "pi", AgentName: "pi", Session: "dotfiles"},
		{Kind: sessionmgr.KindZoxide, Name: "~/code"},
	}

	vis := m.visibleItems()
	if len(vis) != 3 {
		t.Fatalf("visibleItems() in ModeAll = %d, want 3 (scope must not filter)", len(vis))
	}
}

// TestAgentsScopeKeyOnlyActsInAgentsTab drives the 'o' key and asserts it
// toggles the scope boolean in place (no source switch) in the Agents tab and
// is a no-op elsewhere.
func TestAgentsScopeKeyOnlyActsInAgentsTab(t *testing.T) {
	m := New()
	m.config.TypeFirst.Enabled = false
	m.source = sessionmgr.ModeAgents
	m.currentSession = "seshagy"
	m.items = agentItems("seshagy", "dotfiles")
	m.agentsCurrentOnly = false

	model, _ := m.handleKey(keyMsg("o"))
	got := model.(Model)
	if !got.agentsCurrentOnly {
		t.Fatal("agentsCurrentOnly = false after pressing 'o' in Agents tab")
	}
	if got.source != sessionmgr.ModeAgents {
		t.Fatalf("source = %v, want ModeAgents (no tab switch)", got.source)
	}
	if got := len(got.visibleItems()); got != 1 {
		t.Fatalf("visible after 'o' = %d, want 1 (current session only)", got)
	}

	// 'o' toggles back off.
	model, _ = got.handleKey(keyMsg("o"))
	got = model.(Model)
	if got.agentsCurrentOnly {
		t.Fatal("agentsCurrentOnly = true after second 'o', want false")
	}

	// 'o' is a no-op outside the Agents tab.
	got.source = sessionmgr.ModeSessions
	before := got.agentsCurrentOnly
	model, _ = got.handleKey(keyMsg("o"))
	got = model.(Model)
	if got.agentsCurrentOnly != before {
		t.Fatalf("'o' outside Agents tab mutated scope: %v -> %v", before, got.agentsCurrentOnly)
	}
}
