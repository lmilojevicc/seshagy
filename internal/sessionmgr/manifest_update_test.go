package sessionmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompareManifestVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026.06.10.1", "2026.06.10.1", 0},
		{"2026.06.10.1", "2026.06.24.1", -1},
		{"2026.06.24.1", "2026.06.10.1", 1},
		{"1.10", "1.9", 1},
		{"1.9", "1.10", -1},
		{"1.2", "1.2.0", 0},
		{"1.2", "1.2.1", -1},
		{"", "", 0},
	}
	for _, tc := range cases {
		got := compareManifestVersion(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareManifestVersion(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFetchManifestUpdatesWritesCache(t *testing.T) {
	t.Setenv(allowHTTPCatalogEnv, "1")
	mux := http.NewServeMux()
	mux.HandleFunc("/index.toml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`schema_version = 1

[[agents]]
id = "testagent1"
path = "testagent1.toml"

[[agents]]
id = "testagent2"
path = "testagent2.toml"
`))
	})
	mux.HandleFunc("/testagent1.toml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`id = "testagent1"
version = "2026.06.10.1"
[[rules]]
id = "r1"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["permission prompt"]
`))
	})
	mux.HandleFunc("/testagent2.toml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`id = "testagent2"
version = "2026.06.10.1"
[[rules]]
id = "r2"
state = "working"
priority = 100
region = "whole_recent"
contains = ["spinner"]
`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Redirect cache dir to temp.
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	result, err := FetchManifestUpdates(context.Background(), server.URL+"/index.toml")
	if err != nil {
		t.Fatalf("FetchManifestUpdates: %v", err)
	}
	if len(result.Fetched) != 2 {
		t.Fatalf("expected 2 fetched, got %d (skipped: %v)", len(result.Fetched), result.Skipped)
	}

	// Verify cache files exist.
	cacheDir, _ := cachedManifestDir()
	for _, id := range []string{"testagent1", "testagent2"} {
		path := filepath.Join(cacheDir, id+".toml")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("cache file %s not written: %v", path, err)
		}
	}
}

func TestFetchManifestUpdatesRejectsMalformed(t *testing.T) {
	t.Setenv(allowHTTPCatalogEnv, "1")
	mux := http.NewServeMux()
	mux.HandleFunc("/index.toml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`schema_version = 1

[[agents]]
id = "good"
path = "good.toml"

[[agents]]
id = "bad"
path = "bad.toml"
`))
	})
	mux.HandleFunc("/good.toml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`id = "good"
version = "1.0"
[[rules]]
id = "r1"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["prompt"]
`))
	})
	mux.HandleFunc("/bad.toml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`id = "bad"
[[rules]]
id = "empty"
state = "blocked"
`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	result, err := FetchManifestUpdates(context.Background(), server.URL+"/index.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Fetched) != 1 || result.Fetched[0] != "good" {
		t.Errorf("expected [good] fetched, got %v", result.Fetched)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "bad" {
		t.Errorf("expected [bad] skipped, got %v", result.Skipped)
	}
}

func TestReloadManifestsPicksUpCache(t *testing.T) {
	t.Cleanup(ReloadManifests) // runs AFTER t.Setenv cleanup (LIFO) — restores real dirs
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	cacheDir, _ := cachedManifestDir()
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write a manifest for a NEW agent id not in the embed.
	mustWriteFile(t, filepath.Join(cacheDir, "customagent.toml"), `id = "customagent"
version = "1.0"
[[rules]]
id = "custom"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["custom permission text"]
`)

	ReloadManifests()

	m, ok := manifestForAgent("customagent")
	if !ok {
		t.Fatal("customagent manifest not found after reload")
	}
	if len(m.rules) != 1 || m.rules[0].id != "custom" {
		t.Errorf("unexpected rules: %+v", m.rules)
	}
}

func TestBuildManifestCachePrecedenceOverrideBeatsCacheBeatsEmbed(t *testing.T) {
	t.Cleanup(ReloadManifests) // runs AFTER t.Setenv cleanup (LIFO)
	tmpCache := t.TempDir()
	tmpConfig := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)
	t.Setenv("XDG_CONFIG_HOME", tmpConfig)

	// Use a known embed agent (antigravity has id "agy").
	// Write a cache manifest v2 and an override manifest v3 for the same id.
	cacheDir, _ := cachedManifestDir()
	mustMkdirAll(t, cacheDir)
	mustWriteFile(t, filepath.Join(cacheDir, "agy.toml"), `id = "agy"
version = "2026.07.01.1"
aliases = ["antigravity"]
[[rules]]
id = "cache-rule"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["cache version text"]
`)

	overrideDir, _ := overrideManifestDir()
	mustMkdirAll(t, overrideDir)
	mustWriteFile(t, filepath.Join(overrideDir, "agy.toml"), `id = "agy"
version = "2026.08.01.1"
aliases = ["antigravity"]
[[rules]]
id = "override-rule"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["override version text"]
`)

	ReloadManifests()

	m, ok := manifestForAgent("agy")
	if !ok {
		t.Fatal("agy manifest not found")
	}
	// Override (v3) should win — its rule id is "override-rule".
	if len(m.rules) != 1 || m.rules[0].id != "override-rule" {
		t.Errorf("expected override-rule to win, got: %+v", m.rules)
	}
}

func TestBuildManifestCacheRemoteMustNotDowngrade(t *testing.T) {
	t.Cleanup(ReloadManifests) // runs AFTER t.Setenv cleanup (LIFO)
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)

	// The embed antigravity.toml is version "2026.06.10.1". Write a cache
	// manifest with a LOWER version — it must NOT shadow the embed.
	cacheDir, _ := cachedManifestDir()
	mustMkdirAll(t, cacheDir)
	mustWriteFile(t, filepath.Join(cacheDir, "agy.toml"), `id = "agy"
version = "0.0.1"
aliases = ["antigravity"]
[[rules]]
id = "stale-cache-rule"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["stale text"]
`)

	ReloadManifests()

	m, ok := manifestForAgent("agy")
	if !ok {
		t.Fatal("agy manifest not found")
	}
	// Embed should win — should have the embed's rules (permission_prompt etc).
	for _, rule := range m.rules {
		if rule.id == "stale-cache-rule" {
			t.Error("stale cache manifest should NOT shadow the embed")
		}
	}
}

func TestHttpsOnlyByDefault(t *testing.T) {
	// http:// must be refused by default.
	_, err := FetchManifestUpdates(context.Background(), "http://localhost/index.toml")
	if err == nil {
		t.Error("expected error for http:// URL without env override")
	}

	// With env override, http:// is allowed (will fail on network, but should
	// pass validation).
	t.Setenv(allowHTTPCatalogEnv, "1")
	_, err = FetchManifestUpdates(context.Background(), "http://localhost:1/index.toml")
	if err != nil && strings.Contains(err.Error(), "must be HTTPS") {
		t.Error("http:// should be allowed with env override")
	}
}

func TestOverrideAlwaysWinsRegardlessOfVersion(t *testing.T) {
	t.Cleanup(ReloadManifests)
	tmpCache := t.TempDir()
	tmpConfig := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)
	t.Setenv("XDG_CONFIG_HOME", tmpConfig)

	// Cache manifest has version 9 (very high).
	cacheDir, _ := cachedManifestDir()
	mustMkdirAll(t, cacheDir)
	mustWriteFile(t, filepath.Join(cacheDir, "agy.toml"), `id = "agy"
version = "9.0.0.0"
aliases = ["antigravity"]
[[rules]]
id = "cache-rule"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["cache version text"]
`)

	// Override has version 1 (much lower). It should STILL win (always-wins).
	overrideDir, _ := overrideManifestDir()
	mustMkdirAll(t, overrideDir)
	mustWriteFile(t, filepath.Join(overrideDir, "agy.toml"), `id = "agy"
version = "1.0.0.0"
aliases = ["antigravity"]
[[rules]]
id = "override-rule"
state = "blocked"
priority = 100
region = "whole_recent"
contains = ["override version text"]
`)

	ReloadManifests()

	m, ok := manifestForAgent("agy")
	if !ok {
		t.Fatal("agy manifest not found")
	}
	if len(m.rules) != 1 || m.rules[0].id != "override-rule" {
		t.Errorf("override should win regardless of version, got rules: %+v", m.rules)
	}
}
