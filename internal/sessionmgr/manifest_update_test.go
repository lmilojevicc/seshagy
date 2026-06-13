package sessionmgr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteManifestFixture(version, contains string) string {
	return fmt.Sprintf(`
id = "codex"
version = %q
min_engine_version = 1
updated_at = "2026-06-10T12:00:00Z"

[[rules]]
id = "idle"
state = "idle"
contains = [%q]
`, version, contains)
}

func withManifestStateDir(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	oldConfig := os.Getenv("XDG_CONFIG_HOME")
	oldState := os.Getenv("XDG_STATE_HOME")
	oldCatalog := os.Getenv(manifestCatalogURLEnv)

	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	stateDir := filepath.Join(dir, "state")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config) = %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(state) = %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv(manifestCatalogURLEnv, "")

	ReloadManifests()
	t.Cleanup(func() {
		if oldConfig == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", oldConfig)
		}
		if oldState == "" {
			os.Unsetenv("XDG_STATE_HOME")
		} else {
			os.Setenv("XDG_STATE_HOME", oldState)
		}
		if oldCatalog == "" {
			os.Unsetenv(manifestCatalogURLEnv)
		} else {
			os.Setenv(manifestCatalogURLEnv, oldCatalog)
		}
		ReloadManifests()
	})

	fn(t)
}

func TestManifestVersionComparesDottedNumericSegments(t *testing.T) {
	left, err := ParseManifestVersion("2026.6.10.1")
	if err != nil {
		t.Fatalf("ParseManifestVersion(left) = %v", err)
	}
	right, err := ParseManifestVersion("2026.6.9.9")
	if err != nil {
		t.Fatalf("ParseManifestVersion(right) = %v", err)
	}
	if CompareManifestVersion(left, right) <= 0 {
		t.Fatalf("expected %s > %s", left, right)
	}

	oneTwoZero, _ := ParseManifestVersion("1.2.0")
	oneTwo, _ := ParseManifestVersion("1.2")
	if CompareManifestVersion(oneTwoZero, oneTwo) != 0 {
		t.Fatalf("expected 1.2.0 == 1.2")
	}

	oneTwoOne, _ := ParseManifestVersion("1.2.1")
	if CompareManifestVersion(oneTwoOne, oneTwo) <= 0 {
		t.Fatalf("expected 1.2.1 > 1.2")
	}
}

func TestManifestVersionRejectsNonNumericSegments(t *testing.T) {
	cases := []string{"", "2026.06.alpha", "2026..06", "2026.999999999999999999999999999999"}
	for _, value := range cases {
		if _, err := ParseManifestVersion(value); err == nil {
			t.Fatalf("ParseManifestVersion(%q) expected error", value)
		}
	}
}

func TestProcessAgentManifestUpdateCommitsNewerManifest(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		content := remoteManifestFixture("2026.06.10.3", "ready")
		commit, err := processAgentManifestUpdate("codex", content)
		if err != nil {
			t.Fatalf("processAgentManifestUpdate() error = %v", err)
		}
		if commit == nil {
			t.Fatal("expected commit")
		}
		if commit.AgentID != "codex" {
			t.Fatalf("AgentID = %q, want codex", commit.AgentID)
		}
		got, err := os.ReadFile(remoteManifestPath("codex"))
		if err != nil {
			t.Fatalf("ReadFile() = %v", err)
		}
		if string(got) != content {
			t.Fatalf("cached manifest mismatch")
		}
	})
}

func TestProcessAgentManifestUpdateRejectsDowngrade(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		current := remoteManifestFixture("2026.06.10.4", "current")
		if _, err := processAgentManifestUpdate("codex", current); err != nil {
			t.Fatalf("seed current manifest: %v", err)
		}
		older := remoteManifestFixture("2026.06.10.3", "older")
		if _, err := processAgentManifestUpdate("codex", older); err == nil {
			t.Fatal("expected downgrade error")
		}
		got, err := os.ReadFile(remoteManifestPath("codex"))
		if err != nil {
			t.Fatalf("ReadFile() = %v", err)
		}
		if string(got) != current {
			t.Fatalf("cached manifest changed on downgrade")
		}
	})
}

func TestProcessAgentManifestUpdateRejectsEqualVersionContentChange(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		current := remoteManifestFixture("2026.06.10.3", "current")
		if _, err := processAgentManifestUpdate("codex", current); err != nil {
			t.Fatalf("seed current manifest: %v", err)
		}
		changed := remoteManifestFixture("2026.06.10.3", "changed")
		if _, err := processAgentManifestUpdate("codex", changed); err == nil {
			t.Fatal("expected equal-version content change error")
		}
		got, err := os.ReadFile(remoteManifestPath("codex"))
		if err != nil {
			t.Fatalf("ReadFile() = %v", err)
		}
		if string(got) != current {
			t.Fatalf("cached manifest changed")
		}
	})
}

func TestProcessAgentManifestUpdateSkipsSameVersionSameContent(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		current := remoteManifestFixture("2026.06.10.3", "current")
		if _, err := processAgentManifestUpdate("codex", current); err != nil {
			t.Fatalf("seed current manifest: %v", err)
		}
		commit, err := processAgentManifestUpdate("codex", current)
		if err != nil {
			t.Fatalf("processAgentManifestUpdate() error = %v", err)
		}
		if commit != nil {
			t.Fatalf("expected nil commit, got %+v", commit)
		}
	})
}

func TestParseRemoteManifestRejectsUnsupportedEngine(t *testing.T) {
	content := remoteManifestFixture("2026.06.10.3", "ready")
	content = strings.Replace(
		content,
		"min_engine_version = 1",
		fmt.Sprintf("min_engine_version = %d", ManifestEngineVersion+1),
		1,
	)
	if _, err := parseRemoteManifestForAgent("codex", content); err == nil {
		t.Fatal("expected engine gate error")
	}
}

func TestOverrideManifestWinsOverRemote(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		remote := remoteManifestFixture("2099.01.01.1", "remote-marker")
		if err := commitRemoteManifest("codex", remote); err != nil {
			t.Fatalf("commitRemoteManifest() = %v", err)
		}
		overridePath := manifestOverridePath("codex")
		if err := os.MkdirAll(filepath.Dir(overridePath), 0o700); err != nil {
			t.Fatalf("MkdirAll() = %v", err)
		}
		override := `
id = "codex"
version = "local-override"

[[rules]]
id = "override_idle"
state = "idle"
contains = ["override-marker"]
`
		if err := os.WriteFile(overridePath, []byte(override), 0o600); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		ReloadManifests()

		info, ok := ManifestInfoForAgent("codex")
		if !ok {
			t.Fatal("expected manifest info")
		}
		if info.Source.Kind != ManifestSourceOverride {
			t.Fatalf("source = %q, want override", info.Source.Kind)
		}
		result := detectManifest("codex", manifestDetectionInput{screen: "override-marker\n"})
		if !result.Matched || result.RuleID != "override_idle" {
			t.Fatalf("detectManifest() = %+v, want override_idle", result)
		}
	})
}

func TestCheckAndUpdateManifestsFromHTTPServer(t *testing.T) {
	withManifestStateDir(t, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/index.toml":
				_, _ = w.Write([]byte(`
schema_version = 1

[[agents]]
id = "codex"
path = "codex.toml"
`))
			case "/codex.toml":
				_, _ = w.Write([]byte(remoteManifestFixture("2026.06.10.3", "httptest-ready")))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(server.Close)

		output, err := checkAndUpdateFromURL(server.URL + "/index.toml")
		if err != nil {
			t.Fatalf("checkAndUpdateFromURL() error = %v", err)
		}
		if len(output.Updated) != 1 {
			t.Fatalf("updated = %d, want 1", len(output.Updated))
		}
		if output.Updated[0].AgentID != "codex" {
			t.Fatalf("updated agent = %q, want codex", output.Updated[0].AgentID)
		}
	})
}

func TestParseManifestCatalogRejectsDuplicatesAndUnsafePaths(t *testing.T) {
	if _, err := parseManifestCatalog(`
schema_version = 1

[[agents]]
id = "codex"
path = "codex.toml"

[[agents]]
id = "codex"
path = "codex-2.toml"
`); err == nil {
		t.Fatal("expected duplicate catalog error")
	}

	if _, err := parseManifestCatalog(`
schema_version = 1

[[agents]]
id = "codex"
path = "../codex.toml"
`); err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestManifestAutoUpdateEnabledRespectsEnv(t *testing.T) {
	t.Setenv("SESHAGY_MANIFEST_AUTO_UPDATE", "0")
	if ManifestAutoUpdateEnabled(true) {
		t.Fatal("expected auto update disabled via env")
	}
	t.Setenv("SESHAGY_MANIFEST_AUTO_UPDATE", "")
	if !ManifestAutoUpdateEnabled(true) {
		t.Fatal("expected auto update enabled from config")
	}
}
