package sessionmgr

import "testing"

// isolateManifestCache isolates manifest loading from the host filesystem so a
// test resolves to the bundled embed deterministically (issue #36).
//
// The manifest cache is assembled from three layers (local override > cached
// remote > bundled embed) read from XDG dirs. Without isolation a newer
// host-cached manifest (e.g. ~/.cache/seshagy/agent-detection/claude.toml)
// shadows the bundled rules and flips test outcomes, even though CI (no cache)
// stays green.
//
// The compiled manifests are also held in a package-global cache that is built
// once per process: a previous test that loaded from the real dirs would keep
// winning even after the env is redirected. This helper therefore points both
// the cache and override dirs at empty temp dirs AND drops the global cache so
// the next access rebuilds from the bundled embed.
//
// Call it as the first statement of any test that loads manifests
// (ApplyManifestFallback / detectManifest / manifestForAgent).
func isolateManifestCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(resetManifestCache)
	resetManifestCache()
}

// resetManifestCache drops the package-global compiled-manifest cache so the
// next ensureManifestsLoaded rebuilds it from the currently-configured dirs.
func resetManifestCache() {
	manifestMu.Lock()
	manifestByAgent = nil
	manifestErr = nil
	manifestLoaded = false
	manifestMu.Unlock()
}
