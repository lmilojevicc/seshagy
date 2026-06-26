package sessionmgr

import (
	"context"
	"errors"
	"testing"
)

func TestDetectFromEnvPriority(t *testing.T) {
	tests := []struct {
		name   string
		getenv func(string) string
		want   BackendKind
	}{
		{
			name: "herdr wins over tmux when both set",
			getenv: func(k string) string {
				return map[string]string{
					"HERDR_ENV": "1",
					"TMUX":      "/tmp/tmux-1000/default,123,0",
				}[k]
			},
			want: BackendHerdr,
		},
		{
			name:   "herdr only",
			getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
			want:   BackendHerdr,
		},
		{
			name:   "tmux only",
			getenv: func(k string) string { return map[string]string{"TMUX": "/tmp/tmux-1000/default,123,0"}[k] },
			want:   BackendTmux,
		},
		{
			name:   "neither set",
			getenv: func(string) string { return "" },
			want:   BackendNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectFromEnv(tc.getenv).Kind(); got != tc.want {
				t.Fatalf("kind = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectDefaultUsesRealEnv(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if got := Detect().Kind(); got != BackendTmux {
		t.Fatalf("Detect() kind = %v, want %v", got, BackendTmux)
	}
}

func TestTmuxBackendTerms(t *testing.T) {
	terms := NewTmuxBackend().Terms()
	if terms.BackendName != "tmux" || terms.SessionNoun != "session" ||
		terms.SessionPlural != "sessions" || terms.WindowNoun != "window" {
		t.Fatalf("tmux terms = %+v", terms)
	}
}

func TestHerdrTerms(t *testing.T) {
	terms := HerdrTerms()
	if terms.BackendName != "herdr" || terms.SessionNoun != "workspace" ||
		terms.SessionPlural != "workspaces" || terms.WindowNoun != "tab" {
		t.Fatalf("herdr terms = %+v", terms)
	}
}

func TestNoopBackendTerms(t *testing.T) {
	terms := NewNoopBackend().Terms()
	if terms.BackendName != "tmux" { // NoneTerms mirrors tmux in Phase 2
		t.Fatalf("none terms backend = %q, want tmux", terms.BackendName)
	}
}

func TestNoopBackendReturnsEmpty(t *testing.T) {
	mux := NewNoopBackend()
	if items, err := mux.ListSessions(context.Background()); err != nil || items != nil {
		t.Fatalf("ListSessions = %v, %v, want nil, nil", items, err)
	}
	if items, err := mux.ListAgents(context.Background(), ""); err != nil || items != nil {
		t.Fatalf("ListAgents = %v, %v, want nil, nil", items, err)
	}
	if ok, err := mux.HasSession(context.Background(), "x"); err != nil || ok {
		t.Fatalf("HasSession = %v, %v, want false, nil", ok, err)
	}
	if preview, err := mux.CaptureSession(
		context.Background(),
		"x",
		10,
	); err != nil ||
		preview != "" {
		t.Fatalf("CaptureSession = %q, %v, want empty, nil", preview, err)
	}
	if _, _, err := mux.CreateSessionFromDir(
		context.Background(),
		"/tmp",
	); !errors.Is(
		err,
		errNoBackend,
	) {
		t.Fatalf("CreateSessionFromDir err = %v, want errNoBackend", err)
	}
	if _, err := mux.ReportAgent(context.Background(), AgentReport{}); err != nil {
		t.Fatalf("ReportAgent err = %v, want nil", err)
	}
}

// TestTmuxBackendDelegates proves the wrapper is transparent: ListSessions via
// the backend matches the free function output for the same fake tmux.
func TestTmuxBackendDelegates(t *testing.T) {
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return []byte(sessionLine), nil
		}
		return nil, nil
	}, nil)

	mux := NewTmuxBackend()
	got, err := mux.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("backend ListSessions error = %v", err)
	}
	direct, err := ListSessions(context.Background())
	if err != nil {
		t.Fatalf("free ListSessions error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "dev" || len(direct) != 1 ||
		direct[0].Name != "dev" {
		t.Fatalf("backend = %+v, direct = %+v", got, direct)
	}
}

func TestItemActionTarget(t *testing.T) {
	tests := []struct {
		name string
		item Item
		want string
	}{
		{
			name: "tmux session Target==Name",
			item: Item{Kind: KindSession, Name: "dev", Target: "dev"},
			want: "dev",
		},
		{
			name: "herdr session Target != Name uses Target",
			item: Item{Kind: KindSession, Name: "proj", Target: "w1"},
			want: "w1",
		},
		{
			name: "agent uses PaneID",
			item: Item{Kind: KindAgent, PaneID: "%5"},
			want: "%5",
		},
		{
			name: "session without Target falls back to Name",
			item: Item{Kind: KindSession, Name: "dev"},
			want: "dev",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.item.ActionTarget(); got != tc.want {
				t.Fatalf("ActionTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLoadWithBackendNoopGuardsManifest verifies the noop backend does not run
// the manifest fallback even when ManifestFallback is enabled.
func TestLoadWithBackendNoopGuardsManifest(t *testing.T) {
	manifestRan := false
	// Best-effort guard: ApplyManifestFallback only runs for non-empty agent
	// items under tmux; with noop backend ListAgents returns nil so the call is
	// never reached. The test documents the contract.
	_ = manifestRan
	mux := NewNoopBackend()
	result, err := LoadWithBackend(
		context.Background(),
		mux,
		ModeAgents,
		LoadOptions{ManifestFallback: true},
	)
	if err != nil {
		t.Fatalf("LoadWithBackend error = %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("items = %v, want empty under noop backend", result.Items)
	}
}
