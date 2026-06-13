package sessionmgr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type manifestCatalog struct {
	SchemaVersion int                    `toml:"schema_version"`
	Agents        []manifestCatalogAgent `toml:"agents"`
}

type manifestCatalogAgent struct {
	ID   string `toml:"id"`
	Path string `toml:"path"`
}

type catalogAgentEntry struct {
	agentID string
	path    string
}

func CheckAndUpdateManifests(catalogURL string) (ManifestUpdateOutput, error) {
	return checkAndUpdateFromURL(ResolveManifestCatalogURL(catalogURL))
}

func checkAndUpdateFromURL(catalogURL string) (ManifestUpdateOutput, error) {
	catalogContent, err := fetchManifestText(catalogURL)
	if err != nil {
		return ManifestUpdateOutput{}, err
	}
	catalog, err := parseManifestCatalog(catalogContent)
	if err != nil {
		return ManifestUpdateOutput{}, err
	}
	baseURL, err := manifestCatalogBaseURL(catalogURL)
	if err != nil {
		return ManifestUpdateOutput{}, err
	}

	status := LoadManifestUpdateStatus()
	checkTime := manifestCheckUnix()
	status.LastCheckUnix = &checkTime
	checked := "checked"
	status.LastResult = &checked

	bundledIDs, err := bundledManifestAgentIDs()
	if err != nil {
		return ManifestUpdateOutput{}, err
	}

	var updated []ManifestUpdateCommit
	for _, entry := range catalog {
		if !bundledIDs[entry.agentID] {
			continue
		}
		manifestURL, err := joinManifestCatalogURL(baseURL, entry.path)
		if err != nil {
			return ManifestUpdateOutput{}, fmt.Errorf("catalog entry %s: %w", entry.agentID, err)
		}
		content, fetchErr := fetchManifestText(manifestURL)
		switch {
		case fetchErr != nil:
			recordAgentUpdateFailure(&status, entry.agentID, checkTime, fetchErr.Error())
		default:
			commit, processErr := processAgentManifestUpdate(entry.agentID, content)
			switch {
			case processErr != nil:
				recordAgentUpdateFailure(&status, entry.agentID, checkTime, processErr.Error())
			case commit != nil:
				recordAgentUpdateSuccess(&status, entry.agentID, checkTime, commit.Version.String())
				updated = append(updated, *commit)
			default:
				recordAgentUpdateCurrent(&status, entry.agentID, checkTime)
			}
		}
	}

	if err := saveManifestUpdateStatus(status); err != nil {
		failed := fmt.Sprintf("failed_to_save_status: %v", err)
		status.LastResult = &failed
	}
	return ManifestUpdateOutput{Updated: updated, Status: status}, nil
}

func processAgentManifestUpdate(agentID, content string) (*ManifestUpdateCommit, error) {
	parsed, err := parseRemoteManifestForAgent(agentID, content)
	if err != nil {
		return nil, err
	}
	if current, ok := cachedRemoteVersion(agentID); ok {
		switch CompareManifestVersion(parsed.version, current) {
		case -1:
			return nil, fmt.Errorf(
				"remote version %s is older than cached %s",
				parsed.version,
				current,
			)
		case 0:
			committed, readErr := os.ReadFile(remoteManifestPath(agentID))
			if readErr != nil {
				return nil, readErr
			}
			if string(committed) != content {
				return nil, fmt.Errorf(
					"remote version %s changed content without a version bump",
					parsed.version,
				)
			}
			return nil, nil
		}
	}
	if err := commitRemoteManifest(agentID, content); err != nil {
		return nil, err
	}
	return &ManifestUpdateCommit{
		AgentID: agentID,
		Version: parsed.version,
	}, nil
}

func parseManifestCatalog(content string) ([]catalogAgentEntry, error) {
	var catalog manifestCatalog
	if _, err := toml.Decode(content, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog TOML: %w", err)
	}
	if catalog.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported catalog schema_version %d", catalog.SchemaVersion)
	}

	seen := map[string]struct{}{}
	entries := make([]catalogAgentEntry, 0, len(catalog.Agents))
	for _, entry := range catalog.Agents {
		agentID := strings.ToLower(strings.TrimSpace(entry.ID))
		if agentID == "" {
			return nil, fmt.Errorf("catalog entry has an empty id")
		}
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			return nil, fmt.Errorf("catalog entry %s has an empty path", entry.ID)
		}
		if err := validateCatalogManifestPath(path); err != nil {
			return nil, fmt.Errorf(
				"catalog entry %s has an unsafe path %q: %w",
				entry.ID,
				path,
				err,
			)
		}
		if _, ok := seen[agentID]; ok {
			return nil, fmt.Errorf("catalog contains duplicate agent %s", entry.ID)
		}
		seen[agentID] = struct{}{}
		entries = append(entries, catalogAgentEntry{agentID: agentID, path: path})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].agentID < entries[j].agentID })
	return entries, nil
}

func validateCatalogManifestPath(path string) error {
	if strings.Contains(path, "://") || strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute or remote paths are not allowed")
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal is not allowed")
		}
	}
	return nil
}

func manifestCatalogBaseURL(catalogURL string) (string, error) {
	parsed, err := url.Parse(catalogURL)
	if err != nil {
		return "", fmt.Errorf("catalog URL %q is invalid: %w", catalogURL, err)
	}
	if parsed.Scheme == "file" {
		dir := filepathDir(parsed.Path)
		if dir == "" {
			return "", fmt.Errorf("catalog URL %q has no base path", catalogURL)
		}
		return parsed.Scheme + "://" + dir, nil
	}
	idx := strings.LastIndex(catalogURL, "/")
	if idx < 0 {
		return "", fmt.Errorf("catalog URL %q has no base path", catalogURL)
	}
	return catalogURL[:idx], nil
}

func joinManifestCatalogURL(baseURL, path string) (string, error) {
	if err := validateCatalogManifestPath(path); err != nil {
		return "", err
	}
	if strings.HasPrefix(baseURL, "file://") {
		return baseURL + "/" + strings.TrimPrefix(path, "/"), nil
	}
	return strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/"), nil
}

func filepathDir(path string) string {
	path = strings.TrimSuffix(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return ""
}

func fetchManifestText(rawURL string) (string, error) {
	if strings.HasPrefix(rawURL, "file://") {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("invalid file URL %q: %w", rawURL, err)
		}
		data, err := os.ReadFile(parsed.Path)
		if err != nil {
			return "", fmt.Errorf("failed to read %q: %w", rawURL, err)
		}
		if len(data) > maxManifestFetchBytes {
			return "", fmt.Errorf(
				"response from %s exceeded %d bytes",
				rawURL,
				maxManifestFetchBytes,
			)
		}
		return string(data), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request for %q: %w", rawURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %q: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to fetch %q: HTTP %d", rawURL, resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxManifestFetchBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("failed to read response from %q: %w", rawURL, err)
	}
	if len(data) > maxManifestFetchBytes {
		return "", fmt.Errorf("response from %q exceeded %d bytes", rawURL, maxManifestFetchBytes)
	}
	return string(data), nil
}

func recordAgentUpdateSuccess(
	status *ManifestUpdateStatus,
	agentID string,
	checkTime uint64,
	version string,
) {
	cached := version
	attempted := version
	status.Agents[agentID] = AgentRemoteStatus{
		CachedVersion:    &cached,
		AttemptedVersion: &attempted,
		LastCheckedUnix:  &checkTime,
		LastResult:       "updated",
	}
}

func recordAgentUpdateCurrent(status *ManifestUpdateStatus, agentID string, checkTime uint64) {
	var cached *string
	if version, ok := cachedRemoteVersion(agentID); ok {
		value := version.String()
		cached = &value
	}
	status.Agents[agentID] = AgentRemoteStatus{
		CachedVersion:   cached,
		LastCheckedUnix: &checkTime,
		LastResult:      "current",
	}
}

func recordAgentUpdateFailure(
	status *ManifestUpdateStatus,
	agentID string,
	checkTime uint64,
	errMsg string,
) {
	var cached *string
	if version, ok := cachedRemoteVersion(agentID); ok {
		value := version.String()
		cached = &value
	}
	status.Agents[agentID] = AgentRemoteStatus{
		CachedVersion:   cached,
		LastCheckedUnix: &checkTime,
		LastResult:      "failed",
		LastError:       &errMsg,
	}
}

func bundledManifestAgentIDs() (map[string]bool, error) {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := manifestFS.ReadFile("manifests/" + entry.Name())
		if err != nil {
			return nil, err
		}
		manifest, err := parseLocalManifest(string(data))
		if err != nil {
			return nil, err
		}
		ids[strings.ToLower(strings.TrimSpace(manifest.ID))] = true
	}
	return ids, nil
}

func ManifestAutoUpdateEnabled(configured bool) bool {
	switch strings.TrimSpace(os.Getenv("SESHAGY_MANIFEST_AUTO_UPDATE")) {
	case "0", "false", "False", "FALSE":
		return false
	}
	return configured
}

func StartManifestAutoUpdate(catalogURL string, enabled bool) {
	if !ManifestAutoUpdateEnabled(enabled) {
		return
	}
	go func() {
		runManifestAutoUpdate(catalogURL)
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runManifestAutoUpdate(catalogURL)
		}
	}()
}

func runManifestAutoUpdate(catalogURL string) {
	output, err := CheckAndUpdateManifests(catalogURL)
	if err != nil {
		return
	}
	if len(output.Updated) > 0 {
		ReloadManifests()
	}
}
