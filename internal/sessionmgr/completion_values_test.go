package sessionmgr

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCompletionPanesTmuxUsesSingleRead(t *testing.T) {
	calls := 0
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		if !MatchListPanes(args) {
			t.Fatalf("unexpected tmux args: %v", args)
		}
		return []byte("%opaque\x1f/tmp/a b\x1f0\n%dead\x1f/tmp\x1f1\nmalformed"), nil
	}, func(context.Context, ...string) error {
		t.Fatal("pane completion attempted a state write")
		return nil
	})
	values, err := CompletionPanes(context.Background(), NewTmuxBackend())
	if err != nil {
		t.Fatal(err)
	}
	want := []CompletionPane{{ID: "%opaque", Cwd: "/tmp/a b"}}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestCompletionPanesTmuxSkipsAgentDiscovery(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1\x1f/tmp\x1f0"), nil
	}, nil)
	original := completionProcessSnapshot
	completionProcessSnapshot = func(context.Context) (map[int32]procEntry, error) {
		t.Fatal("pane/cwd completion walked the process tree")
		return nil, nil
	}
	t.Cleanup(func() { completionProcessSnapshot = original })
	if _, err := CompletionPanes(context.Background(), NewTmuxBackend()); err != nil {
		t.Fatal(err)
	}
}

func TestCompletionAgentPanesTmuxUsesBoundedDescendantDetection(t *testing.T) {
	SetTmuxHooksForTest(t, func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("%1\x1f/tmp\x1fnode\x1f10\x1f0\x1f\x1f"), nil
	}, nil)
	original := completionProcessSnapshot
	completionProcessSnapshot = func(ctx context.Context) (map[int32]procEntry, error) {
		if ctx == nil {
			t.Fatal("process snapshot received nil context")
		}
		return map[int32]procEntry{11: {pid: 11, ppid: 10, comm: "pi"}}, nil
	}
	t.Cleanup(func() { completionProcessSnapshot = original })
	values, err := CompletionAgentPanes(context.Background(), NewTmuxBackend())
	if err != nil || len(values) != 1 || values[0].Agent != "pi" {
		t.Fatalf("values = %#v, err=%v", values, err)
	}
}

func TestCompletionPanesHerdrKeepsOpaqueIDsAndUsesForegroundCwd(t *testing.T) {
	calls := 0
	setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		return []byte(
			`{"type":"pane_list","panes":[{"pane_id":"id:with spaces;$()","cwd":"/base","foreground_cwd":"/front","agent":"claude"}]}`,
		), nil
	}, nil)
	values, err := CompletionPanes(context.Background(), NewHerdrBackend())
	if err != nil {
		t.Fatal(err)
	}
	want := []CompletionPane{{ID: "id:with spaces;$()", Cwd: "/front"}}
	if !reflect.DeepEqual(values, want) || calls != 1 {
		t.Fatalf("values = %#v calls=%d", values, calls)
	}
}

func TestCompletionValuesNoopAndFailure(t *testing.T) {
	values, err := CompletionPanes(context.Background(), NewNoopBackend())
	if err != nil || len(values) != 0 {
		t.Fatalf("noop = %#v, %v", values, err)
	}
	SetTmuxHooksForTest(t, func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("unavailable")
	}, nil)
	if _, err := CompletionPanes(context.Background(), NewTmuxBackend()); err == nil {
		t.Fatal("backend failure was swallowed")
	}
}

func TestCompletionSessionsHerdrUsesOnlyWorkspaceList(t *testing.T) {
	var got []string
	setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		got = append(got, args...)
		return []byte(
			`{"type":"workspace_list","workspaces":[` +
				`{"workspace_id":"opaque-1","label":"demo","focused":true},` +
				`{"workspace_id":"opaque-2","label":"demo","focused":false}]}`,
		), nil
	}, nil)
	items, err := CompletionSessions(context.Background(), NewHerdrBackend())
	if err != nil || len(items) != 2 || items[0].Target != "opaque-1" ||
		items[1].Target != "opaque-2" || items[0].Name != "demo" || items[1].Name != "demo" {
		t.Fatalf("items = %#v, err=%v", items, err)
	}
	if !reflect.DeepEqual(got, []string{"workspace", "list"}) {
		t.Fatalf("backend args = %v", got)
	}
}
