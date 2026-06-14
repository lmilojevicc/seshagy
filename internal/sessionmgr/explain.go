package sessionmgr

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

type agentExplain struct {
	PaneID     string
	Location   string
	Path       string
	Visible    bool
	Listed     bool
	SkipReason string

	IdentitySource string
	AgentName      string
	Command        string
	Title          string

	StateSource      string
	HookStateRaw     string
	HookStateStale   bool
	EffectiveStatus  AgentState
	DetectedStatus   AgentState
	TrackingOverride bool

	AgentSource    string
	AgentSeq       string
	AgentSessionID string
	AgentMessage   string
	AgentUpdated   string

	LifecycleAuthority bool

	LastState  string
	LastStatus string
	LastSeen   string

	IntegrationStatus string
	ManifestFallback  string
	ManifestSource    string
	ManifestVersion   string
	CachedRemoteVer   string
	ManifestWarning   string
}

func ExplainAgent(ctx context.Context, pane string, opts LoadOptions) (string, error) {
	report, err := ExplainAgentReport(ctx, pane, opts)
	if err != nil {
		return "", err
	}
	return formatAgentExplain(reportToAgentExplain(report)), nil
}

func ExplainAgentReport(
	ctx context.Context,
	pane string,
	opts LoadOptions,
) (AgentExplainReport, error) {
	resolved, err := ResolvePane(ctx, pane)
	if err != nil {
		return AgentExplainReport{}, err
	}
	line, err := displayPane(ctx, resolved, agentFormat)
	if err != nil {
		return AgentExplainReport{}, fmt.Errorf("pane metadata: %w", err)
	}
	parts := strings.Split(line, paneSep)
	if len(parts) < agentPaneMinFields {
		return AgentExplainReport{}, fmt.Errorf("unexpected pane metadata fields: %d", len(parts))
	}
	info := buildAgentExplain(ctx, resolved, parts, opts.ManifestFallback)
	if opts.ManifestFallback {
		info.ManifestFallback = manifestExplainLine(
			ctx,
			resolved,
			info.AgentName,
			info.AgentSource,
			info.Title,
			info.DetectedStatus,
		)
	}
	if info.AgentName != "" {
		applyManifestExplainMeta(&info)
	}
	return agentExplainToReport(info), nil
}

func reportToAgentExplain(report AgentExplainReport) agentExplain {
	return agentExplain{
		PaneID:             report.PaneID,
		Location:           report.Location,
		Path:               report.Path,
		Visible:            report.Visible,
		Listed:             report.Listed,
		SkipReason:         report.SkipReason,
		IdentitySource:     report.IdentitySource,
		AgentName:          report.AgentName,
		Command:            report.Command,
		Title:              report.Title,
		StateSource:        report.StateSource,
		HookStateRaw:       report.HookState,
		HookStateStale:     report.HookStateStale,
		EffectiveStatus:    report.EffectiveStatus,
		DetectedStatus:     report.DetectedStatus,
		TrackingOverride:   report.TrackingOverride,
		AgentSource:        report.Source,
		AgentSeq:           report.Seq,
		AgentSessionID:     report.SessionID,
		AgentMessage:       report.Message,
		AgentUpdated:       report.UpdatedAt,
		LifecycleAuthority: report.LifecycleAuthority,
		LastState:          report.LastState,
		LastStatus:         report.LastStatus,
		LastSeen:           report.LastSeen,
		IntegrationStatus:  report.Integration,
		ManifestFallback:   report.ManifestFallback,
		ManifestSource:     report.ManifestSource,
		ManifestVersion:    report.ManifestVersion,
		CachedRemoteVer:    report.CachedRemoteVer,
		ManifestWarning:    report.ManifestWarning,
	}
}

func buildAgentExplain(
	ctx context.Context,
	pane string,
	parts []string,
	skipTitleInference bool,
) agentExplain {
	command := cleanField(parts[9])
	title := cleanField(parts[10])
	hookName := cleanField(parts[11])
	hookStateRaw := cleanField(parts[12])

	info := agentExplain{
		PaneID:         pane,
		Location:       fmt.Sprintf("%s:%s.%s", parts[1], parts[2], parts[3]),
		Path:           ContractHome(parts[4]),
		Visible:        parts[5] == "1" && parts[6] == "1" && parts[7] != "0",
		Command:        command,
		Title:          title,
		HookStateRaw:   hookStateRaw,
		AgentMessage:   cleanField(parts[13]),
		AgentUpdated:   cleanField(parts[14]),
		AgentSource:    cleanField(parts[15]),
		AgentSessionID: cleanField(parts[16]),
		AgentSeq:       cleanField(parts[17]),
		LastState:      cleanField(mustShowPaneOption(ctx, pane, "@agent_last_state")),
		LastStatus:     cleanField(mustShowPaneOption(ctx, pane, "@agent_last_status")),
		LastSeen:       cleanField(mustShowPaneOption(ctx, pane, "@agent_last_seen")),
	}

	if parts[8] == "1" {
		info.Listed = false
		info.SkipReason = "pane is dead"
		return info
	}

	hookReported := hookName != ""
	name := hookName
	panePID := panePIDFromParts(parts)
	if name == "" {
		name = detectAgentName(command, title, panePID)
		if name != "" {
			info.IdentitySource = "process detection (command/title)"
		}
	} else {
		info.IdentitySource = "hook (@agent_name)"
	}
	info.AgentName = name

	unhooked := !hookReported && integrations.HookCapableAgent(name)
	if name == "" {
		info.Listed = false
		info.SkipReason = "no agent identity (missing @agent_name and process detection)"
	} else if unhooked {
		info.Listed = true
		if info.AgentSource == "" {
			info.AgentSource = agentSourceUnhooked
		}
	} else {
		info.Listed = true
		if !hookReported && info.AgentSource == "" {
			info.AgentSource = "process"
		}
	}

	now := agentResolveNow()
	detected := NormalizeAgentState(hookStateRaw)
	stale := isHookStateStale(detected, info.AgentUpdated, now)
	info.HookStateStale = stale
	if stale {
		info.StateSource = "hook state stale (TTL exceeded)"
		detected = AgentUnknown
	} else {
		switch {
		case hookStateRaw != "":
			info.StateSource = fmt.Sprintf(
				"hook (@agent_state): %s",
				agentStateLabel(detected),
			)
		default:
			if skipTitleInference {
				info.StateSource = "default (unknown)"
			} else if shouldSupplementStateFromTitle(
				hookStateRaw,
				NormalizeAgentState(hookStateRaw),
				name,
				info.AgentSource,
				info.AgentUpdated,
				now,
			) {
				if inferred := InferStateFromTitle(name, title); inferred != AgentUnknown {
					detected = inferred
					info.StateSource = fmt.Sprintf("title inference: %s", agentStateLabel(inferred))
				} else {
					info.StateSource = "default (unknown)"
				}
			} else {
				info.StateSource = "default (unknown)"
			}
		}
	}

	lifecycle := HasLifecycleAuthority(name, info.AgentSource)
	info.LifecycleAuthority = lifecycle

	if info.LastStatus != "" {
		info.EffectiveStatus = NormalizeAgentState(info.LastStatus)
	} else {
		info.EffectiveStatus = detected
	}
	info.DetectedStatus = detected

	semantic := semanticAgentState(detected)
	if info.LastStatus != "" && info.EffectiveStatus != semantic {
		info.TrackingOverride = true
	} else if info.LastState != "" &&
		semanticAgentState(NormalizeAgentState(info.LastState)) != semantic {
		info.TrackingOverride = true
	}

	if integrations.HookCapableAgent(name) {
		info.IntegrationStatus = integrationStatusLine(name)
	}

	return info
}

func mustShowPaneOption(ctx context.Context, pane, opt string) string {
	value, _ := showPaneOption(ctx, pane, opt)
	return value
}

func applyManifestExplainMeta(info *agentExplain) {
	meta, ok := ManifestInfoForAgent(info.AgentName)
	if !ok {
		return
	}
	info.ManifestSource = meta.Source.KindLabel()
	if meta.Source.Path != "" && meta.Source.Kind != ManifestSourceBundled {
		info.ManifestSource = meta.Source.Label()
	}
	if meta.Version != "" {
		info.ManifestVersion = meta.Version
	}
	if meta.CachedRemoteVersion != "" {
		info.CachedRemoteVer = meta.CachedRemoteVersion
	}
	info.ManifestWarning = meta.Warning
}

func integrationStatusLine(agentName string) string {
	target, ok := authorityTarget(agentName)
	if !ok {
		return fmt.Sprintf("%s: unknown integration target", agentName)
	}
	for _, rec := range integrations.Scan() {
		if rec.Target != target {
			continue
		}
		state := string(rec.State)
		if rec.State == integrations.StatusCurrent {
			state = fmt.Sprintf("current (v%d)", rec.Version)
		}
		line := fmt.Sprintf("%s: %s (%s authority)", rec.Label, state, rec.Authority)
		if !rec.Installable && rec.AgentAvailable && rec.Reason != "" {
			line += "; " + rec.Reason
		}
		return line
	}
	return fmt.Sprintf("%s: integration not found", agentName)
}

func formatAgentExplain(info agentExplain) string {
	var b strings.Builder
	fmt.Fprintf(&b, "pane: %s\n", info.PaneID)
	fmt.Fprintf(&b, "location: %s\n", info.Location)
	if info.Path != "" {
		fmt.Fprintf(&b, "path: %s\n", info.Path)
	}
	fmt.Fprintf(&b, "visible: %t\n", info.Visible)
	fmt.Fprintf(&b, "listed: %t", info.Listed)
	if !info.Listed && info.SkipReason != "" {
		fmt.Fprintf(&b, " (%s)", info.SkipReason)
	}
	b.WriteByte('\n')
	b.WriteByte('\n')

	if info.IdentitySource != "" {
		fmt.Fprintf(&b, "identity source: %s\n", info.IdentitySource)
	} else {
		b.WriteString("identity source: none\n")
	}
	if info.AgentName != "" {
		fmt.Fprintf(&b, "agent name: %s\n", info.AgentName)
	}
	if info.Command != "" {
		fmt.Fprintf(&b, "command: %s\n", info.Command)
	}
	if info.Title != "" {
		fmt.Fprintf(&b, "title: %s\n", info.Title)
	}

	b.WriteByte('\n')
	fmt.Fprintf(&b, "state source: %s\n", info.StateSource)
	if info.HookStateRaw != "" {
		fmt.Fprintf(&b, "@agent_state: %s\n", info.HookStateRaw)
		if info.HookStateStale {
			fmt.Fprintf(&b, "hook freshness: stale (TTL exceeded)\n")
		}
	}
	fmt.Fprintf(&b, "effective status: %s", agentStateLabel(info.EffectiveStatus))
	if info.TrackingOverride {
		b.WriteString(" (tracking override)")
	}
	b.WriteByte('\n')

	b.WriteByte('\n')
	fmt.Fprintf(&b, "@agent_source: %s\n", displayEmpty(info.AgentSource))
	fmt.Fprintf(&b, "@agent_seq: %s\n", displayEmpty(info.AgentSeq))
	fmt.Fprintf(&b, "@agent_session_id: %s\n", displayEmpty(info.AgentSessionID))
	if info.AgentMessage != "" {
		fmt.Fprintf(&b, "@agent_message: %s\n", info.AgentMessage)
	}
	if info.AgentUpdated != "" {
		fmt.Fprintf(&b, "@agent_updated: %s\n", info.AgentUpdated)
	}

	b.WriteByte('\n')
	fmt.Fprintf(
		&b,
		"lifecycle authority: %s\n",
		yesNo(info.LifecycleAuthority),
	)

	b.WriteByte('\n')
	fmt.Fprintf(&b, "@agent_last_state: %s\n", displayEmpty(info.LastState))
	fmt.Fprintf(&b, "@agent_last_status: %s\n", displayEmpty(info.LastStatus))
	fmt.Fprintf(&b, "@agent_last_seen: %s\n", formatLastSeen(info.LastSeen))

	if info.IntegrationStatus != "" {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "integration: %s\n", info.IntegrationStatus)
	}

	if info.ManifestFallback != "" {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "manifest fallback: %s\n", info.ManifestFallback)
	}

	if info.ManifestSource != "" {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "manifest source: %s\n", info.ManifestSource)
		if info.ManifestVersion != "" {
			fmt.Fprintf(&b, "manifest version: %s\n", info.ManifestVersion)
		}
		if info.CachedRemoteVer != "" {
			fmt.Fprintf(&b, "cached remote version: %s\n", info.CachedRemoteVer)
		}
		if info.ManifestWarning != "" {
			fmt.Fprintf(&b, "manifest warning: %s\n", info.ManifestWarning)
		}
	}

	return b.String()
}

func displayEmpty(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatLastSeen(raw string) string {
	if raw == "" {
		return "(unset)"
	}
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%s (%s)", raw, time.Unix(ts, 0).Format(time.RFC3339))
}
