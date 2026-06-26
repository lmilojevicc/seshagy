package sessionmgr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// defaultManifestCatalogURL is the herdr public catalog — the source of truth
// for detection manifests. Verified live (HTTP 200, TOML, 20+ agents). herdr
// actively maintains these rules; our bundled embeds are the offline fallback.
const defaultManifestCatalogURL = "https://herdr.dev/agent-detection/index.toml"

const (
	manifestFetchTimeout   = 10 * time.Second
	allowHTTPCatalogEnv    = "SESHAGY_ALLOW_HTTP_CATALOG"
	agentDetectionCacheDir = "agent-detection"
	manifestCacheAppDir    = "seshagy"
)

// manifestCatalogIndex is the TOML shape of the herdr index:
//
//	schema_version = 1
//	[[agents]]
//	id = "agy"
//	path = "antigravity.toml"
type manifestCatalogIndex struct {
	SchemaVersion int                    `toml:"schema_version"`
	Agents        []manifestCatalogEntry `toml:"agents"`
}

type manifestCatalogEntry struct {
	ID   string `toml:"id"`
	Path string `toml:"path"`
}

// ManifestFetchResult summarises a catalog refresh pass.
type ManifestFetchResult struct {
	Fetched []string // agent ids written to cache
	Skipped []string // agent ids that failed (parse/compile/network)
	Err     error    // non-nil only on index-level failures (not per-agent)
}

// FetchManifestUpdates fetches the herdr catalog index and each manifest into
// the local cache dir. Each manifest is compile-validated before writing so a
// malformed remote rule can never poison the cache. Per-agent failures are
// collected in Skipped; only index-level failures set Err.
func FetchManifestUpdates(ctx context.Context, catalogURL string) (ManifestFetchResult, error) {
	if err := validateCatalogURL(catalogURL); err != nil {
		return ManifestFetchResult{}, err
	}
	result := ManifestFetchResult{}

	client := &http.Client{Timeout: manifestFetchTimeout}
	index, err := fetchCatalogIndex(ctx, client, catalogURL)
	if err != nil {
		return result, err
	}

	cacheDir, err := cachedManifestDir()
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return result, err
	}

	catalogBase := catalogBaseURL(catalogURL)
	for _, entry := range index.Agents {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		if err := fetchAndCacheManifest(ctx, client, catalogBase, entry, cacheDir); err != nil {
			result.Skipped = append(result.Skipped, id)
			continue
		}
		result.Fetched = append(result.Fetched, id)
	}
	return result, nil
}

func fetchCatalogIndex(
	ctx context.Context,
	client *http.Client,
	catalogURL string,
) (*manifestCatalogIndex, error) {
	body, err := httpGet(ctx, client, catalogURL)
	if err != nil {
		return nil, fmt.Errorf("fetch catalog index: %w", err)
	}
	var index manifestCatalogIndex
	if _, err := toml.Decode(string(body), &index); err != nil {
		return nil, fmt.Errorf("parse catalog index: %w", err)
	}
	return &index, nil
}

func fetchAndCacheManifest(
	ctx context.Context,
	client *http.Client,
	catalogBase string,
	entry manifestCatalogEntry,
	cacheDir string,
) error {
	manifestURL := strings.TrimRight(catalogBase, "/") + "/" + entry.Path
	body, err := httpGet(ctx, client, manifestURL)
	if err != nil {
		return err
	}
	// Validate before writing: a malformed remote rule must never poison the
	// cache (we'd rather fall back to the embed).
	var parsed agentManifest
	if _, err := toml.Decode(string(body), &parsed); err != nil {
		return fmt.Errorf("parse %s: %w", entry.ID, err)
	}
	if _, err := compileManifest(parsed); err != nil {
		return fmt.Errorf("compile %s: %w", entry.ID, err)
	}
	// Atomic write: temp + rename.
	dest := filepath.Join(cacheDir, entry.ID+".toml")
	tmp, err := os.CreateTemp(cacheDir, ".manifest-*.toml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// ReloadManifests resets the in-memory manifest cache and rebuilds it from
// (embed + cache + override). Safe to call from any goroutine; the refresh
// command calls it after a successful fetch so new rules take effect without a
// restart.
func ReloadManifests() {
	manifestMu.Lock()
	manifestByAgent = nil
	manifestErr = nil
	manifestLoaded = false
	manifestMu.Unlock()
	ensureManifestsLoaded()
}

func cachedManifestDir() (string, error) {
	dir, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, manifestCacheAppDir, agentDetectionCacheDir), nil
}

func overrideManifestDir() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, manifestCacheAppDir, agentDetectionCacheDir), nil
}

func httpGet(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
}

// validateCatalogURL enforces HTTPS by default. http:// is allowed only when
// SESHAGY_ALLOW_HTTP_CATALOG=1 is set (local/dev catalog testing).
func validateCatalogURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid catalog URL: %w", err)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && os.Getenv(allowHTTPCatalogEnv) == "1" {
		return nil
	}
	return fmt.Errorf("catalog URL must be HTTPS (or set %s=1): %s", allowHTTPCatalogEnv, raw)
}

// catalogBaseURL returns the directory portion of the catalog index URL so
// per-manifest paths can be resolved relative to it.
func catalogBaseURL(catalogURL string) string {
	idx := strings.LastIndexByte(catalogURL, '/')
	if idx < 0 {
		return catalogURL
	}
	return catalogURL[:idx]
}

// compareManifestVersion compares two dotted-numeric version strings (e.g.
// "2026.06.10.1" vs "2026.06.24.1"). Returns -1, 0, +1. Non-numeric segments
// compare lexicographically; missing segments are treated as "0".
func compareManifestVersion(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	max := len(pa)
	if len(pb) > max {
		max = len(pb)
	}
	for i := 0; i < max; i++ {
		sa := "0"
		sb := "0"
		if i < len(pa) {
			sa = pa[i]
		}
		if i < len(pb) {
			sb = pb[i]
		}
		na, ea := parseUint(sa)
		nb, eb := parseUint(sb)
		if ea == nil && eb == nil {
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
			continue
		}
		if sa < sb {
			return -1
		}
		if sa > sb {
			return 1
		}
	}
	return 0
}

func parseUint(s string) (uint64, error) {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + uint64(c-'0')
	}
	return n, nil
}

// DefaultManifestCatalogURL returns the default herdr catalog URL.
func DefaultManifestCatalogURL() string { return defaultManifestCatalogURL }

// userCacheDir returns the cache directory, honoring XDG_CACHE_HOME on all
// platforms (os.UserCacheDir ignores it on macOS, returning ~/Library/Caches).
func userCacheDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return xdg, nil
	}
	return os.UserCacheDir()
}

// userConfigDir returns the config directory, honoring XDG_CONFIG_HOME on
// all platforms (os.UserConfigDir ignores it on macOS, returning
// ~/Library/Application Support).
func userConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	return os.UserConfigDir()
}
