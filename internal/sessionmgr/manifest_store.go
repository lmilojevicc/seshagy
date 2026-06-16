package sessionmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/lmilojevicc/seshagy/internal/xdg"
)

const (
	ManifestEngineVersion      = 2
	defaultManifestCatalogURL  = "https://herdr.dev/agent-detection/index.toml"
	manifestCatalogURLEnv      = "SESHAGY_AGENT_DETECTION_MANIFEST_CATALOG_URL"
	maxManifestFetchBytes      = 256 * 1024
	manifestDetectionConfigDir = "seshagy/agent-detection"
	manifestDetectionStateDir  = "seshagy/agent-detection"
)

type ManifestSourceKind string

const (
	ManifestSourceBundled  ManifestSourceKind = "bundled"
	ManifestSourceRemote   ManifestSourceKind = "remote"
	ManifestSourceOverride ManifestSourceKind = "override"
)

type ManifestSource struct {
	Kind    ManifestSourceKind
	Path    string
	Version string
}

func (s ManifestSource) Label() string {
	switch s.Kind {
	case ManifestSourceBundled:
		return "bundled"
	case ManifestSourceRemote:
		if s.Path != "" {
			return "remote:" + s.Path
		}
		return "remote"
	case ManifestSourceOverride:
		if s.Path != "" {
			return s.Path
		}
		return "local override"
	default:
		return string(s.Kind)
	}
}

func (s ManifestSource) KindLabel() string {
	switch s.Kind {
	case ManifestSourceBundled:
		return "bundled"
	case ManifestSourceRemote:
		return "remote"
	case ManifestSourceOverride:
		return "local override"
	default:
		return string(s.Kind)
	}
}

type LoadedManifestInfo struct {
	Source                       ManifestSource
	Version                      string
	CachedRemoteVersion          string
	LocalOverrideShadowingRemote bool
	Warning                      string
}

type ManifestVersion struct {
	raw string
}

func ParseManifestVersion(value string) (ManifestVersion, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ManifestVersion{}, fmt.Errorf("version must not be empty")
	}
	for _, segment := range strings.Split(trimmed, ".") {
		if segment == "" {
			return ManifestVersion{}, fmt.Errorf("version %q contains an empty segment", trimmed)
		}
		for _, ch := range segment {
			if ch < '0' || ch > '9' {
				return ManifestVersion{}, fmt.Errorf("version %q must be dotted numeric", trimmed)
			}
		}
		if _, err := strconv.ParseUint(segment, 10, 64); err != nil {
			return ManifestVersion{}, fmt.Errorf(
				"version %q contains an oversized segment",
				trimmed,
			)
		}
	}
	return ManifestVersion{raw: trimmed}, nil
}

func (v ManifestVersion) String() string {
	return v.raw
}

func CompareManifestVersion(left, right ManifestVersion) int {
	leftParts := strings.Split(left.raw, ".")
	rightParts := strings.Split(right.raw, ".")
	for {
		switch {
		case len(leftParts) > 0 && len(rightParts) > 0:
			leftVal, _ := strconv.ParseUint(leftParts[0], 10, 64)
			rightVal, _ := strconv.ParseUint(rightParts[0], 10, 64)
			leftParts = leftParts[1:]
			rightParts = rightParts[1:]
			switch {
			case leftVal < rightVal:
				return -1
			case leftVal > rightVal:
				return 1
			}
		case len(leftParts) > 0:
			leftVal, _ := strconv.ParseUint(leftParts[0], 10, 64)
			leftParts = leftParts[1:]
			if leftVal == 0 {
				continue
			}
			return 1
		case len(rightParts) > 0:
			rightVal, _ := strconv.ParseUint(rightParts[0], 10, 64)
			rightParts = rightParts[1:]
			if rightVal == 0 {
				continue
			}
			return -1
		default:
			return 0
		}
	}
}

type AgentRemoteStatus struct {
	CachedVersion    *string `toml:"cached_version,omitempty"`
	AttemptedVersion *string `toml:"attempted_version,omitempty"`
	LastCheckedUnix  *uint64 `toml:"last_checked_unix,omitempty"`
	LastResult       string  `toml:"last_result"`
	LastError        *string `toml:"last_error,omitempty"`
}

type ManifestUpdateStatus struct {
	LastCheckUnix *uint64                      `toml:"last_check_unix,omitempty"`
	LastResult    *string                      `toml:"last_result,omitempty"`
	Agents        map[string]AgentRemoteStatus `toml:"agents"`
}

func (s ManifestUpdateStatus) AgentStatus(agentID string) (AgentRemoteStatus, bool) {
	if s.Agents == nil {
		return AgentRemoteStatus{}, false
	}
	status, ok := s.Agents[agentID]
	return status, ok
}

type ManifestUpdateCommit struct {
	AgentID string
	Version ManifestVersion
}

type ManifestUpdateOutput struct {
	Updated []ManifestUpdateCommit
	Status  ManifestUpdateStatus
}

type AgentManifestSummary struct {
	AgentID                      string
	ActiveSource                 ManifestSource
	ActiveVersion                string
	CachedRemoteVersion          string
	LocalOverrideShadowingRemote bool
	Warning                      string
}

func ResolveManifestCatalogURL(configured string) string {
	if value := strings.TrimSpace(os.Getenv(manifestCatalogURLEnv)); value != "" {
		return value
	}
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	return defaultManifestCatalogURL
}

func manifestDetectionConfigRoot() string {
	return filepath.Join(xdg.ConfigHome(), manifestDetectionConfigDir)
}

func manifestDetectionStateRoot() string {
	return filepath.Join(xdg.StateHome(), manifestDetectionStateDir)
}

func manifestOverridePath(agentID string) string {
	return filepath.Join(manifestDetectionConfigRoot(), agentID+".toml")
}

func remoteManifestPath(agentID string) string {
	return filepath.Join(manifestDetectionStateRoot(), "remote", agentID+".toml")
}

// RemoteManifestPath returns the on-disk path for a cached remote manifest.
func RemoteManifestPath(agentID string) string {
	return remoteManifestPath(agentID)
}

func manifestStatusPath() string {
	return filepath.Join(manifestDetectionStateRoot(), "status.toml")
}

func LoadManifestUpdateStatus() ManifestUpdateStatus {
	path := manifestStatusPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return ManifestUpdateStatus{Agents: map[string]AgentRemoteStatus{}}
	}
	var status ManifestUpdateStatus
	if _, err := toml.Decode(string(data), &status); err != nil {
		return ManifestUpdateStatus{Agents: map[string]AgentRemoteStatus{}}
	}
	if status.Agents == nil {
		status.Agents = map[string]AgentRemoteStatus{}
	}
	return status
}

func saveManifestUpdateStatus(status ManifestUpdateStatus) error {
	path := manifestStatusPath()
	if status.Agents == nil {
		status.Agents = map[string]AgentRemoteStatus{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := encodeManifestStatus(status)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data)
}

func encodeManifestStatus(status ManifestUpdateStatus) ([]byte, error) {
	var buf strings.Builder
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(status); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func cachedRemoteVersion(agentID string) (ManifestVersion, bool) {
	content, err := os.ReadFile(remoteManifestPath(agentID))
	if err != nil {
		return ManifestVersion{}, false
	}
	parsed, err := parseRemoteManifestForAgent(agentID, string(content))
	if err != nil {
		return ManifestVersion{}, false
	}
	return parsed.version, true
}

func commitRemoteManifest(agentID, content string) error {
	path := remoteManifestPath(agentID)
	return atomicWriteFile(path, []byte(content))
}

func atomicWriteFile(path string, data []byte) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	tmpPath := filepath.Join(
		parent,
		fmt.Sprintf(
			".%s.%d.%d.tmp",
			filepath.Base(path),
			os.Getpid(),
			time.Now().UnixNano(),
		),
	)
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if dir, err := os.Open(parent); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

type parsedRemoteManifest struct {
	manifest agentManifest
	version  ManifestVersion
}

func parseRemoteManifestForAgent(agentID, content string) (parsedRemoteManifest, error) {
	var manifest agentManifest
	if _, err := toml.Decode(content, &manifest); err != nil {
		return parsedRemoteManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := validateAgentManifest(&manifest); err != nil {
		return parsedRemoteManifest{}, err
	}
	if !manifestMatchesAgentID(&manifest, agentID) {
		return parsedRemoteManifest{}, fmt.Errorf(
			"manifest id %q does not match %q",
			manifest.ID,
			agentID,
		)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return parsedRemoteManifest{}, fmt.Errorf("remote manifest must include version")
	}
	version, err := ParseManifestVersion(manifest.Version)
	if err != nil {
		return parsedRemoteManifest{}, err
	}
	if manifest.MinEngineVersion == 0 {
		return parsedRemoteManifest{}, fmt.Errorf("remote manifest must include min_engine_version")
	}
	if manifest.MinEngineVersion > ManifestEngineVersion {
		return parsedRemoteManifest{}, fmt.Errorf(
			"manifest requires engine %d, current engine is %d",
			manifest.MinEngineVersion,
			ManifestEngineVersion,
		)
	}
	return parsedRemoteManifest{manifest: manifest, version: version}, nil
}

func parseLocalManifest(content string) (agentManifest, error) {
	var manifest agentManifest
	if _, err := toml.Decode(content, &manifest); err != nil {
		return agentManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := validateAgentManifest(&manifest); err != nil {
		return agentManifest{}, err
	}
	return manifest, nil
}

func validateAgentManifest(manifest *agentManifest) error {
	if strings.TrimSpace(manifest.ID) == "" {
		return fmt.Errorf("manifest id is required")
	}
	if len(manifest.Rules) == 0 {
		return fmt.Errorf("manifest %q must contain at least one rule", manifest.ID)
	}
	if len(manifest.Rules) > maxRulesPerManifest {
		return fmt.Errorf(
			"manifest %q contains %d rules, max is %d",
			manifest.ID,
			len(manifest.Rules),
			maxRulesPerManifest,
		)
	}
	return nil
}

func manifestMatchesAgentID(manifest *agentManifest, agentID string) bool {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if strings.ToLower(strings.TrimSpace(manifest.ID)) == agentID {
		return true
	}
	for _, alias := range manifest.Aliases {
		if strings.ToLower(strings.TrimSpace(alias)) == agentID {
			return true
		}
	}
	return false
}

func manifestVersionString(manifest agentManifest) string {
	return strings.TrimSpace(manifest.Version)
}

func manifestCheckUnix() uint64 {
	return uint64(time.Now().Unix())
}
