package completion

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestRuntimeProviderIncludesCanonicalAgentsAndSourcesWithoutBackend(t *testing.T) {
	provider := NewRuntimeProviderWithMux(sessionmgr.NewNoopBackend())
	agents, err := provider.Values(context.Background(), ValueAgent)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := provider.Values(context.Background(), ValueSource)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pi", "codex", "claude", "droid", "opencode"} {
		if !containsCandidate(agents, want) {
			t.Errorf("agents missing %q: %#v", want, agents)
		}
		if !containsCandidate(sources, "seshagy:"+want) {
			t.Errorf("sources missing %q: %#v", want, sources)
		}
	}
}

func TestRuntimeProviderHerdrSessionLinesRoundTripOpaqueTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	binDir := t.TempDir()
	herdr := filepath.Join(binDir, "herdr")
	payload := `{"type":"workspace_list","workspaces":[` +
		`{"workspace_id":"workspace:opaque-1","label":"duplicate label"},` +
		`{"workspace_id":"workspace:opaque-2","label":"duplicate label"}]}`
	if err := os.WriteFile(
		herdr,
		[]byte("#!/bin/sh\nprintf '%s\\n' '"+payload+"'\n"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	provider := NewRuntimeProviderWithMux(sessionmgr.NewHerdrBackend())
	candidates, err := provider.Values(context.Background(), ValueSessionLine)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want only two unambiguous raw targets", candidates)
	}
	for _, target := range []string{"workspace:opaque-1", "workspace:opaque-2"} {
		if !containsCandidateWithDescription(candidates, target, "duplicate label") {
			t.Errorf("candidates missing raw target %q with label: %#v", target, candidates)
		}
	}
}

func TestSessionLineCandidatesPreserveOpaqueTargetsAndDuplicateLabels(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "duplicate label", Target: "workspace:opaque-1"},
		{Kind: sessionmgr.KindSession, Name: "duplicate label", Target: "workspace:opaque-2"},
	}
	candidates := sessionLineCandidates(items, sessionmgr.DefaultIconSet(), "workspace")
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want two raw targets", candidates)
	}
	for _, item := range items {
		if !containsCandidateWithDescription(candidates, item.Target, item.Name) {
			t.Errorf("candidates missing raw target %q with label: %#v", item.Target, candidates)
		}
	}
	rendered := sessionmgr.StripANSI(
		sessionmgr.FormatLineWithIcons(items[0], sessionmgr.DefaultIconSet()),
	)
	if containsCandidate(candidates, rendered) {
		t.Errorf("candidates include ambiguous rendered line %q: %#v", rendered, candidates)
	}
}

func TestSessionLineCandidatesIncludePathLikeOpaqueTargetsWithIconsDisabled(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "duplicate", Target: "/"},
		{Kind: sessionmgr.KindSession, Name: "duplicate", Target: "~/"},
		{Kind: sessionmgr.KindSession, Name: "different label", Target: "./"},
		{Kind: sessionmgr.KindSession, Name: "id mismatch", Target: "../"},
	}
	icons := sessionmgr.DefaultIconSet()
	icons.Enabled = false
	candidates := sessionLineCandidates(items, icons, "workspace")
	for _, item := range items[:2] {
		if !containsCandidateWithDescription(candidates, item.Target, item.Name) {
			t.Errorf(
				"candidates missing collision-safe path-like ID %q: %#v",
				item.Target,
				candidates,
			)
		}
	}
	for _, item := range items[2:] {
		if !containsCandidateWithDescription(candidates, item.Name, item.Name) {
			t.Errorf("candidates missing unique friendly label %q: %#v", item.Name, candidates)
		}
		if containsCandidate(candidates, item.Target) {
			t.Errorf(
				"candidates include duplicate raw alternative %q: %#v",
				item.Target,
				candidates,
			)
		}
	}
	if containsCandidate(candidates, "duplicate") {
		t.Errorf("candidates include ambiguous rendered label: %#v", candidates)
	}
}

func TestSessionLineCandidatesAvoidRenderedValueThatResolvesAsAnotherRawTarget(t *testing.T) {
	items := []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "opaque-2", Target: "opaque-1"},
		{Kind: sessionmgr.KindSession, Name: "friendly", Target: "opaque-2"},
	}
	icons := sessionmgr.DefaultIconSet()
	icons.Enabled = false
	candidates := sessionLineCandidates(items, icons, "workspace")
	if !containsCandidateWithDescription(candidates, "opaque-1", "opaque-2") {
		t.Fatalf("collision did not fall back to first raw target: %#v", candidates)
	}
	if !containsCandidateWithDescription(candidates, "friendly", "friendly") {
		t.Fatalf("unique session lost friendly candidate: %#v", candidates)
	}
	if containsCandidateWithDescription(candidates, "opaque-2", "opaque-2") {
		t.Fatalf("unsafe rendered candidate shadows another raw target: %#v", candidates)
	}
}

func TestUniqueCandidatesDeduplicatesSortsAndFiltersPrefixes(t *testing.T) {
	values := []Candidate{
		{Value: "z"},
		{Value: "alpha", Description: "first"},
		{Value: "alpha"},
		{Value: ""},
	}
	got := filterCandidates(values, "a")
	if len(got) != 1 || got[0].Value != "alpha" || got[0].Description != "first" {
		t.Fatalf("filtered candidates = %#v", got)
	}
}

func containsCandidateWithDescription(candidates []Candidate, value, description string) bool {
	for _, candidate := range candidates {
		if candidate.Value == value && candidate.Description == description {
			return true
		}
	}
	return false
}

func containsCandidate(candidates []Candidate, want string) bool {
	for _, candidate := range candidates {
		if candidate.Value == want {
			return true
		}
	}
	return false
}
