package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type testCompletionProvider struct {
	panes, dirs, agents, sources, deletes []completionCandidate
	delay                                 time.Duration
	err                                   error
}

func (p testCompletionProvider) result(
	ctx context.Context,
	values []completionCandidate,
) ([]completionCandidate, error) {
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return values, p.err
}

func (p testCompletionProvider) Panes(ctx context.Context) ([]completionCandidate, error) {
	return p.result(ctx, p.panes)
}

func (p testCompletionProvider) Directories(ctx context.Context) ([]completionCandidate, error) {
	return p.result(ctx, p.dirs)
}

func (p testCompletionProvider) Agents(ctx context.Context) ([]completionCandidate, error) {
	return p.result(ctx, p.agents)
}

func (p testCompletionProvider) Sources(ctx context.Context) ([]completionCandidate, error) {
	return p.result(ctx, p.sources)
}

func (p testCompletionProvider) DeleteTargets(ctx context.Context) ([]completionCandidate, error) {
	return p.result(ctx, p.deletes)
}

func completionOutput(t *testing.T, provider completionProvider, args ...string) string {
	t.Helper()
	old := activeCompletionProvider
	activeCompletionProvider = provider
	t.Cleanup(func() { activeCompletionProvider = old })
	out, err := captureStdout(t, func() error { return run(args) })
	if err != nil {
		t.Fatalf("run(%q) error = %v", args, err)
	}
	return out
}

func TestCompletionRootOrderAndPseudoCommandTranslation(t *testing.T) {
	out := completionOutput(t, testCompletionProvider{}, cobra.ShellCompRequestCmd, "")
	for _, value := range []string{
		"completion\t", "config\t", "diagnostics\t", "help\t", "integration\t",
		"keybind\t", "version\t", "--get-all\t", "--report-agent\t",
		"--release-agent\t", "--delete-item\t", "--version\t", "--ephemeral\t",
	} {
		if !strings.Contains(out, value) {
			t.Errorf("root completion missing %q:\n%s", value, out)
		}
	}
	helpLines := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "help\t") {
			helpLines++
		}
	}
	if helpLines != 1 {
		t.Fatalf("help candidate count = %d, want 1:\n%s", helpLines, out)
	}
	if strings.Index(out, "config\t") > strings.Index(out, "--get-all\t") ||
		strings.Index(out, "--get-all\t") > strings.Index(out, "--ephemeral\t") {
		t.Fatalf("root ordering is not subcommands, pseudo commands, flags:\n%s", out)
	}

	out = completionOutput(
		t,
		testCompletionProvider{},
		cobra.ShellCompRequestCmd,
		"--report-agent",
		"",
	)
	for _, flag := range []string{"--agent\t", "--cwd\t", "--json\t", "--pane\t", "--state\t"} {
		if !strings.Contains(out, flag) {
			t.Errorf("report completion missing %q:\n%s", flag, out)
		}
	}
	if strings.Contains(out, "--ephemeral\t") {
		t.Fatalf("root flag leaked into report command:\n%s", out)
	}

	out = completionOutput(t, testCompletionProvider{}, cobra.ShellCompRequestCmd, "--g")
	if !strings.Contains(out, "--get-all\t") || strings.Contains(out, "config\t") {
		t.Fatalf("dash prefix did not narrow to canonical flags:\n%s", out)
	}

	deleteProvider := testCompletionProvider{deletes: []completionCandidate{{"target", "active"}}}
	before := completionOutput(t, deleteProvider, cobra.ShellCompRequestCmd, "--delete-item", "")
	invalid := completionOutput(
		t,
		deleteProvider,
		cobra.ShellCompRequestCmd,
		"--delete-item",
		"--j",
	)
	after := completionOutput(
		t,
		deleteProvider,
		cobra.ShellCompRequestCmd,
		"--delete-item",
		"target",
		"",
	)
	if !strings.Contains(before, "target\tactive") || strings.Contains(before, "--json") ||
		strings.Contains(invalid, "--json") || !strings.Contains(after, "--json") {
		t.Fatalf(
			"delete positional/flag ordering before=%q invalid=%q after=%q",
			before,
			invalid,
			after,
		)
	}
}

func TestCompletionTreeCanonicalMetadata(t *testing.T) {
	root := newCompletionRoot(testCompletionProvider{}, "")
	if strings.Join(keybindTargets, ",") != "tmux,herdr" {
		t.Fatalf("operational keybind targets = %q", keybindTargets)
	}
	paths := []string{
		"completion bash", "completion zsh", "completion fish",
		"config", "config path", "config show", "config init", "diagnostics",
		"integration install", "integration uninstall", "keybind install tmux",
		"keybind install herdr", "keybind uninstall tmux", "keybind uninstall herdr",
		"version", "help", "__get_all", "__get_sessions", "__get_zoxide", "__get_fd",
		"__get_agents", "__get_current_session_agents", "__report_agent", "__release_agent",
		"__delete_item",
	}
	for _, path := range paths {
		command, _, err := root.Find(strings.Fields(path))
		if err != nil || command == root {
			t.Errorf("completion tree missing %q: command=%q err=%v", path, command.Name(), err)
		}
	}

	expectedIntegrations := integrationsForTest()
	if fmt.Sprint(integrations.Available()) != fmt.Sprint(expectedIntegrations) {
		t.Fatalf(
			"operational integrations = %q, want %q",
			integrations.Available(),
			expectedIntegrations,
		)
	}
	for _, action := range []string{"install", "uninstall"} {
		command, _, _ := root.Find([]string{"integration", action})
		if command.Use != action+" <name>" {
			t.Errorf("integration positional metadata = %q", command.Use)
		}
		got, _ := command.ValidArgsFunction(command, nil, "")
		if len(got) != len(expectedIntegrations) {
			t.Errorf("integration %s candidates = %q", action, got)
		}
		for _, name := range expectedIntegrations {
			if !candidateExists(got, name) {
				t.Errorf("integration %s missing %q: %q", action, name, got)
			}
			if parsed, err := parseIntegrationCommand([]string{action, name}); err != nil ||
				parsed != (integrationCommand{action, name}) {
				t.Errorf("operational integration %s/%s = %#v, %v", action, name, parsed, err)
			}
		}
	}
	deleteCommand, _, _ := root.Find([]string{"__delete_item"})
	if !strings.HasSuffix(deleteCommand.Use, " <target>") {
		t.Errorf("delete positional metadata = %q", deleteCommand.Use)
	}
	for _, tc := range []struct {
		path []string
		want []string
	}{
		{[]string{"keybind", "install", "tmux"}, []string{"popup", "window", "pane", "pane-zoomed"}},
		{[]string{"keybind", "install", "herdr"}, []string{"pane", "popup"}},
	} {
		if tc.path[2] == "tmux" &&
			strings.Join(tmuxModeStrings(), ",") != strings.Join(tc.want, ",") {
			t.Errorf("operational tmux modes = %q, want %q", tmuxModeStrings(), tc.want)
		}
		if tc.path[2] == "herdr" &&
			strings.Join(herdrModeStrings(), ",") != strings.Join(tc.want, ",") {
			t.Errorf("operational herdr modes = %q, want %q", herdrModeStrings(), tc.want)
		}
		command, _, _ := root.Find(tc.path)
		complete, ok := command.GetFlagCompletionFunc("mode")
		if !ok {
			t.Fatalf("%v mode completion missing", tc.path)
		}
		got, _ := complete(command, nil, "")
		if len(got) != len(tc.want) {
			t.Errorf("%v mode candidates = %q", tc.path, got)
		}
		for _, value := range tc.want {
			if !candidateExists(got, value) {
				t.Errorf("%v mode missing %q: %q", tc.path, value, got)
			}
			if tc.path[2] == "tmux" {
				if _, err := parseTmuxLaunchMode(value); err != nil {
					t.Errorf("operational tmux mode %q: %v", value, err)
				}
			} else if _, err := parseHerdrLaunchMode(value); err != nil {
				t.Errorf("operational herdr mode %q: %v", value, err)
			}
		}
	}
}

func TestCompletionFlagAndRegistrySnapshots(t *testing.T) {
	root := newCompletionRoot(testCompletionProvider{}, "")
	tests := []struct {
		path string
		want []string
	}{
		{"", []string{"ephemeral"}},
		{
			"__report_agent",
			[]string{
				"agent",
				"cwd",
				"json",
				"message",
				"pane",
				"seq",
				"session-id",
				"source",
				"state",
			},
		},
		{"__release_agent", []string{"cwd", "json", "pane", "seq", "source"}},
		{"__delete_item", []string{"json"}},
		{"config", []string{"json"}},
		{"config path", []string{"json"}},
		{"config show", []string{"json"}},
		{"config init", []string{"force", "json"}},
		{"diagnostics", []string{"json"}},
		{"version", []string{"json"}},
		{"keybind install tmux", []string{"key", "mode", "persistent"}},
		{"keybind install herdr", []string{"height", "key", "mode", "persistent", "width"}},
	}
	expectedGets := []getCommand{
		{"--get-all", sessionmgr.ModeAll, "print all configured sources"},
		{"--get-sessions", sessionmgr.ModeSessions, "print sessions or workspaces"},
		{"--get-zoxide", sessionmgr.ModeZoxide, "print zoxide directories"},
		{"--get-fd", sessionmgr.ModeFD, "print fd directories"},
		{"--get-agents", sessionmgr.ModeAgents, "print agent panes"},
		{
			"--get-current-session-agents",
			sessionmgr.ModeCurrentAgents,
			"print current-session agents",
		},
	}
	if fmt.Sprint(getCommands) != fmt.Sprint(expectedGets) {
		t.Fatalf("operational get registry = %#v, want %#v", getCommands, expectedGets)
	}
	for _, get := range expectedGets {
		tests = append(tests, struct {
			path string
			want []string
		}{pseudoName(get.Name), []string{"json"}})
		parsed, err := parseOperationalCommand([]string{get.Name, "--json"})
		if err != nil || parsed.mode != get.Mode || !parsed.jsonOutput {
			t.Errorf("operational get route %s = %#v, err=%v", get.Name, parsed, err)
		}
	}
	for _, tc := range tests {
		command, _, err := root.Find(strings.Fields(tc.path))
		if err != nil {
			t.Fatalf("find %q: %v", tc.path, err)
		}
		var got []string
		command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name != "help" {
				got = append(got, flag.Name)
			}
		})
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s flags = %v, want %v", tc.path, got, tc.want)
		}
	}
	if _, err := parseReportAgentCommand([]string{
		"--pane", "%1", "--cwd", "/tmp", "--agent", "pi", "--state", "working",
		"--source", "custom", "--seq", "1", "--message", "status",
		"--session-id", "session", "--json",
	}); err != nil {
		t.Errorf("operational report FlagSet parity: %v", err)
	}
	if _, err := parseReleaseAgentCommand([]string{
		"--pane", "%1", "--cwd", "/tmp", "--source", "custom", "--seq", "1", "--json",
	}); err != nil {
		t.Errorf("operational release FlagSet parity: %v", err)
	}
	report, _, _ := root.Find([]string{"__report_agent"})
	complete, _ := report.GetFlagCompletionFunc("state")
	states, _ := complete(report, nil, "")
	expectedStates := []string{"idle", "working", "blocked", "done", "unknown"}
	if fmt.Sprint(sessionmgr.AgentStates()) != fmt.Sprint(expectedStates) {
		t.Errorf("operational agent states = %q, want %q", sessionmgr.AgentStates(), expectedStates)
	}
	if len(states) != len(expectedStates) {
		t.Errorf("report state completions = %q", states)
	}
	for _, state := range expectedStates {
		if !candidateExists(states, state) {
			t.Errorf("report state completion missing %q: %q", state, states)
		}
	}
}

func integrationsForTest() []string {
	return []string{"pi", "codex", "claude", "droid", "opencode"}
}

func candidateExists(candidates []string, value string) bool {
	for _, candidate := range candidates {
		if strings.SplitN(candidate, "\t", 2)[0] == value {
			return true
		}
	}
	return false
}

func TestCompletionDynamicSanitizationLiteralValuesAndTimeout(t *testing.T) {
	hostile := "value with spaces;'quoted';*.go;$(touch /tmp/never)"
	provider := testCompletionProvider{panes: []completionCandidate{
		{Value: hostile, Description: "literal shell syntax"},
		{Value: hostile, Description: "duplicate"},
		{Value: "control\nvalue", Description: "rejected"},
		{Value: strings.Repeat("x", completionMaxValueBytes+1), Description: "too long"},
		{Value: "valid", Description: "bad\tdescription"},
	}}
	out := completionOutput(t, provider, cobra.ShellCompRequestCmd, "--report-agent", "--pane", "")
	if !strings.Contains(out, hostile+"\tliteral shell syntax") ||
		strings.Count(out, hostile) != 1 {
		t.Fatalf("literal/dedupe completion mismatch:\n%s", out)
	}
	if strings.Contains(out, "control") || strings.Contains(out, "too long") ||
		!strings.Contains(out, "valid\n") {
		t.Fatalf("sanitization mismatch:\n%s", out)
	}

	started := time.Now()
	out = completionOutput(
		t,
		testCompletionProvider{delay: time.Second},
		cobra.ShellCompRequestCmd,
		"--report-agent",
		"--pane",
		"",
	)
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("completion latency = %v, want <200ms", elapsed)
	}
	if !strings.HasPrefix(out, ":") {
		t.Fatalf("timeout did not fall back silently: %q", out)
	}

	many := make([]completionCandidate, 200)
	for i := range many {
		many[i] = completionCandidate{Value: fmt.Sprintf("a-%03d", i)}
	}
	many[199] = completionCandidate{Value: "z-match", Description: "after cap"}
	root := newCompletionRoot(testCompletionProvider{panes: many, agents: many}, "")
	report, _, _ := root.Find([]string{pseudoName("--report-agent")})
	paneComplete, _ := report.GetFlagCompletionFunc("pane")
	got, _ := paneComplete(report, nil, "z")
	if len(got) != 1 || got[0] != "z-match\tafter cap" {
		t.Fatalf("prefix filtering before cap = %q", got)
	}
	agentComplete, _ := report.GetFlagCompletionFunc("agent")
	got, _ = agentComplete(report, nil, "")
	if len(got) != completionMaxCandidates || got[0] != "pi\tagent integration" {
		t.Fatalf("fixed candidates did not survive saturation: first=%q count=%d", got[0], len(got))
	}

	timeoutRoot := newCompletionRoot(testCompletionProvider{delay: time.Second}, "")
	timeoutReport, _, _ := timeoutRoot.Find([]string{pseudoName("--report-agent")})
	timeoutAgent, _ := timeoutReport.GetFlagCompletionFunc("agent")
	got, _ = timeoutAgent(timeoutReport, nil, "")
	if len(got) != len(integrationsForTest()) {
		t.Fatalf("fixed timeout fallback = %q", got)
	}
}

func TestProductionProvidersMergeObservedAndFixedCandidates(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	t.Setenv("TMUX", "active")
	t.Run("observed", func(t *testing.T) {
		sessionmgr.SetTmuxHooksForTest(t, func(context.Context, ...string) ([]byte, error) {
			return []byte("%1\x1fdev\x1f/tmp/dev\x1fzsh\x1fcustom-agent\x1fcustom:source"), nil
		}, nil)
		root := newCompletionRoot(productionCompletionProvider{}, "")
		report, _, _ := root.Find([]string{pseudoName("--report-agent")})
		agentComplete, _ := report.GetFlagCompletionFunc("agent")
		sourceComplete, _ := report.GetFlagCompletionFunc("source")
		agents, _ := agentComplete(report, nil, "")
		sources, _ := sourceComplete(report, nil, "")
		if agents[0] != "pi\tagent integration" || !candidateExists(agents, "custom-agent") ||
			sources[0] != "seshagy:pi\tintegration report source" ||
			!candidateExists(sources, "custom:source") {
			t.Fatalf("agents=%#v sources=%#v", agents, sources)
		}
	})
	t.Run("backend error", func(t *testing.T) {
		sessionmgr.SetTmuxHooksForTest(t, func(context.Context, ...string) ([]byte, error) {
			return nil, fmt.Errorf("backend unavailable")
		}, nil)
		root := newCompletionRoot(productionCompletionProvider{}, "")
		report, _, _ := root.Find([]string{pseudoName("--report-agent")})
		agentComplete, _ := report.GetFlagCompletionFunc("agent")
		sourceComplete, _ := report.GetFlagCompletionFunc("source")
		agents, _ := agentComplete(report, nil, "")
		sources, _ := sourceComplete(report, nil, "")
		if len(agents) != len(integrationsForTest()) || len(sources) != len(integrationsForTest()) {
			t.Fatalf("fixed fallback agents=%#v sources=%#v", agents, sources)
		}
	})
}

func TestDirectoryCompletionUsesDynamicCandidatesOrFilterDirs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider completionProvider
		want     cobra.ShellCompDirective
	}{
		{"available", testCompletionProvider{dirs: []completionCandidate{{"/tmp/project/", "pane cwd"}}}, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveKeepOrder},
		{"failed", testCompletionProvider{err: fmt.Errorf("unavailable")}, cobra.ShellCompDirectiveFilterDirs},
		{"unusable", testCompletionProvider{dirs: []completionCandidate{{"bad\npath", "invalid"}}}, cobra.ShellCompDirectiveFilterDirs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newCompletionRoot(tc.provider, "")
			report, _, _ := root.Find([]string{pseudoName("--report-agent")})
			complete, _ := report.GetFlagCompletionFunc("cwd")
			got, directive := complete(report, nil, "")
			if directive != tc.want || (tc.name == "available" &&
				(len(got) != 1 || got[0] != "/tmp/project/\tpane cwd")) {
				t.Fatalf("candidates=%q directive=%d, want=%d", got, directive, tc.want)
			}
		})
	}
}

func TestHerdrDeleteProviderIncludesEmptyLabeledWorkspaces(t *testing.T) {
	dir := t.TempDir()
	herdr := filepath.Join(dir, "herdr")
	body := `#!/bin/sh
printf '%s\n' '{"type":"workspace_list","workspaces":[{"workspace_id":"opaque:one","label":"One"},{"workspace_id":"opaque:empty","label":"Empty workspace","pane_count":0}]}'
`
	if err := os.WriteFile(herdr, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("TMUX", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := (productionCompletionProvider{}).DeleteTargets(context.Background())
	if err != nil || len(got) != 2 ||
		got[1] != (completionCandidate{"opaque:empty", "Empty workspace"}) {
		t.Fatalf("delete targets=%#v err=%v", got, err)
	}
}

func TestCompletionGeneratedScriptsArePinnedSafeAndGrouped(t *testing.T) {
	root := newCompletionRoot(testCompletionProvider{}, "")
	generators := []struct {
		name string
		gen  func(*cobra.Command, *bytes.Buffer) error
	}{
		{
			"bash",
			func(root *cobra.Command, out *bytes.Buffer) error {
				return generateAndPatch(
					out,
					func(w io.Writer) error { return root.GenBashCompletionV2(w, true) },
					completionPatch{bashEndpointMarker, bashEndpointReplacement},
				)
			},
		},
		{
			"zsh",
			func(root *cobra.Command, out *bytes.Buffer) error {
				return generateAndPatch(out, root.GenZshCompletion,
					completionPatch{zshEndpointMarker, zshEndpointReplacement},
					completionPatch{zshDescribeMarker, zshGroupedReplacement})
			},
		},
		{
			"fish",
			func(root *cobra.Command, out *bytes.Buffer) error {
				return generateAndPatch(
					out,
					func(w io.Writer) error { return root.GenFishCompletion(w, true) },
					completionPatch{fishEndpointMarker, fishEndpointReplacement},
				)
			},
		},
	}
	for _, tc := range generators {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.gen(root, &out); err != nil {
				t.Fatalf("generate: %v", err)
			}
			script := out.String()
			for _, unsafe := range []string{"eval ${requestComp}", "eval \"${requestComp}\"", "eval $requestComp"} {
				if strings.Contains(script, unsafe) {
					t.Fatalf("generated script retained executable eval: %q", unsafe)
				}
			}
			if !strings.Contains(script, "__complete") || !strings.Contains(script, "seshagy") {
				t.Fatal("generated script lost endpoint or registration")
			}
			if tc.name == "zsh" {
				subcommands := strings.Index(script, "-t subcommands")
				flags := strings.Index(script, "-t flags")
				if subcommands < 0 || flags < subcommands ||
					strings.Contains(script, zshDescribeMarker) {
					t.Fatal("zsh named groups missing or out of order")
				}
			}
		})
	}
	badPatches := []struct {
		input string
		patch completionPatch
	}{
		{"no marker", completionPatch{zshDescribeMarker, zshGroupedReplacement}},
		{
			zshDescribeMarker + zshDescribeMarker,
			completionPatch{zshDescribeMarker, zshGroupedReplacement},
		},
		{bashEndpointMarker, completionPatch{bashEndpointMarker, bashEndpointReplacement}},
	}
	for _, tc := range badPatches {
		var out bytes.Buffer
		err := generateAndPatch(&out, func(w io.Writer) error {
			_, writeErr := io.WriteString(w, tc.input)
			return writeErr
		}, tc.patch)
		if err == nil || out.Len() != 0 {
			t.Fatalf(
				"generateAndPatch was not fail-closed for %q: err=%v output=%q",
				tc.input,
				err,
				out.String(),
			)
		}
	}
}

func TestGeneratedBashWorksStandaloneAndDoesNotExecuteInput(t *testing.T) {
	bash := "/bin/bash"
	if _, err := os.Stat(bash); err != nil {
		t.Skip("/bin/bash unavailable")
	}
	root := newCompletionRoot(testCompletionProvider{}, "")
	var generated bytes.Buffer
	if err := genSafeBashCompletion(root, &generated); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	completionPath := filepath.Join(dir, "completion.bash")
	if err := os.WriteFile(completionPath, generated.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "executed")
	endpoint := filepath.Join(dir, "seshagy")
	endpointBody := `#!/bin/sh
if [ "$1" != __complete ]; then printf 'ARGV'; for arg do printf '<%s>' "$arg"; done; printf '\n'; exit; fi
last=
for last do :; done
case "$last" in
  *filter*) printf '%s\n' ':16' ;;
  *nofile*) printf '%s\n' ':4' ;;
  *nospace*) printf '%b\n' 'nospace-token\tnospace' ':38' ;;
  *candidate*) printf '%b\n' 'candidate with space\tdynamic' ':36' ;;
  *hostile*) printf '%b\n' "$HOSTILE_VALUE\thostile" ':36' ;;
  *ordinary*) printf '%b\n' 'ordinary-value\tordinary' ':36' ;;
  *files*) printf '%s\n' ':0' ;;
  *dynamic*) printf '%b\n' "$TEST_DIR/dynamic/\tdirectory" ':38' ;;
  *) printf '%b\n' 'value with space\tdescription' 'config\tmanage configuration' '--help\thelp' ':36' ;;
esac
`
	if err := os.WriteFile(endpoint, []byte(endpointBody), 0o700); err != nil {
		t.Fatal(err)
	}
	script := "source " + shellQuote(completionPath) + "; " +
		"COMP_WORDS=(seshagy '$(touch " + marker + ")'); COMP_CWORD=1; " +
		"COMP_LINE=\"seshagy \\$(touch x)\"; __start_seshagy; printf '<%s>\\n' \"${COMPREPLY[@]}\""
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", script)
	cmd.Env = append(
		os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TEST_DIR="+dir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash completion failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unfinished input executed; stat error=%v", err)
	}
	for _, name := range []string{"filter-dir", "dynamic", "files nested", "files nested/dir two"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	weirdFile := "weird 'quote' *.go $HOME"
	for _, name := range []string{"filter-file", "files nested/file one", "files nested/dir two/final file", "files nested/" + weirdFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rootScript := "source " + shellQuote(
		completionPath,
	) + "; printf 'BASH_MAJOR=%s\\n' \"${BASH_VERSINFO[0]}\"; complete -p seshagy; " +
		"run(){ COMP_WORDS=(seshagy \"$1\"); COMP_CWORD=1; COMP_LINE=\"seshagy $1\"; __start_seshagy; printf \"$2 count=%s\\n\" \"${#COMPREPLY[@]}\"; printf \"$2<%s>\\n\" \"${COMPREPLY[@]}\"; }; " +
		"run '' ordinary:; run '" + filepath.Join(
		dir,
		"filter",
	) + "' filter:; " +
		"run nofile nofile:; run nospace nospace:; " +
		"run '" + filepath.Join(
		dir,
		"files nested",
		"f",
	) + "' files:; " +
		"run '" + filepath.Join(
		dir,
		"files nested",
		"d",
	) + "' filedir:; " +
		"run '" + filepath.Join(
		dir,
		"dynamic",
	) + "' dynamic:"
	cmd = exec.Command(bash, "--noprofile", "--norc", "-c", rootScript)
	cmd.Env = append(
		os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TEST_DIR="+dir,
	)
	out, err = cmd.CombinedOutput()
	text := string(out)
	escape := func(value string) string { return strings.ReplaceAll(value, " ", "\\ ") }
	if err != nil || strings.Contains(text, "-o default") {
		t.Fatalf("standalone Bash directive behavior failed: %v\n%s", err, out)
	}
	if strings.Contains(text, "BASH_MAJOR=3") {
		if strings.Contains(text, "-o filenames") ||
			!strings.Contains(text, "complete -o nospace") ||
			!strings.Contains(text, "ordinary:<value\\ with\\ space >") ||
			!strings.Contains(text, "filter:<"+filepath.Join(dir, "filter-dir")+"/>") ||
			strings.Contains(text, "filter:<"+filepath.Join(dir, "filter-file")+">") ||
			!strings.Contains(text, "nofile: count=0") ||
			!strings.Contains(text, "nospace:<nospace-token>") ||
			!strings.Contains(
				text,
				"files:<"+escape(filepath.Join(dir, "files nested", "file one"))+" >",
			) ||
			!strings.Contains(
				text,
				"filedir:<"+escape(filepath.Join(dir, "files nested", "dir two"))+"/>",
			) ||
			!strings.Contains(text, "dynamic:<"+filepath.Join(dir, "dynamic")+"/>") {
			t.Fatalf("standalone Bash 3 directive behavior failed:\n%s", out)
		}
	} else if !strings.Contains(text, "complete -o filenames") {
		t.Fatalf("modern Bash registration lost filenames:\n%s", out)
	}

	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the Bash PTY regression")
	}
	harness := `zmodload zsh/zpty
zmodload zsh/zselect
zpty child "$BASH_UNDER_TEST" --noprofile --norc -i
read_until() {
  local expected=$1 i chunk
  REPLY=''
  for i in {1..500}; do
    if zpty -r -t child chunk; then REPLY+=$chunk; [[ $REPLY == *"$expected"* ]] && return 0; else zselect -t 2; fi
  done
  print -u2 -- "timeout waiting ${(qqq)expected}: ${(qqq)REPLY}"
  return 1
}
zpty -w child $'stty -echo; PS1=""; source "$COMPLETION_SCRIPT"; [[ -z "$BIND_COMMAND" ]] || bind "$BIND_COMMAND"; echo READY\n'
read_until READY || exit 1
for ((pair = 1; pair <= PAIR_COUNT; pair++)); do
  command_var=CMD$pair; want_var=WANT$pair
  zpty -w -n child "${(P)command_var}"$'\n'
  read_until "${(P)want_var}" || exit 1
  print -r -- "$REPLY"
done
zpty -d child
`
	harnessPath := filepath.Join(dir, "bash-pty.zsh")
	if err := os.WriteFile(harnessPath, []byte(harness), 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "files nested", "file one")
	dirPrefix := filepath.Join(dir, "files nested", "d")
	nested := filepath.Join(dir, "files nested", "dir two", "final file")
	hostileMarker := filepath.Join(dir, "bash-executed")
	hostile := "hostile;'quoted';*.go;$(touch " + hostileMarker + ")"
	bashes := []string{bash}
	if modernBash := "/opt/homebrew/bin/bash"; modernBash != bash {
		if _, err := os.Stat(modernBash); err == nil {
			bashes = append(bashes, modernBash)
		}
	}
	for _, bashUnderTest := range bashes {
		baseEnv := append(
			os.Environ(),
			"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"COMPLETION_SCRIPT="+completionPath,
			"BASH_UNDER_TEST="+bashUnderTest,
			"CMD1=seshagy candidate\t",
			"WANT1=ARGV<candidate with space>",
			"CMD2=seshagy "+escape(
				filepath.Join(dir, "files nested", "f"),
			)+"\t",
			"WANT2=ARGV<"+file+">",
			"CMD3=seshagy "+escape(dirPrefix)+"\tf\t",
			"WANT3=ARGV<"+nested+">",
			"CMD4=seshagy nospace\tsuffix",
			"WANT4=ARGV<nospace-tokensuffix>",
			"CMD5=seshagy ordinary\ttail",
			"WANT5=ARGV<ordinary-value><tail>",
			"CMD6=seshagy hostile\t",
			"WANT6=ARGV<"+hostile+">",
			"HOSTILE_VALUE="+hostile,
			"CMD7=seshagy "+escape(filepath.Join(dir, "files nested", "w"))+"\t",
			"WANT7=ARGV<"+filepath.Join(dir, "files nested", weirdFile)+">",
		)
		runPTY := func(name, bind, pairCount string) {
			t.Helper()
			cmd = exec.Command(zsh, harnessPath)
			cmd.Env = append([]string{}, baseEnv...)
			cmd.Env = append(cmd.Env, "BIND_COMMAND="+bind, "PAIR_COUNT="+pairCount)
			out, err := cmd.CombinedOutput()
			if err != nil || strings.Contains(string(out), `ARGV<candidate\`) {
				t.Fatalf(
					"Bash Readline PTY regression (%s %s): %v\n%q",
					bashUnderTest,
					name,
					err,
					out,
				)
			}
		}
		runPTY("ordinary", "", "7")
		runPTY("menu-complete", `"\C-i": menu-complete`, "1")
		if _, err := os.Stat(hostileMarker); !os.IsNotExist(err) {
			t.Fatalf("Bash hostile completion executed (%s): %v", bashUnderTest, err)
		}
	}
}

func TestGeneratedZshGroupOrderAndFishRegistration(t *testing.T) {
	root := newCompletionRoot(testCompletionProvider{}, "")
	dir := t.TempDir()
	endpoint := filepath.Join(dir, "seshagy")
	endpointBody := "#!/bin/sh\nif [ -n \"$ARG_LOG\" ]; then { printf 'CALL %s\\n' \"$#\"; for arg do printf '<%s>\\n' \"$arg\"; done; } >>\"$ARG_LOG\"; fi\nlast=; for last do :; done\ncase \"$last\" in *fishfilter*) printf '%s\\n' ':16'; exit;; esac\nprintf '%s\\n' 'value with space\tdescription' 'config\tconfig command' '--help\thelp flag' ':36'\n"
	if err := os.WriteFile(endpoint, []byte(endpointBody), 0o700); err != nil {
		t.Fatal(err)
	}
	pathEnv := dir + string(os.PathListSeparator) + os.Getenv("PATH")

	if zsh, err := exec.LookPath("zsh"); err == nil {
		var generated bytes.Buffer
		if err := genSafeZshCompletion(root, &generated); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "_seshagy")
		if err := os.WriteFile(path, generated.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		groups := filepath.Join(dir, "groups")
		script := "compdef(){ :; }; source " + shellQuote(path) + "; " +
			"_describe(){ print -r -- \"$*\" >> " + shellQuote(groups) + "; return 0; }; " +
			"words=(seshagy ''); CURRENT=2; _seshagy"
		cmd := exec.Command(zsh, "-dfc", script)
		argLog := filepath.Join(dir, "zsh-argc")
		cmd.Env = append(os.Environ(), "PATH="+pathEnv, "ARG_LOG="+argLog)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("zsh completion failed: %v\n%s", err, out)
		}
		argc, err := os.ReadFile(argLog)
		if err != nil || string(argc) != "CALL 2\n<__complete>\n<>\n" {
			t.Fatalf("zsh endpoint argv = %q, err=%v", argc, err)
		}
		data, err := os.ReadFile(groups)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 2 || !strings.Contains(lines[0], "-t subcommands") ||
			!strings.Contains(lines[1], "-t flags") {
			t.Fatalf("zsh group calls = %q", lines)
		}
		if err := os.WriteFile(argLog, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(dir, "zsh-executed")
		hostile := "agent with space;$(touch " + marker + ")"
		probe := "compdef(){ :; }; source " + shellQuote(path) + "; _describe(){ return 0; }; " +
			"words=(seshagy integration install " + shellQuote(hostile) + " '' ignored); CURRENT=5; _seshagy; " +
			"words=(seshagy --report-agent --message " + shellQuote(hostile) + " --state wor --source ignored); CURRENT=6; _seshagy"
		cmd = exec.Command(zsh, "-dfc", probe)
		cmd.Env = append(os.Environ(), "PATH="+pathEnv, "ARG_LOG="+argLog)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("zsh argv probe failed: %v\n%s", err, out)
		}
		logged, err := os.ReadFile(argLog)
		want := "CALL 5\n<__complete>\n<integration>\n<install>\n<" + hostile + ">\n<>\n" +
			"CALL 6\n<__complete>\n<--report-agent>\n<--message>\n<" + hostile + ">\n<--state>\n<wor>\n"
		if err != nil || string(logged) != want {
			t.Fatalf("zsh preserved argv = %q, want %q, err=%v", logged, want, err)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("zsh unfinished input executed: %v", err)
		}
	} else {
		t.Log("zsh unavailable; group runtime skipped")
	}

	if fish, err := exec.LookPath("fish"); err == nil {
		if err := os.Mkdir(filepath.Join(dir, "fishfilter-dir"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fishfilter-file"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		var generated bytes.Buffer
		if err := genSafeFishCompletion(root, &generated); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "seshagy.fish")
		if err := os.WriteFile(path, generated.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(
			fish,
			"--no-config",
			"-c",
			"source "+shellQuote(path)+"; complete -C 'seshagy '",
		)
		cmd.Env = append(os.Environ(), "PATH="+pathEnv)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fish completion failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "value with space\tdescription") {
			t.Fatalf("fish lost literal candidate: %q", out)
		}
		prefix := filepath.Join(dir, "fishfilter")
		marker := filepath.Join(dir, "fish-executed")
		probe := "source " + shellQuote(
			path,
		) + "; complete -C " + shellQuote(
			"seshagy --report-agent --cwd "+prefix,
		) +
			"; complete -C " + shellQuote(
			"seshagy --report-agent --message $(touch "+marker+")",
		)
		cmd = exec.Command(fish, "--no-config", "-c", probe)
		cmd.Env = append(os.Environ(), "PATH="+pathEnv)
		out, err = cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "fishfilter-dir/") ||
			strings.Contains(string(out), "fishfilter-file") {
			t.Fatalf("fish FilterDirs failed: %v\n%s", err, out)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("fish unfinished input executed: %v", err)
		}
	} else {
		t.Log("fish unavailable; registration runtime skipped")
	}
}

func TestGeneratedShellSyntax(t *testing.T) {
	root := newCompletionRoot(testCompletionProvider{}, "")
	tests := []struct {
		name, binary string
		args         []string
		generate     func(*cobra.Command, *bytes.Buffer) error
	}{
		{
			"bash",
			"/bin/bash",
			[]string{"-n"},
			func(root *cobra.Command, out *bytes.Buffer) error { return genSafeBashCompletion(root, out) },
		},
		{
			"zsh",
			"zsh",
			[]string{"-n"},
			func(root *cobra.Command, out *bytes.Buffer) error { return genSafeZshCompletion(root, out) },
		},
		{
			"fish",
			"fish",
			[]string{"-n"},
			func(root *cobra.Command, out *bytes.Buffer) error { return genSafeFishCompletion(root, out) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binary, err := exec.LookPath(tc.binary)
			if err != nil {
				t.Skip(tc.binary + " unavailable")
			}
			var generated bytes.Buffer
			if err := tc.generate(root, &generated); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "completion."+tc.name)
			if err := os.WriteFile(path, generated.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			args := append(append([]string(nil), tc.args...), path)
			if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
				t.Fatalf("syntax check failed: %v\n%s", err, out)
			}
		})
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func TestCompletionLimitsStayWithinProtocolBudget(t *testing.T) {
	input := make([]completionCandidate, 200)
	for i := range input {
		input[i] = completionCandidate{
			Value:       "candidate-" + strconv.Itoa(i),
			Description: strings.Repeat("d", 700),
		}
	}
	got := encodeCandidates(input)
	if len(got) > completionMaxCandidates {
		t.Fatalf("candidate count = %d", len(got))
	}
	total := 0
	for _, candidate := range got {
		total += len(candidate) + 1
	}
	if total > completionMaxTotalBytes {
		t.Fatalf("candidate bytes = %d", total)
	}
}
