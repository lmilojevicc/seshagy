package sessionmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installModeAllStrictFakeTmux(t *testing.T, sessionLine string) {
	t.Helper()
	f := NewFakeTmux()
	NewStrictFakeTmux(t, f).
		AllowPaneOptions().
		AllowOutput(MatchListSessions).
		AllowOutput(MatchListPanes).
		AllowOutput(func(args []string) bool {
			return len(args) >= 3 && args[0] == "display-message" && args[1] == "-p" &&
				args[2] == "#S"
		}).
		HandleOutput(MatchListSessions, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(sessionLine), nil
		}).
		HandleOutput(func(args []string) bool {
			return len(args) >= 3 && args[0] == "display-message" && args[1] == "-p" &&
				args[2] == "#S"
		}, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte("dev"), nil
		}).
		Install(t)
}

func installBrokenZoxideOnPath(t *testing.T) {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Executable on PATH but bad interpreter → start error (exit errors are ignored).
	if err := os.WriteFile(
		filepath.Join(binDir, "zoxide"),
		[]byte("#!/nonexistent-zoxide-interpreter\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestLoadWithOptionsModeAllPartialWhenFDFails(t *testing.T) {
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	installModeAllStrictFakeTmux(t, sessionLine)
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := LoadWithOptions(ctx, ModeAll, LoadOptions{FDCommand: "sleep 30"})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v, want partial success", err)
	}
	if result.Warning == "" {
		t.Fatal("LoadWithOptions() warning = empty, want fd failure warning")
	}
	if !strings.Contains(result.Warning, "fd command") {
		t.Fatalf("warning = %q, want fd command failure", result.Warning)
	}
	gotCounts := map[Kind]int{}
	for _, item := range result.Items {
		gotCounts[item.Kind]++
	}
	if gotCounts[KindSession] != 1 {
		t.Fatalf("items = %v, want one session", gotCounts)
	}
}

func TestLoadWithOptionsModeAllPartialWhenZoxideFails(t *testing.T) {
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	installModeAllStrictFakeTmux(t, sessionLine)
	installBrokenZoxideOnPath(t)
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")
	fdDir := t.TempDir()

	result, err := LoadWithOptions(
		context.Background(),
		ModeAll,
		LoadOptions{FDCommand: "printf '%s\\n' " + fdDir},
	)
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v, want partial success", err)
	}
	if result.Warning == "" || !strings.Contains(result.Warning, "zoxide query") {
		t.Fatalf("warning = %q, want zoxide failure", result.Warning)
	}
	gotCounts := map[Kind]int{}
	for _, item := range result.Items {
		gotCounts[item.Kind]++
	}
	if gotCounts[KindSession] != 1 || gotCounts[KindFD] != 1 {
		t.Fatalf("items = %v, want session and fd", gotCounts)
	}
}

func TestLoadWithOptionsModeFDFailsWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := LoadWithOptions(ctx, ModeFD, LoadOptions{FDCommand: "sleep 30"})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want fd failure")
	}
	if !strings.Contains(err.Error(), "fd command") {
		t.Fatalf("error = %q, want fd command failure", err.Error())
	}
}

func TestLoadWithOptionsModeDispatch(t *testing.T) {
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	fdDir := t.TempDir()
	installModeAllStrictFakeTmux(t, sessionLine)
	t.Setenv("TMUX", "/tmp/fake-tmux-sock,12345,0")

	ctx := context.Background()
	fdOpts := LoadOptions{FDCommand: "printf '%s\\n' " + fdDir}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	tests := []struct {
		name       string
		mode       SourceMode
		opts       LoadOptions
		ctx        context.Context
		wantCounts map[Kind]int
		wantErr    bool
		wantWarn   string
	}{
		{
			name:       "sessions",
			mode:       ModeSessions,
			wantCounts: map[Kind]int{KindSession: 1},
		},
		{
			name:       "fd",
			mode:       ModeFD,
			opts:       fdOpts,
			wantCounts: map[Kind]int{KindFD: 1},
		},
		{
			name: "all merges sessions and fd",
			mode: ModeAll,
			opts: fdOpts,
			wantCounts: map[Kind]int{
				KindSession: 1,
				KindFD:      1,
			},
		},
		{
			name:    "fd canceled context",
			mode:    ModeFD,
			opts:    LoadOptions{FDCommand: "sleep 30"},
			ctx:     cancelCtx,
			wantErr: true,
		},
		{
			name:     "zoxide missing binary",
			mode:     ModeZoxide,
			ctx:      ctx,
			wantErr:  false,
			wantWarn: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCtx := tt.ctx
			if testCtx == nil {
				testCtx = ctx
			}
			if tt.name == "zoxide missing binary" {
				t.Setenv("PATH", t.TempDir())
			}
			opts := tt.opts
			result, err := LoadWithOptions(testCtx, tt.mode, opts)
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadWithOptions() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadWithOptions() error = %v", err)
			}
			gotCounts := map[Kind]int{}
			for _, item := range result.Items {
				gotCounts[item.Kind]++
			}
			for kind, want := range tt.wantCounts {
				if gotCounts[kind] != want {
					t.Fatalf(
						"%s count = %d, want %d (all=%v)",
						kind,
						gotCounts[kind],
						want,
						gotCounts,
					)
				}
			}
			if tt.wantWarn != "" && !strings.Contains(result.Warning, tt.wantWarn) {
				t.Fatalf("warning = %q, want %q", result.Warning, tt.wantWarn)
			}
			if tt.name == "zoxide missing binary" && len(result.Items) != 0 {
				t.Fatalf("items = %d, want 0 when zoxide binary missing", len(result.Items))
			}
		})
	}
}
