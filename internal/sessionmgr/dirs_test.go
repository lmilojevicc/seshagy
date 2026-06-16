package sessionmgr

import (
	"context"
	"os"
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
