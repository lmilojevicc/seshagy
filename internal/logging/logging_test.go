package logging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func clearLogEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SESHAGY_LOG_LEVEL", "")
	t.Setenv("SESHAGY_LOG_FILE", "")
}

func TestResolvePrecedenceAndOff(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	lookup := func(values map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		}
	}
	tests := []struct {
		name     string
		cfg      Config
		env      map[string]string
		level    string
		enabled  bool
		explicit bool
		wantErr  bool
	}{
		{name: "empty is off", cfg: Config{}, level: "off"},
		{
			name:     "file alone stays off",
			cfg:      Config{File: "debug.jsonl"},
			level:    "off",
			explicit: true,
		},
		{name: "case insensitive", cfg: Config{Level: "DeBuG"}, level: "debug", enabled: true},
		{
			name: "environment overrides",
			cfg:  Config{Level: "warn"},
			env: map[string]string{
				"SESHAGY_LOG_LEVEL": "INFO",
				"SESHAGY_LOG_FILE":  "env.jsonl",
			},
			level:    "info",
			enabled:  true,
			explicit: true,
		},
		{
			name:    "empty environment does not override",
			cfg:     Config{Level: "error"},
			env:     map[string]string{"SESHAGY_LOG_LEVEL": ""},
			level:   "error",
			enabled: true,
		},
		{name: "invalid", cfg: Config{Level: "verbose"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.cfg, lookup(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil &&
				(got.LevelName != tt.level || got.Enabled != tt.enabled || got.Explicit != tt.explicit) {
				t.Fatalf("Resolve() = %+v", got)
			}
		})
	}
}

func TestSafetyHandlerFiltersUnknownAndNonDebugIDs(t *testing.T) {
	var buf bytes.Buffer
	handler := newSafetyHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	logger := slog.New(handler).With(
		slog.Int("schema_version", SchemaVersion),
		slog.String("run_id", "run"), slog.String("app_version", "test"),
	)
	LogAttrs(
		context.Background(),
		logger,
		slog.LevelInfo,
		EventSessionKill,
		ComponentSession,
		slog.String(
			"backend",
			"tmux",
		),
		slog.String("result", "success"),
		slog.Int64("duration_ms", 1),
		slog.String("pane_id", "SECRETID"),
		slog.String("unknown", "SECRETVALUE"),
	)
	if bytes.Contains(buf.Bytes(), []byte("SECRET")) {
		t.Fatalf("unsafe attributes reached output: %s", buf.String())
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	if record["event"] != string(EventSessionKill) || record["backend"] != "tmux" {
		t.Fatalf("unexpected record: %#v", record)
	}

	buf.Reset()
	LogAttrs(
		context.Background(),
		logger,
		slog.LevelDebug,
		EventAgentReport,
		ComponentAgents,
		slog.String(
			"backend",
			"tmux",
		),
		slog.String("pane_id", "%3"),
		slog.String("state", "working"),
		slog.Int64("seq", 2),
		slog.String("result", "applied"),
	)
	if !bytes.Contains(buf.Bytes(), []byte(`"pane_id":"%3"`)) {
		t.Fatalf("debug id missing: %s", buf.String())
	}
}

type changingLogValuer struct{ calls *int }

func (v changingLogValuer) LogValue() slog.Value {
	*v.calls++
	if *v.calls == 1 {
		return slog.StringValue("success")
	}
	return slog.StringValue("SecretLogValuerLeakABC123")
}

func TestSafetyHandlerResolvesDynamicValuesOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newSafetyHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)).With(
		slog.Int("schema_version", SchemaVersion),
		slog.String("run_id", "run"),
		slog.String("app_version", "test"),
	)
	calls := 0
	LogAttrs(context.Background(), logger, slog.LevelInfo, EventSessionKill, ComponentSession,
		slog.String("backend", "tmux"),
		slog.Any("result", changingLogValuer{calls: &calls}),
		slog.Int64("duration_ms", 1))
	if calls != 1 || bytes.Contains(buf.Bytes(), []byte("SecretLogValuerLeakABC123")) ||
		!bytes.Contains(buf.Bytes(), []byte(`"result":"success"`)) {
		t.Fatalf("calls=%d output=%s", calls, buf.String())
	}
}

func TestSafetyHandlerValidatesCombinationsAndTypes(t *testing.T) {
	newLogger := func(buf *bytes.Buffer) *slog.Logger {
		return slog.New(newSafetyHandler(
			slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)).With(
			slog.Int("schema_version", SchemaVersion),
			slog.String("run_id", "run"),
			slog.String("app_version", "test"),
		)
	}
	t.Run("info drops debug id", func(t *testing.T) {
		var buf bytes.Buffer
		LogAttrs(context.Background(), newLogger(&buf), slog.LevelInfo,
			EventAgentReport, ComponentAgents,
			slog.String("backend", "tmux"), slog.String("pane_id", "SecretPaneID"),
			slog.String("state", "working"), slog.Int64("seq", 1),
			slog.String("result", "applied"))
		if bytes.Contains(buf.Bytes(), []byte("SecretPaneID")) {
			t.Fatalf("info record retained debug id: %s", buf.String())
		}
	})
	t.Run("with attrs preserves validated scalar", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(&buf).With(slog.String("backend", "tmux"))
		LogAttrs(context.Background(), logger, slog.LevelInfo,
			EventSessionKill, ComponentSession,
			slog.String("result", "success"), slog.Int64("duration_ms", 1))
		if !bytes.Contains(buf.Bytes(), []byte(`"backend":"tmux"`)) {
			t.Fatalf("validated WithAttrs value missing: %s", buf.String())
		}
	})
	t.Run("with group rejects record", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(&buf).WithGroup("unsafe")
		LogAttrs(context.Background(), logger, slog.LevelInfo,
			EventSessionKill, ComponentSession,
			slog.String("backend", "tmux"), slog.String("result", "success"),
			slog.Int64("duration_ms", 1))
		if buf.Len() != 0 {
			t.Fatalf("grouped record reached output: %s", buf.String())
		}
	})
	t.Run("wrong types enums and any are dropped", func(t *testing.T) {
		var buf bytes.Buffer
		LogAttrs(context.Background(), newLogger(&buf), slog.LevelInfo,
			EventSessionKill, ComponentSession,
			slog.String("backend", "SecretBackend"), slog.String("result", "success"),
			slog.String("duration_ms", "SecretDuration"),
			slog.Any("error_class", struct{ Secret string }{"SecretAny"}))
		if bytes.Contains(buf.Bytes(), []byte("Secret")) {
			t.Fatalf("invalid values reached output: %s", buf.String())
		}
	})
	t.Run("unregistered combination is rejected", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(&buf)
		logger.LogAttrs(context.Background(), slog.LevelInfo, "wrong.message",
			slog.String("event", string(EventSessionKill)),
			slog.String("component", string(ComponentSession)),
			slog.String("backend", "tmux"), slog.String("result", "success"),
			slog.Int64("duration_ms", 1))
		if buf.Len() != 0 {
			t.Fatalf("unregistered message reached output: %s", buf.String())
		}
	})
	t.Run("copilot agent type is allowed", func(t *testing.T) {
		var buf bytes.Buffer
		LogAttrs(context.Background(), newLogger(&buf), slog.LevelDebug,
			EventManifestStateChange, ComponentManifest,
			slog.String("pane_id", "%1"), slog.String("agent_type", "copilot"),
			slog.String("previous_state", "idle"), slog.String("state", "working"))
		if !bytes.Contains(buf.Bytes(), []byte(`"agent_type":"copilot"`)) {
			t.Fatalf("copilot type missing: %s", buf.String())
		}
	})
}

func TestOffRuntimeHasNoSideEffects(t *testing.T) {
	clearLogEnv(t)
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", state)
	resolved, err := Resolve(Config{}, os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("off mode touched state directory: %v", err)
	}
}

func TestRuntimeDoesNotMutateGoLogGlobals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	resolved, err := Resolve(
		Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
	beforeSlog, beforeWriter := slog.Default(), log.Writer()
	beforeFlags, beforePrefix := log.Flags(), log.Prefix()
	runtime, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Activate()
	t.Cleanup(func() { _ = runtime.Shutdown() })
	if slog.Default() != beforeSlog || log.Writer() != beforeWriter || log.Flags() != beforeFlags ||
		log.Prefix() != beforePrefix {
		t.Fatal("activation mutated Go log globals")
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeOutOfOrderShutdownRestoresLiveLogger(t *testing.T) {
	before := Default()
	openRuntime := func(name string) *Runtime {
		t.Helper()
		resolved, err := Resolve(
			Config{Level: "debug", File: filepath.Join(t.TempDir(), name)},
			func(string) (string, bool) { return "", false },
		)
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := Open(resolved, Metadata{AppVersion: "test"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runtime.Shutdown() })
		return runtime
	}
	first := openRuntime("first.jsonl")
	second := openRuntime("second.jsonl")
	first.Activate()
	second.Activate()
	if Default() != second.Logger() {
		t.Fatal("newest runtime is not package default")
	}
	if err := first.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if Default() != second.Logger() {
		t.Fatal("closing an older runtime replaced the live default")
	}
	if err := second.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if Default() != before {
		t.Fatal("closing the final runtime did not restore the base logger")
	}
}

func TestDefaultLogNameValidation(t *testing.T) {
	valid := "seshagy-20250101T000000.000000000Z-123-0123456789abcdef.jsonl"
	invalid := []string{
		"seshagy-99999999T999999.999999999Z-123-0123456789abcdef.jsonl",
		"seshagy-20250101T000000.000000000Z-0-0123456789abcdef.jsonl",
		"seshagy-20250101T000000.000000000Z-notapid-0123456789abcdef.jsonl",
	}
	if !isDefaultLogName(valid) {
		t.Fatalf("valid default name rejected: %s", valid)
	}
	for _, name := range invalid {
		if isDefaultLogName(name) {
			t.Fatalf("invalid default name accepted: %s", name)
		}
	}
}

func TestExplicitFileLocksBeforeTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	if err := os.WriteFile(path, []byte("old evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(
		Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Shutdown() })
	LogAttrs(context.Background(), first.Logger(), slog.LevelInfo, EventAppStart, ComponentApp,
		slog.String("backend", "none"), slog.String("log_level", "debug"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(resolved, Metadata{AppVersion: "test"}); err == nil {
		t.Fatal("second opener unexpectedly acquired explicit destination")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("contending opener changed live log")
	}
	if err := first.Shutdown(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRetentionSkipsLockedLiveLog(t *testing.T) {
	dir := t.TempDir()
	var locked *os.File
	for i := range 12 {
		name := fmt.Sprintf("seshagy-20250101T0000%02d.000000000Z-1-%016x.jsonl", i, i+1)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			var err error
			locked, err = os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := tryLockFile(locked); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = unlockFile(locked)
				_ = locked.Close()
			})
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := Resolved{
		LevelName: "debug",
		MinLevel:  slog.LevelDebug,
		Enabled:   true,
		Directory: dir,
	}
	runtime, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if isDefaultLogName(entry.Name()) {
			count++
		}
	}
	if count != 11 {
		t.Fatalf("matching logs = %d, want 11 (10 newest plus locked old log)", count)
	}
	if _, err := os.Stat(filepath.Join(dir, "unrelated.txt")); err != nil {
		t.Fatal(err)
	}
	_ = unlockFile(locked)
	_ = locked.Close()
}

func TestRetentionSkipsLockedLogAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	var oldest string
	for i := range 12 {
		name := fmt.Sprintf("seshagy-20240101T0000%02d.000000000Z-2-%016x.jsonl", i, i+20)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(i+1), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldest = path
		}
	}
	cmd := exec.Command(
		os.Args[0],
		"-test.run=^TestExplicitFileLockAcrossProcesses$",
		"-test.count=1",
	)
	cmd.Env = append(os.Environ(), "SESHAGY_LOCK_HELPER="+oldest)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("helper readiness = %q, %v", line, err)
	}
	oldStamp := time.Unix(1, 0)
	if err := os.Chtimes(oldest, oldStamp, oldStamp); err != nil {
		t.Fatal(err)
	}

	resolved := Resolved{
		LevelName: "debug",
		MinLevel:  slog.LevelDebug,
		Enabled:   true,
		Directory: dir,
	}
	runtime, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Shutdown(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if isDefaultLogName(entry.Name()) {
			count++
		}
	}
	if count != 11 {
		t.Fatalf("matching logs = %d, want 11", count)
	}
	if _, err := os.Stat(oldest); err != nil {
		t.Fatalf("locked live log was pruned: %v", err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitFileLockAcrossProcesses(t *testing.T) {
	if path := os.Getenv("SESHAGY_LOCK_HELPER"); path != "" {
		resolved, err := Resolve(
			Config{Level: "debug", File: path},
			func(string) (string, bool) { return "", false },
		)
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := Open(resolved, Metadata{AppVersion: "test"})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = os.Stdout.Write([]byte("locked\n"))
		var wait [1]byte
		_, _ = os.Stdin.Read(wait[:])
		if err := runtime.Shutdown(); err != nil {
			t.Fatal(err)
		}
		return
	}

	path := filepath.Join(t.TempDir(), "shared.jsonl")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(
		os.Args[0],
		"-test.run=^TestExplicitFileLockAcrossProcesses$",
		"-test.count=1",
	)
	cmd.Env = append(os.Environ(), "SESHAGY_LOCK_HELPER="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("helper readiness = %q, %v", line, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := Resolve(
		Config{Level: "debug", File: path},
		func(string) (string, bool) { return "", false },
	)
	contender, openErr := Open(resolved, Metadata{AppVersion: "test"})
	if openErr == nil {
		_ = contender.Shutdown()
		t.Fatal("second process lock unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("contending process truncated live file")
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

type shortWriter struct{ calls int }

func (w *shortWriter) Write(data []byte) (int, error) {
	w.calls++
	return len(data) / 2, nil
}

func TestCappedWriterStopsAfterShortWrite(t *testing.T) {
	underlying := &shortWriter{}
	writer, err := newCappedWriter(underlying, 128, []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("{\"event\":\"first\"}\n"))
	_, _ = writer.Write([]byte("{\"event\":\"second\"}\n"))
	if !errors.Is(writer.Err(), io.ErrShortWrite) || underlying.calls != 1 {
		t.Fatalf("Err() = %v, calls = %d", writer.Err(), underlying.calls)
	}
}

func TestCappedWriterWritesOneWholeMarker(t *testing.T) {
	marker := []byte("{\"event\":\"log.limit_reached\"}\n")
	var buf bytes.Buffer
	writer, err := newCappedWriter(&buf, 64, marker)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte("{\"event\":\"first\"}\n"))
	_, _ = writer.Write([]byte("{\"event\":\"second-record-is-too-large\"}\n"))
	_, _ = writer.Write([]byte("{\"event\":\"third\"}\n"))
	if bytes.Count(buf.Bytes(), []byte("log.limit_reached")) != 1 || int64(buf.Len()) > 64 {
		t.Fatalf("capped output = %q (%d bytes)", buf.String(), buf.Len())
	}
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var value any
		if err := json.Unmarshal(line, &value); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
	}
}
