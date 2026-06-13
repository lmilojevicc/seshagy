package sessionmgr

import (
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

//go:embed manifests/*.toml
var manifestFS embed.FS

const manifestCaptureLines = 20

type manifestMatch struct {
	State          AgentState
	VisibleBlocker bool
	RuleID         string
}

type agentManifest struct {
	ID      string         `toml:"id"`
	Aliases []string       `toml:"aliases"`
	Rules   []manifestRule `toml:"rules"`
}

type manifestRule struct {
	ID             string         `toml:"id"`
	State          string         `toml:"state"`
	Priority       int            `toml:"priority"`
	Region         string         `toml:"region"`
	VisibleBlocker bool           `toml:"visible_blocker"`
	Contains       []string       `toml:"contains"`
	Regex          []string       `toml:"regex"`
	All            []manifestGate `toml:"all"`
	Any            []manifestGate `toml:"any"`
	Not            []manifestGate `toml:"not"`
}

type manifestGate struct {
	All      []manifestGate `toml:"all"`
	Any      []manifestGate `toml:"any"`
	Not      []manifestGate `toml:"not"`
	Contains []string       `toml:"contains"`
	Regex    []string       `toml:"regex"`
}

type compiledManifest struct {
	agent string
	rules []compiledManifestRule
}

type compiledManifestRule struct {
	id             string
	state          AgentState
	priority       int
	region         string
	visibleBlocker bool
	gate           compiledGate
}

type compiledGate struct {
	all      []compiledGate
	any      []compiledGate
	not      []compiledGate
	contains []string
	regex    []*regexp.Regexp
}

var (
	manifestOnce    sync.Once
	manifestByAgent map[string]*compiledManifest
	manifestErr     error
)

func initManifests() {
	manifestByAgent = make(map[string]*compiledManifest)
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		manifestErr = err
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		data, err := manifestFS.ReadFile("manifests/" + entry.Name())
		if err != nil {
			manifestErr = fmt.Errorf("read manifest %s: %w", entry.Name(), err)
			return
		}
		var parsed agentManifest
		if _, err := toml.Decode(string(data), &parsed); err != nil {
			manifestErr = fmt.Errorf("parse manifest %s: %w", entry.Name(), err)
			return
		}
		compiled, err := compileManifest(parsed)
		if err != nil {
			manifestErr = fmt.Errorf("compile manifest %s: %w", entry.Name(), err)
			return
		}
		manifestByAgent[strings.ToLower(parsed.ID)] = compiled
		for _, alias := range parsed.Aliases {
			manifestByAgent[strings.ToLower(alias)] = compiled
		}
	}
}

func compileManifest(parsed agentManifest) (*compiledManifest, error) {
	if strings.TrimSpace(parsed.ID) == "" {
		return nil, fmt.Errorf("manifest id is required")
	}
	if len(parsed.Rules) == 0 {
		return nil, fmt.Errorf("manifest %q must contain at least one rule", parsed.ID)
	}
	compiled := &compiledManifest{agent: parsed.ID}
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
		if rule.VisibleBlocker && state != AgentBlocked {
			return nil, fmt.Errorf("rule %q sets visible_blocker without blocked state", rule.ID)
		}
		gate, err := compileManifestGate(manifestGate{
			All:      rule.All,
			Any:      rule.Any,
			Not:      rule.Not,
			Contains: rule.Contains,
			Regex:    rule.Regex,
		})
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", rule.ID, err)
		}
		compiled.rules = append(compiled.rules, compiledManifestRule{
			id:             rule.ID,
			state:          state,
			priority:       rule.Priority,
			region:         region,
			visibleBlocker: rule.VisibleBlocker,
			gate:           gate,
		})
	}
	return compiled, nil
}

func compileManifestGate(gate manifestGate) (compiledGate, error) {
	compiled := compiledGate{
		contains: lowerStrings(gate.Contains),
	}
	for _, pattern := range gate.Regex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return compiledGate{}, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		compiled.regex = append(compiled.regex, re)
	}
	for _, nested := range gate.All {
		child, err := compileManifestGate(nested)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.all = append(compiled.all, child)
	}
	for _, nested := range gate.Any {
		child, err := compileManifestGate(nested)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.any = append(compiled.any, child)
	}
	for _, nested := range gate.Not {
		child, err := compileManifestGate(nested)
		if err != nil {
			return compiledGate{}, err
		}
		compiled.not = append(compiled.not, child)
	}
	if !gateHasMatcher(gate) {
		return compiledGate{}, fmt.Errorf("gate must contain a matcher")
	}
	return compiled, nil
}

func gateHasMatcher(gate manifestGate) bool {
	if len(gate.Contains) > 0 || len(gate.Regex) > 0 {
		return true
	}
	for _, nested := range gate.All {
		if gateHasMatcher(nested) {
			return true
		}
	}
	for _, nested := range gate.Any {
		if gateHasMatcher(nested) {
			return true
		}
	}
	for _, nested := range gate.Not {
		if gateHasMatcher(nested) {
			return true
		}
	}
	return false
}

func validateManifestRegion(region string) error {
	if region == "whole_recent" {
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
	manifestOnce.Do(initManifests)
	if manifestErr != nil {
		return nil, manifestErr
	}
	agentName = strings.ToLower(strings.TrimSpace(agentName))
	if agentName == "" {
		return nil, nil
	}
	return manifestByAgent[agentName], nil
}

func detectStateFromManifest(agentName, screen string) (manifestMatch, bool) {
	manifest, err := manifestForAgent(agentName)
	if err != nil || manifest == nil {
		return manifestMatch{}, false
	}
	var (
		best      *compiledManifestRule
		bestScore int
	)
	for i := range manifest.rules {
		rule := &manifest.rules[i]
		regionText := manifestRegion(screen, rule.region)
		if !compiledGateMatches(rule.gate, regionText) {
			continue
		}
		if best == nil || rule.priority > bestScore {
			best = rule
			bestScore = rule.priority
		}
	}
	if best == nil || best.state == AgentUnknown {
		return manifestMatch{}, false
	}
	return manifestMatch{
		State:          best.state,
		VisibleBlocker: best.visibleBlocker && best.state == AgentBlocked,
		RuleID:         best.id,
	}, true
}

func shouldApplyManifestFallback(state AgentState, agentName, source string) bool {
	if state != AgentUnknown {
		return false
	}
	return !HasLifecycleAuthority(agentName, source)
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

func manifestRegion(screen, spec string) string {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "", "whole_recent":
		return screen
	default:
		if count, ok := manifestRegionCount(spec, "bottom_lines"); ok {
			return manifestBottomLines(screen, count)
		}
		if count, ok := manifestRegionCount(spec, "bottom_non_empty_lines"); ok {
			return manifestBottomNonEmptyLines(screen, count)
		}
		return ""
	}
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
	lines := strings.Split(content, "\n")
	start := max(len(lines)-count, 0)
	return manifestSliceFromLineIndex(content, lines, start)
}

func manifestBottomNonEmptyLines(content string, count int) string {
	lines := strings.Split(content, "\n")
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
	offset := 0
	for i := 0; i < index && i < len(lines); i++ {
		offset += len(lines[i]) + 1
	}
	if offset > len(content) {
		offset = len(content)
	}
	return content[offset:]
}

func lowerStrings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.ToLower(value)
	}
	return out
}
