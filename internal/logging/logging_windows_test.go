//go:build windows

package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsExplicitFileTruncatesAndLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnostics.jsonl")
	if err := os.WriteFile(path, []byte("old evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := Resolved{
		LevelName: "debug",
		MinLevel:  slog.LevelDebug,
		Enabled:   true,
		Explicit:  true,
		File:      path,
		Directory: filepath.Dir(path),
	}
	first, err := Open(resolved, Metadata{AppVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Shutdown() })
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("explicit destination = (%v, %v)", info, err)
	}
	if _, err := Open(resolved, Metadata{AppVersion: "test"}); err == nil {
		t.Fatal("contending opener unexpectedly acquired explicit destination")
	}
}

func TestWindowsRetentionDeletesUnlockedLogs(t *testing.T) {
	dir := t.TempDir()
	for i := range 12 {
		name := "seshagy-20250101T0000" + string(rune('0'+i/10)) +
			string(rune('0'+i%10)) + ".000000000Z-1-" +
			[]string{
				"0000000000000001", "0000000000000002", "0000000000000003",
				"0000000000000004", "0000000000000005", "0000000000000006",
				"0000000000000007", "0000000000000008", "0000000000000009",
				"000000000000000a", "000000000000000b", "000000000000000c",
			}[i] + ".jsonl"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
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
	if count != RetainFiles {
		t.Fatalf("retained logs = %d, want %d", count, RetainFiles)
	}
}
