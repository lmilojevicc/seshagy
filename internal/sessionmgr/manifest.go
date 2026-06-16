package sessionmgr

import (
	"context"
	"embed"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed manifests/*.toml
var manifestFS embed.FS

const (
	manifestCaptureLines = 30
	maxRulesPerManifest  = 128
	maxGateDepth         = 8
	maxTotalGates        = 512
	maxMatchersPerGate   = 32
	maxTotalMatchers     = 1024
	maxMatcherChars      = 512
)

type manifestDetectionResult struct {
	State           AgentState
	Matched         bool
	SkipStateUpdate bool
	VisibleIdle     bool
	VisibleBlocker  bool
	VisibleWorking  bool
	RuleID          string
	Region          string
	FallbackReason  string
}

type agentManifest struct {
	ID               string         `toml:"id"`
	Version          string         `toml:"version"`
	MinEngineVersion int            `toml:"min_engine_version"`
	UpdatedAt        string         `toml:"updated_at"`
	Aliases          []string       `toml:"aliases"`
	Rules            []manifestRule `toml:"rules"`
}

type manifestRule struct {
	ID              string         `toml:"id"`
	State           string         `toml:"state"`
	Priority        int            `toml:"priority"`
	Region          string         `toml:"region"`
	VisibleIdle     bool           `toml:"visible_idle"`
	VisibleBlocker  bool           `toml:"visible_blocker"`
	VisibleWorking  bool           `toml:"visible_working"`
	SkipStateUpdate bool           `toml:"skip_state_update"`
	Contains        []string       `toml:"contains"`
	Regex           []string       `toml:"regex"`
	LineRegex       []string       `toml:"line_regex"`
	All             []manifestGate `toml:"all"`
	Any             []manifestGate `toml:"any"`
	Not             []manifestGate `toml:"not"`
}

type manifestGate struct {
	All       []manifestGate `toml:"all"`
	Any       []manifestGate `toml:"any"`
	Not       []manifestGate `toml:"not"`
	Contains  []string       `toml:"contains"`
	Regex     []string       `toml:"regex"`
	LineRegex []string       `toml:"line_regex"`
}

type compiledManifest struct {
	agent   string
	aliases []string
	rules   []compiledManifestRule
}

type compiledManifestRule struct {
	id              string
	state           AgentState
	priority        int
	region          string
	visibleIdle     bool
	visibleBlocker  bool
	visibleWorking  bool
	skipStateUpdate bool
	gate            compiledGate
}

type compiledGate struct {
	all       []compiledGate
	any       []compiledGate
	not       []compiledGate
	contains  []string
	regex     []*regexp.Regexp
	lineRegex []*regexp.Regexp
}

type manifestComplexity struct {
	totalGates    int
	totalMatchers int
}

type manifestCacheEntry struct {
	compiled *compiledManifest
	info     LoadedManifestInfo
}

var (
	manifestMu      sync.RWMutex
	manifestByAgent map[string]*manifestCacheEntry
	manifestErr     error
	manifestLoaded  bool

	manifestBrailleClass = "[\U00002800-\U000028FF]"
	reManifestHexEscape  = regexp.MustCompile(`\\x\{([0-9A-Fa-f]+)\}`)
)

func ReloadManifests() []AgentManifestSummary {
	manifestMu.Lock()
	defer manifestMu.Unlock()
	manifestByAgent, manifestErr = buildManifestCache()
	manifestLoaded = true
	return manifestSummariesLocked()
}

func buildManifestCache() (map[string]*manifestCacheEntry, error) {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, err
	}

	cache := make(map[string]*manifestCacheEntry)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := manifestFS.ReadFile("manifests/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", entry.Name(), err)
		}
		bundled, err := parseLocalManifest(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", entry.Name(), err)
		}
		loaded, err := loadManifestForAgent(bundled)
		if err != nil {
			return nil, fmt.Errorf("load manifest %s: %w", entry.Name(), err)
		}
		registerManifestEntry(cache, loaded)
	}
	return cache, nil
}

func registerManifestEntry(cache map[string]*manifestCacheEntry, loaded *manifestCacheEntry) {
	agentID := strings.ToLower(strings.TrimSpace(loaded.compiled.agent))
	cache[agentID] = loaded
	for _, alias := range loaded.compiled.aliases {
		cache[strings.ToLower(strings.TrimSpace(alias))] = loaded
	}
}

func loadManifestForAgent(bundled agentManifest) (*manifestCacheEntry, error) {
	agentID := strings.ToLower(strings.TrimSpace(bundled.ID))
	remote := readRemoteManifestEntry(agentID, bundled)
	cachedRemoteVersion := remoteCachedVersion(remote)

	overridePath := manifestOverridePath(agentID)
	localOverrideShadowingRemote := fileExists(overridePath) && cachedRemoteVersion != ""
	if remote != nil {
		remote.info.CachedRemoteVersion = cachedRemoteVersion
		remote.info.LocalOverrideShadowingRemote = localOverrideShadowingRemote
	}

	if !fileExists(overridePath) {
		if remote != nil {
			return remote, nil
		}
		return bundledManifestEntry(bundled, "", cachedRemoteVersion, localOverrideShadowingRemote)
	}

	content, err := os.ReadFile(overridePath)
	if err != nil {
		return fallbackManifestEntry(
			bundled,
			remote,
			cachedRemoteVersion,
			localOverrideShadowingRemote,
			fmt.Sprintf(
				"ignored override %s because it could not be loaded: %v",
				overridePath,
				err,
			),
		)
	}
	override, err := parseLocalManifest(string(content))
	if err != nil {
		return fallbackManifestEntry(
			bundled,
			remote,
			cachedRemoteVersion,
			localOverrideShadowingRemote,
			fmt.Sprintf(
				"ignored override %s because it could not be loaded: %v",
				overridePath,
				err,
			),
		)
	}
	if !manifestMatchesAgentID(&override, agentID) {
		return fallbackManifestEntry(
			bundled,
			remote,
			cachedRemoteVersion,
			localOverrideShadowingRemote,
			fmt.Sprintf(
				"ignored override %s because manifest id %q does not match %q",
				overridePath,
				override.ID,
				agentID,
			),
		)
	}
	entry, err := compileManifestEntry(
		override,
		ManifestSource{
			Kind:    ManifestSourceOverride,
			Path:    overridePath,
			Version: manifestVersionString(override),
		},
		"",
		cachedRemoteVersion,
		localOverrideShadowingRemote,
	)
	if err != nil {
		return fallbackManifestEntry(
			bundled,
			remote,
			cachedRemoteVersion,
			localOverrideShadowingRemote,
			fmt.Sprintf(
				"ignored override %s because it could not be compiled: %v",
				overridePath,
				err,
			),
		)
	}
	return entry, nil
}

func readRemoteManifestEntry(agentID string, bundled agentManifest) *manifestCacheEntry {
	path := remoteManifestPath(agentID)
	if !fileExists(path) {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return bundledManifestEntryWithWarning(
			bundled,
			fmt.Sprintf(
				"ignored remote manifest %s because it could not be loaded: %v",
				path,
				err,
			),
			"",
		)
	}
	parsed, err := parseRemoteManifestForAgent(agentID, string(content))
	if err != nil {
		return bundledManifestEntryWithWarning(
			bundled,
			fmt.Sprintf(
				"ignored remote manifest %s because it could not be loaded: %v",
				path,
				err,
			),
			"",
		)
	}
	remoteVersion := parsed.version.String()
	if bundledVersion, err := ParseManifestVersion(strings.TrimSpace(bundled.Version)); err == nil {
		if CompareManifestVersion(parsed.version, bundledVersion) < 0 {
			return bundledManifestEntryWithWarning(
				bundled,
				fmt.Sprintf(
					"ignored remote manifest %s because cached version %s is older than bundled %s",
					path,
					parsed.version,
					bundledVersion,
				),
				remoteVersion,
			)
		}
	}
	entry, err := compileManifestEntry(
		parsed.manifest,
		ManifestSource{
			Kind:    ManifestSourceRemote,
			Path:    path,
			Version: remoteVersion,
		},
		"",
		"",
		false,
	)
	if err != nil {
		return bundledManifestEntryWithWarning(
			bundled,
			fmt.Sprintf(
				"ignored remote manifest %s because it could not be compiled: %v",
				path,
				err,
			),
			remoteVersion,
		)
	}
	return entry
}

func remoteCachedVersion(remote *manifestCacheEntry) string {
	if remote == nil {
		return ""
	}
	if remote.info.Source.Kind == ManifestSourceRemote {
		return remote.info.Source.Version
	}
	return remote.info.CachedRemoteVersion
}

func bundledManifestEntry(
	bundled agentManifest,
	warning string,
	cachedRemoteVersion string,
	localOverrideShadowingRemote bool,
) (*manifestCacheEntry, error) {
	entry, err := compileManifestEntry(
		bundled,
		ManifestSource{Kind: ManifestSourceBundled, Version: manifestVersionString(bundled)},
		warning,
		cachedRemoteVersion,
		localOverrideShadowingRemote,
	)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func bundledManifestEntryWithWarning(
	bundled agentManifest,
	warning string,
	cachedRemoteVersion string,
) *manifestCacheEntry {
	entry, err := bundledManifestEntry(
		bundled,
		warning,
		cachedRemoteVersion,
		false,
	)
	if err != nil {
		panic(fmt.Sprintf("bundled %q manifest could not be compiled: %v", bundled.ID, err))
	}
	return entry
}

func fallbackManifestEntry(
	bundled agentManifest,
	remote *manifestCacheEntry,
	cachedRemoteVersion string,
	localOverrideShadowingRemote bool,
	warning string,
) (*manifestCacheEntry, error) {
	if remote != nil {
		if warning != "" {
			remote.info.Warning = warning
		}
		return remote, nil
	}
	return bundledManifestEntry(
		bundled,
		warning,
		cachedRemoteVersion,
		localOverrideShadowingRemote,
	)
}

func compileManifestEntry(
	parsed agentManifest,
	source ManifestSource,
	warning string,
	cachedRemoteVersion string,
	localOverrideShadowingRemote bool,
) (*manifestCacheEntry, error) {
	compiled, err := compileManifest(parsed)
	if err != nil {
		return nil, err
	}
	return &manifestCacheEntry{
		compiled: compiled,
		info: LoadedManifestInfo{
			Source:                       source,
			Version:                      manifestVersionString(parsed),
			CachedRemoteVersion:          cachedRemoteVersion,
			LocalOverrideShadowingRemote: localOverrideShadowingRemote,
			Warning:                      warning,
		},
	}, nil
}

func manifestSummariesLocked() []AgentManifestSummary {
	summaries := make([]AgentManifestSummary, 0)
	seen := map[string]struct{}{}
	for agentName, entry := range manifestByAgent {
		agentID := strings.ToLower(strings.TrimSpace(entry.compiled.agent))
		if agentName != agentID {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		summaries = append(summaries, AgentManifestSummary{
			AgentID:                      agentID,
			ActiveSource:                 entry.info.Source,
			ActiveVersion:                entry.info.Version,
			CachedRemoteVersion:          entry.info.CachedRemoteVersion,
			LocalOverrideShadowingRemote: entry.info.LocalOverrideShadowingRemote,
			Warning:                      entry.info.Warning,
		})
	}
	sortManifestSummaries(summaries)
	return summaries
}

func sortManifestSummaries(summaries []AgentManifestSummary) {
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].AgentID < summaries[j].AgentID
	})
}

func ManifestSummaries() []AgentManifestSummary {
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	if !manifestLoaded {
		return nil
	}
	return manifestSummariesLocked()
}

func ActiveManifestSummaries() []AgentManifestSummary {
	ensureManifestsLoaded()
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	if manifestErr != nil {
		return nil
	}
	return manifestSummariesLocked()
}

func ManifestInfoForAgent(agentName string) (LoadedManifestInfo, bool) {
	entry, err := manifestEntryForAgent(agentName)
	if err != nil || entry == nil {
		return LoadedManifestInfo{}, false
	}
	return entry.info, true
}

func ensureManifestsLoaded() {
	manifestMu.RLock()
	loaded := manifestLoaded
	manifestMu.RUnlock()
	if loaded {
		return
	}
	ReloadManifests()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func compileManifest(parsed agentManifest) (*compiledManifest, error) {
	if strings.TrimSpace(parsed.ID) == "" {
		return nil, fmt.Errorf("manifest id is required")
	}
	if len(parsed.Rules) == 0 {
		return nil, fmt.Errorf("manifest %q must contain at least one rule", parsed.ID)
	}
	if len(parsed.Rules) > maxRulesPerManifest {
		return nil, fmt.Errorf(
			"manifest %q contains %d rules, max is %d",
			parsed.ID,
			len(parsed.Rules),
			maxRulesPerManifest,
		)
	}

	complexity := manifestComplexity{}
	compiled := &compiledManifest{
		agent:   parsed.ID,
		aliases: append([]string(nil), parsed.Aliases...),
	}
	for _, rule := range parsed.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return nil, fmt.Errorf("manifest %q has a rule with an empty id", parsed.ID)
		}
		region := strings.TrimSpace(rule.Region)
		if region == "" {
			region = "whole_recent"
		}
		if err := validateManifestRegion(region); err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		state := NormalizeAgentState(rule.State)
		if rule.SkipStateUpdate {
			if state != AgentUnknown {
				return nil, fmt.Errorf(
					"rule %q uses skip_state_update without state = \"unknown\"",
					rule.ID,
				)
			}
			if rule.VisibleIdle || rule.VisibleBlocker || rule.VisibleWorking {
				return nil, fmt.Errorf(
					"rule %q uses skip_state_update with visible state evidence",
					rule.ID,
				)
			}
		}
		gate, err := compileManifestGate(manifestGateFromRule(rule), "rule", 0, &complexity)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		compiled.rules = append(compiled.rules, compiledManifestRule{
			id:              rule.ID,
			state:           state,
			priority:        rule.Priority,
			region:          region,
			visibleIdle:     rule.VisibleIdle,
			visibleBlocker:  rule.VisibleBlocker,
			visibleWorking:  rule.VisibleWorking,
			skipStateUpdate: rule.SkipStateUpdate,
			gate:            gate,
		})
	}
	return compiled, nil
}

func manifestGateFromRule(rule manifestRule) manifestGate {
	return manifestGate{
		All:       rule.All,
		Any:       rule.Any,
		Not:       rule.Not,
		Contains:  rule.Contains,
		Regex:     rule.Regex,
		LineRegex: rule.LineRegex,
	}
}

func compileManifestGate(
	gate manifestGate,
	context string,
	depth int,
	complexity *manifestComplexity,
) (compiledGate, error) {
	if depth > maxGateDepth {
		return compiledGate{}, fmt.Errorf("%s exceeds max gate depth %d", context, maxGateDepth)
	}
	complexity.totalGates++
	if complexity.totalGates > maxTotalGates {
		return compiledGate{}, fmt.Errorf("manifest exceeds max gate count %d", maxTotalGates)
	}
	if err := validateMatcherLimits(gate, context, complexity); err != nil {
		return compiledGate{}, err
	}
	if !gateHasPositiveMatcher(gate) {
		return compiledGate{}, fmt.Errorf("%s must contain a positive matcher", context)
	}

	compiled := compiledGate{
		contains: lowerStrings(gate.Contains),
	}
	for _, pattern := range gate.Regex {
		re, err := compileManifestRegex(pattern, context, "regex")
		if err != nil {
			return compiledGate{}, err
		}
		compiled.regex = append(compiled.regex, re)
	}
	for _, pattern := range gate.LineRegex {
		re, err := compileManifestRegex(pattern, context, "line_regex")
		if err != nil {
			return compiledGate{}, err
		}
		compiled.lineRegex = append(compiled.lineRegex, re)
	}
	for _, nested := range gate.All {
		child, err := compileManifestGate(nested, "all gate", depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.all = append(compiled.all, child)
	}
	for _, nested := range gate.Any {
		child, err := compileManifestGate(nested, "any gate", depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.any = append(compiled.any, child)
	}
	for _, nested := range gate.Not {
		if !gateHasAnyMatcher(nested) {
			return compiledGate{}, fmt.Errorf("%s contains an empty not gate", context)
		}
		child, err := compileManifestNotGate(nested, depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.not = append(compiled.not, child)
	}
	return compiled, nil
}

func compileManifestNotGate(
	gate manifestGate,
	depth int,
	complexity *manifestComplexity,
) (compiledGate, error) {
	if depth > maxGateDepth {
		return compiledGate{}, fmt.Errorf("not gate exceeds max gate depth %d", maxGateDepth)
	}
	complexity.totalGates++
	if complexity.totalGates > maxTotalGates {
		return compiledGate{}, fmt.Errorf("manifest exceeds max gate count %d", maxTotalGates)
	}
	if err := validateMatcherLimits(gate, "not gate", complexity); err != nil {
		return compiledGate{}, err
	}
	if !gateHasAnyMatcher(gate) {
		return compiledGate{}, fmt.Errorf("not gate must contain a matcher")
	}

	compiled := compiledGate{
		contains: lowerStrings(gate.Contains),
	}
	for _, pattern := range gate.Regex {
		re, err := compileManifestRegex(pattern, "not gate", "regex")
		if err != nil {
			return compiledGate{}, err
		}
		compiled.regex = append(compiled.regex, re)
	}
	for _, pattern := range gate.LineRegex {
		re, err := compileManifestRegex(pattern, "not gate", "line_regex")
		if err != nil {
			return compiledGate{}, err
		}
		compiled.lineRegex = append(compiled.lineRegex, re)
	}
	for _, nested := range gate.All {
		child, err := compileManifestGate(nested, "not all gate", depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.all = append(compiled.all, child)
	}
	for _, nested := range gate.Any {
		child, err := compileManifestGate(nested, "not any gate", depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.any = append(compiled.any, child)
	}
	for _, nested := range gate.Not {
		child, err := compileManifestNotGate(nested, depth+1, complexity)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.not = append(compiled.not, child)
	}
	return compiled, nil
}

func compileManifestRegex(pattern, context, field string) (*regexp.Regexp, error) {
	normalized, err := normalizeManifestRegexPattern(pattern)
	if err != nil {
		return nil, fmt.Errorf(
			"%s contains invalid %s pattern %q: %w",
			context,
			field,
			pattern,
			err,
		)
	}
	re, err := regexp.Compile(normalized)
	if err != nil {
		return nil, fmt.Errorf(
			"%s contains invalid %s pattern %q: %w",
			context,
			field,
			pattern,
			err,
		)
	}
	return re, nil
}

func normalizeManifestRegexPattern(pattern string) (string, error) {
	pattern = strings.ReplaceAll(pattern, `\p{Alphabetic}`, `\p{Letter}`)
	pattern = strings.ReplaceAll(pattern, `[\x{2800}-\x{28FF}]`, manifestBrailleClass)
	pattern = strings.ReplaceAll(pattern, `[\u2800-\u28FF]`, manifestBrailleClass)

	var normalizeErr error
	pattern = reManifestHexEscape.ReplaceAllStringFunc(pattern, func(match string) string {
		if normalizeErr != nil {
			return match
		}
		submatches := reManifestHexEscape.FindStringSubmatch(match)
		if len(submatches) < 2 {
			normalizeErr = fmt.Errorf("invalid hex escape %q", match)
			return match
		}
		code, err := strconv.ParseUint(submatches[1], 16, 32)
		if err != nil {
			normalizeErr = fmt.Errorf("invalid hex escape %q: %w", match, err)
			return match
		}
		return string(rune(code))
	})
	if normalizeErr != nil {
		return "", normalizeErr
	}
	return pattern, nil
}

func validateMatcherLimits(
	gate manifestGate,
	context string,
	complexity *manifestComplexity,
) error {
	matcherCount := len(gate.Contains) + len(gate.Regex) + len(gate.LineRegex)
	if matcherCount > maxMatchersPerGate {
		return fmt.Errorf(
			"%s has %d direct matchers, max is %d",
			context,
			matcherCount,
			maxMatchersPerGate,
		)
	}
	complexity.totalMatchers += matcherCount
	if complexity.totalMatchers > maxTotalMatchers {
		return fmt.Errorf("manifest exceeds max matcher count %d", maxTotalMatchers)
	}
	for _, value := range gate.Contains {
		if len([]rune(value)) > maxMatcherChars {
			return fmt.Errorf("%s matcher exceeds max length %d", context, maxMatcherChars)
		}
	}
	for _, value := range gate.Regex {
		if len([]rune(value)) > maxMatcherChars {
			return fmt.Errorf("%s matcher exceeds max length %d", context, maxMatcherChars)
		}
	}
	for _, value := range gate.LineRegex {
		if len([]rune(value)) > maxMatcherChars {
			return fmt.Errorf("%s matcher exceeds max length %d", context, maxMatcherChars)
		}
	}
	return nil
}

func gateHasPositiveMatcher(gate manifestGate) bool {
	if len(gate.Contains) > 0 || len(gate.Regex) > 0 || len(gate.LineRegex) > 0 {
		return true
	}
	for _, nested := range gate.All {
		if gateHasPositiveMatcher(nested) {
			return true
		}
	}
	for _, nested := range gate.Any {
		if gateHasPositiveMatcher(nested) {
			return true
		}
	}
	return false
}

func gateHasAnyMatcher(gate manifestGate) bool {
	return gateHasPositiveMatcher(gate) || len(gate.Not) > 0
}

func validateManifestRegion(region string) error {
	switch region {
	case "whole_recent",
		"after_last_prompt_marker",
		"before_current_prompt_marker",
		"whole_recent_without_current_prompt_marker",
		"current_prompt_block_marker",
		"after_current_prompt_block_marker",
		"prompt_box_body",
		"above_prompt_box",
		"last_non_empty_above_prompt_box",
		"after_last_horizontal_rule",
		"osc_title",
		"osc_progress":
		return nil
	}
	if _, ok := manifestRegionCount(region, "bottom_lines"); ok {
		return nil
	}
	if _, ok := manifestRegionCount(region, "bottom_non_empty_lines"); ok {
		return nil
	}
	return fmt.Errorf("unsupported region %q", region)
}

func manifestForAgent(agentName string) (*compiledManifest, error) {
	entry, err := manifestEntryForAgent(agentName)
	if err != nil || entry == nil {
		return nil, err
	}
	return entry.compiled, nil
}

func manifestEntryForAgent(agentName string) (*manifestCacheEntry, error) {
	ensureManifestsLoaded()
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	if manifestErr != nil {
		return nil, manifestErr
	}
	agentName = strings.ToLower(strings.TrimSpace(agentName))
	if agentName == "" {
		return nil, nil
	}
	return manifestByAgent[agentName], nil
}

func detectManifest(agentName string, input manifestDetectionInput) manifestDetectionResult {
	manifest, err := manifestForAgent(agentName)
	if err != nil || manifest == nil {
		return manifestDetectionResult{}
	}

	var (
		best      *compiledManifestRule
		bestScore int
	)
	for i := range manifest.rules {
		rule := &manifest.rules[i]
		regionText := manifestRegion(input, rule.region)
		if !compiledGateMatches(rule.gate, regionText) {
			continue
		}
		if best == nil || rule.priority > bestScore {
			best = rule
			bestScore = rule.priority
		}
	}

	if best == nil {
		return manifestDetectionResult{State: AgentUnknown}
	}

	state := best.state
	if best.skipStateUpdate {
		state = AgentUnknown
	}

	result := manifestDetectionResult{
		State:           state,
		Matched:         true,
		SkipStateUpdate: best.skipStateUpdate,
		RuleID:          best.id,
		Region:          best.region,
	}
	switch state {
	case AgentIdle:
		result.VisibleIdle = best.visibleIdle
	case AgentWorking:
		result.VisibleWorking = best.visibleWorking
	case AgentBlocked:
		result.VisibleBlocker = best.visibleBlocker
	}
	return result
}

func shouldApplyManifestFallback(state AgentState, agentName, source string) bool {
	_ = agentName
	_ = source
	return state == AgentUnknown
}

func manifestExplainLine(
	ctx context.Context,
	pane, agentName, source, title string,
	state AgentState,
) string {
	if !shouldApplyManifestFallback(state, agentName, source) {
		return "manifest skipped"
	}
	screen, err := captureAgentPaneCached(ctx, nil, pane, manifestCaptureLines)
	if err != nil {
		return "manifest skipped"
	}
	result := detectManifest(agentName, manifestDetectionInput{
		screen:      screen,
		oscTitle:    strings.TrimSpace(StripANSI(title)),
		oscProgress: "",
	})
	return formatManifestExplain(result)
}

func formatManifestExplain(result manifestDetectionResult) string {
	if result.FallbackReason != "" {
		return fmt.Sprintf("fallback %s → %s", result.FallbackReason, AgentStateLabel(result.State))
	}
	if !result.Matched {
		return "manifest skipped"
	}
	if result.SkipStateUpdate {
		return fmt.Sprintf(
			"rule %s (region %s) → skip [skip_state_update]",
			result.RuleID,
			result.Region,
		)
	}
	line := fmt.Sprintf(
		"rule %s (region %s) → %s",
		result.RuleID,
		result.Region,
		AgentStateLabel(result.State),
	)
	var flags []string
	if result.VisibleIdle {
		flags = append(flags, "visible_idle")
	}
	if result.VisibleBlocker {
		flags = append(flags, "visible_blocker")
	}
	if result.VisibleWorking {
		flags = append(flags, "visible_working")
	}
	if len(flags) > 0 {
		line += " [" + strings.Join(flags, ", ") + "]"
	}
	return line
}

type manifestCaptureCache map[string]string

func captureAgentPaneCached(
	ctx context.Context,
	cache manifestCaptureCache,
	pane string,
	lines int,
) (string, error) {
	if cache != nil {
		if screen, ok := cache[pane]; ok {
			return screen, nil
		}
	}
	screen, err := CaptureAgentPane(ctx, pane, lines)
	if err != nil {
		return "", err
	}
	if cache != nil {
		cache[pane] = screen
	}
	return screen, nil
}

func compiledGateMatches(gate compiledGate, text string) bool {
	lowerText := strings.ToLower(text)
	for _, needle := range gate.contains {
		if !strings.Contains(lowerText, needle) {
			return false
		}
	}
	for _, re := range gate.regex {
		if !re.MatchString(text) {
			return false
		}
	}
	for _, re := range gate.lineRegex {
		if !lineRegexMatches(re, text) {
			return false
		}
	}
	for _, nested := range gate.all {
		if !compiledGateMatches(nested, text) {
			return false
		}
	}
	if len(gate.any) > 0 {
		matched := false
		for _, nested := range gate.any {
			if compiledGateMatches(nested, text) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, nested := range gate.not {
		if compiledGateMatches(nested, text) {
			return false
		}
	}
	return true
}

func lineRegexMatches(re *regexp.Regexp, text string) bool {
	for _, line := range manifestLines(text) {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func manifestRegionCount(spec, name string) (int, bool) {
	rest, ok := strings.CutPrefix(spec, name)
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutPrefix(rest, "(")
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutSuffix(rest, ")")
	if !ok {
		return 0, false
	}
	count, err := strconv.Atoi(rest)
	if err != nil || count < 0 {
		return 0, false
	}
	return count, true
}

func manifestBottomLines(content string, count int) string {
	lines := manifestLines(content)
	start := max(len(lines)-count, 0)
	return manifestSliceFromLineIndex(content, lines, start)
}

func manifestBottomNonEmptyLines(content string, count int) string {
	lines := manifestLines(content)
	startIndex := -1
	seen := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		seen++
		startIndex = i
		if seen >= count {
			break
		}
	}
	if startIndex < 0 {
		return ""
	}
	return manifestSliceFromLineIndex(content, lines, startIndex)
}

func manifestSliceFromLineIndex(content string, lines []string, index int) string {
	if index < 0 {
		index = 0
	}
	if index >= len(lines) {
		return ""
	}
	return content[lineStartOffset(content, lines, index):]
}

func lowerStrings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.ToLower(value)
	}
	return out
}
