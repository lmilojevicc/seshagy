package sessionmgr

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Manifest engine: capture-pane screen-rule classification for agent panes
// that lack fresh hook state (hook-less agents: opencode, cursor, antigravity,
// grok). Ported + simplified from the prior implementation. Runs only when
// [agents].manifest_fallback is enabled (default on) and only for agent panes
// whose @seshagy_agent_state is absent or stale (60s window). The classifier
// sets AgentState in-memory — it never writes tmux options, so the seq/flock/
// tombstone invariant is untouched.

//go:embed manifests/*.toml
var manifestFS embed.FS

const (
	manifestCaptureLines = 30
	maxRulesPerManifest  = 128
	maxGateDepth         = 8
	maxMatchersPerGate   = 32
	maxMatcherChars      = 512
)

var (
	manifestBrailleClass = "[\U00002800-\U000028FF]"
	reManifestHexEscape  = regexp.MustCompile(`\\x\{([0-9A-Fa-f]+)\}`)
)

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

type manifestDetectionInput struct {
	screen   string
	oscTitle string
}

type manifestDetectionResult struct {
	State           AgentState
	Matched         bool
	SkipStateUpdate bool
	RuleID          string
	Region          string
}

var (
	manifestMu      sync.RWMutex
	manifestByAgent map[string]*compiledManifest
	manifestErr     error
	manifestLoaded  bool
)

func ensureManifestsLoaded() {
	manifestMu.RLock()
	loaded := manifestLoaded
	manifestMu.RUnlock()
	if loaded {
		return
	}
	manifestMu.Lock()
	defer manifestMu.Unlock()
	if manifestLoaded {
		return
	}
	manifestByAgent, manifestErr = buildManifestCache()
	manifestLoaded = true
}

// buildManifestCache assembles the manifest cache from three layers with
// precedence: local override > cached remote > bundled embed. A remote/cache
// manifest shadows the embed only when its version is >= the embed's version
// (equal versions = remote wins, since it's the freshest copy). Malformed
// override/cache files are skipped so they can never poison the cache.
func buildManifestCache() (map[string]*compiledManifest, error) {
	cache := make(map[string]*compiledManifest)
	versions := make(map[string]string) // agent id → version string (winner)

	// Layer 1: bundled embed (offline fallback).
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := manifestFS.ReadFile("manifests/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", entry.Name(), err)
		}
		registerFromTOML(cache, versions, string(data))
	}

	// Layer 2: cached remote (from herdr catalog refresh).
	loadManifestLayer(cache, versions, cachedManifestDir)
	// Layer 3: local override (user edits — always wins).
	loadManifestLayer(cache, versions, overrideManifestDir)

	return cache, nil
}

// loadManifestLayer walks a filesystem dir, parsing + compiling each *.toml
// and applying the version-guarded precedence against the current cache.
func loadManifestLayer(
	cache map[string]*compiledManifest,
	versions map[string]string,
	dirFn func() (string, error),
) {
	dir, err := dirFn()
	if err != nil || dir == "" {
		return
	}
	files, err := readDirToml(dir)
	if err != nil {
		return
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		registerFromTOML(cache, versions, string(data))
	}
}

// registerFromTOML parses + compiles a single manifest TOML blob and, if valid,
// registers it in the cache with version-guarded precedence.
func registerFromTOML(
	cache map[string]*compiledManifest,
	versions map[string]string,
	data string,
) {
	var parsed agentManifest
	if _, err := toml.Decode(data, &parsed); err != nil {
		return // skip malformed
	}
	compiled, err := compileManifest(parsed)
	if err != nil {
		return // skip malformed
	}
	id := strings.ToLower(strings.TrimSpace(compiled.agent))
	if existing, ok := cache[id]; ok {
		// Version guard: don't let an older override/cache downgrade a newer
		// embed. Equal versions: the new one wins (freshest copy).
		if compareManifestVersion(parsed.Version, versions[id]) < 0 {
			return
		}
		_ = existing
	}
	versions[id] = parsed.Version
	registerManifest(cache, compiled)
}

// readDirToml returns *.toml filenames in a dir (best-effort; ignores errors).
func readDirToml(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		files = append(files, entry.Name())
	}
	return files, nil
}

func registerManifest(cache map[string]*compiledManifest, compiled *compiledManifest) {
	agentID := strings.ToLower(strings.TrimSpace(compiled.agent))
	cache[agentID] = compiled
	for _, alias := range compiled.aliases {
		cache[strings.ToLower(strings.TrimSpace(alias))] = compiled
	}
}

func manifestForAgent(agentName string) (*compiledManifest, bool) {
	ensureManifestsLoaded()
	manifestMu.RLock()
	defer manifestMu.RUnlock()
	if manifestErr != nil {
		return nil, false
	}
	name := strings.ToLower(strings.TrimSpace(agentName))
	if name == "" {
		return nil, false
	}
	m, ok := manifestByAgent[name]
	return m, ok
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
		gate, err := compileManifestGate(manifestGateFromRule(rule), "rule", 0)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		compiled.rules = append(compiled.rules, compiledManifestRule{
			id:              rule.ID,
			state:           state,
			priority:        rule.Priority,
			region:          region,
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

func compileManifestGate(gate manifestGate, context string, depth int) (compiledGate, error) {
	if depth > maxGateDepth {
		return compiledGate{}, fmt.Errorf("%s exceeds max gate depth %d", context, maxGateDepth)
	}
	if err := validateMatcherLimits(gate, context); err != nil {
		return compiledGate{}, err
	}
	if !gateHasPositiveMatcher(gate) {
		return compiledGate{}, fmt.Errorf("%s must contain a positive matcher", context)
	}
	compiled := compiledGate{contains: lowerStrings(gate.Contains)}
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
		child, err := compileManifestGate(nested, "all gate", depth+1)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.all = append(compiled.all, child)
	}
	for _, nested := range gate.Any {
		child, err := compileManifestGate(nested, "any gate", depth+1)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.any = append(compiled.any, child)
	}
	for _, nested := range gate.Not {
		if !gateHasAnyMatcher(nested) {
			return compiledGate{}, fmt.Errorf("%s contains an empty not gate", context)
		}
		child, err := compileManifestGate(nested, "not gate", depth+1)
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
		return nil, fmt.Errorf("%s invalid %s pattern %q: %w", context, field, pattern, err)
	}
	re, err := regexp.Compile(normalized)
	if err != nil {
		return nil, fmt.Errorf("%s invalid %s pattern %q: %w", context, field, pattern, err)
	}
	return re, nil
}

// normalizeManifestRegexPattern translates RE2-incompatible escapes used in
// manifest TOMLs into Go regexp equivalents.
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

func validateMatcherLimits(gate manifestGate, context string) error {
	matcherCount := len(gate.Contains) + len(gate.Regex) + len(gate.LineRegex)
	if matcherCount > maxMatchersPerGate {
		return fmt.Errorf(
			"%s has %d direct matchers, max is %d",
			context,
			matcherCount,
			maxMatchersPerGate,
		)
	}
	for _, v := range gate.Contains {
		if len([]rune(v)) > maxMatcherChars {
			return fmt.Errorf("%s matcher exceeds max length %d", context, maxMatcherChars)
		}
	}
	for _, v := range gate.Regex {
		if len([]rune(v)) > maxMatcherChars {
			return fmt.Errorf("%s matcher exceeds max length %d", context, maxMatcherChars)
		}
	}
	for _, v := range gate.LineRegex {
		if len([]rune(v)) > maxMatcherChars {
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
	case "whole_recent", "osc_title":
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

// detectManifest classifies the captured screen against the agent's rules.
// No-match returns AgentIdle (the 4-state model has no unknown state). A
// skip_state_update guard match returns SkipStateUpdate=true so the caller
// keeps the prior state. Highest-priority matching rule wins.
func detectManifest(agentName string, input manifestDetectionInput) manifestDetectionResult {
	manifest, ok := manifestForAgent(agentName)
	if !ok || manifest == nil {
		return manifestDetectionResult{}
	}
	var best *compiledManifestRule
	var bestScore int
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
		return manifestDetectionResult{State: AgentIdle}
	}
	if best.skipStateUpdate {
		return manifestDetectionResult{
			State:           AgentIdle,
			Matched:         false,
			SkipStateUpdate: true,
			RuleID:          best.id,
			Region:          best.region,
		}
	}
	return manifestDetectionResult{
		State:   best.state,
		Matched: true,
		RuleID:  best.id,
		Region:  best.region,
	}
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
	for i, v := range values {
		out[i] = strings.ToLower(v)
	}
	return out
}
