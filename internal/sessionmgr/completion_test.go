package sessionmgr

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestReadCompletionSnapshotTmuxUsesOneReadWithoutProcessWalk(t *testing.T) {
	calls := 0
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if strings.Join(args, " ") != "list-panes -a -F "+completionPaneFormat {
			return nil, fmt.Errorf("unexpected args: %v", args)
		}
		return []byte(strings.Join([]string{
			"%1\x1fdev\x1f/tmp/a project\x1fzsh\x1fpi\x1fcustom:hook",
			"%opaque:id\x1fdev\x1f/tmp/other\x1fzsh\x1f\x1f",
			"%3\x1fprod\x1f/tmp/prod\x1fcodex-aarch64-a\x1f\x1fseshagy:codex",
		}, "\n")), nil
	}, nil)

	got, err := ReadCompletionSnapshot(context.Background(), NewTmuxBackend())
	if err != nil {
		t.Fatalf("ReadCompletionSnapshot() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("backend reads = %d, want 1", calls)
	}
	if len(got.Panes) != 3 || got.Panes[0].ID != "%1" || got.Panes[1].ID != "%opaque:id" {
		t.Fatalf("panes = %#v", got.Panes)
	}
	if got.Panes[0].Agent != "pi" || got.Panes[0].Source != "custom:hook" ||
		got.Panes[1].Agent != "" || got.Panes[2].Agent != "codex" ||
		got.Panes[2].Source != "seshagy:codex" {
		t.Fatalf("agent/source panes = %#v", got.Panes)
	}
	if len(got.Sessions) != 2 || got.Sessions[0].Target != "dev" ||
		got.Sessions[1].Target != "prod" {
		t.Fatalf("sessions = %#v", got.Sessions)
	}
}

func TestReadCompletionSnapshotHerdrPreservesOpaqueIDsInOneRead(t *testing.T) {
	calls := 0
	old := herdrOutput
	herdrOutput = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		switch strings.Join(args, " ") {
		case "pane list":
			return []byte(
				`{"type":"pane_list","panes":[{"pane_id":"pane:opaque/1","workspace_id":"workspace:opaque/7","cwd":"/old","foreground_cwd":"/current","agent":"pi"},{"pane_id":"pane two","workspace_id":"workspace:opaque/7","cwd":"/two"}]}`,
			), nil
		case "workspace list":
			return []byte(
				`{"type":"workspace_list","workspaces":[{"workspace_id":"workspace:opaque/7","label":"Development","pane_count":2},{"workspace_id":"empty:opaque/9","label":"Empty workspace","pane_count":0}]}`,
			), nil
		default:
			return nil, fmt.Errorf("unexpected args: %v", args)
		}
	}
	t.Cleanup(func() { herdrOutput = old })

	got, err := ReadCompletionSnapshot(context.Background(), NewHerdrBackend())
	if err != nil {
		t.Fatalf("ReadCompletionSnapshot() error = %v", err)
	}
	if calls != 1 || got.Panes[0].ID != "pane:opaque/1" || got.Panes[0].Cwd != "/current" {
		t.Fatalf("calls=%d snapshot=%#v", calls, got)
	}
	sessions, err := ReadCompletionSessions(context.Background(), NewHerdrBackend())
	if err != nil || calls != 2 || len(sessions) != 2 ||
		sessions[0] != (CompletionSession{"workspace:opaque/7", "Development"}) ||
		sessions[1] != (CompletionSession{"empty:opaque/9", "Empty workspace"}) {
		t.Fatalf("calls=%d sessions=%#v err=%v", calls, sessions, err)
	}
}

func TestReadCompletionSnapshotNoopDoesNoWork(t *testing.T) {
	got, err := ReadCompletionSnapshot(context.Background(), NewNoopBackend())
	if err != nil || len(got.Panes) != 0 || len(got.Sessions) != 0 {
		t.Fatalf("snapshot=%#v err=%v", got, err)
	}
}
