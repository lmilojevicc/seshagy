package sessionmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirectoryPreviewReadDirFallback(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "beta"), 0o755); err != nil {
		t.Fatalf("Mkdir() = %v", err)
	}

	got, err := ListDirectoryPreview(context.Background(), dir, 0)
	if err != nil {
		t.Fatalf("ListDirectoryPreview() = %v", err)
	}
	if !strings.Contains(got, "alpha.txt") || !strings.Contains(got, "beta") {
		t.Fatalf("preview = %q, want alpha.txt and beta entries", got)
	}
}

func TestLimitLinesTruncates(t *testing.T) {
	input := "one\ntwo\nthree\nfour\n"
	if got := limitLines(input, 2); got != "one\ntwo" {
		t.Fatalf("limitLines() = %q, want two lines", got)
	}
	if got := limitLines(input, 0); got != input {
		t.Fatalf("limitLines(0) = %q, want full input", got)
	}
}

func TestRunYaziCommandUsesCwdFile(t *testing.T) {
	cmd := RunYaziCommand("/tmp/seshagy-yazi-cwd")
	if filepath.Base(cmd.Path) != "yazi" {
		t.Fatalf("command = %q, want yazi", cmd.Path)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "--cwd-file" || cmd.Args[2] != "/tmp/seshagy-yazi-cwd" {
		t.Fatalf("args = %#v", cmd.Args)
	}
	empty := RunYaziCommand("")
	if !strings.HasSuffix(empty.Args[2], "seshagy-yazi-cwd") {
		t.Fatalf("default cwd file = %q", empty.Args[2])
	}
}

func TestListDirectoryPreviewMissingDir(t *testing.T) {
	_, err := ListDirectoryPreview(context.Background(), filepath.Join(t.TempDir(), "missing"), 0)
	if err == nil {
		t.Fatal("expected missing directory error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("error = %v, want not exist", err)
	}
}

func TestDirItems(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		kind  Kind
		want  []string
	}{
		{
			name:  "zoxide paths",
			input: []byte("/tmp/a\n\n/tmp/b\n"),
			kind:  KindZoxide,
			want:  []string{"/tmp/a", "/tmp/b"},
		},
		{
			name:  "fd paths",
			input: []byte("~/Projects\n"),
			kind:  KindFD,
			want:  []string{"~/Projects"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := dirItems(tt.input, tt.kind)
			if len(items) != len(tt.want) {
				t.Fatalf("items = %d, want %d", len(items), len(tt.want))
			}
			for i, item := range items {
				if item.Kind != tt.kind {
					t.Fatalf("item[%d].Kind = %q, want %q", i, item.Kind, tt.kind)
				}
				if item.Path != tt.want[i] || item.Name != tt.want[i] {
					t.Fatalf("item[%d] = %#v, want path %q", i, item, tt.want[i])
				}
			}
		})
	}
}

// TestListFDirsWithCommandFallback covers the default-command fallback: when
// `fd` is not on PATH the default command is replaced by an equivalent `find`
// that includes hidden directories and prunes .git/node_modules/.Trash; a
// user-supplied custom command is never rewritten.
func TestListFDirsWithCommandFallback(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{"alpha", "beta", ".hidden", ".git", "node_modules/skip", "deep/child"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) = %v", dir, err)
		}
	}

	// A PATH that can run `sh` and `find` but not `fd`.
	realFind, err := exec.LookPath("find")
	if err != nil {
		t.Skipf("find not available: %v", err)
	}
	realSh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	binDir := t.TempDir()
	for name, target := range map[string]string{"find": realFind, "sh": realSh} {
		if err := os.Symlink(target, filepath.Join(binDir, name)); err != nil {
			t.Fatalf("Symlink(%s) = %v", name, err)
		}
	}

	tests := []struct {
		name        string
		command     string
		fdOnPath    bool
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "default command falls back to find",
			command:     "",
			wantContain: []string{"alpha", "beta", ".hidden", "deep", "child"},
			wantAbsent:  []string{"node_modules", ".git"},
		},
		{
			name:        "explicit default command falls back to find",
			command:     DefaultFDCommand,
			wantContain: []string{"alpha"},
			wantAbsent:  []string{"node_modules", ".git"},
		},
		{
			name:        "custom command is run unchanged",
			command:     `find "$HOME" -mindepth 1 -maxdepth 1 -type d -print`,
			wantContain: []string{"alpha"},
			wantAbsent:  []string{"child"},
		},
		{
			name:        "fd on path is used",
			command:     "",
			fdOnPath:    true,
			wantContain: []string{"from-fd"},
			wantAbsent:  []string{"alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fdPath := filepath.Join(binDir, "fd")
			if tt.fdOnPath {
				script := "#!/bin/sh\nprintf '%s\\n' \"$HOME/from-fd\"\n"
				if err := os.WriteFile(fdPath, []byte(script), 0o755); err != nil {
					t.Fatalf("WriteFile(fd) = %v", err)
				}
				t.Cleanup(func() { _ = os.Remove(fdPath) })
			}
			t.Setenv("PATH", binDir)
			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", "")

			items, err := ListFDirsWithCommand(context.Background(), tt.command)
			if err != nil {
				t.Fatalf("ListFDirsWithCommand() = %v", err)
			}
			joined := make([]string, 0, len(items))
			for _, item := range items {
				if item.Kind != KindFD {
					t.Fatalf("item.Kind = %q, want %q", item.Kind, KindFD)
				}
				if !filepath.IsAbs(item.Target) {
					t.Fatalf("item.Target = %q, want absolute path", item.Target)
				}
				joined = append(joined, item.Path)
			}
			all := strings.Join(joined, "\n")
			for _, want := range tt.wantContain {
				if !strings.Contains(all, want) {
					t.Fatalf("paths = %q, want to contain %q", all, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(all, absent) {
					t.Fatalf("paths = %q, want to exclude %q", all, absent)
				}
			}
		})
	}
}

func TestFDAvailable(t *testing.T) {
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, "fd"))
	if err != nil {
		t.Fatalf("Create() = %v", err)
	}
	_ = file.Close()
	if err := os.Chmod(filepath.Join(dir, "fd"), 0o755); err != nil {
		t.Fatalf("Chmod() = %v", err)
	}
	t.Setenv("PATH", dir)
	if !FDAvailable() {
		t.Fatal("FDAvailable() = false with fd on PATH")
	}
	t.Setenv("PATH", t.TempDir())
	if FDAvailable() {
		t.Fatal("FDAvailable() = true with fd absent from PATH")
	}
}
