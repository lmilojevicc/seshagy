package sessionmgr

import (
	"strconv"
	"time"
)

// ItemJSON is the script-friendly representation of a list item.
type ItemJSON struct {
	Kind      string `json:"kind"`
	Key       string `json:"key,omitempty"`
	Name      string `json:"name,omitempty"`
	Target    string `json:"target,omitempty"`
	Path      string `json:"path,omitempty"`
	Line      string `json:"line,omitempty"`
	LinePlain string `json:"line_plain,omitempty"`
	Attached  bool   `json:"attached"`
	Windows   int    `json:"windows,omitempty"`

	CreatedAt  *time.Time `json:"created_at,omitempty"`
	ActivityAt *time.Time `json:"activity_at,omitempty"`

	PaneID    string `json:"pane_id,omitempty"`
	Session   string `json:"session,omitempty"`
	Window    string `json:"window,omitempty"`
	Pane      string `json:"pane,omitempty"`
	Location  string `json:"location,omitempty"`
	PaneTitle string `json:"pane_title,omitempty"`
	Visible   bool   `json:"visible"`

	AgentName        string     `json:"agent_name,omitempty"`
	DisplayName      string     `json:"display_name,omitempty"`
	State            AgentState `json:"state,omitempty"`
	Message          string     `json:"message,omitempty"`
	Source           string     `json:"source,omitempty"`
	UpdatedAt        string     `json:"updated_at,omitempty"`
	UpdatedAtRFC3339 string     `json:"updated_at_rfc3339,omitempty"`
	SessionID        string     `json:"session_id,omitempty"`
	Seq              string     `json:"seq,omitempty"`
}

// ItemsJSON wraps a mode query result.
type ItemsJSON struct {
	SchemaVersion int        `json:"schema_version"`
	Ok            bool       `json:"ok"`
	Mode          string     `json:"mode"`
	Warning       string     `json:"warning,omitempty"`
	Items         []ItemJSON `json:"items"`
}

func ItemToJSON(item Item, icons IconSet) ItemJSON {
	formattedLine := FormatLineWithIcons(item, icons)
	out := ItemJSON{
		Kind:      string(item.Kind),
		Key:       item.Key(),
		Name:      item.Name,
		Target:    item.Target,
		Path:      item.Path,
		Line:      formattedLine,
		LinePlain: StripANSI(formattedLine),
		Attached:  item.Attached,
		Windows:   item.Windows,
		PaneID:    item.PaneID,
		Session:   item.Session,
		Window:    item.Window,
		Pane:      item.Pane,
		Location:  item.Location,
		PaneTitle: item.PaneTitle,
		Visible:   item.Visible,
	}
	if !item.Created.IsZero() {
		created := item.Created.UTC()
		out.CreatedAt = &created
	}
	if !item.Activity.IsZero() {
		activity := item.Activity.UTC()
		out.ActivityAt = &activity
	}
	if item.Kind == KindAgent {
		out.AgentName = item.AgentName
		if item.AgentDisplayName != "" {
			out.DisplayName = item.AgentDisplayName
		}
		out.State = item.AgentState
		out.Message = item.AgentMessage
		out.Source = item.AgentSource
		out.UpdatedAt = item.AgentUpdated
		if _, rfc := parseUpdatedAtRFC3339(item.AgentUpdated); rfc != "" {
			out.UpdatedAtRFC3339 = rfc
		}
		out.SessionID = item.AgentSessionID
		out.Seq = item.AgentSeq
	}
	return out
}

func ItemsToJSON(mode SourceMode, items []Item, icons IconSet, warning string) ItemsJSON {
	out := make([]ItemJSON, 0, len(items))
	for _, item := range items {
		out = append(out, ItemToJSON(item, icons))
	}
	return ItemsJSON{
		SchemaVersion: 1,
		Ok:            true,
		Mode:          mode.Names().ConfigToken,
		Warning:       warning,
		Items:         out,
	}
}

const JSONSchemaVersion = 1

// IntegrationExplainJSON is the structured integration status for agent explain.
type IntegrationExplainJSON struct {
	Label     string `json:"label"`
	Target    string `json:"target"`
	State     string `json:"state"`
	Version   int    `json:"version,omitempty"`
	Authority string `json:"authority"`
}

// AgentExplainReport is the structured explain payload for JSON output.
type AgentExplainReport struct {
	Ok            bool   `json:"ok"`
	SchemaVersion int    `json:"schema_version"`
	PaneID        string `json:"pane_id"`
	Location      string `json:"location"`
	Path          string `json:"path,omitempty"`
	Visible       bool   `json:"visible"`
	Listed        bool   `json:"listed"`
	SkipReason    string `json:"skip_reason,omitempty"`

	IdentitySource string `json:"identity_source"`
	AgentName      string `json:"agent_name,omitempty"`
	Command        string `json:"command,omitempty"`
	Title          string `json:"title,omitempty"`

	StateSource      string     `json:"state_source"`
	HookState        string     `json:"hook_state,omitempty"`
	HookStateStale   bool       `json:"hook_state_stale,omitempty"`
	EffectiveStatus  AgentState `json:"effective_status"`
	DetectedStatus   AgentState `json:"detected_status"`
	TrackingOverride bool       `json:"tracking_override"`

	Source    string `json:"source,omitempty"`
	Seq       string `json:"seq,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`

	LifecycleAuthority bool `json:"lifecycle_authority"`

	LastState       string `json:"last_state,omitempty"`
	LastStatus      string `json:"last_status,omitempty"`
	LastSeen        string `json:"last_seen,omitempty"`
	LastSeenRFC3339 string `json:"last_seen_rfc3339,omitempty"`

	Integration      *IntegrationExplainJSON `json:"integration,omitempty"`
	ManifestFallback string                  `json:"manifest_fallback,omitempty"`
	ManifestSource   string                  `json:"manifest_source,omitempty"`
	ManifestVersion  string                  `json:"manifest_version,omitempty"`
	CachedRemoteVer  string                  `json:"cached_remote_version,omitempty"`
	ManifestWarning  string                  `json:"manifest_warning,omitempty"`
}

func agentExplainToReport(info agentExplain) AgentExplainReport {
	report := AgentExplainReport{
		Ok:                 true,
		SchemaVersion:      JSONSchemaVersion,
		PaneID:             info.PaneID,
		Location:           info.Location,
		Path:               info.Path,
		Visible:            info.Visible,
		Listed:             info.Listed,
		SkipReason:         info.SkipReason,
		IdentitySource:     info.IdentitySource,
		AgentName:          info.AgentName,
		Command:            info.Command,
		Title:              info.Title,
		StateSource:        info.StateSource,
		HookState:          info.HookStateRaw,
		HookStateStale:     info.HookStateStale,
		EffectiveStatus:    info.EffectiveStatus,
		DetectedStatus:     info.DetectedStatus,
		TrackingOverride:   info.TrackingOverride,
		Source:             info.AgentSource,
		Seq:                info.AgentSeq,
		SessionID:          info.AgentSessionID,
		Message:            info.AgentMessage,
		UpdatedAt:          info.AgentUpdated,
		LifecycleAuthority: info.LifecycleAuthority,
		LastState:          info.LastState,
		LastStatus:         info.LastStatus,
		LastSeen:           info.LastSeen,
		Integration:        info.Integration,
		ManifestFallback:   info.ManifestFallback,
		ManifestSource:     info.ManifestSource,
		ManifestVersion:    info.ManifestVersion,
		CachedRemoteVer:    info.CachedRemoteVer,
		ManifestWarning:    info.ManifestWarning,
	}
	if ts, rfc := parseLastSeenRFC3339(info.LastSeen); rfc != "" {
		report.LastSeen = ts
		report.LastSeenRFC3339 = rfc
	}
	return report
}

func parseLastSeenRFC3339(raw string) (timestamp, rfc3339 string) {
	return parseUnixTimestampRFC3339(raw)
}

func parseUpdatedAtRFC3339(raw string) (timestamp, rfc3339 string) {
	return parseUnixTimestampRFC3339(raw)
}

func parseUnixTimestampRFC3339(raw string) (timestamp, rfc3339 string) {
	if raw == "" {
		return "", ""
	}
	ts, err := strconvParseInt(raw)
	if err != nil {
		return raw, ""
	}
	return raw, time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func strconvParseInt(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

// ManifestSourceJSON is the JSON view of a manifest source.
type ManifestSourceJSON struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

func ManifestSourceToJSON(source ManifestSource) ManifestSourceJSON {
	return ManifestSourceJSON{
		Kind:    source.KindLabel(),
		Path:    source.Path,
		Version: source.Version,
	}
}

// AgentManifestSummaryJSON is the JSON view of an active manifest summary.
type AgentManifestSummaryJSON struct {
	AgentID                      string             `json:"agent_id"`
	ActiveSource                 ManifestSourceJSON `json:"active_source"`
	ActiveVersion                string             `json:"active_version"`
	CachedRemoteVersion          string             `json:"cached_remote_version,omitempty"`
	LocalOverrideShadowingRemote bool               `json:"local_override_shadowing_remote,omitempty"`
	Warning                      string             `json:"warning,omitempty"`
}

func AgentManifestSummaryToJSON(summary AgentManifestSummary) AgentManifestSummaryJSON {
	return AgentManifestSummaryJSON{
		AgentID:                      summary.AgentID,
		ActiveSource:                 ManifestSourceToJSON(summary.ActiveSource),
		ActiveVersion:                summary.ActiveVersion,
		CachedRemoteVersion:          summary.CachedRemoteVersion,
		LocalOverrideShadowingRemote: summary.LocalOverrideShadowingRemote,
		Warning:                      summary.Warning,
	}
}

func AgentManifestSummariesToJSON(summaries []AgentManifestSummary) []AgentManifestSummaryJSON {
	out := make([]AgentManifestSummaryJSON, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, AgentManifestSummaryToJSON(summary))
	}
	return out
}

// AgentRemoteStatusJSON is the JSON view of per-agent remote update status.
type AgentRemoteStatusJSON struct {
	CachedVersion    *string `json:"cached_version,omitempty"`
	AttemptedVersion *string `json:"attempted_version,omitempty"`
	LastCheckedUnix  *uint64 `json:"last_checked_unix,omitempty"`
	LastResult       string  `json:"last_result,omitempty"`
	LastError        *string `json:"last_error,omitempty"`
}

func AgentRemoteStatusToJSON(status AgentRemoteStatus) AgentRemoteStatusJSON {
	return AgentRemoteStatusJSON(status)
}

// ManifestUpdateStatusJSON is the JSON view of manifest update status.
type ManifestUpdateStatusJSON struct {
	LastCheckUnix *uint64                          `json:"last_check_unix,omitempty"`
	LastResult    *string                          `json:"last_result,omitempty"`
	Agents        map[string]AgentRemoteStatusJSON `json:"agents,omitempty"`
}

func ManifestUpdateStatusToJSON(status ManifestUpdateStatus) ManifestUpdateStatusJSON {
	agents := make(map[string]AgentRemoteStatusJSON, len(status.Agents))
	for agentID, agentStatus := range status.Agents {
		agents[agentID] = AgentRemoteStatusToJSON(agentStatus)
	}
	return ManifestUpdateStatusJSON{
		LastCheckUnix: status.LastCheckUnix,
		LastResult:    status.LastResult,
		Agents:        agents,
	}
}

// ManifestUpdateCommitJSON is the JSON view of a manifest update commit.
type ManifestUpdateCommitJSON struct {
	AgentID string `json:"agent_id"`
	Version string `json:"version"`
}

func ManifestUpdateCommitToJSON(commit ManifestUpdateCommit) ManifestUpdateCommitJSON {
	return ManifestUpdateCommitJSON{
		AgentID: commit.AgentID,
		Version: commit.Version.String(),
	}
}

// ManifestUpdateOutputJSON is the JSON view of a manifest update result.
type ManifestUpdateOutputJSON struct {
	Ok            bool                       `json:"ok"`
	SchemaVersion int                        `json:"schema_version"`
	Updated       []ManifestUpdateCommitJSON `json:"updated"`
	Status        ManifestUpdateStatusJSON   `json:"status"`
}

func ManifestUpdateOutputToJSON(output ManifestUpdateOutput) ManifestUpdateOutputJSON {
	updated := make([]ManifestUpdateCommitJSON, 0, len(output.Updated))
	for _, commit := range output.Updated {
		updated = append(updated, ManifestUpdateCommitToJSON(commit))
	}
	return ManifestUpdateOutputJSON{
		Ok:            true,
		SchemaVersion: JSONSchemaVersion,
		Updated:       updated,
		Status:        ManifestUpdateStatusToJSON(output.Status),
	}
}
