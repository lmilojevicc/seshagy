package sessionmgr

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestListAgentsEmptyOnTmuxExitOne(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-panes" {
			return nil, exec.Command("false").Run()
		}
		return nil, nil
	}, nil)

	items, err := ListAgents(context.Background(), "", LoadOptions{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if items != nil {
		t.Fatalf("ListAgents() = %#v, want nil on exit 1", items)
	}
}

func TestListAgentsWithManifestFallback(t *testing.T) {
	const pane = "%14"
	screen := "Some output above\nRun a dynamic workflow? (esc to cancel)\n"
	fields := agentExplainFields(pane, map[int]string{12: ""})
	listOut := []byte(strings.Join(fields, paneSep) + "\n")
	InstallListAgentsFakeTmux(t, pane, listOut, fakePaneVisibilityActive, screen)

	items, err := ListAgents(context.Background(), "", LoadOptions{ManifestFallback: true})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 || items[0].AgentState != AgentBlocked {
		t.Fatalf("items = %#v, want blocked manifest fallback state", items)
	}
}

func TestListAgentsManifestFallbackFalseSkipsCapturePane(t *testing.T) {
	const pane = "%15"
	fields := agentExplainFields(pane, map[int]string{12: ""})
	listOut := []byte(strings.Join(fields, paneSep) + "\n")
	InstallListAgentsFakeTmux(t, pane, listOut, fakePaneVisibilityActive, "")

	items, err := ListAgents(context.Background(), "", LoadOptions{ManifestFallback: false})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].AgentState != AgentUnknown {
		t.Fatalf("AgentState = %q, want unknown without manifest fallback", items[0].AgentState)
	}
}

func TestListAgentsAppliesStatusTracking(t *testing.T) {
	const pane = "%9"
	now := time.Unix(1_700_000_000, 0)
	SetFixedTrackTime(t, now)

	fields := agentExplainFields(pane, map[int]string{
		5:  "0",
		6:  "0",
		7:  "1",
		12: "idle",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, paneSep) + "\n")

	f := InstallListAgentsFakeTmux(t, pane, listOut, "0 0 1", "")
	f.Set(pane, "@agent_name", "claude")
	f.Set(pane, "@agent_last_state", string(AgentWorking))
	f.Set(pane, "@agent_last_status", string(AgentWorking))
	f.Set(pane, "@agent_startup_grace", formatUnix(now.Add(-agentStartupGraceWindow)))
	seedPendingIdleConfirmed(f, pane, now)

	items, err := ListAgents(context.Background(), "", LoadOptions{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].AgentState != AgentDone {
		t.Fatalf("AgentState = %q, want done after tracking", items[0].AgentState)
	}
	if got := f.Get(pane, "@agent_last_status"); got != string(AgentDone) {
		t.Fatalf("@agent_last_status = %q, want done", got)
	}
}

func TestListAgentsSkipsTrackingAfterRelease(t *testing.T) {
	const pane = "%16"
	now := time.Unix(1_700_000_000, 0)
	SetFixedTrackTime(t, now)

	fields := agentExplainFields(pane, map[int]string{
		11: "claude",
		12: "idle",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, paneSep) + "\n")
	f := InstallListAgentsFakeTmux(t, pane, listOut, fakePaneVisibilityActive, "")
	f.Set(pane, "@agent_last_state", string(AgentWorking))
	f.Set(pane, "@agent_last_status", string(AgentWorking))
	f.Set(pane, "@agent_seq", "88")

	items, err := ListAgents(context.Background(), "", LoadOptions{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].AgentState != AgentIdle {
		t.Fatalf(
			"AgentState = %q, want list-panes idle without tracking writes",
			items[0].AgentState,
		)
	}
	if got := f.Get(pane, "@agent_last_status"); got != string(AgentWorking) {
		t.Fatalf("@agent_last_status = %q, want unchanged working", got)
	}
}

func TestCaptureAgentPane(t *testing.T) {
	const pane = "%7"
	var capturedArgs []string
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "capture-pane" {
			capturedArgs = append([]string(nil), args...)
			return []byte("pane text\n"), nil
		}
		return nil, nil
	}, nil)

	out, err := CaptureAgentPane(context.Background(), pane, 80)
	if err != nil || out != "pane text\n" {
		t.Fatalf("CaptureAgentPane() = (%q, %v)", out, err)
	}
	want := []string{"capture-pane", "-ep", "-t", pane, "-S", "-80"}
	if len(capturedArgs) != len(want) {
		t.Fatalf("capture-pane args = %v, want %v", capturedArgs, want)
	}
	for i, arg := range want {
		if capturedArgs[i] != arg {
			t.Fatalf("capture-pane args = %v, want %v", capturedArgs, want)
		}
	}
}

func TestKillAgentPane(t *testing.T) {
	t.Run("runs tmux kill-pane", func(t *testing.T) {
		var killed string
		SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
			if len(args) >= 3 && args[0] == "kill-pane" {
				killed = args[2]
			}
			return nil
		})
		if err := KillAgentPane(context.Background(), "%9"); err != nil {
			t.Fatalf("KillAgentPane() error = %v", err)
		}
		if killed != "%9" {
			t.Fatalf("kill-pane target = %q, want %%9", killed)
		}
	})

	t.Run("propagates error", func(t *testing.T) {
		SetTmuxHooksForTest(t, nil, func(_ context.Context, args ...string) error {
			if len(args) >= 1 && args[0] == "kill-pane" {
				return fmt.Errorf("boom")
			}
			return nil
		})
		if err := KillAgentPane(context.Background(), "%9"); err == nil {
			t.Fatal("KillAgentPane() expected error")
		} else if !strings.Contains(err.Error(), "tmux kill-pane") {
			t.Fatalf("KillAgentPane() error = %v", err)
		}
	})
}

func TestTrackAgentPaneUpdatesTracking(t *testing.T) {
	const pane = "%13"
	now := time.Unix(1_700_000_000, 0)
	SetAgentTrackNowForTest(t, now)
	fields := agentExplainFields(pane, map[int]string{
		11: "claude",
		12: "working",
		15: "seshagy:claude",
	})
	f := InstallExplainFakeTmux(t, pane, fields, "")
	f.Set(pane, "@agent_name", "claude")

	state, err := TrackAgentPane(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("TrackAgentPane() error = %v", err)
	}
	if state != AgentWorking {
		t.Fatalf("TrackAgentPane() = %q, want working", state)
	}
}

func TestReadProcessArgsUsesHook(t *testing.T) {
	orig := readProcessArgsHook
	readProcessArgsHook = func(pid string) string {
		if pid == "123" {
			return "claude --foo"
		}
		return ""
	}
	t.Cleanup(func() { readProcessArgsHook = orig })

	if got := readProcessArgs("123"); got != "claude --foo" {
		t.Fatalf("readProcessArgs() = %q", got)
	}
	if got := readProcessArgs("0"); got != "" {
		t.Fatalf("readProcessArgs(0) = %q, want empty", got)
	}
}

func TestListAgentsSkipsPaneOnLockFailure(t *testing.T) {
	const pane = "%16"
	fields := agentExplainFields(pane, map[int]string{
		11: "claude",
		12: "idle",
		15: "seshagy:claude",
	})
	listOut := []byte(strings.Join(fields, paneSep) + "\n")
	InstallListAgentsFakeTmux(t, pane, listOut, fakePaneVisibilityActive, "")

	var tracked bool
	lockErr := fmt.Errorf("lock unavailable")
	SetAgentPaneLockHookForTest(t, func(_ string, fn func() error) error {
		tracked = true
		_ = fn
		return lockErr
	})

	items, err := ListAgents(context.Background(), "", LoadOptions{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v, want nil despite lock failure", err)
	}
	if !tracked {
		t.Fatal("ListAgents() did not attempt to lock the pane")
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (failed lock must not drop the pane)", len(items))
	}
	if items[0].AgentState != AgentIdle {
		t.Fatalf(
			"AgentState = %q, want detected idle retained when lock fails",
			items[0].AgentState,
		)
	}
}
