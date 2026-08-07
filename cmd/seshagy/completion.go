package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/lmilojevicc/seshagy/internal/cli"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

const (
	completionTimeout       = 150 * time.Millisecond
	completionMaxCandidates = 128
	completionMaxValueBytes = 4 * 1024
	completionMaxDescBytes  = 1024
	completionMaxTotalBytes = 64 * 1024
)

type completionCandidate struct {
	Value, Description string
}

type completionProvider interface {
	Panes(context.Context) ([]completionCandidate, error)
	Directories(context.Context) ([]completionCandidate, error)
	Agents(context.Context) ([]completionCandidate, error)
	Sources(context.Context) ([]completionCandidate, error)
	DeleteTargets(context.Context) ([]completionCandidate, error)
}

type productionCompletionProvider struct{}

type paneCandidateFunc func(sessionmgr.CompletionPane) (completionCandidate, bool)

func paneCandidates(
	ctx context.Context,
	makeCandidate paneCandidateFunc,
) ([]completionCandidate, error) {
	snapshot, err := sessionmgr.ReadCompletionSnapshot(ctx, sessionmgr.Detect())
	if err != nil {
		return nil, err
	}
	out := make([]completionCandidate, 0, len(snapshot.Panes))
	for _, pane := range snapshot.Panes {
		if candidate, ok := makeCandidate(pane); ok {
			out = append(out, candidate)
		}
	}
	return out, nil
}

func (productionCompletionProvider) Panes(ctx context.Context) ([]completionCandidate, error) {
	return paneCandidates(ctx, func(pane sessionmgr.CompletionPane) (completionCandidate, bool) {
		description := "pane"
		if pane.Session != "" {
			description += " in " + pane.Session
		}
		return completionCandidate{pane.ID, description}, pane.ID != ""
	})
}

func (productionCompletionProvider) Directories(
	ctx context.Context,
) ([]completionCandidate, error) {
	return paneCandidates(ctx, func(pane sessionmgr.CompletionPane) (completionCandidate, bool) {
		value := pane.Cwd
		if value != "" && !strings.HasSuffix(value, string(filepath.Separator)) {
			value += string(filepath.Separator)
		}
		return completionCandidate{value, "pane working directory"}, value != ""
	})
}

func integrationCandidates(prefix, description string) []completionCandidate {
	out := make([]completionCandidate, 0, len(integrations.Available()))
	for _, name := range integrations.Available() {
		out = append(out, completionCandidate{prefix + name, description})
	}
	return out
}

func (productionCompletionProvider) Agents(ctx context.Context) ([]completionCandidate, error) {
	return paneCandidates(ctx, func(pane sessionmgr.CompletionPane) (completionCandidate, bool) {
		return completionCandidate{pane.Agent, "active agent"}, pane.Agent != ""
	})
}

func (productionCompletionProvider) Sources(ctx context.Context) ([]completionCandidate, error) {
	return paneCandidates(ctx, func(pane sessionmgr.CompletionPane) (completionCandidate, bool) {
		return completionCandidate{pane.Source, "active report source"}, pane.Source != ""
	})
}

func (productionCompletionProvider) DeleteTargets(
	ctx context.Context,
) ([]completionCandidate, error) {
	sessions, err := sessionmgr.ReadCompletionSessions(ctx, sessionmgr.Detect())
	if err != nil {
		return nil, err
	}
	out := make([]completionCandidate, 0, len(sessions))
	for _, session := range sessions {
		description := "active session or workspace"
		if session.Label != "" && session.Label != session.Target {
			description = session.Label
		}
		out = append(out, completionCandidate{session.Target, description})
	}
	return out, nil
}

var activeCompletionProvider completionProvider = productionCompletionProvider{}

var machineCommands = []completionCandidate{
	{"--report-agent", "report agent state"},
	{"--release-agent", "release agent state"},
	{"--delete-item", "delete an active session or workspace"},
	{"--version", "print version"},
}

func pseudoName(name string) string {
	return "__" + strings.ReplaceAll(strings.TrimPrefix(name, "--"), "-", "_")
}

func isPseudoCommand(name string) bool {
	if _, ok := getMode(name); ok {
		return true
	}
	for _, command := range machineCommands {
		if command.Value == name {
			return true
		}
	}
	return false
}

func runCompletion(args []string) (bool, error) {
	if len(args) == 0 || (args[0] != "completion" &&
		args[0] != cobra.ShellCompRequestCmd &&
		args[0] != cobra.ShellCompNoDescRequestCmd) {
		return false, nil
	}
	translated := append([]string(nil), args...)
	rootPrefix := ""
	if len(translated) > 1 {
		canonical := translated[1]
		if isPseudoCommand(canonical) {
			translated[1] = pseudoName(canonical)
			if canonical == "--delete-item" && len(translated) == 3 &&
				strings.HasPrefix(translated[2], "-") {
				translated[2] = "__target_required"
			}
		} else if len(translated) == 2 && strings.HasPrefix(translated[1], "-") {
			rootPrefix, translated[1] = translated[1], "__root_flags"
			translated = append(translated, "")
		}
	}
	root := newCompletionRoot(activeCompletionProvider, rootPrefix)
	root.SetArgs(translated)
	root.SetOut(cli.Default.StdoutWriter())
	root.SetErr(cli.Default.StderrWriter())
	return true, root.Execute()
}

func newCompletionRoot(provider completionProvider, rootPrefix string) *cobra.Command {
	root := &cobra.Command{
		Use:                   "seshagy",
		Short:                 "Agent-aware terminal dashboard",
		SilenceErrors:         true,
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.Flags().Bool("ephemeral", false, "exit the dashboard on focus loss")

	root.AddCommand(newCompletionCommand(root))
	root.AddCommand(newConfigCompletionCommand())
	root.AddCommand(leafCompletionCommand("diagnostics", "show diagnostic status", true))
	root.AddCommand(newIntegrationCompletionCommand())
	root.AddCommand(newKeybindCompletionCommand())
	versionCommand := leafCompletionCommand("version", "print version", true)
	versionCommand.Aliases = []string{"__version"}
	root.AddCommand(versionCommand)
	helpCommand := &cobra.Command{
		Use: "help", Short: "show help", Args: cobra.NoArgs, Run: completionNoop,
		ValidArgsFunction: commandBoundaryCompletion(),
	}
	root.SetHelpCommand(helpCommand)
	root.InitDefaultHelpCmd()
	for _, command := range getCommands {
		root.AddCommand(newGetCompletionCommand(command))
	}
	root.AddCommand(newReportCompletionCommand(provider))
	root.AddCommand(newReleaseCompletionCommand(provider))
	root.AddCommand(newDeleteCompletionCommand(provider))

	rootCandidates := make([]completionCandidate, 0, len(getCommands)+5)
	for _, command := range getCommands {
		rootCandidates = append(
			rootCandidates,
			completionCandidate{command.Name, command.Description},
		)
	}
	rootCandidates = append(rootCandidates, machineCommands...)
	rootCandidates = append(rootCandidates, completionCandidate{"-h", "show help"})
	root.ValidArgsFunction = commandBoundaryCompletion(rootCandidates...)
	if rootPrefix != "" {
		partialCandidates := append([]completionCandidate(nil), rootCandidates...)
		partialCandidates = append(partialCandidates,
			completionCandidate{"--ephemeral", "exit the dashboard on focus loss"},
			completionCandidate{"--help", "help for seshagy"},
		)
		root.AddCommand(&cobra.Command{
			Use: "__root_flags", Hidden: true, Run: completionNoop,
			ValidArgsFunction: boundaryCompletion(rootPrefix, partialCandidates...),
		})
	}
	for _, command := range root.Commands() {
		hideCompletionHelpFlags(command)
	}
	return root
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	command := &cobra.Command{Use: "completion", Short: "Generate shell completion scripts"}
	generators := []struct {
		name string
		run  func(io.Writer) error
	}{
		{name: "bash", run: func(w io.Writer) error { return genSafeBashCompletion(root, w) }},
		{name: "zsh", run: func(w io.Writer) error { return genSafeZshCompletion(root, w) }},
		{name: "fish", run: func(w io.Writer) error { return genSafeFishCompletion(root, w) }},
	}
	for _, generator := range generators {
		child := &cobra.Command{
			Use: generator.name, Short: "Generate " + generator.name + " completions",
			Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
				return generator.run(cmd.OutOrStdout())
			},
		}
		child.ValidArgsFunction = commandBoundaryCompletion()
		command.AddCommand(child)
	}
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func newConfigCompletionCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage configuration", Run: completionNoop}
	addJSONFlag(command)
	path := leafCompletionCommand("path", "print config file path", true)
	show := leafCompletionCommand("show", "print effective config", true)
	initCommand := leafCompletionCommand("init", "initialize configuration", true)
	initCommand.Flags().Bool("force", false, "replace existing configuration")
	initCommand.ValidArgsFunction = commandBoundaryCompletion()
	command.AddCommand(path, show, initCommand)
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func newIntegrationCompletionCommand() *cobra.Command {
	command := &cobra.Command{Use: "integration", Short: "Manage agent integrations"}
	candidates := integrationCandidates("", "agent integration")
	for _, action := range []string{"install", "uninstall"} {
		child := &cobra.Command{
			Use: action + " <name>", Short: action + " an agent integration",
			Args: cobra.ExactArgs(1), Run: completionNoop,
			ValidArgsFunction: commandBoundaryCompletion(candidates...),
		}
		command.AddCommand(child)
	}
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func newKeybindCompletionCommand() *cobra.Command {
	command := &cobra.Command{Use: "keybind", Short: "Manage multiplexer keybinds"}
	install := &cobra.Command{Use: "install", Short: "Install a keybind"}
	uninstall := &cobra.Command{Use: "uninstall", Short: "Uninstall a keybind"}
	for _, target := range keybindTargets {
		var installTarget *cobra.Command
		switch target {
		case "tmux":
			installTarget = leafCompletionCommand("tmux", "install a tmux keybind", false)
			installTarget.Flags().String("key", defaultTmuxKey, "prefix key")
			installTarget.Flags().String("mode", string(tmuxModePopup), "launch mode")
			installTarget.Flags().Bool("persistent", false, "stay open on focus loss")
			_ = installTarget.RegisterFlagCompletionFunc(
				"mode",
				fixedCompletion(tmuxModeStrings()...),
			)
		case "herdr":
			installTarget = leafCompletionCommand("herdr", "install a Herdr keybind", false)
			installTarget.Flags().String("key", defaultTmuxKey, "prefix key")
			installTarget.Flags().String("mode", string(herdrModePane), "launch mode")
			installTarget.Flags().String("width", defaultHerdrPopupWidth, "popup width")
			installTarget.Flags().String("height", defaultHerdrPopupHeight, "popup height")
			installTarget.Flags().Bool("persistent", false, "stay open on focus loss")
			_ = installTarget.RegisterFlagCompletionFunc(
				"mode",
				fixedCompletion(herdrModeStrings()...),
			)
		}
		installTarget.ValidArgsFunction = commandBoundaryCompletion()
		install.AddCommand(installTarget)
		uninstall.AddCommand(
			leafCompletionCommand(target, "uninstall the "+target+" keybind", false),
		)
	}
	install.ValidArgsFunction = commandBoundaryCompletion()
	uninstall.ValidArgsFunction = commandBoundaryCompletion()
	command.AddCommand(install, uninstall)
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func tmuxModeStrings() []string  { return stringValues(tmuxLaunchModes) }
func herdrModeStrings() []string { return stringValues(herdrLaunchModes) }

func stringValues[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = string(values[i])
	}
	return out
}

func newGetCompletionCommand(command getCommand) *cobra.Command {
	child := leafCompletionCommand(pseudoName(command.Name), command.Description, true)
	child.Hidden = true
	return child
}

func newReportCompletionCommand(provider completionProvider) *cobra.Command {
	command := leafCompletionCommand(pseudoName("--report-agent"), "report agent state", true)
	command.Hidden = true
	command.Flags().String("pane", "", "target pane id")
	command.Flags().String("cwd", "", "target working directory")
	command.Flags().String("agent", "", "agent name")
	command.Flags().String("state", "", "agent state")
	command.Flags().String("source", "", "report source")
	command.Flags().Int64("seq", 0, "monotonic sequence number")
	command.Flags().String("message", "", "optional status message")
	command.Flags().String("session-id", "", "optional agent session id")
	registerProviderFlag(command, "pane", provider, completionProvider.Panes, nil, false)
	registerProviderFlag(command, "cwd", provider, completionProvider.Directories, nil, true)
	registerProviderFlag(command, "agent", provider, completionProvider.Agents,
		integrationCandidates("", "agent integration"), false)
	registerProviderFlag(command, "source", provider, completionProvider.Sources,
		integrationCandidates("seshagy:", "integration report source"), false)
	states := make([]string, 0, len(sessionmgr.AgentStates()))
	for _, state := range sessionmgr.AgentStates() {
		states = append(states, string(state))
	}
	_ = command.RegisterFlagCompletionFunc("state", fixedCompletion(states...))
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func newReleaseCompletionCommand(provider completionProvider) *cobra.Command {
	command := leafCompletionCommand(pseudoName("--release-agent"), "release agent state", true)
	command.Hidden = true
	command.Flags().String("pane", "", "target pane id")
	command.Flags().String("cwd", "", "target working directory")
	command.Flags().String("source", "", "report source")
	command.Flags().Int64("seq", 0, "monotonic sequence number")
	registerProviderFlag(command, "pane", provider, completionProvider.Panes, nil, false)
	registerProviderFlag(command, "cwd", provider, completionProvider.Directories, nil, true)
	registerProviderFlag(command, "source", provider, completionProvider.Sources,
		integrationCandidates("seshagy:", "integration report source"), false)
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func newDeleteCompletionCommand(provider completionProvider) *cobra.Command {
	command := leafCompletionCommand(
		pseudoName("--delete-item"),
		"delete an active session or workspace",
		true,
	)
	command.Hidden = true
	command.Use += " <target>"
	command.Args = cobra.ExactArgs(1)
	command.ValidArgsFunction = providerBoundaryCompletion(
		provider,
		completionProvider.DeleteTargets,
	)
	return command
}

func leafCompletionCommand(use, short string, jsonFlag bool) *cobra.Command {
	command := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, Run: completionNoop}
	if jsonFlag {
		addJSONFlag(command)
	}
	command.ValidArgsFunction = commandBoundaryCompletion()
	return command
}

func addJSONFlag(command *cobra.Command)      { command.Flags().Bool("json", false, "emit JSON") }
func completionNoop(*cobra.Command, []string) {}

func hideCompletionHelpFlags(command *cobra.Command) {
	command.InitDefaultHelpFlag()
	_ = command.Flags().MarkHidden("help")
	for _, child := range command.Commands() {
		hideCompletionHelpFlags(child)
	}
}

func commandBoundaryCompletion(positionals ...completionCandidate) cobra.CompletionFunc {
	return boundaryCompletion("", positionals...)
}

func boundaryCompletion(override string, positionals ...completionCandidate) cobra.CompletionFunc {
	return func(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if override != "" {
			toComplete = override
		}
		var candidates []completionCandidate
		if len(args) == 0 {
			wantFlags := strings.HasPrefix(toComplete, "-")
			for _, candidate := range positionals {
				isFlag := strings.HasPrefix(candidate.Value, "-")
				if (toComplete == "" || wantFlags == isFlag) &&
					strings.HasPrefix(candidate.Value, toComplete) {
					candidates = append(candidates, candidate)
				}
			}
		}
		if toComplete == "" || strings.HasPrefix(toComplete, "-") {
			command.Flags().VisitAll(func(flag *pflag.Flag) {
				value := "--" + flag.Name
				if !flag.Changed && !flag.Hidden && flag.Deprecated == "" &&
					strings.HasPrefix(value, toComplete) {
					candidates = append(candidates, completionCandidate{value, flag.Usage})
				}
			})
		}
		return encodeCandidates(
			candidates,
		), cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
	}
}

func providerBoundaryCompletion(
	provider completionProvider,
	call providerCall,
) cobra.CompletionFunc {
	return func(command *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			if strings.HasPrefix(toComplete, "-") {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return encodeCandidates(matchingCandidates(
				boundedProviderCall(provider, call, nil), toComplete,
			)), cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
		}
		return commandBoundaryCompletion()(command, args, toComplete)
	}
}

func fixedCompletion(values ...string) cobra.CompletionFunc {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return values, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
	}
}

type providerCall func(completionProvider, context.Context) ([]completionCandidate, error)

func registerProviderFlag(
	command *cobra.Command,
	name string,
	provider completionProvider,
	call providerCall,
	fixed []completionCandidate,
	directory bool,
) {
	_ = command.RegisterFlagCompletionFunc(name,
		func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			usable := encodeCandidates(matchingCandidates(
				boundedProviderCall(provider, call, fixed), toComplete,
			))
			if directory && len(usable) == 0 {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
			directive := cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
			if directory {
				directive |= cobra.ShellCompDirectiveNoSpace
			}
			return usable, directive
		})
}

func matchingCandidates(candidates []completionCandidate, prefix string) []completionCandidate {
	matched := make([]completionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.Value, prefix) {
			matched = append(matched, candidate)
		}
	}
	return matched
}

func boundedProviderCall(
	provider completionProvider,
	call providerCall,
	fixed []completionCandidate,
) []completionCandidate {
	if provider == nil || call == nil {
		return fixed
	}
	ctx, cancel := context.WithTimeout(context.Background(), completionTimeout)
	defer cancel()
	type result struct {
		candidates []completionCandidate
		err        error
	}
	results := make(chan result, 1)
	go func() {
		candidates, err := call(provider, ctx)
		results <- result{candidates: candidates, err: err}
	}()
	select {
	case <-ctx.Done():
		return fixed
	case result := <-results:
		if result.err != nil {
			return fixed
		}
		return append(fixed, result.candidates...)
	}
}

func sanitizeCandidates(input []completionCandidate) []completionCandidate {
	out := make([]completionCandidate, 0, min(len(input), completionMaxCandidates))
	seen := map[string]bool{}
	total := 0
	for _, candidate := range input {
		if len(out) == completionMaxCandidates {
			break
		}
		if !validCompletionText(candidate.Value, completionMaxValueBytes) || seen[candidate.Value] {
			continue
		}
		description := candidate.Description
		if !validCompletionText(description, completionMaxDescBytes) {
			description = ""
		}
		size := len(candidate.Value) + len(description) + 1
		if total+size > completionMaxTotalBytes {
			break
		}
		seen[candidate.Value] = true
		total += size
		out = append(out, completionCandidate{Value: candidate.Value, Description: description})
	}
	return out
}

func validCompletionText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func encodeCandidates(candidates []completionCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range sanitizeCandidates(candidates) {
		if candidate.Description == "" {
			out = append(out, candidate.Value)
		} else {
			out = append(out, cobra.CompletionWithDesc(candidate.Value, candidate.Description))
		}
	}
	return out
}

func generateAndPatch(
	w io.Writer,
	generate func(io.Writer) error,
	patches ...completionPatch,
) error {
	var generated bytes.Buffer
	if err := generate(&generated); err != nil {
		return err
	}
	text := generated.String()
	for _, patch := range patches {
		count := strings.Count(text, patch.marker)
		if count != 1 {
			return fmt.Errorf("completion patch marker: expected 1, found %d", count)
		}
		text = strings.Replace(text, patch.marker, patch.replacement, 1)
		switch patch.marker {
		case bashEndpointMarker:
			var err error
			if text, err = finishBashPatch(text); err != nil {
				return err
			}
		case zshEndpointMarker:
			old := `words=("${=words[1,CURRENT]}")`
			if strings.Count(text, old) != 1 {
				return fmt.Errorf("zsh cursor marker mismatch")
			}
			text = strings.Replace(text, old, `words=("${(@)words[1,CURRENT]}")`, 1)
		}
	}
	_, err := io.WriteString(w, text)
	return err
}

type completionPatch struct{ marker, replacement string }

func finishBashPatch(script string) (string, error) {
	markers := [][2]string{
		{"_get_comp_words_by_ref \"$@\" cur prev words cword", bashInitReplacement},
		{"requestComp=\"${requestComp} ''\"", "args+=(\"\")"},
		{
			"# Use eval to handle any environment variables and such",
			"# Invoke completion without evaluating input",
		},
		{"COMPREPLY[0]=$(printf \"%q\" \"${COMPREPLY[0]}\")", ":"},
		{
			"COMPREPLY[0]=$(printf \"%q\" \"${COMPREPLY[0]%%$tab*}\")",
			"COMPREPLY[0]=${COMPREPLY[0]%%$tab*}",
		},
		{"    __seshagy_process_completion_results", bashFinalizeReplacement},
	}
	for _, replacement := range markers {
		if strings.Count(script, replacement[0]) != 1 {
			return "", fmt.Errorf("bash completion marker mismatch")
		}
		script = strings.Replace(script, replacement[0], replacement[1], 1)
	}
	if strings.Count(script, "_filedir") != 5 {
		return "", fmt.Errorf("bash helper marker mismatch")
	}
	script = strings.ReplaceAll(script, "_filedir", "__seshagy_complete_files")
	marker := "if [[ $(type -t compopt) = \"builtin\" ]]; then\n    complete -o default -F __start_seshagy seshagy\nelse\n    complete -o default -o nospace -F __start_seshagy seshagy\nfi"
	if strings.Count(script, marker) != 1 {
		return "", fmt.Errorf("bash registration marker mismatch")
	}
	return strings.Replace(script, marker, bashRegistrationReplacement, 1), nil
}

const (
	bashRegistrationReplacement = `if [[ ${BASH_VERSINFO[0]} -lt 4 ]]; then complete -o nospace -F __start_seshagy seshagy; else complete -o filenames -F __start_seshagy seshagy; fi`
	bashFinalizeReplacement     = `    __seshagy_process_completion_results; local hasDirectory=0; for i in "${!COMPREPLY[@]}"; do candidate=${COMPREPLY[i]}; [[ $candidate != */ ]] || hasDirectory=1; if [[ ${COMP_TYPE:-} != 37 && ${COMP_TYPE:-} != 42 ]]; then printf -v quoted '%q' "$candidate"; COMPREPLY[i]=$quoted; fi; if [[ ${BASH_VERSINFO[0]} -lt 4 ]]; then [[ $candidate == */ ]] || (((directive & 2) != 0)) || COMPREPLY[i]+=' '; fi; done; if [[ ${BASH_VERSINFO[0]} -ge 4 ]]; then compopt +o filenames; ((hasDirectory == 0)) || compopt -o nospace; fi`
)

const bashInitReplacement = `words=("${COMP_WORDS[@]}"); cword=${COMP_CWORD} cur=${COMP_WORDS[COMP_CWORD]}; if (( COMP_CWORD > 0 )); then prev=${COMP_WORDS[COMP_CWORD-1]}; else prev=""; fi
    __seshagy_complete_files() { local mode=-f candidate; [[ $1 == -d ]] && mode=-d; COMPREPLY=(); while IFS= read -r candidate; do [[ ! -d $candidate || $candidate == */ ]] || candidate+=/; COMPREPLY+=("$candidate"); done < <(compgen "$mode" -- "$cur"); }`

const (
	bashEndpointMarker      = `    out=$(eval "${requestComp}" 2>/dev/null)`
	bashEndpointReplacement = `    out=$("${words[0]}" __complete "${args[@]}" 2>/dev/null); local completionDirective=${out##*:} completionBody=${out%:*} candidate; if [[ -z $completionBody ]] && (((completionDirective & 4) == 0)); then COMPREPLY=(); __seshagy_complete_files; out=; for candidate in "${COMPREPLY[@]}"; do out+="${candidate}"$'\n'; done; out+=":${completionDirective}"; fi`
)

const zshEndpointMarker = `    # Prepare the command to obtain completions
    requestComp="${words[1]} __complete ${words[2,-1]}"
    if [ "${lastChar}" = "" ]; then
        # If the last parameter is complete (there is a space following it)
        # We add an extra empty parameter so we can indicate this to the go completion code.
        __seshagy_debug "Adding extra empty parameter"
        requestComp="${requestComp} \"\""
    fi

    __seshagy_debug "About to call: eval ${requestComp}"

    # Use eval to handle any environment variables and such
    out=$(eval ${requestComp} 2>/dev/null)`

const zshEndpointReplacement = `    local -a requestArgs
    requestArgs=("${(@)words[2,-1]}")
    if [ "${lastChar}" = "" ]; then
        (( ${#requestArgs} )) && requestArgs[-1]=()
        requestArgs+=("")
    fi
    out=$("${words[1]}" __complete "${requestArgs[@]}" 2>/dev/null)`

const zshDescribeMarker = `        __seshagy_debug "Calling _describe"
        if eval _describe $keepOrder "completions" completions $flagPrefix $noSpace; then
            __seshagy_debug "_describe found some completions"

            # Return the success of having called _describe
            return 0
        else
            __seshagy_debug "_describe did not find completions."
`

const zshGroupedReplacement = `        local -a subcommandCompletions=() flagCompletions=() describeOptions=(); local completion
        for completion in "${completions[@]}"; do if [[ "$completion" == -* ]]; then flagCompletions+=("$completion"); else subcommandCompletions+=("$completion"); fi; done
        [[ -n "$keepOrder" ]] && describeOptions+=(-V); [[ -n "$flagPrefix" ]] && describeOptions+=(-P "$BASH_REMATCH"); [[ -n "$noSpace" ]] && describeOptions+=(-S '')
        local describeResult=1
        if _describe "${describeOptions[@]}" -t subcommands 'subcommands' subcommandCompletions; then
            describeResult=0
        fi
        if _describe "${describeOptions[@]}" -t flags 'flags' flagCompletions; then
            describeResult=0
        fi
        if (( describeResult == 0 )); then
            __seshagy_debug "_describe found some completions"
            return 0
        else
            __seshagy_debug "_describe did not find completions."
`

const fishEndpointMarker = `    # Extract all args except the last one
    set -l args (commandline -opc)
    # Extract the last arg and escape it in case it is a space
    set -l lastArg (string escape -- (commandline -ct))

    __seshagy_debug "args: $args"
    __seshagy_debug "last arg: $lastArg"

    # Disable ActiveHelp which is not supported for fish shell
    set -l requestComp "SESHAGY_ACTIVE_HELP=0 $args[1] __complete $args[2..-1] $lastArg"

    __seshagy_debug "Calling $requestComp"
    set -l results (eval $requestComp 2> /dev/null)`

const fishEndpointReplacement = `    set -l args (commandline -opc)
    set -l lastArg (commandline -ct)
    set -lx SESHAGY_ACTIVE_HELP 0
    set -l results ($args[1] __complete $args[2..-1] "$lastArg" 2> /dev/null)
    set -l directive (string sub --start 2 $results[-1])
    if test (math (math --scale 0 $directive / 16) % 2) -eq 1
        set results (__fish_complete_directories "$lastArg") :4
    end`

func genSafeBashCompletion(root *cobra.Command, w io.Writer) error {
	return generateAndPatch(w,
		func(out io.Writer) error { return root.GenBashCompletionV2(out, true) },
		completionPatch{bashEndpointMarker, bashEndpointReplacement},
	)
}

func genSafeZshCompletion(root *cobra.Command, w io.Writer) error {
	return generateAndPatch(w, root.GenZshCompletion,
		completionPatch{zshEndpointMarker, zshEndpointReplacement},
		completionPatch{zshDescribeMarker, zshGroupedReplacement},
	)
}

func genSafeFishCompletion(root *cobra.Command, w io.Writer) error {
	return generateAndPatch(w,
		func(out io.Writer) error { return root.GenFishCompletion(out, true) },
		completionPatch{fishEndpointMarker, fishEndpointReplacement},
	)
}
