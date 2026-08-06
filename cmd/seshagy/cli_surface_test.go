package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/completion"
	"github.com/lmilojevicc/seshagy/internal/integrations"
)

type cliParserSurface struct {
	commands map[string][]string
	flags    map[string][]string
}

var parserEntryPaths = map[string]string{
	"run":                      "seshagy",
	"runEarlyCompletion":       "seshagy",
	"parseOperationalCommand":  "seshagy",
	"runConfig":                "seshagy config",
	"runDiagnostics":           "seshagy diagnostics",
	"parseIntegrationCommand":  "seshagy integration",
	"parseKeybindCommand":      "seshagy keybind",
	"parseReportAgentCommand":  "seshagy --report-agent",
	"parseReleaseAgentCommand": "seshagy --release-agent",
	"parseDeleteItemArgs":      "seshagy --delete-item",
}

// Every install target must name the parser that owns its --mode enum. Keeping
// this map explicit lets the drift test fail at the new target's path instead
// of silently skipping a parser added in the future.
var keybindModeParserFunctions = map[string]string{
	"tmux":  "parseTmuxLaunchMode",
	"herdr": "parseHerdrLaunchMode",
}

func TestCLIParserSurfaceMatchesCompletionMetadata(t *testing.T) {
	files := parseProductionCLIFiles(t)
	surface := extractCLIParserSurface(t, files)

	assertSurfaceValues(
		t,
		"seshagy",
		specChildren(&completion.Root, true),
		surface.commands["seshagy"],
	)
	for _, path := range []string{"seshagy config", "seshagy integration", "seshagy keybind"} {
		assertSurfaceValues(
			t,
			path,
			specChildren(specCommand(t, path), false),
			surface.commands[path],
		)
	}
	for path, parserFlags := range surface.flags {
		assertSurfaceValues(t, path, specFlagsForPath(t, path), parserFlags)
	}
	for _, path := range allSpecTerminalPaths() {
		if _, ok := surface.flags[path]; !ok && len(specFlagsForPath(t, path)) > 0 {
			t.Errorf(
				"completion metadata exposes flags at %s without a parser-owned flag site",
				path,
			)
		}
	}

	assertSurfaceValues(
		t,
		"seshagy integration install name",
		specPositionalValues(t, "seshagy integration install"),
		integrations.Available(),
	)
	assertSurfaceValues(
		t,
		"seshagy integration uninstall name",
		specPositionalValues(t, "seshagy integration uninstall"),
		integrations.Available(),
	)
	keybindTargets := keybindTargetsByAction(t, function(t, files, "parseKeybindCommand"))
	installTargets := specPositionalValues(t, "seshagy keybind install")
	uninstallTargets := specPositionalValues(t, "seshagy keybind uninstall")
	assertSurfaceValues(
		t,
		"seshagy keybind install target",
		installTargets,
		keybindTargets["install"],
	)
	assertSurfaceValues(
		t,
		"seshagy keybind uninstall target",
		uninstallTargets,
		keybindTargets["uninstall"],
	)
	assertSurfaceValues(
		t,
		"seshagy keybind install target mode-parser metadata",
		installTargets,
		sortedMapKeys(keybindModeParserFunctions),
	)
	for _, problem := range keybindModeMetadataProblems(
		"seshagy keybind install",
		specCommand(t, "seshagy keybind install"),
		keybindModeParserFunctions,
		nil,
	) {
		t.Error(problem)
	}
	for _, target := range installTargets {
		parserName, ok := keybindModeParserFunctions[target]
		if !ok {
			t.Errorf("seshagy keybind install %s --mode: missing mode parser metadata", target)
			continue
		}
		assertSurfaceValues(
			t,
			"seshagy keybind install "+target+" --mode",
			specFlagValues(t, "seshagy keybind install", "--mode", target),
			switchCaseValues(t, files, parserName),
		)
	}
	assertSurfaceValues(
		t,
		"seshagy completion shell",
		specPositionalValues(t, "seshagy completion"),
		switchCaseValuesFromFile(t, "../../internal/completion/scripts.go", "Script"),
	)

	assertRequiredPositionals(
		t,
		files,
		"seshagy integration install",
		"parseIntegrationCommand",
		"args",
	)
	assertRequiredPositionals(
		t,
		files,
		"seshagy integration uninstall",
		"parseIntegrationCommand",
		"args",
	)
	assertRequiredPositionals(t, files, "seshagy keybind install", "parseKeybindCommand", "args")
	assertRequiredPositionals(
		t,
		files,
		"seshagy keybind uninstall",
		"parseKeybindCommand",
		"args",
	)
	assertRequiredPositionals(t, files, "seshagy completion", "runEarlyCompletion", "filtered")
	if got := len(specCommand(t, "seshagy --delete-item").Positionals); got != 1 {
		t.Errorf("required positionals at seshagy --delete-item = %d, want 1", got)
	}

	assertRoutingParsersRegistered(t, files)
}

func TestFlagSetExtractorRecognizesValueAndVarRegistrations(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package main
import "flag"
func parse(args []string) {
    fs := flag.NewFlagSet("synthetic", flag.ContinueOnError)
    _ = fs.String("string", "", "")
    _ = fs.Bool("bool", false, "")
    fs.Func("func", "", func(string) error { return nil })
    var destination int
    fs.IntVar(&destination, "int-var", 0, "")
    fs.SetOutput(nil)
    _ = fs.Args()
    _ = fs.Parse(args)
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn := function(t, map[string]*ast.File{"synthetic.go": file}, "parse")
	got, err := flagSetNames(fn)
	if err != nil {
		t.Fatal(err)
	}
	assertSurfaceValues(
		t,
		"synthetic FlagSet",
		[]string{"--bool", "--func", "--int-var", "--string"},
		got,
	)
}

func TestFlagSetExtractorRejectsUnknownMethodOnKnownReceiver(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package main
import "flag"
func parse() {
    fs := flag.NewFlagSet("synthetic", flag.ContinueOnError)
    fs.FutureRegistration("future", "")
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = flagSetNames(function(t, map[string]*ast.File{"synthetic.go": file}, "parse"))
	if err == nil || !strings.Contains(err.Error(), "FutureRegistration") {
		t.Fatalf("flagSetNames() error = %v, want unknown method failure", err)
	}
}

func TestKeybindExtractorKeepsActionTargetSetsSeparate(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package main
func parseKeybindCommand(args []string) {
    if args[0] == "uninstall" {
        if args[1] != "tmux" && args[1] != "herdr" {}
        return
    }
    cmd := struct{ target string }{target: args[1]}
    switch cmd.target { case "tmux", "herdr", "future": }
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	targets := keybindTargetsByAction(
		t,
		function(t, map[string]*ast.File{"synthetic.go": file}, "parseKeybindCommand"),
	)
	assertSurfaceValues(
		t,
		"synthetic keybind install target",
		[]string{"future", "herdr", "tmux"},
		targets["install"],
	)
	assertSurfaceValues(
		t,
		"synthetic keybind uninstall target",
		[]string{"herdr", "tmux"},
		targets["uninstall"],
	)
}

func TestKeybindMetadataReportsSyntheticThirdTargetPath(t *testing.T) {
	install := *specCommand(t, "seshagy keybind install")
	install.Positionals = append([]completion.Positional(nil), install.Positionals...)
	install.Positionals[0].Value.Values = []string{"tmux", "herdr", "future"}
	problems := keybindModeMetadataProblems(
		"seshagy keybind install",
		&install,
		keybindModeParserFunctions,
		nil,
	)
	joined := strings.Join(problems, "\n")
	for _, want := range []string{
		"seshagy keybind install future --mode: missing completion mode values",
		"seshagy keybind install future --mode: missing mode parser metadata",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("synthetic target did not report %q; problems:\n%s", want, joined)
		}
	}
}

func TestCLIExtractorReportsSyntheticNestedActionAndTargetFlagDrift(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package main
func parseKeybindCommand(args []string) {
    if args[0] != "install" && args[0] != "uninstall" && args[0] != "future" {}
    for i := 2; i < len(args); i++ {
        switch args[i] { case "--key", "--mode", "--width", "--height", "--persistent": }
        if args[1] == "herdr" && args[i] == "--future-target-flag" {}
    }
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{"synthetic.go": file}
	surface := extractCLIParserSurface(t, files)
	var problems []string
	problems = append(
		problems,
		compareSurface(
			"seshagy keybind",
			specChildren(specCommand(t, "seshagy keybind"), false),
			surface.commands["seshagy keybind"],
		)...)
	problems = append(
		problems,
		compareSurface(
			"seshagy keybind install herdr",
			specFlagsForPath(t, "seshagy keybind install herdr"),
			surface.flags["seshagy keybind install herdr"],
		)...)
	joined := strings.Join(problems, "\n")
	for _, want := range []string{
		"parser exposes seshagy keybind future without completion metadata",
		"parser exposes seshagy keybind install herdr --future-target-flag without completion metadata",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("extractor did not report %q; problems:\n%s", want, joined)
		}
	}
}

func TestPublicHelpUsageMatchesCompletionMetadataByPath(t *testing.T) {
	usages := parsePublicHelpUsages(t, helpText())
	byPath := map[string][]helpUsage{}
	for _, usage := range usages {
		if usage.path == "" {
			t.Errorf("public help exposes unknown command usage %q", usage.syntax)
			continue
		}
		byPath[usage.path] = append(byPath[usage.path], usage)
	}

	for _, path := range allSpecTerminalPaths() {
		entries := byPath[path]
		if len(entries) == 0 {
			t.Errorf("completion command path %s is absent from public help", path)
			continue
		}
		command := specCommand(t, path)
		for _, entry := range entries {
			target := entry.target(command)
			actualFlags := regexp.MustCompile(`--[a-z][a-z-]*`).FindAllString(entry.arguments, -1)
			expectedFlags := specFlagsForPath(t, strings.TrimSpace(path+" "+target))
			for _, problem := range compareSurface(strings.TrimSpace(path+" "+target), expectedFlags, actualFlags) {
				t.Error(problem)
			}
			if got, want := entry.requiredPositionals(
				command,
			), len(
				command.Positionals,
			); got != want {
				t.Errorf(
					"public help required positionals at %s = %d, completion metadata = %d (usage %q)",
					path,
					got,
					want,
					entry.syntax,
				)
			}
		}
		for _, alias := range command.Aliases {
			found := false
			for _, entry := range entries {
				found = found || containsHelpToken(entry.block, alias)
			}
			if !found {
				t.Errorf("public help path %s omits alias %q", path, alias)
			}
		}
		for i, positional := range command.Positionals {
			if positional.Value.Kind != completion.ValueEnum {
				continue
			}
			actual := helpPositionalEnumValues(entries, i)
			assertSurfaceValues(
				t,
				fmt.Sprintf("%s positional %d", path, i+1),
				positional.Value.Values,
				actual,
			)
		}
		for _, flag := range command.Flags {
			if flag.Value.Kind != completion.ValueEnum {
				continue
			}
			if flag.Value.ValuesByTarget != nil {
				if len(command.Positionals) == 0 {
					t.Errorf("%s %s has target modes without a target positional", path, flag.Name)
					continue
				}
				for _, target := range command.Positionals[0].Value.Values {
					expected, declared := flag.Value.ValuesByTarget[target]
					if !declared {
						t.Errorf("%s %s %s has no enum metadata", path, target, flag.Name)
						continue
					}
					actual := helpFlagEnumValues(
						helpEntriesForTarget(entries, command, target),
						flag.Name,
					)
					assertSurfaceValues(
						t,
						path+" "+target+" "+flag.Name,
						expected,
						actual,
					)
				}
			} else {
				actual := helpFlagEnumValues(entries, flag.Name)
				assertSurfaceValues(
					t,
					path+" "+flag.Name,
					flag.Value.Values,
					actual,
				)
			}
		}
	}

	for path := range byPath {
		if specCommandOrNil(path) == nil {
			t.Errorf("public help exposes command path %s without completion metadata", path)
		}
	}
}

func extractCLIParserSurface(t *testing.T, files map[string]*ast.File) cliParserSurface {
	t.Helper()
	surface := cliParserSurface{commands: map[string][]string{}, flags: map[string][]string{}}
	add := func(destination map[string][]string, path string, values ...string) {
		destination[path] = uniqueSorted(append(destination[path], values...))
	}

	if fn := optionalFunction(files, "run"); fn != nil {
		add(surface.commands, "seshagy", indexedComparisonValues(t, fn, "args")...)
		add(surface.flags, "seshagy", identifierComparisonValues(fn, "arg")...)
	}
	if fn := optionalFunction(files, "runEarlyCompletion"); fn != nil {
		add(surface.commands, "seshagy", indexedComparisonValues(t, fn, "filtered")...)
		for _, value := range indexedComparisonValues(t, fn, "args") {
			if value != "__complete" {
				add(surface.commands, "seshagy", value)
			}
		}
	}
	if fn := optionalFunction(files, "parseOperationalCommand"); fn != nil {
		add(surface.commands, "seshagy", indexedComparisonValues(t, fn, "args")...)
		add(surface.commands, "seshagy", stringMapKeys(fn, "modes")...)
		for _, name := range stringMapKeys(fn, "modes") {
			add(surface.flags, "seshagy "+name, "--json")
		}
		add(surface.flags, "seshagy --delete-item", "--json")
	}
	if fn := optionalFunction(files, "runConfig"); fn != nil {
		actions := indexedComparisonValues(t, fn, "rest")
		add(surface.commands, "seshagy config", actions...)
		for _, action := range actions {
			add(surface.flags, "seshagy config "+action, "--json")
		}
		for action, flags := range flagsBySwitchCase(fn, "rest", 0) {
			add(surface.flags, "seshagy config "+action, flags...)
		}
	}
	if optionalFunction(files, "runDiagnostics") != nil {
		add(surface.flags, "seshagy diagnostics", "--json")
	}
	if fn := optionalFunction(files, "parseIntegrationCommand"); fn != nil {
		add(surface.commands, "seshagy integration", indexedComparisonValues(t, fn, "args")...)
	}
	if fn := optionalFunction(files, "parseKeybindCommand"); fn != nil {
		add(surface.commands, "seshagy keybind", indexedComparisonValues(t, fn, "args")...)
		for path, flags := range keybindFlagSites(t, fn) {
			add(surface.flags, path, flags...)
		}
	}
	for functionName, path := range map[string]string{
		"parseReportAgentCommand":  "seshagy --report-agent",
		"parseReleaseAgentCommand": "seshagy --release-agent",
	} {
		if fn := optionalFunction(files, functionName); fn != nil {
			names, err := flagSetNames(fn)
			if err != nil {
				t.Fatalf("extract %s FlagSet registrations: %v", functionName, err)
			}
			add(surface.flags, path, names...)
		}
	}
	if fn := optionalFunction(files, "run"); fn != nil {
		add(surface.flags, "seshagy --version", "--json")
	}
	return surface
}

func keybindFlagSites(t *testing.T, fn *ast.FuncDecl) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	var walkBlock func(*ast.BlockStmt, string, string)
	walkBlock = func(block *ast.BlockStmt, action, target string) {
		if block == nil {
			return
		}
		for _, statement := range block.List {
			switch typed := statement.(type) {
			case *ast.IfStmt:
				nextAction, nextTarget := action, target
				if values := comparisonValuesForIndex(typed.Cond, 0); len(values) == 1 {
					nextAction = values[0]
				}
				if values := comparisonValuesForIndex(typed.Cond, 1); len(values) == 1 {
					nextTarget = values[0]
				}
				if flags := dynamicArgsComparisonValues(typed.Cond); len(flags) > 0 {
					if nextAction == "" {
						nextAction = "install"
					}
					path := "seshagy keybind " + nextAction
					if nextTarget != "" {
						path += " " + nextTarget
					}
					result[path] = append(result[path], flags...)
				}
				walkBlock(typed.Body, nextAction, nextTarget)
				if alternate, ok := typed.Else.(*ast.BlockStmt); ok {
					walkBlock(alternate, action, target)
				}
			case *ast.ForStmt:
				walkBlock(typed.Body, action, target)
			case *ast.RangeStmt:
				walkBlock(typed.Body, action, target)
			case *ast.SwitchStmt:
				variable, index, indexed := indexedExpression(typed.Tag)
				for _, rawClause := range typed.Body.List {
					clause := rawClause.(*ast.CaseClause)
					values := resolvedStrings(clause.List)
					caseAction, caseTarget := action, target
					if indexed && variable == "args" && index == 0 && len(values) == 1 {
						caseAction = values[0]
					} else if indexed && variable == "args" && index == 1 && len(values) == 1 {
						caseTarget = values[0]
					} else if switchUsesArgsDynamicIndex(typed.Tag) {
						path := "seshagy keybind "
						if caseAction == "" {
							caseAction = "install"
						}
						path += caseAction
						if caseTarget != "" {
							path += " " + caseTarget
						}
						for _, value := range values {
							if strings.HasPrefix(value, "--") {
								result[path] = append(result[path], value)
							}
						}
					}
					walkBlock(&ast.BlockStmt{List: clause.Body}, caseAction, caseTarget)
				}
			}
		}
	}
	walkBlock(fn.Body, "", "")
	for path := range result {
		result[path] = uniqueSorted(result[path])
	}
	return result
}

func parseProductionCLIFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = file
	}
	// Resolve shared completion constants referenced by command parser switches.
	const completionSpec = "../../internal/completion/spec.go"
	file, err := parser.ParseFile(token.NewFileSet(), completionSpec, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	files[completionSpec] = file
	return files
}

func assertRoutingParsersRegistered(t *testing.T, files map[string]*ast.File) {
	t.Helper()
	for _, router := range []string{"run", "runEarlyCompletion", "parseOperationalCommand"} {
		fn := optionalFunction(files, router)
		if fn == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || !strings.HasPrefix(name.Name, "parse") ||
				(!strings.HasSuffix(name.Name, "Command") && !strings.HasSuffix(name.Name, "Args")) {
				return true
			}
			path, registered := parserEntryPaths[name.Name]
			if !registered {
				t.Errorf(
					"top-level routing function %s calls parser surface %s without command-path metadata",
					router,
					name.Name,
				)
			} else if specCommandOrNil(path) == nil && path != "seshagy" {
				t.Errorf("parser surface %s owns unknown completion path %s", name.Name, path)
			}
			return true
		})
	}
}

type helpUsage struct {
	syntax    string
	arguments string
	path      string
	block     string
}

func parsePublicHelpUsages(t *testing.T, help string) []helpUsage {
	t.Helper()
	lines := strings.Split(help, "\n")
	var usages []helpUsage
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "  seshagy ") {
			continue
		}
		block := lines[i]
		usageText := strings.TrimSpace(lines[i])
		if fields := regexp.MustCompile(`\s{2,}`).Split(usageText, 2); len(fields) > 0 {
			usageText = fields[0]
		}
		for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "  seshagy "); j++ {
			trimmed := strings.TrimSpace(lines[j])
			block += "\n" + lines[j]
			if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "--") {
				if fields := regexp.MustCompile(`\s{2,}`).Split(trimmed, 2); len(fields) > 0 {
					usageText += " " + fields[0]
				}
			}
		}
		if usageText == "seshagy" || usageText == "seshagy --ephemeral" {
			continue
		}
		path, arguments := resolveHelpPath(usageText)
		usages = append(
			usages,
			helpUsage{syntax: usageText, arguments: arguments, path: path, block: block},
		)
	}
	return usages
}

func resolveHelpPath(syntax string) (string, string) {
	words := strings.Fields(strings.TrimPrefix(syntax, "seshagy "))
	command := &completion.Root
	path := "seshagy"
	consumed := 0
	for consumed < len(words) {
		word := strings.Trim(words[consumed], "()[]")
		child := completion.FindChild(command, word)
		if child == nil {
			break
		}
		command = child
		path += " " + child.Name
		consumed++
	}
	if path == "seshagy" {
		return "", strings.Join(words, " ")
	}
	return path, strings.Join(words[consumed:], " ")
}

func (usage helpUsage) target(command *completion.Command) string {
	if len(command.Positionals) == 0 || len(command.Positionals[0].Value.Values) == 0 {
		return ""
	}
	for _, value := range command.Positionals[0].Value.Values {
		if containsHelpToken(usage.arguments, value) {
			return value
		}
	}
	return ""
}

func (usage helpUsage) requiredPositionals(command *completion.Command) int {
	count := 0
	for _, positional := range command.Positionals {
		matched := strings.Contains(usage.arguments, "<"+positional.Name+">")
		if !matched {
			for _, value := range positional.Value.Values {
				matched = matched || containsHelpToken(usage.arguments, value)
			}
		}
		if matched {
			count++
		}
	}
	return count
}

func helpEntriesForTarget(
	entries []helpUsage,
	command *completion.Command,
	target string,
) []helpUsage {
	var filtered []helpUsage
	for _, entry := range entries {
		if entry.target(command) == target {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func helpPositionalEnumValues(entries []helpUsage, index int) []string {
	var values []string
	for _, entry := range entries {
		positionals := 0
		for _, field := range strings.Fields(entry.arguments) {
			field = strings.Trim(field, "[]()")
			if field == "" || strings.HasPrefix(field, "--") {
				continue
			}
			if strings.HasPrefix(field, "<") {
				continue // value owned by the preceding flag
			}
			if positionals == index {
				values = append(values, strings.Split(field, "|")...)
				break
			}
			positionals++
		}
	}
	return uniqueSorted(values)
}

func helpFlagEnumValues(entries []helpUsage, flag string) []string {
	pattern := regexp.MustCompile(regexp.QuoteMeta(flag) + `\s+([^\s\]\)]+)`)
	var values []string
	for _, entry := range entries {
		match := pattern.FindStringSubmatch(entry.arguments)
		if len(match) == 2 {
			values = append(values, strings.Split(strings.Trim(match[1], "[]()"), "|")...)
		}
	}
	return uniqueSorted(values)
}

func containsHelpToken(text, token string) bool {
	pattern := `(^|[^[:alnum:]_-])` + regexp.QuoteMeta(token) + `([^[:alnum:]_-]|$)`
	return regexp.MustCompile(pattern).MatchString(text)
}

func allSpecTerminalPaths() []string {
	var paths []string
	var walk func(string, *completion.Command)
	walk = func(path string, command *completion.Command) {
		if command.Terminal {
			paths = append(paths, path)
		}
		for i := range command.Children {
			child := &command.Children[i]
			walk(strings.TrimSpace(path+" "+child.Name), child)
		}
	}
	walk("seshagy", &completion.Root)
	sort.Strings(paths)
	return paths
}

func specCommand(t *testing.T, path string) *completion.Command {
	t.Helper()
	command := specCommandOrNil(path)
	if command == nil {
		t.Fatalf("completion metadata path %s missing", path)
	}
	return command
}

func specCommandOrNil(path string) *completion.Command {
	command := &completion.Root
	for _, word := range strings.Fields(strings.TrimPrefix(path, "seshagy")) {
		child := completion.FindChild(command, word)
		if child == nil {
			// Concrete target suffixes belong to a positional, not a child command.
			if len(command.Positionals) > 0 && contains(command.Positionals[0].Value.Values, word) {
				continue
			}
			return nil
		}
		command = child
	}
	return command
}

func specChildren(command *completion.Command, aliases bool) []string {
	var values []string
	for _, child := range command.Children {
		values = append(values, child.Name)
		if aliases {
			values = append(values, child.Aliases...)
		}
	}
	return uniqueSorted(values)
}

func specFlagsForPath(t *testing.T, path string) []string {
	t.Helper()
	command := &completion.Root
	target := ""
	for _, word := range strings.Fields(strings.TrimPrefix(path, "seshagy")) {
		if child := completion.FindChild(command, word); child != nil {
			command = child
		} else if len(command.Positionals) > 0 && contains(command.Positionals[0].Value.Values, word) {
			target = word
		} else {
			t.Fatalf("completion metadata path %s missing at %q", path, word)
		}
	}
	var flags []string
	for _, flag := range command.Flags {
		if len(flag.Targets) == 0 || target == "" || contains(flag.Targets, target) {
			flags = append(flags, flag.Name)
		}
	}
	return uniqueSorted(flags)
}

func specPositionalValues(t *testing.T, path string) []string {
	command := specCommand(t, path)
	if len(command.Positionals) == 0 {
		t.Fatalf("completion positional at %s missing", path)
	}
	return uniqueSorted(command.Positionals[0].Value.Values)
}

func specFlagValues(t *testing.T, path, name, target string) []string {
	command := specCommand(t, path)
	for _, flag := range command.Flags {
		if flag.Name == name {
			if target != "" && flag.Value.ValuesByTarget != nil {
				return uniqueSorted(flag.Value.ValuesByTarget[target])
			}
			return uniqueSorted(flag.Value.Values)
		}
	}
	t.Fatalf("completion flag %s at %s missing", name, path)
	return nil
}

func assertRequiredPositionals(
	t *testing.T,
	files map[string]*ast.File,
	path, functionName, variable string,
) {
	t.Helper()
	const parserMin = 2
	if got := minimumLenCheck(function(t, files, functionName), variable); got != parserMin {
		t.Fatalf("parser minimum length in %s = %d, want %d", functionName, got, parserMin)
	}
	if got, want := len(specCommand(t, path).Positionals), parserMin-1; got != want {
		t.Errorf("required positionals at %s: completion=%d parser=%d", path, got, want)
	}
}

func function(t *testing.T, files map[string]*ast.File, name string) *ast.FuncDecl {
	t.Helper()
	fn := optionalFunction(files, name)
	if fn == nil {
		t.Fatalf("function %s not found", name)
	}
	return fn
}

func optionalFunction(files map[string]*ast.File, name string) *ast.FuncDecl {
	for _, file := range files {
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == name {
				return fn
			}
		}
	}
	return nil
}

func indexedComparisonValues(t *testing.T, fn *ast.FuncDecl, variable string) []string {
	t.Helper()
	values := map[string]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.EQL && typed.Op != token.NEQ {
				return true
			}
			if matchesIndexed(typed.X, variable, 0) {
				if value, ok := resolvedString(typed.Y); ok {
					values[value] = true
				}
			}
			if matchesIndexed(typed.Y, variable, 0) {
				if value, ok := resolvedString(typed.X); ok {
					values[value] = true
				}
			}
		case *ast.SwitchStmt:
			if matchesIndexed(typed.Tag, variable, 0) {
				for _, clause := range typed.Body.List {
					for _, value := range resolvedStrings(clause.(*ast.CaseClause).List) {
						values[value] = true
					}
				}
			}
		}
		return true
	})
	return sortedSet(values)
}

func dynamicArgsComparisonValues(expression ast.Expr) []string {
	values := map[string]bool{}
	ast.Inspect(expression, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
			return true
		}
		if switchUsesArgsDynamicIndex(binary.X) {
			if value, ok := resolvedString(binary.Y); ok && strings.HasPrefix(value, "--") {
				values[value] = true
			}
		}
		if switchUsesArgsDynamicIndex(binary.Y) {
			if value, ok := resolvedString(binary.X); ok && strings.HasPrefix(value, "--") {
				values[value] = true
			}
		}
		return true
	})
	return sortedSet(values)
}

func comparisonValuesForIndex(expression ast.Expr, index int) []string {
	values := map[string]bool{}
	ast.Inspect(expression, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
			return true
		}
		if matchesIndexed(binary.X, "args", index) {
			if value, ok := resolvedString(binary.Y); ok {
				values[value] = true
			}
		}
		if matchesIndexed(binary.Y, "args", index) {
			if value, ok := resolvedString(binary.X); ok {
				values[value] = true
			}
		}
		return true
	})
	return sortedSet(values)
}

func identifierComparisonValues(fn *ast.FuncDecl, identifier string) []string {
	values := map[string]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
			return true
		}
		if id, ok := binary.X.(*ast.Ident); ok && id.Name == identifier {
			if value, ok := resolvedString(binary.Y); ok {
				values[value] = true
			}
		}
		if id, ok := binary.Y.(*ast.Ident); ok && id.Name == identifier {
			if value, ok := resolvedString(binary.X); ok {
				values[value] = true
			}
		}
		return true
	})
	return sortedSet(values)
}

func flagsBySwitchCase(fn *ast.FuncDecl, variable string, index int) map[string][]string {
	result := map[string][]string{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switchStatement, ok := node.(*ast.SwitchStmt)
		if !ok || !matchesIndexed(switchStatement.Tag, variable, index) {
			return true
		}
		for _, raw := range switchStatement.Body.List {
			clause := raw.(*ast.CaseClause)
			for _, action := range resolvedStrings(clause.List) {
				ast.Inspect(&ast.BlockStmt{List: clause.Body}, func(child ast.Node) bool {
					if value, ok := resolvedString(child); ok && strings.HasPrefix(value, "--") {
						result[action] = append(result[action], value)
					}
					return true
				})
			}
		}
		return true
	})
	for action := range result {
		result[action] = uniqueSorted(result[action])
	}
	return result
}

func keybindTargetsByAction(t *testing.T, fn *ast.FuncDecl) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	var walkBlock func(*ast.BlockStmt, string)
	walkBlock = func(block *ast.BlockStmt, action string) {
		if block == nil {
			return
		}
		for _, statement := range block.List {
			switch typed := statement.(type) {
			case *ast.IfStmt:
				nextAction := action
				if values := comparisonValuesForIndex(typed.Cond, 0); len(values) == 1 {
					nextAction = values[0]
				}
				if nextAction == "install" || nextAction == "uninstall" {
					result[nextAction] = append(
						result[nextAction],
						comparisonValuesForIndex(typed.Cond, 1)...,
					)
				}
				walkBlock(typed.Body, nextAction)
				if alternate, ok := typed.Else.(*ast.BlockStmt); ok {
					walkBlock(alternate, action)
				}
			case *ast.SwitchStmt:
				for _, rawClause := range typed.Body.List {
					clause := rawClause.(*ast.CaseClause)
					caseAction := action
					if variable, index, indexed := indexedExpression(typed.Tag); indexed &&
						variable == "args" && index == 0 {
						values := resolvedStrings(clause.List)
						if len(values) == 1 {
							caseAction = values[0]
						}
					}
					if selector, ok := typed.Tag.(*ast.SelectorExpr); ok &&
						selector.Sel.Name == "target" {
						result["install"] = append(
							result["install"],
							resolvedStrings(clause.List)...)
					}
					walkBlock(&ast.BlockStmt{List: clause.Body}, caseAction)
				}
			case *ast.ForStmt:
				walkBlock(typed.Body, action)
			case *ast.RangeStmt:
				walkBlock(typed.Body, action)
			}
		}
	}
	walkBlock(fn.Body, "")
	for action := range result {
		result[action] = nonEmpty(uniqueSorted(result[action]))
	}
	return result
}

func keybindModeMetadataProblems(
	path string,
	command *completion.Command,
	modeParsers map[string]string,
	emptyModesExplicitlyValid map[string]bool,
) []string {
	if len(command.Positionals) == 0 {
		return []string{path + ": missing target metadata"}
	}
	targets := command.Positionals[0].Value.Values
	targetSet := toSet(targets)
	var mode *completion.Flag
	for i := range command.Flags {
		if command.Flags[i].Name == "--mode" {
			mode = &command.Flags[i]
			break
		}
	}
	if mode == nil {
		return []string{path + " --mode: missing completion metadata"}
	}
	var problems []string
	for _, target := range targets {
		values, declared := mode.Value.ValuesByTarget[target]
		targetPath := path + " " + target + " --mode"
		if !declared {
			problems = append(problems, targetPath+": missing completion mode values")
		} else if len(values) == 0 && !emptyModesExplicitlyValid[target] {
			problems = append(problems, targetPath+": empty modes are not explicitly valid")
		}
		if _, declared := modeParsers[target]; !declared {
			problems = append(problems, targetPath+": missing mode parser metadata")
		}
	}
	for target := range mode.Value.ValuesByTarget {
		if !targetSet[target] {
			problems = append(
				problems,
				path+" "+target+" --mode: completion modes belong to an undeclared target",
			)
		}
	}
	for target := range modeParsers {
		if !targetSet[target] {
			problems = append(
				problems,
				path+" "+target+" --mode: parser metadata belongs to an undeclared target",
			)
		}
	}
	sort.Strings(problems)
	return problems
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func flagSetNames(fn *ast.FuncDecl) ([]string, error) {
	knownReceivers := map[string]bool{}
	isFlagSetType := func(expression ast.Expr) bool {
		if pointer, ok := expression.(*ast.StarExpr); ok {
			expression = pointer.X
		}
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, packageOK := selector.X.(*ast.Ident)
		return packageOK && pkg.Name == "flag" && selector.Sel.Name == "FlagSet"
	}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			if !isFlagSetType(field.Type) {
				continue
			}
			for _, name := range field.Names {
				knownReceivers[name.Name] = true
			}
		}
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			for i, right := range typed.Rhs {
				call, ok := right.(*ast.CallExpr)
				if !ok || i >= len(typed.Lhs) {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, packageOK := selector.X.(*ast.Ident)
				left, leftOK := typed.Lhs[i].(*ast.Ident)
				if packageOK && leftOK && pkg.Name == "flag" &&
					selector.Sel.Name == "NewFlagSet" {
					knownReceivers[left.Name] = true
				}
			}
		case *ast.DeclStmt:
			declaration, ok := typed.Decl.(*ast.GenDecl)
			if !ok {
				break
			}
			for _, rawSpec := range declaration.Specs {
				spec, ok := rawSpec.(*ast.ValueSpec)
				if !ok || spec.Type == nil || !isFlagSetType(spec.Type) {
					continue
				}
				for _, name := range spec.Names {
					knownReceivers[name.Name] = true
				}
			}
		}
		return true
	})

	registrationNameArgument := map[string]int{
		"Bool": 0, "BoolFunc": 0, "Duration": 0, "Float64": 0, "Func": 0,
		"Int": 0, "Int64": 0, "String": 0, "Uint": 0, "Uint64": 0,
		"BoolVar": 1, "DurationVar": 1, "Float64Var": 1, "IntVar": 1,
		"Int64Var": 1, "StringVar": 1, "TextVar": 1, "UintVar": 1,
		"Uint64Var": 1, "Var": 1,
	}
	nonRegistrationMethods := map[string]bool{
		"Arg": true, "Args": true, "ErrorHandling": true, "Init": true, "Lookup": true,
		"NArg": true, "NFlag": true, "Name": true, "Output": true, "Parse": true,
		"Parsed": true, "PrintDefaults": true, "Set": true, "SetOutput": true,
		"Visit": true, "VisitAll": true,
	}
	var values, problems []string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		receiver, receiverOK := selector.X.(*ast.Ident)
		if !ok || !receiverOK || !knownReceivers[receiver.Name] {
			return true
		}
		argument, registration := registrationNameArgument[selector.Sel.Name]
		if !registration {
			if !nonRegistrationMethods[selector.Sel.Name] {
				problems = append(
					problems,
					fmt.Sprintf(
						"unrecognized flag.FlagSet method %s.%s",
						receiver.Name,
						selector.Sel.Name,
					),
				)
			}
			return true
		}
		if argument >= len(call.Args) {
			problems = append(
				problems,
				fmt.Sprintf(
					"flag registration %s.%s has no name argument",
					receiver.Name,
					selector.Sel.Name,
				),
			)
			return true
		}
		value, resolved := resolvedString(call.Args[argument])
		if !resolved {
			problems = append(
				problems,
				fmt.Sprintf(
					"flag registration %s.%s has a non-static name",
					receiver.Name,
					selector.Sel.Name,
				),
			)
			return true
		}
		values = append(values, "--"+value)
		return true
	})
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return uniqueSorted(values), nil
}

func stringMapKeys(fn *ast.FuncDecl, variable string) []string {
	var values []string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, left := range assignment.Lhs {
			id, ok := left.(*ast.Ident)
			if !ok || id.Name != variable || i >= len(assignment.Rhs) {
				continue
			}
			literal, ok := assignment.Rhs[i].(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if value, valid := resolvedString(pair.Key); valid {
					values = append(values, value)
				}
			}
		}
		return true
	})
	return uniqueSorted(values)
}

func switchCaseValues(t *testing.T, files map[string]*ast.File, functionName string) []string {
	t.Helper()
	return nonEmpty(resolvedCaseValues(files, function(t, files, functionName)))
}

func switchCaseValuesFromFile(t *testing.T, path, functionName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(path), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{path: file}
	return nonEmpty(resolvedCaseValues(files, function(t, files, functionName)))
}

func resolvedCaseValues(files map[string]*ast.File, fn *ast.FuncDecl) []string {
	var values []string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if ok {
			for _, expression := range clause.List {
				if value, resolved := resolvedStringFromFiles(files, expression, nil); resolved {
					values = append(values, value)
				}
			}
		}
		return true
	})
	return uniqueSorted(values)
}

func resolvedStringFromFiles(
	files map[string]*ast.File,
	expression ast.Expr,
	seen map[string]bool,
) (string, bool) {
	if value, ok := resolvedString(expression); ok {
		return value, true
	}
	if call, ok := expression.(*ast.CallExpr); ok && len(call.Args) == 1 {
		return resolvedStringFromFiles(files, call.Args[0], seen)
	}
	name := ""
	switch typed := expression.(type) {
	case *ast.Ident:
		name = typed.Name
	case *ast.SelectorExpr:
		name = typed.Sel.Name
	}
	if name == "" {
		return "", false
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	if seen[name] {
		return "", false
	}
	seen[name] = true
	defer delete(seen, name)
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, rawSpec := range general.Specs {
				spec := rawSpec.(*ast.ValueSpec)
				for i, identifier := range spec.Names {
					if identifier.Name == name && i < len(spec.Values) {
						return resolvedStringFromFiles(files, spec.Values[i], seen)
					}
				}
			}
		}
	}
	return "", false
}

func minimumLenCheck(fn *ast.FuncDecl, variable string) int {
	minimum := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		call, ok := binary.X.(*ast.CallExpr)
		literal, literalOK := binary.Y.(*ast.BasicLit)
		if !ok || len(call.Args) != 1 || !literalOK || literal.Kind != token.INT {
			return true
		}
		name, nameOK := call.Fun.(*ast.Ident)
		argument, argumentOK := call.Args[0].(*ast.Ident)
		if !nameOK || name.Name != "len" || !argumentOK || argument.Name != variable {
			return true
		}
		value, err := strconv.Atoi(literal.Value)
		if err == nil && value > minimum {
			minimum = value
		}
		return true
	})
	return minimum
}

func matchesIndexed(expression ast.Expr, variable string, index int) bool {
	gotVariable, gotIndex, ok := indexedExpression(expression)
	return ok && gotVariable == variable && gotIndex == index
}

func indexedExpression(expression ast.Expr) (string, int, bool) {
	indexed, ok := expression.(*ast.IndexExpr)
	if !ok {
		return "", 0, false
	}
	id, ok := indexed.X.(*ast.Ident)
	literal, literalOK := indexed.Index.(*ast.BasicLit)
	if !ok || !literalOK || literal.Kind != token.INT {
		return "", 0, false
	}
	value, err := strconv.Atoi(literal.Value)
	return id.Name, value, err == nil
}

func switchUsesArgsDynamicIndex(expression ast.Expr) bool {
	indexed, ok := expression.(*ast.IndexExpr)
	if !ok {
		return false
	}
	id, ok := indexed.X.(*ast.Ident)
	_, constant := indexed.Index.(*ast.BasicLit)
	return ok && id.Name == "args" && !constant
}

func resolvedStrings(expressions []ast.Expr) []string {
	var values []string
	for _, expression := range expressions {
		if value, ok := resolvedString(expression); ok {
			values = append(values, value)
		}
	}
	return uniqueSorted(values)
}

func resolvedString(node ast.Node) (string, bool) {
	switch typed := node.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(typed.Value)
		return value, err == nil
	case *ast.CallExpr:
		if len(typed.Args) == 1 {
			return resolvedString(typed.Args[0])
		}
	}
	return "", false
}

func assertSurfaceValues(t *testing.T, path string, expected, actual []string) {
	t.Helper()
	for _, problem := range compareSurface(path, expected, actual) {
		t.Error(problem)
	}
}

func compareSurface(path string, specValues, parserValues []string) []string {
	spec := toSet(specValues)
	parserValues = uniqueSorted(parserValues)
	parser := toSet(parserValues)
	var problems []string
	for _, value := range parserValues {
		if !spec[value] {
			problems = append(
				problems,
				fmt.Sprintf("parser exposes %s %s without completion metadata", path, value),
			)
		}
	}
	for _, value := range uniqueSorted(specValues) {
		if !parser[value] {
			problems = append(
				problems,
				fmt.Sprintf(
					"completion metadata exposes %s %s without parser support",
					path,
					value,
				),
			)
		}
	}
	sort.Strings(problems)
	return problems
}

func uniqueSorted(values []string) []string {
	return sortedSet(toSet(values))
}

func sortedSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nonEmpty(values []string) []string {
	var filtered []string
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return uniqueSorted(filtered)
}
