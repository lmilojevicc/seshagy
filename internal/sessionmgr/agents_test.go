package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func agentPaneLine(pane, session, win, paneIdx, path, cmd, pid, dead string) string {
	return strings.Join(
		[]string{pane, session, win, paneIdx, path, cmd, pid, dead, "", "", ""},
		"\x1f",
	)
}

func TestDetectAgentNameMatchesKnownAgents(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"pi", "pi"},
		{"opencode", "opencode"},
		{"codex", "codex"},
		{"codex-aarch64-a", "codex"},
		{"claude", "claude"},
		{"cursor-agent", "cursor"},
		{"cursor", "cursor"},
		{"agy", "antigravity"},
		{"antigravity", "antigravity"},
		{"droid", "droid"},
		{"factory", "droid"},
		{"grok", "grok"},
		{"copilot", "copilot"},
		{"PI", "pi"},
		{"/usr/local/bin/claude", "claude"},
		{"bash", ""},
		{"node", ""},
		{"zsh", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := detectAgentName(tc.command); got != tc.want {
			t.Errorf("detectAgentName(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestParseAgentsSkipsDeadPanesAndFormatsLocation(t *testing.T) {
	raw := strings.Join([]string{
		agentPaneLine("%1", "seshagy", "1", "2", "/home/proj", "pi", "111", "0"),
		agentPaneLine("%2", "dotfiles", "0", "1", "/home/dots", "claude", "222", "1"),
		agentPaneLine("%3", "app", "2", "0", "/home/app", "codex", "333", "0"),
		agentPaneLine("%4", "misc", "3", "1", "/home/misc", "bash", "444", "0"),
	}, "\n")

	items := ParseAgents([]byte(raw), "")
	if len(items) != 2 {
		t.Fatalf("ParseAgents() = %d items, want 2 (dead pane and non-agent skipped)", len(items))
	}

	first := items[0]
	if first.AgentName != "pi" {
		t.Errorf("items[0].AgentName = %q, want pi", first.AgentName)
	}
	if first.AgentState != AgentIdle {
		t.Errorf("items[0].AgentState = %q, want idle", first.AgentState)
	}
	if first.Location != "seshagy:1.2" {
		t.Errorf("items[0].Location = %q, want seshagy:1.2", first.Location)
	}
	if first.PaneID != "%1" {
		t.Errorf("items[0].PaneID = %q, want %%1", first.PaneID)
	}
	if first.Session != "seshagy" {
		t.Errorf("items[0].Session = %q, want seshagy", first.Session)
	}

	second := items[1]
	if second.AgentName != "codex" {
		t.Errorf("items[1].AgentName = %q, want codex", second.AgentName)
	}
	if second.Location != "app:2.0" {
		t.Errorf("items[1].Location = %q, want app:2.0", second.Location)
	}
}

func TestParseAgentsFiltersBySession(t *testing.T) {
	raw := strings.Join([]string{
		agentPaneLine("%1", "seshagy", "1", "2", "/home/proj", "pi", "111", "0"),
		agentPaneLine("%2", "dotfiles", "0", "1", "/home/dots", "claude", "222", "0"),
	}, "\n")

	items := ParseAgents([]byte(raw), "seshagy")
	if len(items) != 1 {
		t.Fatalf("ParseAgents(filter=seshagy) = %d items, want 1", len(items))
	}
	if items[0].Session != "seshagy" {
		t.Errorf("items[0].Session = %q, want seshagy", items[0].Session)
	}
}

func TestParseAgentsEmpty(t *testing.T) {
	if items := ParseAgents(nil, ""); items != nil {
		t.Fatalf("ParseAgents(nil) = %#v, want nil", items)
	}
	if items := ParseAgents([]byte("  \n  "), ""); items != nil {
		t.Fatalf("ParseAgents(blank) = %#v, want nil", items)
	}
}

func TestListAgentsViaTmuxSeam(t *testing.T) {
	called := false
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			called = true
			if args[1] != "-a" || args[2] != "-F" {
				t.Errorf("list-panes args = %v, want -a -F <format>", args[1:])
			}
			return []byte(agentPaneLine("%1", "work", "0", "0", "/home/work", "pi", "1", "0")), nil
		}
		return nil, nil
	}, nil)

	items, err := ListAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if !called {
		t.Fatal("list-panes was not called")
	}
	if len(items) != 1 || items[0].AgentName != "pi" {
		t.Fatalf("ListAgents() = %#v, want one pi agent", items)
	}
}

func TestListAgentsEmptyWhenTmuxExitOne(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			return nil, exec.Command("false").Run()
		}
		return nil, nil
	}, nil)

	items, err := ListAgents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if items != nil {
		t.Fatalf("ListAgents() = %#v, want nil on exit 1", items)
	}
}

func TestListAgentsPropagatesError(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			return nil, fmt.Errorf("tmux unavailable")
		}
		return nil, nil
	}, nil)

	if _, err := ListAgents(context.Background(), ""); err == nil {
		t.Fatal("ListAgents() expected error")
	} else if !strings.Contains(err.Error(), "tmux list-panes") {
		t.Fatalf("ListAgents() error = %v", err)
	}
}

func agentPaneLineWithState(
	pane, session, win, paneIdx, path, cmd, pid, dead, state, updated, seq string,
) string {
	return strings.Join(
		[]string{pane, session, win, paneIdx, path, cmd, pid, dead, state, updated, seq},
		"\x1f",
	)
}

func TestParseAgentsReadsHookState(t *testing.T) {
	now := time.Now().Format(time.RFC3339Nano)
	raw := agentPaneLineWithState(
		"%1",
		"work",
		"0",
		"0",
		"/home/work",
		"pi",
		"111",
		"0",
		"working",
		now,
		"123",
	)
	items := ParseAgents([]byte(raw), "")
	if len(items) != 1 {
		t.Fatalf("ParseAgents() = %d items, want 1", len(items))
	}
	if items[0].AgentState != AgentWorking {
		t.Errorf("AgentState = %q, want working", items[0].AgentState)
	}
}

func TestParseAgentsFallsBackToIdleWhenNoHook(t *testing.T) {
	raw := agentPaneLineWithState(
		"%1",
		"work",
		"0",
		"0",
		"/home/work",
		"pi",
		"111",
		"0",
		"",
		"",
		"",
	)
	items := ParseAgents([]byte(raw), "")
	if len(items) != 1 {
		t.Fatalf("ParseAgents() = %d items, want 1", len(items))
	}
	if items[0].AgentState != AgentIdle {
		t.Errorf("AgentState = %q, want idle", items[0].AgentState)
	}
}

func TestParseAgentsStaleStateFallsBackToIdle(t *testing.T) {
	old := time.Now().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	raw := agentPaneLineWithState(
		"%1",
		"work",
		"0",
		"0",
		"/home/work",
		"pi",
		"111",
		"0",
		"working",
		old,
		"123",
	)
	items := ParseAgents([]byte(raw), "")
	if len(items) != 1 {
		t.Fatalf("ParseAgents() = %d items, want 1", len(items))
	}
	if items[0].AgentState != AgentIdle {
		t.Errorf("AgentState = %q, want idle (stale report)", items[0].AgentState)
	}
}
