package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// isQuitCmd reports whether cmd is the tea.Quit command. Only call it when the
// key under test is expected to quit; invoking other commands would run them.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestSigilSelectsScopeOnEmptyQuery(t *testing.T) {
	tests := []struct {
		sigil string
		want  scope
	}{
		{"@", scopeAgents},
		{"!", scopeAttention},
		{"$", scopeSessions},
		{"*", scopeDirs},
		{"~", scopeZoxide},
		{"/", scopeFind},
		{"#", scopeHosts},
	}
	for _, tt := range tests {
		m := newTestModel(t)
		m, _ = press(t, m, tt.sigil)
		if m.scope != tt.want {
			t.Fatalf("typed %q: scope = %v, want %v", tt.sigil, m.scope, tt.want)
		}
		if m.query != "" {
			t.Fatalf("typed %q: query = %q, want empty", tt.sigil, m.query)
		}
		// The same sigil again is idempotent, and a different one switches.
		m, _ = press(t, m, tt.sigil)
		if m.scope != tt.want || m.query != "" {
			t.Fatalf("retyped %q: scope = %v query = %q", tt.sigil, m.scope, m.query)
		}
		m, _ = press(t, m, "$")
		if tt.want != scopeSessions && m.scope != scopeSessions {
			t.Fatalf("switching with $ from %v: scope = %v", tt.want, m.scope)
		}
	}
}

func TestSigilInsertedMidQuery(t *testing.T) {
	m := newTestModel(t)
	m, _ = press(t, m, "a")
	m, _ = press(t, m, "@")
	if m.query != "a@" {
		t.Fatalf("query = %q, want %q", m.query, "a@")
	}
	if m.scope != scopeAll {
		t.Fatalf("scope = %v, want scopeAll", m.scope)
	}
}

func TestTypingAppendsToQueryAndBackspaceDeletes(t *testing.T) {
	m := newTestModel(t)
	m, _ = press(t, m, "claude")
	m, _ = press(t, m, "space")
	m, _ = press(t, m, "code")
	if m.query != "claude code" {
		t.Fatalf("query = %q", m.query)
	}
	m, _ = press(t, m, "backspace")
	if m.query != "claude cod" {
		t.Fatalf("query after backspace = %q", m.query)
	}
}

func TestBackspaceOnEmptyQueryClearsScope(t *testing.T) {
	m := newTestModel(t)
	m, _ = press(t, m, "@")
	if m.scope != scopeAgents {
		t.Fatalf("scope = %v, want scopeAgents", m.scope)
	}
	m, _ = press(t, m, "backspace")
	if m.scope != scopeAll {
		t.Fatalf("scope after backspace = %v, want scopeAll", m.scope)
	}
}

func TestEscClearsQueryAndScopeThenQuits(t *testing.T) {
	m := newTestModel(t)
	m, _ = press(t, m, "@")
	m, _ = press(t, m, "pi")
	model, cmd := m.Update(escMsg())
	m = model.(Model)
	if m.query != "" || m.scope != scopeAll {
		t.Fatalf("esc: query = %q scope = %v, want both cleared", m.query, m.scope)
	}
	if isQuitCmd(cmd) {
		t.Fatal("esc with an active filter must not quit")
	}
	_, cmd = m.Update(escMsg())
	if !isQuitCmd(cmd) {
		t.Fatal("esc on an empty query and scope must quit")
	}
}

func TestCtrlCQuitsWhileRenaming(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{{Kind: sessionmgr.KindSession, Name: "demo"}})
	m, _ = press(t, m, "ctrl+r")
	if !m.renameActive {
		t.Fatal("ctrl+r did not start rename")
	}
	staged := m
	staged.killArmed = true
	_, cmd := staged.Update(keyMsg(tea.KeyCtrlC))
	if !isQuitCmd(cmd) {
		t.Fatal("ctrl+c must quit even in rename/kill modes")
	}
}

func TestTabAndShiftTabCycleScopes(t *testing.T) {
	m := newTestModel(t)
	for i := 1; i <= int(scopeCount); i++ {
		m, _ = press(t, m, "tab")
		if want := scope(i % int(scopeCount)); m.scope != want {
			t.Fatalf("tab #%d: scope = %v, want %v", i, m.scope, want)
		}
	}
	m, _ = press(t, m, "shift+tab")
	if m.scope != scopeHosts {
		t.Fatalf("shift+tab from all: scope = %v, want scopeHosts", m.scope)
	}
}

func TestCtrlUClearsQueryAndCtrlWDeletesWord(t *testing.T) {
	m := newTestModel(t)
	m, _ = press(t, m, "claude code")
	m, _ = press(t, m, "ctrl+w")
	if m.query != "claude " {
		t.Fatalf("ctrl+w query = %q, want %q", m.query, "claude ")
	}
	m, _ = press(t, m, "ctrl+u")
	if m.query != "" {
		t.Fatalf("ctrl+u query = %q, want empty", m.query)
	}
}

func TestEnterActivatesSessionsAgentsDirectoriesAndHosts(t *testing.T) {
	t.Run("session attaches", func(t *testing.T) {
		m := newTestModel(t)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		_, cmd := m.activateSelected()
		if cmd == nil {
			t.Fatal("session activation returned no command")
		}
	})

	t.Run("agent is marked visited and focused", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{agentItem("claude", "%7", "frontend")})
		_, cmd := m.activateSelected()
		if cmd == nil {
			t.Fatal("agent activation returned no command")
		}
		if len(mux.visited) != 1 || mux.visited[0] != "%7" {
			t.Fatalf("MarkAgentVisited calls = %v, want the pane id %%7", mux.visited)
		}
	})

	t.Run("directory creates a session", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo", Name: "/tmp/demo"},
		})
		_, cmd := m.activateSelected()
		if cmd == nil {
			t.Fatal("directory activation returned no command")
		}
		if msg := cmd().(createDoneMsg); msg.err != nil {
			t.Fatalf("createSessionCmd = %#v", msg)
		}
		if mux.createdDir != "/tmp/demo" {
			t.Fatalf("created dir = %q, want /tmp/demo", mux.createdDir)
		}
	})

	t.Run("host points at seshagy run", func(t *testing.T) {
		m := newTestModel(t)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindHost, Name: "box", Target: "box"},
		})
		model, cmd := m.activateSelected()
		m = model.(Model)
		if cmd != nil {
			t.Fatalf("host activation returned a command: %v", cmd)
		}
		if got := latestNotificationText(m); got != "hosts need `seshagy run`" {
			t.Fatalf("host notification = %q", got)
		}
	})
}

func TestKillConfirmationArmConfirmAndCancel(t *testing.T) {
	t.Run("session confirm runs the kill", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		m, cmd := press(t, m, "ctrl+x")
		if !m.killArmed || cmd != nil {
			t.Fatalf("ctrl+x: armed = %v cmd = %v", m.killArmed, cmd)
		}
		m, cmd = press(t, m, "enter")
		if m.killArmed {
			t.Fatal("enter did not disarm the kill confirmation")
		}
		if cmd == nil {
			t.Fatal("confirm returned no kill command")
		}
		if msg := cmd().(actionDoneMsg); msg.err != nil {
			t.Fatalf("deleteSessionCmd = %#v", msg)
		}
		if len(mux.killed) != 1 || mux.killed[0] != "demo" {
			t.Fatalf("killed = %v, want [demo]", mux.killed)
		}
		if !m.killInFlight {
			t.Fatal("killInFlight not set after a confirmed kill")
		}
	})

	t.Run("y confirms and other keys cancel", func(t *testing.T) {
		for _, key := range []string{"y", "enter"} {
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{
				{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
			})
			m, _ = press(t, m, "ctrl+x")
			m, cmd := press(t, m, key)
			if m.killArmed || cmd == nil {
				t.Fatalf("%q: armed = %v cmd = %v", key, m.killArmed, cmd)
			}
		}
		for _, key := range []string{"n", "esc", "down", "space"} {
			m := newTestModel(t)
			m = withItems(m, []sessionmgr.Item{
				{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
			})
			m, _ = press(t, m, "ctrl+x")
			m, cmd := press(t, m, key)
			if m.killArmed || cmd != nil {
				t.Fatalf("%q: armed = %v cmd = %v, want a plain cancel", key, m.killArmed, cmd)
			}
		}
	})

	t.Run("agent pane is killed by pane id", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{agentItem("claude", "%9", "frontend")})
		m, _ = press(t, m, "delete")
		if !m.killArmed {
			t.Fatal("delete key did not arm the kill confirmation")
		}
		_, cmd := press(t, m, "y")
		if cmd == nil {
			t.Fatal("confirm returned no kill command")
		}
		_ = cmd()
		if len(mux.killed) != 1 || mux.killed[0] != "%9" {
			t.Fatalf("killed = %v, want the pane id %%9", mux.killed)
		}
	})

	t.Run("non-killable kinds only warn", func(t *testing.T) {
		m := newTestModel(t)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo", Name: "/tmp/demo"},
		})
		model, cmd := m.armKill()
		m = model.(Model)
		if m.killArmed || cmd != nil {
			t.Fatalf("armed = %v cmd = %v, want a warning only", m.killArmed, cmd)
		}
		if latestNotificationSeverity(m) != sevWarning {
			t.Fatalf("notification = %#v, want a warning", m.notifications)
		}
	})
}

func TestRenameFlow(t *testing.T) {
	t.Run("session rename is applied on enter", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		m, _ = press(t, m, "ctrl+r")
		if !m.renameActive || m.renameText != "demo" {
			t.Fatalf("ctrl+r: active = %v text = %q", m.renameActive, m.renameText)
		}
		m, _ = press(t, m, "ctrl+u")
		m, _ = press(t, m, "renamed")
		m, cmd := press(t, m, "enter")
		if m.renameActive {
			t.Fatal("enter did not leave rename mode")
		}
		if cmd == nil {
			t.Fatal("enter returned no rename command")
		}
		_ = cmd()
		if len(mux.renamed) != 1 || mux.renamed[0] != "demo=renamed" {
			t.Fatalf("renamed = %v, want [demo=renamed]", mux.renamed)
		}
	})

	t.Run("agent rename targets the pane and keeps the agent type", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{agentItem("claude", "%7", "frontend")})
		m, _ = press(t, m, "ctrl+r")
		if m.renameKind != sessionmgr.KindAgent || m.renameText != "claude" {
			t.Fatalf("ctrl+r: kind = %v text = %q", m.renameKind, m.renameText)
		}
		m, _ = press(t, m, "ctrl+u")
		m, _ = press(t, m, "alias")
		_, cmd := press(t, m, "enter")
		if cmd == nil {
			t.Fatal("enter returned no rename command")
		}
		_ = cmd()
		if len(mux.renamed) != 1 || mux.renamed[0] != "%7=alias" {
			t.Fatalf("renamed = %v, want %%7=alias", mux.renamed)
		}
	})

	t.Run("esc cancels without renaming", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		m, _ = press(t, m, "ctrl+r")
		m, _ = press(t, m, "ctrl+u")
		m, _ = press(t, m, "other")
		m, cmd := press(t, m, "esc")
		if m.renameActive || m.renameText != "" || cmd != nil {
			t.Fatalf("esc: active = %v text = %q cmd = %v", m.renameActive, m.renameText, cmd)
		}
		if len(mux.renamed) != 0 {
			t.Fatalf("cancelled rename still called RenameSession: %v", mux.renamed)
		}
	})

	t.Run("unchanged name is a no-op", func(t *testing.T) {
		mux := newFakeMux()
		m := newTestModelWith(t, mux)
		m = withItems(m, []sessionmgr.Item{
			{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
		})
		m, _ = press(t, m, "ctrl+r")
		_, cmd := press(t, m, "enter")
		if cmd != nil || len(mux.renamed) != 0 {
			t.Fatalf("unchanged rename: cmd = %v renamed = %v", cmd, mux.renamed)
		}
	})
}

func TestCursorFollowsSelectedKeyAcrossReorder(t *testing.T) {
	first := sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "alpha", Target: "alpha"}
	second := sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "bravo", Target: "bravo"}
	third := sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "charlie", Target: "charlie"}

	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{first, second, third})
	m = m.setCursor(2)
	if m.cursorKey != third.Key() {
		t.Fatalf("cursorKey = %q, want %q", m.cursorKey, third.Key())
	}

	// A refresh reorders the list: the cursor follows the item, not the index.
	m.items = []sessionmgr.Item{third, first, second}
	m.syncCursor()
	if m.cursor != 0 || m.cursorKey != third.Key() {
		t.Fatalf("after reorder: cursor = %d key = %q", m.cursor, m.cursorKey)
	}

	// A refresh drops the selected item: the cursor clamps and re-anchors.
	m.items = []sessionmgr.Item{first}
	m.syncCursor()
	if m.cursor != 0 || m.cursorKey != first.Key() {
		t.Fatalf("after removal: cursor = %d key = %q", m.cursor, m.cursorKey)
	}
}

func TestPreviewToggleAndInstallMenuKeys(t *testing.T) {
	m := newTestModel(t)
	if !m.previewVisible() {
		t.Fatal("preview should be visible at 120 columns by default")
	}
	m, _ = press(t, m, "ctrl+o")
	if m.showPreview {
		t.Fatal("ctrl+o did not disable the preview")
	}
	m, _ = press(t, m, "ctrl+o")
	if !m.showPreview {
		t.Fatal("ctrl+o did not re-enable the preview")
	}
	m, _ = press(t, m, "ctrl+a")
	if !m.installMenu.active {
		t.Fatal("ctrl+a did not open the install menu")
	}
}

func TestCtrlLInvalidatesCachesAndReloads(t *testing.T) {
	m := newTestModel(t)
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.inflightRefresh[sessionmgr.ModeSessions] = 3
	m, cmd := press(t, m, "ctrl+l")
	if cmd == nil {
		t.Fatal("ctrl+l returned no reload command")
	}
	if len(m.cache) != 0 {
		t.Fatalf("cache not invalidated: %#v", m.cache)
	}
	for _, source := range tuiSources {
		if !m.refreshInflight(source) {
			t.Fatalf("ctrl+l did not start a refresh for %v", source)
		}
	}
}
