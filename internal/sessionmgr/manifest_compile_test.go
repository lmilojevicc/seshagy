package sessionmgr

import (
	"strings"
	"testing"
)

func TestCompileManifestNotGateRejectsEmptyMatcher(t *testing.T) {
	complexity := &manifestComplexity{}
	_, err := compileManifestNotGate(manifestGate{}, 1, complexity)
	if err == nil {
		t.Fatal("expected empty not gate error")
	}
	if !strings.Contains(err.Error(), "not gate must contain a matcher") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileManifestNotGateAcceptsMatcher(t *testing.T) {
	complexity := &manifestComplexity{}
	gate, err := compileManifestNotGate(manifestGate{Contains: []string{"busy"}}, 1, complexity)
	if err != nil {
		t.Fatalf("compileManifestNotGate() = %v", err)
	}
	if !compiledGateMatches(gate, "system is busy") {
		t.Fatal("expected not gate to match busy text")
	}
	if compiledGateMatches(gate, "all clear") {
		t.Fatal("expected not gate to reject unrelated text")
	}
}

func TestCompileManifestGateNestedAllAny(t *testing.T) {
	complexity := &manifestComplexity{}
	gate, err := compileManifestGate(manifestGate{
		All: []manifestGate{
			{Contains: []string{"hello"}},
			{Any: []manifestGate{
				{Contains: []string{"foo"}},
				{Contains: []string{"bar"}},
			}},
		},
	}, "rule", 0, complexity)
	if err != nil {
		t.Fatalf("compileManifestGate() = %v", err)
	}
	if !compiledGateMatches(gate, "hello world bar") {
		t.Fatal("expected nested all/any gate to match")
	}
	if compiledGateMatches(gate, "hello only") {
		t.Fatal("expected nested all/any gate to reject missing any branch")
	}
}

func TestCompileManifestGateDepthExceeded(t *testing.T) {
	complexity := &manifestComplexity{}
	_, err := compileManifestGate(deepManifestGate(10), "rule", 0, complexity)
	if err == nil {
		t.Fatal("expected depth exceeded error")
	}
	if !strings.Contains(err.Error(), "exceeds max gate depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMatcherLimitsRejectsPerGateBudget(t *testing.T) {
	gate := gateWithContains(maxMatchersPerGate+1, "x")
	cases := []struct {
		name string
		fn   func(manifestGate, *manifestComplexity) error
	}{
		{
			name: "validate",
			fn: func(g manifestGate, c *manifestComplexity) error {
				return validateMatcherLimits(g, "rule", c)
			},
		},
		{
			name: "not gate",
			fn: func(g manifestGate, c *manifestComplexity) error {
				_, err := compileManifestNotGate(g, 0, c)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn(gate, &manifestComplexity{})
			if err == nil {
				t.Fatal("expected per-gate matcher limit error")
			}
			if !strings.Contains(err.Error(), "direct matchers") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMatcherLimitsRejectsTotalBudget(t *testing.T) {
	complexity := &manifestComplexity{totalMatchers: maxTotalMatchers}
	err := validateMatcherLimits(manifestGate{Contains: []string{"x"}}, "rule", complexity)
	if err == nil {
		t.Fatal("expected total matcher limit error")
	}
	if !strings.Contains(err.Error(), "max matcher count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMatcherLimitsRejectsLongMatchers(t *testing.T) {
	long := strings.Repeat("a", maxMatcherChars+1)
	cases := []struct {
		name string
		gate manifestGate
	}{
		{name: "contains", gate: manifestGate{Contains: []string{long}}},
		{name: "regex", gate: manifestGate{Regex: []string{long}}},
		{name: "line_regex", gate: manifestGate{LineRegex: []string{long}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMatcherLimits(tc.gate, tc.name, &manifestComplexity{})
			if err == nil {
				t.Fatal("expected matcher length error")
			}
			if !strings.Contains(err.Error(), "max length") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompileManifestNotGateCompilesRegex(t *testing.T) {
	gate, err := compileManifestNotGate(
		manifestGate{Regex: []string{"busy|wait"}},
		0,
		&manifestComplexity{},
	)
	if err != nil {
		t.Fatalf("compileManifestNotGate() = %v", err)
	}
	if !compiledGateMatches(gate, "system busy") {
		t.Fatal("expected regex not gate to match busy text")
	}
	if compiledGateMatches(gate, "all clear") {
		t.Fatal("expected regex not gate to reject unrelated text")
	}
}

func TestCompileManifestNotGateCompilesLineRegexAndNestedAll(t *testing.T) {
	gate, err := compileManifestNotGate(manifestGate{
		All: []manifestGate{
			{Contains: []string{"busy"}},
			{LineRegex: []string{`^wait$`}},
		},
	}, 0, &manifestComplexity{})
	if err != nil {
		t.Fatalf("compileManifestNotGate() = %v", err)
	}
	if !compiledGateMatches(gate, "busy\nwait") {
		t.Fatal("expected nested all not gate to match busy wait lines")
	}
	if compiledGateMatches(gate, "all clear") {
		t.Fatal("expected nested all not gate to reject unrelated text")
	}
}

func TestCompileManifestNotGateDepthExceeded(t *testing.T) {
	_, err := compileManifestNotGate(deepNotManifestGate(10), 0, &manifestComplexity{})
	if err == nil {
		t.Fatal("expected not gate depth error")
	}
	if !strings.Contains(err.Error(), "not gate exceeds max gate depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileManifestNotGateRejectsTotalGateBudget(t *testing.T) {
	complexity := &manifestComplexity{totalGates: maxTotalGates}
	_, err := compileManifestNotGate(manifestGate{Contains: []string{"busy"}}, 0, complexity)
	if err == nil {
		t.Fatal("expected total gate limit error")
	}
	if !strings.Contains(err.Error(), "max gate count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileManifestGateRejectsTotalGateBudget(t *testing.T) {
	complexity := &manifestComplexity{totalGates: maxTotalGates}
	_, err := compileManifestGate(manifestGate{Contains: []string{"ready"}}, "rule", 0, complexity)
	if err == nil {
		t.Fatal("expected total gate limit error")
	}
	if !strings.Contains(err.Error(), "max gate count") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileManifestGateCompilesLineRegex(t *testing.T) {
	gate, err := compileManifestGate(
		manifestGate{LineRegex: []string{`^ready$`}},
		"rule",
		0,
		&manifestComplexity{},
	)
	if err != nil {
		t.Fatalf("compileManifestGate() = %v", err)
	}
	if !compiledGateMatches(gate, "ready\n") {
		t.Fatal("expected line regex gate to match ready line")
	}
	if compiledGateMatches(gate, "not ready yet") {
		t.Fatal("expected line regex gate to reject non-matching line")
	}
}

func TestCompileManifestNotGateCompilesNestedAny(t *testing.T) {
	gate, err := compileManifestNotGate(manifestGate{
		Any: []manifestGate{
			{Contains: []string{"busy"}},
			{Contains: []string{"wait"}},
		},
	}, 0, &manifestComplexity{})
	if err != nil {
		t.Fatalf("compileManifestNotGate() = %v", err)
	}
	if !compiledGateMatches(gate, "please wait") {
		t.Fatal("expected nested any not gate to match wait text")
	}
	if compiledGateMatches(gate, "all clear") {
		t.Fatal("expected nested any not gate to reject unrelated text")
	}
}

func TestCompileManifestRegexNormalizesHexEscape(t *testing.T) {
	re, err := compileManifestRegex(`\x{41}`, "rule", "regex")
	if err != nil {
		t.Fatalf("compileManifestRegex() = %v", err)
	}
	if !re.MatchString("A") {
		t.Fatal("expected braced hex escape to compile to letter A")
	}
}

func TestCompileManifestGateRejectsEmptyNotBranch(t *testing.T) {
	complexity := &manifestComplexity{}
	_, err := compileManifestGate(manifestGate{
		Contains: []string{"ready"},
		Not:      []manifestGate{{}},
	}, "rule", 0, complexity)
	if err == nil {
		t.Fatal("expected empty not branch error")
	}
	if !strings.Contains(err.Error(), "empty not gate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileManifestWithNotRuleFixture(t *testing.T) {
	const fixture = `
id = "test-agent"
version = "1.0.0"

[[rules]]
id = "idle_when_ready"
state = "idle"
contains = ["ready"]
not = [{ contains = ["busy"] }]
`
	parsed, err := parseLocalManifest(fixture)
	if err != nil {
		t.Fatalf("parseLocalManifest() = %v", err)
	}
	compiled, err := compileManifest(parsed)
	if err != nil {
		t.Fatalf("compileManifest() = %v", err)
	}
	if len(compiled.rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(compiled.rules))
	}
	gate := compiled.rules[0].gate
	if !compiledGateMatches(gate, "system ready") {
		t.Fatal("expected ready text without busy to match")
	}
	if compiledGateMatches(gate, "system ready but busy") {
		t.Fatal("expected busy exclusion to reject match")
	}
}

func gateWithContains(count int, value string) manifestGate {
	contains := make([]string, count)
	for i := range contains {
		contains[i] = value
	}
	return manifestGate{Contains: contains}
}

func deepNotManifestGate(levels int) manifestGate {
	if levels <= 1 {
		return manifestGate{Contains: []string{"hit"}}
	}
	return manifestGate{Not: []manifestGate{deepNotManifestGate(levels - 1)}}
}

func deepManifestGate(levels int) manifestGate {
	if levels <= 1 {
		return manifestGate{Contains: []string{"hit"}}
	}
	return manifestGate{All: []manifestGate{deepManifestGate(levels - 1)}}
}

func TestFallbackManifestEntryPrefersRemote(t *testing.T) {
	valid := agentManifest{
		ID:      "codex",
		Version: "1.0.0",
		Rules:   []manifestRule{{ID: "idle", State: "idle", Contains: []string{"ready"}}},
	}
	remote, err := compileManifestEntry(
		valid,
		ManifestSource{Kind: ManifestSourceRemote, Path: "remote"},
		"",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := fallbackManifestEntry(
		valid,
		remote,
		"1.0.0",
		false,
		"remote warning",
	)
	if err != nil || entry.info.Warning != "remote warning" {
		t.Fatalf("fallbackManifestEntry(remote) = (%#v, %v)", entry, err)
	}

	bundledEntry, err := fallbackManifestEntry(
		valid,
		nil,
		"",
		false,
		"bundled warning",
	)
	if err != nil || bundledEntry.info.Version == "" {
		t.Fatalf("fallbackManifestEntry(bundled) = (%#v, %v)", bundledEntry, err)
	}
}

func TestBundledManifestEntryWithWarningPanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid bundled manifest")
		}
	}()
	bundledManifestEntryWithWarning(agentManifest{ID: "codex"}, "bad", "")
}
