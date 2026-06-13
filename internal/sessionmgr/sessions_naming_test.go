package sessionmgr

import (
	"path/filepath"
	"testing"
)

func TestParentQualifiedSessionName(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{dir: "/home/u/work/api", want: "work-api"},
		{dir: "/home/u/personal/api", want: "personal-api"},
		{dir: "/api", want: "api"},
	}
	for _, tt := range tests {
		if got := parentQualifiedSessionName(tt.dir); got != tt.want {
			t.Fatalf("parentQualifiedSessionName(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

func TestSessionNameForPathMatchesRegardlessOfName(t *testing.T) {
	sessions := []Item{
		{Kind: KindSession, Name: "work-api", Path: "/home/u/work/api"},
		{Kind: KindSession, Name: "other", Path: "/home/u/other"},
	}
	if got := sessionNameForPath(sessions, "/home/u/work/api/"); got != "work-api" {
		t.Fatalf("sessionNameForPath = %q, want work-api", got)
	}
	if got := sessionNameForPath(sessions, "/home/u/personal/api"); got != "" {
		t.Fatalf("sessionNameForPath = %q, want empty", got)
	}
}

func TestSessionNameForPathMatchesTildeAgainstAbsolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessions := []Item{
		{Kind: KindSession, Name: "api", Path: "~/work/api"},
	}
	if got := sessionNameForPath(sessions, filepath.Join(home, "work", "api")); got != "api" {
		t.Fatalf("sessionNameForPath = %q, want api (tilde should expand)", got)
	}
}

func TestUniqueSessionNameAppendsSuffix(t *testing.T) {
	sessions := []Item{
		{Name: "api"},
		{Name: "api-2"},
	}
	if got := uniqueSessionName(sessions, "api"); got != "api-3" {
		t.Fatalf("uniqueSessionName = %q, want api-3", got)
	}
	if got := uniqueSessionName(sessions, "fresh"); got != "fresh" {
		t.Fatalf("uniqueSessionName = %q, want fresh", got)
	}
}

func TestNormalizePathCleansAndAbsolutizes(t *testing.T) {
	if got := normalizePath("/a/b/../b/c"); got != "/a/b/c" {
		t.Fatalf("normalizePath = %q, want /a/b/c", got)
	}
	rel := normalizePath("rel/dir")
	if !filepath.IsAbs(rel) {
		t.Fatalf("normalizePath(%q) = %q, want absolute", "rel/dir", rel)
	}
}
