package sessionmgr

import (
	"context"
	"testing"
)

// tmuxFocusMock returns canned tmux output driven by the last format argument,
// simulating the three probes TmuxFocusLost runs (session_name, window_active,
// list-clients). A non-empty failFmt makes that probe return an error.
func tmuxFocusMock(t *testing.T, sessionName, windowActive, clients string, failFmt string,
) func(context.Context, ...string) ([]byte, error) {
	t.Helper()
	return func(_ context.Context, args ...string) ([]byte, error) {
		last := args[len(args)-1]
		if failFmt != "" && last == failFmt {
			return nil, errMock("tmux probe failed")
		}
		switch last {
		case "#{session_name}":
			return []byte(sessionName), nil
		case "#{window_active}":
			return []byte(windowActive), nil
		case "#{client_session}":
			return []byte(clients), nil
		}
		return nil, nil
	}
}

type errMock string

func (e errMock) Error() string { return string(e) }

func TestTmuxFocusLost(t *testing.T) {
	t.Setenv("TMUX_PANE", "%5")

	cases := []struct {
		name         string
		sessionName  string
		windowActive string
		clients      string
		failFmt      string
		want         bool
	}{
		{"focused and has client", "main", "1", "main", "", false},
		{"window not active", "main", "0", "main", "", true},
		{"no client attached", "main", "1", "other", "", true},
		{"pane gone (session_name probe fails)", "", "", "", "#{session_name}", true},
		{"window probe fails", "main", "", "main", "#{window_active}", true},
		{"clients probe fails", "main", "1", "", "#{client_session}", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetTmuxHooksForTest(
				t,
				tmuxFocusMock(t, tc.sessionName, tc.windowActive, tc.clients, tc.failFmt),
				nil,
			)
			got := TmuxFocusLost(context.Background(), "", "")
			if got != tc.want {
				t.Fatalf("TmuxFocusLost() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTmuxFocusLostNoPaneEnv(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	SetTmuxHooksForTest(t, tmuxFocusMock(t, "main", "1", "main", ""), nil)
	if got := TmuxFocusLost(context.Background(), "", ""); got {
		t.Fatal("TmuxFocusLost() = true, want false when TMUX_PANE unset")
	}
}

func TestHerdrFocusLost(t *testing.T) {
	const focusedPane = `{"pane_id":"p1","focused":true,"workspace_id":"w1"}`
	const unfocusedPane = `{"pane_id":"p1","focused":false,"workspace_id":"w1"}`
	const sameWorkspace = `{"workspaces":[{"workspace_id":"w1","focused":true}]}`
	const otherWorkspace = `{"workspaces":[{"workspace_id":"w9","focused":true}]}`

	cases := []struct {
		name      string
		paneJSON  string
		workspace string
		wsJSON    string
		paneFail  bool
		want      bool
	}{
		{"focused, same workspace", focusedPane, "w1", sameWorkspace, false, false},
		{"pane not focused", unfocusedPane, "w1", sameWorkspace, false, true},
		{"pane query fails", "", "w1", sameWorkspace, true, true},
		{"focused but workspace changed", focusedPane, "w1", otherWorkspace, false, true},
		{"workspace list fails (ignored)", focusedPane, "w1", "", false, false},
		{"no workspace to compare", focusedPane, "", sameWorkspace, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
				if len(args) >= 2 && args[0] == "pane" && args[1] == "get" {
					if tc.paneFail {
						return nil, errMock("herdr pane get failed")
					}
					return []byte(tc.paneJSON), nil
				}
				if len(args) >= 2 && args[0] == "workspace" && args[1] == "list" {
					if tc.wsJSON == "" {
						return nil, errMock("herdr workspace list failed")
					}
					return []byte(tc.wsJSON), nil
				}
				return nil, nil
			}, nil)
			got := HerdrFocusLost(context.Background(), "p1", tc.workspace)
			if got != tc.want {
				t.Fatalf("HerdrFocusLost() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHerdrFocusLostEmptyPane(t *testing.T) {
	if got := HerdrFocusLost(context.Background(), "", "w1"); got {
		t.Fatal("HerdrFocusLost() = true, want false when paneID empty")
	}
}

func TestResolveHerdrEphemeralTarget(t *testing.T) {
	t.Run("env vars win", func(t *testing.T) {
		t.Setenv("HERDR_PANE_ID", "envPane")
		t.Setenv("HERDR_WORKSPACE_ID", "envWs")
		pane, ws, ok := ResolveHerdrEphemeralTarget(context.Background())
		if !ok || pane != "envPane" || ws != "envWs" {
			t.Fatalf(
				"ResolveHerdrEphemeralTarget() = (%q,%q,%v), want (envPane,envWs,true)",
				pane,
				ws,
				ok,
			)
		}
	})

	t.Run("discovers focused pane when env unset", func(t *testing.T) {
		t.Setenv("HERDR_PANE_ID", "")
		t.Setenv("HERDR_WORKSPACE_ID", "")
		setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
			return []byte(`{"panes":[` +
				`{"pane_id":"pOther","focused":false,"workspace_id":"wOther"},` +
				`{"pane_id":"pMine","focused":true,"workspace_id":"wMine"}` +
				`]}`), nil
		}, nil)
		pane, ws, ok := ResolveHerdrEphemeralTarget(context.Background())
		if !ok || pane != "pMine" || ws != "wMine" {
			t.Fatalf(
				"ResolveHerdrEphemeralTarget() = (%q,%q,%v), want (pMine,wMine,true)",
				pane,
				ws,
				ok,
			)
		}
	})

	t.Run("discovery fails", func(t *testing.T) {
		t.Setenv("HERDR_PANE_ID", "")
		t.Setenv("HERDR_WORKSPACE_ID", "")
		setHerdrHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
			return nil, errMock("herdr unavailable")
		}, nil)
		_, _, ok := ResolveHerdrEphemeralTarget(context.Background())
		if ok {
			t.Fatal("ResolveHerdrEphemeralTarget() ok = true, want false")
		}
	})
}
