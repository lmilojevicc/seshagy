package completion

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

type fakeProvider struct {
	values map[ValueKind][]Candidate
	err    error
	block  bool
	calls  []ValueKind
}

type nonCooperativeProvider struct{}

type deadlineProvider struct {
	remaining chan time.Duration
}

func (nonCooperativeProvider) Values(context.Context, ValueKind) ([]Candidate, error) {
	select {}
}

func (p deadlineProvider) Values(ctx context.Context, _ ValueKind) ([]Candidate, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		p.remaining <- 0
		return nil, nil
	}
	p.remaining <- time.Until(deadline)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p *fakeProvider) Values(ctx context.Context, kind ValueKind) ([]Candidate, error) {
	p.calls = append(p.calls, kind)
	if p.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return p.values[kind], p.err
}

func candidateValues(result Result) []string {
	values := make([]string, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		values = append(values, candidate.Value)
	}
	sort.Strings(values)
	return values
}

func TestResolveCoversCommandContexts(t *testing.T) {
	provider := &fakeProvider{values: map[ValueKind][]Candidate{
		ValuePane:        {{Value: "%7"}},
		ValueDirectory:   {{Value: "/tmp/a b"}},
		ValueAgent:       {{Value: "pi"}},
		ValueSource:      {{Value: "seshagy:pi"}},
		ValueSessionLine: {{Value: "S demo"}},
	}}
	tests := []struct {
		name      string
		words     []string
		contains  string
		directive Directive
	}{
		{"root", []string{"seshagy", ""}, "config", NoFiles},
		{"machine hidden empty", []string{"seshagy", ""}, "config", NoFiles},
		{"machine by prefix", []string{"seshagy", "--r"}, "--report-agent", NoFiles},
		{"config", []string{"seshagy", "config", ""}, "init", NoFiles},
		{"config init", []string{"seshagy", "config", "init", "--"}, "--force", NoFiles},
		{"diagnostics", []string{"seshagy", "diagnostics", "--"}, "--json", NoFiles},
		{"integration", []string{"seshagy", "integration", "install", ""}, "pi", NoFiles},
		{"keybind target", []string{"seshagy", "keybind", "install", ""}, "herdr", NoFiles},
		{
			"tmux mode",
			[]string{"seshagy", "keybind", "install", "tmux", "--mode", "p"},
			"pane-zoomed",
			NoFiles,
		},
		{
			"herdr mode",
			[]string{"seshagy", "keybind", "install", "herdr", "--mode", ""},
			"popup",
			NoFiles,
		},
		{"shell", []string{"seshagy", "completion", ""}, "fish", NoFiles},
		{"state", []string{"seshagy", "--report-agent", "--state", ""}, "unknown", NoFiles},
		{"pane", []string{"seshagy", "--release-agent", "--pane", "%"}, "%7", NoFiles},
		{"cwd", []string{"seshagy", "--report-agent", "--cwd", "/"}, "/tmp/a b", Dirs},
		{"agent", []string{"seshagy", "--report-agent", "--agent", ""}, "pi", NoFiles},
		{"source", []string{"seshagy", "--release-agent", "--source", "s"}, "seshagy:pi", NoFiles},
		{"delete", []string{"seshagy", "--delete-item", ""}, "S demo", NoFiles},
		{"mid global", []string{"seshagy", "config", "--e"}, "--ephemeral", NoFiles},
		{"help alias", []string{"seshagy", "help", ""}, "", NoFiles},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Resolve(context.Background(), test.words, provider)
			if result.Directive != test.directive {
				t.Fatalf("directive = %q, want %q", result.Directive, test.directive)
			}
			values := candidateValues(result)
			found := test.contains == ""
			for _, value := range values {
				if value == test.contains {
					found = true
				}
			}
			if !found {
				t.Fatalf("candidates = %v, missing %q", values, test.contains)
			}
			if test.name == "machine hidden empty" {
				for _, value := range values {
					if value == "--report-agent" {
						t.Fatalf("machine command exposed in empty menu: %v", values)
					}
				}
			}
		})
	}
}

func TestResolveKeybindInstallCandidatesAreTargetAware(t *testing.T) {
	keybind := FindChild(&Root, "keybind")
	install := FindChild(keybind, "install")
	if install == nil || len(install.Positionals) == 0 {
		t.Fatal("keybind install target metadata missing")
	}
	mode := findFlag(install, "--mode")
	if mode == nil {
		t.Fatal("keybind install --mode metadata missing")
	}
	for _, target := range install.Positionals[0].Value.Values {
		t.Run(target, func(t *testing.T) {
			var wantFlags []string
			for _, flag := range install.Flags {
				if len(flag.Targets) == 0 || containsValue(flag.Targets, target) {
					wantFlags = append(wantFlags, flag.Name)
				}
			}
			wantFlags = append(wantFlags, "--ephemeral")
			sort.Strings(wantFlags)
			flags := candidateValues(Resolve(context.Background(), []string{
				"seshagy", "keybind", "install", target, "--",
			}, nil))
			if !equalStrings(flags, wantFlags) {
				t.Fatalf("flag candidates = %v, want %v", flags, wantFlags)
			}
			wantModes, declared := mode.Value.ValuesByTarget[target]
			if !declared {
				t.Fatalf("target %q has no mode metadata", target)
			}
			wantModes = append([]string(nil), wantModes...)
			sort.Strings(wantModes)
			modes := candidateValues(Resolve(context.Background(), []string{
				"seshagy", "keybind", "install", target, "--mode", "",
			}, nil))
			if !equalStrings(modes, wantModes) {
				t.Fatalf("mode candidates = %v, want %v", modes, wantModes)
			}
		})
	}
}

func TestResolveSuppressesUsedFlagsAndDoesNotCallProviderForStaticValues(t *testing.T) {
	provider := &fakeProvider{}
	result := Resolve(
		context.Background(),
		[]string{"seshagy", "config", "init", "--force", "--"},
		provider,
	)
	for _, value := range candidateValues(result) {
		if value == "--force" {
			t.Fatal("used --force was suggested")
		}
	}
	if len(provider.calls) != 0 {
		t.Fatalf("static request called provider: %v", provider.calls)
	}
}

func TestResolveRequiredPositionalsPrecedeFlags(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		want  []string
	}{
		{
			"keybind target only",
			[]string{"seshagy", "keybind", "install", ""},
			[]string{"herdr", "tmux"},
		},
		{"keybind flag prefix rejected", []string{"seshagy", "keybind", "install", "--"}, nil},
		{
			"keybind completed flag ordering rejected",
			[]string{"seshagy", "keybind", "install", "--persistent", ""},
			nil,
		},
		{
			"integration name before globals",
			[]string{"seshagy", "integration", "install", "--"},
			nil,
		},
		{"completion shell before globals", []string{"seshagy", "completion", "--"}, nil},
		{"delete line before json", []string{"seshagy", "--delete-item", "--json", ""}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := candidateValues(Resolve(context.Background(), test.words, nil))
			if !equalStrings(got, test.want) {
				t.Fatalf("candidates = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolveNormalizesPriorExactEphemeralBeforeValues(t *testing.T) {
	provider := &fakeProvider{values: map[ValueKind][]Candidate{
		ValuePane: {{Value: "%9"}},
	}}
	tests := []struct {
		name     string
		words    []string
		contains string
	}{
		{
			"message value",
			[]string{"seshagy", "--report-agent", "--message", "--ephemeral", ""},
			"",
		},
		{
			"keybind mode value",
			[]string{"seshagy", "keybind", "install", "tmux", "--mode", "--ephemeral", "p"},
			"pane",
		},
		{
			"before required target",
			[]string{"seshagy", "keybind", "install", "--ephemeral", ""},
			"tmux",
		},
		{
			"between command and integration name",
			[]string{"seshagy", "integration", "install", "--ephemeral", ""},
			"pi",
		},
		{
			"between flag and dynamic value",
			[]string{"seshagy", "--report-agent", "--pane", "--ephemeral", "%"},
			"%9",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := candidateValues(Resolve(context.Background(), test.words, provider))
			if test.contains == "" {
				if len(values) != 0 {
					t.Fatalf("candidates = %v, want text value context", values)
				}
				return
			}
			if !containsString(values, test.contains) {
				t.Fatalf("candidates = %v, missing %q", values, test.contains)
			}
		})
	}
}

func TestResolveNeverCompletesAttachedFlagValues(t *testing.T) {
	for _, words := range [][]string{
		{"seshagy", "keybind", "install", "tmux", "--mode=p"},
		{"seshagy", "--report-agent", "--state=w"},
		{"seshagy", "--report-agent", "--cwd=/t"},
	} {
		result := Resolve(context.Background(), words, &fakeProvider{})
		if len(result.Candidates) != 0 || result.Directive != NoFiles {
			t.Fatalf("Resolve(%v) = %#v, want no attached candidates", words, result)
		}
	}
}

func TestResolveProviderReceivesApproximatelyTwoHundredMillisecondDeadline(t *testing.T) {
	provider := deadlineProvider{remaining: make(chan time.Duration, 1)}
	started := time.Now()
	result := Resolve(
		context.Background(),
		[]string{"seshagy", "--report-agent", "--pane", ""},
		provider,
	)
	remaining := <-provider.remaining
	if remaining < 150*time.Millisecond || remaining > 250*time.Millisecond {
		t.Fatalf("provider deadline remaining = %v, want approximately 200ms", remaining)
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("wall-clock timeout exceeded loose scheduler ceiling: %v", elapsed)
	}
	if len(result.Candidates) != 0 || result.Directive != NoFiles {
		t.Fatalf("result = %#v, want silent nofiles", result)
	}
}

func TestResolveProviderFailureAndTimeoutAreSilent(t *testing.T) {
	providers := []Provider{
		&fakeProvider{err: errors.New("failed")},
		&fakeProvider{block: true},
		nonCooperativeProvider{},
	}
	for _, provider := range providers {
		started := time.Now()
		result := Resolve(
			context.Background(),
			[]string{"seshagy", "--report-agent", "--pane", ""},
			provider,
		)
		if len(result.Candidates) != 0 || result.Directive != NoFiles {
			t.Fatalf("result = %#v, want silent nofiles", result)
		}
		if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
			t.Fatalf("provider timeout was not bounded: %v", elapsed)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
