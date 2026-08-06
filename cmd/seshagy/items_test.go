package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func installDeleteItemTmuxRecorder(
	t *testing.T,
	wantCmd string,
	onMatch func(args []string),
) {
	t.Helper()
	sessionmgr.SetTmuxHooksForTest(
		t,
		func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("session discovery unavailable in recorder: %v", args)
		},
		func(_ context.Context, args ...string) error {
			if len(args) >= 1 && args[0] == wantCmd {
				onMatch(args)
				return nil
			}
			return fmt.Errorf("unexpected tmux call: %v", args)
		},
	)
}

func writeFDTestConfig(t *testing.T, fdDir string) {
	t.Helper()
	cfg := appconfig.Default()
	cfg.Directories.FDCommand = fmt.Sprintf("printf '%%s\\n' %s", fdDir)
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestPrintItemsJSONEnvelope(t *testing.T) {
	manifestTestDirs(t)
	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return printItems(
			context.Background(),
			sessionmgr.NewTmuxBackend(),
			sessionmgr.ModeFD,
			true,
		)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Mode          string `json:"mode"`
		Items         []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok {
		t.Fatalf("envelope = schema_version:%d ok:%v", payload.SchemaVersion, payload.Ok)
	}
	if payload.Mode != "fd" {
		t.Fatalf("mode = %q, want fd", payload.Mode)
	}
	if len(payload.Items) != 1 || payload.Items[0].Kind != "fd" {
		t.Fatalf("items = %#v, want one fd item", payload.Items)
	}
}

func TestDeleteItemSessionJSONEnvelope(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)

	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), sessionmgr.NewTmuxBackend(), line, true)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Deleted       bool   `json:"deleted"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok || !payload.Deleted {
		t.Fatalf("envelope = %#v", payload)
	}
	if payload.Kind != string(sessionmgr.KindSession) || payload.Name != "demo" {
		t.Fatalf("delete payload = %#v", payload)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestPrintItemsTextOutput(t *testing.T) {
	manifestTestDirs(t)
	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return printItems(
			context.Background(),
			sessionmgr.NewTmuxBackend(),
			sessionmgr.ModeFD,
			false,
		)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}
	if !strings.Contains(out, fdDir) {
		t.Fatalf("text output missing fd path:\n%s", out)
	}
}

func TestPrintItemsWritesWarningToStderr(t *testing.T) {
	manifestTestDirs(t)
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[0] == "list-sessions" {
			return []byte(sessionLine), nil
		}
		return nil, nil
	}, nil)

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	cfg.Directories.FDCommand = "sleep 30"
	if err := appconfig.Save(cfg); err != nil {
		t.Fatalf("config.Save() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stderr, err := captureStderr(t, func() error {
		return printItems(ctx, sessionmgr.NewTmuxBackend(), sessionmgr.ModeAll, false)
	})
	if err != nil {
		t.Fatalf("printItems() error = %v", err)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "fd command") {
		t.Fatalf("stderr = %q, want fd warning", stderr)
	}
}

func TestGetAllJSONIncludesZoxideWarning(t *testing.T) {
	manifestTestDirs(t)
	sessionLine := "dev\x1f100\x1f120\x1f/tmp/dev\x1f1\x1f2"
	sessionmgr.NewStrictFakeTmux(t, sessionmgr.NewFakeTmux()).
		AllowPaneOptions().
		AllowOutput(sessionmgr.MatchListSessions).
		AllowOutput(sessionmgr.MatchListPanes).
		HandleOutput(sessionmgr.MatchListSessions, func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(sessionLine), nil
		}).
		Install(t)

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

	fdDir := t.TempDir()
	writeFDTestConfig(t, fdDir)

	out, err := captureStdout(t, func() error {
		return run([]string{"--get-all", "--json"})
	})
	if err != nil {
		t.Fatalf("run(--get-all --json) error = %v", err)
	}
	var payload struct {
		Ok      bool   `json:"ok"`
		Mode    string `json:"mode"`
		Warning string `json:"warning"`
		Items   []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if !payload.Ok || payload.Mode != "all" {
		t.Fatalf("envelope = %#v", payload)
	}
	if !strings.Contains(payload.Warning, "zoxide query") {
		t.Fatalf("warning = %q, want zoxide failure", payload.Warning)
	}
	kinds := map[string]int{}
	for _, item := range payload.Items {
		kinds[item.Kind]++
	}
	if kinds["session"] != 1 || kinds["fd"] != 1 {
		t.Fatalf("items = %#v, want session and fd", kinds)
	}
}

func TestDeleteItemSessionNonJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return deleteItem(context.Background(), sessionmgr.NewTmuxBackend(), line, false)
	})
	if err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("non-json delete should be silent, got %q", out)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestDeleteItemNonDeletableKind(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindZoxide, Path: "/tmp/demo"},
		cfg.IconSet(),
	)
	sessionmgr.SetTmuxHooksForTest(
		t,
		func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("session discovery unavailable: %v", args)
		},
		nil,
	)
	err = deleteItem(context.Background(), sessionmgr.NewTmuxBackend(), line, false)
	if err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("deleteItem() error = %v, want cannot be deleted", err)
	}
}

func TestRunGetSessionsTextOutput(t *testing.T) {
	manifestTestDirs(t)
	dir := t.TempDir()
	raw := strings.Join([]string{"demo", "100", "200", dir, "0", "1"}, "\x1f") + "\n"
	sessionmgr.SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "list-sessions" {
			return []byte(raw), nil
		}
		return nil, nil
	}, nil)

	out, err := captureStdout(t, func() error {
		return run([]string{"--get-sessions"})
	})
	if err != nil {
		t.Fatalf("run(--get-sessions) error = %v", err)
	}
	if !strings.Contains(out, "demo") {
		t.Fatalf("sessions output missing demo:\n%s", out)
	}
}

func TestRunDeleteItemViaCLI(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	var killedTarget string
	installDeleteItemTmuxRecorder(t, "kill-session", func(args []string) {
		if len(args) >= 3 {
			killedTarget = args[2]
		}
	})

	out, err := captureStdout(t, func() error {
		return run([]string{"--delete-item", line, "--json"})
	})
	if err != nil {
		t.Fatalf("run(--delete-item) error = %v", err)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Deleted       bool   `json:"deleted"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || !payload.Ok || !payload.Deleted {
		t.Fatalf("envelope = %#v", payload)
	}
	if payload.Kind != string(sessionmgr.KindSession) || payload.Name != "demo" {
		t.Fatalf("delete payload = %#v", payload)
	}
	if killedTarget != "=demo" {
		t.Fatalf("kill-session target = %q, want =demo", killedTarget)
	}
}

func TestResolveDeleteItemTmuxUsesCurrentSessionsBeforeLegacyParsing(t *testing.T) {
	iconsOn := sessionmgr.DefaultIconSet()
	iconsOff := iconsOn
	iconsOff.Enabled = false
	tests := []struct {
		name  string
		raw   string
		icons sessionmgr.IconSet
		want  string
		ok    bool
	}{
		{name: "raw path target", raw: "/", icons: iconsOff, want: "/", ok: true},
		{
			name: "icons enabled rendered",
			raw: renderedDeleteLine(
				sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
				iconsOn,
			),
			icons: iconsOn,
			want:  "demo",
			ok:    true,
		},
		{name: "icons disabled label", raw: "demo", icons: iconsOff, want: "demo", ok: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := strings.Join([]string{"/", "1", "2", "/tmp", "0", "1"}, "\x1f") + "\n" +
				strings.Join([]string{"demo", "1", "2", "/tmp", "0", "1"}, "\x1f") + "\n"
			sessionmgr.SetTmuxHooksForTest(
				t,
				func(_ context.Context, args ...string) ([]byte, error) {
					if sessionmgr.MatchListSessions(args) {
						return []byte(raw), nil
					}
					return nil, fmt.Errorf("unexpected tmux call: %v", args)
				},
				nil,
			)
			item, ok := resolveDeleteItem(
				t.Context(),
				sessionmgr.NewTmuxBackend(),
				test.raw,
				test.icons,
			)
			if ok != test.ok || (ok && item.ActionTarget() != test.want) {
				t.Fatalf(
					"resolveDeleteItem() = %#v, %v; want target %q, %v",
					item,
					ok,
					test.want,
					test.ok,
				)
			}
		})
	}
}

func TestResolveDeleteItemHerdrPresentationFormsAndAmbiguity(t *testing.T) {
	binDir := t.TempDir()
	payload := `{"type":"workspace_list","workspaces":[` +
		`{"workspace_id":"/","label":"path label"},` +
		`{"workspace_id":"opaque-1","label":"friendly"},` +
		`{"workspace_id":"opaque-2","label":"duplicate"},` +
		`{"workspace_id":"opaque-3","label":"duplicate"},` +
		`{"workspace_id":" raw target ","label":"duplicate"},` +
		`{"workspace_id":"opaque-path-label","label":"../friendly"}]}`
	if err := os.WriteFile(filepath.Join(binDir, "herdr"), []byte(
		"#!/bin/sh\nprintf '%s\\n' '"+payload+"'\n",
	), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	iconsOn := sessionmgr.DefaultIconSet()
	iconsOff := iconsOn
	iconsOff.Enabled = false
	rendered := renderedDeleteLine(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "friendly"},
		iconsOn,
	)
	tests := []struct {
		name  string
		raw   string
		icons sessionmgr.IconSet
		want  string
		ok    bool
	}{
		{name: "raw opaque", raw: "opaque-1", icons: iconsOn, want: "opaque-1", ok: true},
		{
			name:  "raw whitespace opaque",
			raw:   " raw target ",
			icons: iconsOff,
			want:  " raw target ",
			ok:    true,
		},
		{name: "trimmed whitespace opaque rejected", raw: "raw target", icons: iconsOff, ok: false},
		{name: "raw path-like", raw: "/", icons: iconsOff, want: "/", ok: true},
		{
			name:  "path-like human label",
			raw:   "../friendly",
			icons: iconsOff,
			want:  "opaque-path-label",
			ok:    true,
		},
		{name: "rendered icons", raw: rendered, icons: iconsOn, want: "opaque-1", ok: true},
		{
			name:  "human label icons enabled",
			raw:   "friendly",
			icons: iconsOn,
			want:  "opaque-1",
			ok:    true,
		},
		{
			name:  "human label icons disabled",
			raw:   "friendly",
			icons: iconsOff,
			want:  "opaque-1",
			ok:    true,
		},
		{name: "duplicate label", raw: "duplicate", icons: iconsOff, ok: false},
		{
			name: "duplicate rendered",
			raw: renderedDeleteLine(
				sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "duplicate"},
				iconsOn,
			),
			icons: iconsOn,
			ok:    false,
		},
		{
			name:  "stale icon line while icons disabled",
			raw:   rendered,
			icons: iconsOff,
			ok:    false,
		},
		{name: "unmatched stale line", raw: "removed workspace", icons: iconsOff, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item, ok := resolveDeleteItem(
				t.Context(),
				sessionmgr.NewHerdrBackend(),
				test.raw,
				test.icons,
			)
			if ok != test.ok || (ok && item.ActionTarget() != test.want) {
				t.Fatalf(
					"resolveDeleteItem() = %#v, %v; want target %q, %v",
					item,
					ok,
					test.want,
					test.ok,
				)
			}
		})
	}
}

func TestDeleteItemRejectsStaleIconLineWithoutKill(t *testing.T) {
	manifestTestDirs(t)
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeNone
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}
	staleLine := renderedDeleteLine(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		sessionmgr.DefaultIconSet(),
	)
	killed := false
	sessionmgr.SetTmuxHooksForTest(
		t,
		func(_ context.Context, args ...string) ([]byte, error) {
			if sessionmgr.MatchListSessions(args) {
				return []byte(
					strings.Join([]string{"demo", "1", "2", "/tmp", "0", "1"}, "\x1f"),
				), nil
			}
			return nil, fmt.Errorf("unexpected tmux output call: %v", args)
		},
		func(_ context.Context, args ...string) error {
			if len(args) > 0 && args[0] == "kill-session" {
				killed = true
			}
			return nil
		},
	)
	err := deleteItem(t.Context(), sessionmgr.NewTmuxBackend(), staleLine, false)
	if err == nil || !strings.Contains(err.Error(), "unrecognized item line") {
		t.Fatalf("deleteItem() error = %v, want safe rejection", err)
	}
	if killed {
		t.Fatal("KillSession was invoked for a stale icon line")
	}
}

func TestResolveDeleteItemFallsBackWhenDiscoveryFails(t *testing.T) {
	icons := sessionmgr.DefaultIconSet()
	line := renderedDeleteLine(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "legacy"},
		icons,
	)
	sessionmgr.SetTmuxHooksForTest(
		t,
		func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("discovery unavailable: %v", args)
		},
		nil,
	)
	item, ok := resolveDeleteItem(t.Context(), sessionmgr.NewTmuxBackend(), line, icons)
	if !ok || item.Kind != sessionmgr.KindSession || item.ActionTarget() != "legacy" {
		t.Fatalf("resolveDeleteItem() = %#v, %v, want legacy parser fallback", item, ok)
	}
}

func TestRunDeleteHerdrOpaquePathTargetsWithIconsDisabled(t *testing.T) {
	manifestTestDirs(t)
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("TMUX", "")
	cfg := appconfig.Default()
	cfg.Icons.Mode = appconfig.IconModeNone
	if err := appconfig.Save(cfg); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "closed")
	payload := `{"type":"workspace_list","workspaces":[` +
		`{"workspace_id":"/","label":"duplicate"},` +
		`{"workspace_id":"~/","label":"duplicate"},` +
		`{"workspace_id":"./","label":"different label"},` +
		`{"workspace_id":"../","label":"id mismatch"},` +
		`{"workspace_id":" raw ","label":"duplicate"}]}`
	script := "#!/bin/sh\n" +
		"if [ \"$1 $2\" = 'workspace list' ]; then printf '%s\\n' '" + payload + "'; exit; fi\n" +
		"if [ \"$1 $2\" = 'workspace close' ]; then printf '%s\\n' \"$3\" >>\"$SESHAGY_TEST_RECORD\"; exit; fi\n" +
		"if [ \"$1 $2\" = 'workspace focus' ]; then exit; fi\n" +
		"exit 97\n"
	if err := os.WriteFile(filepath.Join(binDir, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SESHAGY_TEST_RECORD", record)
	for _, target := range []string{"/", "~/", "./", "../", " raw "} {
		if err := run([]string{"--delete-item", target}); err != nil {
			t.Fatalf("delete opaque target %q: %v", target, err)
		}
	}
	closed, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(closed), "/\n~/\n./\n../\n raw \n"; got != want {
		t.Fatalf("closed targets = %q, want %q", got, want)
	}
}

func TestDeleteItemUnrecognizedLine(t *testing.T) {
	manifestTestDirs(t)
	err := deleteItem(
		context.Background(),
		sessionmgr.NewTmuxBackend(),
		"not a valid item line",
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "unrecognized item line") {
		t.Fatalf("deleteItem() error = %v, want unrecognized item line", err)
	}
}

func TestDeleteItemKillFailure(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	killErr := fmt.Errorf("tmux refused kill")
	tests := []struct {
		name    string
		wantCmd string
		line    string
	}{
		{
			name:    "session",
			wantCmd: "kill-session",
			line: sessionmgr.FormatLineWithIcons(
				sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
				cfg.IconSet(),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionmgr.SetTmuxHooksForTest(
				t,
				func(_ context.Context, args ...string) ([]byte, error) {
					return nil, fmt.Errorf("session discovery unavailable: %v", args)
				},
				func(_ context.Context, args ...string) error {
					if len(args) >= 1 && args[0] == tt.wantCmd {
						return killErr
					}
					return fmt.Errorf("unexpected tmux call: %v", args)
				},
			)
			err := deleteItem(context.Background(), sessionmgr.NewTmuxBackend(), tt.line, false)
			if err == nil {
				t.Fatalf("deleteItem() expected error for %s failure", tt.wantCmd)
			}
			if !strings.Contains(err.Error(), "tmux "+tt.wantCmd) {
				t.Fatalf("deleteItem() error = %v, want tmux %s wrapper", err, tt.wantCmd)
			}
		})
	}
}

func TestDeleteItemKillFailureJSON(t *testing.T) {
	manifestTestDirs(t)
	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	killErr := fmt.Errorf("tmux refused kill")
	line := sessionmgr.FormatLineWithIcons(
		sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "demo"},
		cfg.IconSet(),
	)
	sessionmgr.SetTmuxHooksForTest(
		t,
		func(_ context.Context, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("session discovery unavailable: %v", args)
		},
		func(_ context.Context, args ...string) error {
			if len(args) >= 1 && args[0] == "kill-session" {
				return killErr
			}
			return fmt.Errorf("unexpected tmux call: %v", args)
		},
	)

	args := []string{"--delete-item", line, "--json"}
	err = run(args)
	if err == nil {
		t.Fatal("run() error = nil, want kill-session error")
	}
	if !strings.Contains(err.Error(), "tmux kill-session") {
		t.Fatalf("error = %q, want tmux kill-session wrapper", err.Error())
	}
	out, encErr := captureStdout(t, func() error {
		return encodeJSONError(err)
	})
	if encErr != nil {
		t.Fatalf("encodeJSONError() error = %v", encErr)
	}
	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		Ok            bool   `json:"ok"`
		Error         string `json:"error"`
		Code          string `json:"code"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, out=%q", err, out)
	}
	if payload.SchemaVersion != 1 || payload.Ok {
		t.Fatalf("envelope = %#v, want ok=false", payload)
	}
	if !strings.Contains(payload.Error, "tmux kill-session") {
		t.Fatalf("error = %q, want tmux kill-session wrapper", payload.Error)
	}
	if payload.Code != "error" {
		t.Fatalf("code = %q, want error", payload.Code)
	}
}
