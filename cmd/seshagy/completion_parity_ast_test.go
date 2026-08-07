package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type canonicalSurface struct {
	routes map[string]bool
	flags  map[string]map[string]bool
}

func TestCanonicalSourceAndCobraParity(t *testing.T) {
	source := canonicalSourceSurface(t, nil)
	cobraSurface := canonicalCobraSurface()
	if err := compareCanonicalSurfaces(source, cobraSurface); err != nil {
		t.Fatal(err)
	}

	sources := map[string]string{}
	for _, path := range []string{"main.go", "keybind.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[path] = string(data)
	}
	mutations := []struct {
		name, path, old, new string
	}{
		{"route", "main.go", `case "diagnostics":`, `case "diagnostics", "synthetic-route":`},
		{"flag", "main.go", `&cmd.message, "message"`, `&cmd.message, "synthetic-flag"`},
		{
			"integration action",
			"main.go",
			`args[0] != "install" && args[0] != "uninstall"`,
			`args[0] != "install" && args[0] != "uninstall" && args[0] != "upgrade"`,
		},
		{
			"keybind action",
			"keybind.go",
			`args[0] != "install"`,
			`args[0] != "install" && args[0] != "upgrade"`,
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name+" mutation fails", func(t *testing.T) {
			changed := strings.Replace(sources[mutation.path], mutation.old, mutation.new, 1)
			if changed == sources[mutation.path] {
				t.Fatalf("mutation marker %q not found", mutation.old)
			}
			mutated := canonicalSourceSurface(t, map[string]string{mutation.path: changed})
			if compareCanonicalSurfaces(mutated, cobraSurface) == nil {
				t.Fatal("synthetic source drift was not detected")
			}
		})
	}
}

func canonicalSourceSurface(t *testing.T, overrides map[string]string) canonicalSurface {
	t.Helper()
	files := map[string]*ast.File{}
	for _, path := range []string{"main.go", "keybind.go", "completion.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if replacement := overrides[path]; replacement != "" {
			data = []byte(replacement)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files[path] = file
	}
	run := findFunction(files["main.go"], "run")
	routes := map[string]bool{selectStrings(run, "--ephemeral")[0]: true}
	for _, alias := range switchStrings(run) {
		routes[alias] = true
	}
	for _, route := range switchStrings(findFunction(files["main.go"], "parseOperationalCommand")) {
		if route != "integration" && route != "keybind" {
			routes[route] = true
		}
	}
	for _, action := range switchStrings(findFunction(files["main.go"], "runConfig")) {
		routes["config "+action] = true
	}
	for _, get := range getNames(files["main.go"]) {
		routes[get] = true
	}
	for _, shell := range selectStrings(findFunction(files["completion.go"], "newCompletionCommand"), "bash", "zsh", "fish") {
		routes["completion "+shell] = true
	}
	for _, action := range argsZeroStrings(findFunction(files["main.go"], "parseIntegrationCommand")) {
		routes["integration "+action] = true
	}
	targets := vals(files["keybind.go"], "keybindTargets", nil)
	for _, action := range argsZeroStrings(findFunction(files["keybind.go"], "parseKeybindCommand")) {
		for _, target := range targets {
			routes["keybind "+action+" "+target] = true
		}
	}

	flags := map[string]map[string]bool{
		"":                {"ephemeral": true},
		"--report-agent":  flagSetNames(findFunction(files["main.go"], "parseReportAgentCommand")),
		"--release-agent": flagSetNames(findFunction(files["main.go"], "parseReleaseAgentCommand")),
		"--delete-item":   {"json": true},
		"config":          {"json": true},
		"config path":     {"json": true},
		"config show":     {"json": true},
		"config init":     {"force": true, "json": true},
		"diagnostics":     {"json": true},
		"version":         {"json": true},
		"--version":       {"json": true},
	}
	for _, get := range getNames(files["main.go"]) {
		flags[get] = map[string]bool{"json": true}
	}
	allowedKeyFlags := []string{"--key", "--mode", "--width", "--height", "--persistent"}
	keyFlags := selectStrings(
		findFunction(files["keybind.go"], "parseKeybindCommand"),
		allowedKeyFlags...)
	for _, target := range targets {
		set := map[string]bool{}
		for _, flag := range keyFlags {
			if target == "tmux" && (flag == "--width" || flag == "--height") {
				continue
			}
			set[strings.TrimPrefix(flag, "--")] = true
		}
		flags["keybind install "+target] = set
	}
	return canonicalSurface{routes, flags}
}

func canonicalCobraSurface() canonicalSurface {
	root := newCompletionRoot(testCompletionProvider{}, "")
	routes := map[string]bool{"--ephemeral": true, "--help": true, "-h": true}
	flags := map[string]map[string]bool{"": cobraFlagNames(root)}
	pseudo := map[string]string{
		"__get_all":                    "--get-all",
		"__get_sessions":               "--get-sessions",
		"__get_zoxide":                 "--get-zoxide",
		"__get_fd":                     "--get-fd",
		"__get_agents":                 "--get-agents",
		"__get_current_session_agents": "--get-current-session-agents",
		"__report_agent":               "--report-agent",
		"__release_agent":              "--release-agent",
		"__delete_item":                "--delete-item",
		"__version":                    "--version",
	}
	var walk func(*cobra.Command, string)
	walk = func(command *cobra.Command, prefix string) {
		for _, child := range command.Commands() {
			name := child.Name()
			if canonical := pseudo[name]; canonical != "" {
				name = canonical
			}
			path := strings.TrimSpace(prefix + " " + name)
			if child.Run != nil || child.RunE != nil {
				routes[path], flags[path] = true, cobraFlagNames(child)
			}
			for _, alias := range child.Aliases {
				if canonical := pseudo[alias]; canonical != "" {
					routes[canonical], flags[canonical] = true, cobraFlagNames(child)
				}
			}
			walk(child, path)
		}
	}
	walk(root, "")
	return canonicalSurface{routes, flags}
}

func compareCanonicalSurfaces(source, completion canonicalSurface) error {
	for route := range source.routes {
		if !completion.routes[route] {
			return fmt.Errorf("canonical source route missing from Cobra: %s", route)
		}
	}
	for route := range completion.routes {
		if !source.routes[route] {
			return fmt.Errorf("Cobra route missing from canonical source: %s", route)
		}
	}
	for route, sourceFlags := range source.flags {
		completionFlags := completion.flags[route]
		if fmt.Sprint(sourceFlags) != fmt.Sprint(completionFlags) {
			return fmt.Errorf(
				"flag mismatch for %s: source=%v Cobra=%v",
				route,
				sourceFlags,
				completionFlags,
			)
		}
	}
	return nil
}

func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func switchStrings(fn *ast.FuncDecl) []string {
	var values []string
	ast.Inspect(fn, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expression := range clause.List {
			if value, ok := literalString(expression); ok {
				values = append(values, value)
			}
		}
		return true
	})
	return values
}

func argsZeroStrings(fn *ast.FuncDecl) []string {
	var values []string
	seen := map[string]bool{}
	ast.Inspect(fn, func(node ast.Node) bool {
		comparison, ok := node.(*ast.BinaryExpr)
		if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
			return true
		}
		var other ast.Expr
		if isArgsZero(comparison.X) {
			other = comparison.Y
		} else if isArgsZero(comparison.Y) {
			other = comparison.X
		}
		if value, ok := literalString(other); ok && !seen[value] {
			seen[value], values = true, append(values, value)
		}
		return true
	})
	return values
}

func isArgsZero(expression ast.Expr) bool {
	index, ok := expression.(*ast.IndexExpr)
	if !ok {
		return false
	}
	identifier, ok := index.X.(*ast.Ident)
	literal, literalOK := index.Index.(*ast.BasicLit)
	return ok && identifier.Name == "args" && literalOK && literal.Kind == token.INT &&
		literal.Value == "0"
}

func selectStrings(fn *ast.FuncDecl, allowed ...string) []string {
	want, found := map[string]bool{}, map[string]bool{}
	for _, value := range allowed {
		want[value] = true
	}
	ast.Inspect(fn, func(node ast.Node) bool {
		if value, ok := literalString(node); ok && want[value] {
			found[value] = true
		}
		return true
	})
	var out []string
	for _, value := range allowed {
		if found[value] {
			out = append(out, value)
		}
	}
	return out
}

func flagSetNames(fn *ast.FuncDecl) map[string]bool {
	flags := map[string]bool{}
	ast.Inspect(fn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok ||
			(selector.Sel.Name != "StringVar" && selector.Sel.Name != "Int64Var" && selector.Sel.Name != "BoolVar") {
			return true
		}
		if value, ok := literalString(call.Args[1]); ok {
			flags[value] = true
		}
		return true
	})
	return flags
}

func getNames(file *ast.File) []string {
	return vals(file, "getCommands", func(s string) bool { return strings.HasPrefix(s, "--get-") })
}

func vals(file *ast.File, name string, keep func(string) bool) []string {
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != name {
			return true
		}
		ast.Inspect(spec.Values[0], func(child ast.Node) bool {
			if value, ok := literalString(child); ok && (keep == nil || keep(value)) {
				out = append(out, value)
			}
			return true
		})
		return false
	})
	return out
}

func cobraFlagNames(command *cobra.Command) map[string]bool {
	flags := map[string]bool{}
	command.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" {
			flags[flag.Name] = true
		}
	})
	return flags
}

func literalString(node ast.Node) (string, bool) {
	if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
		value, err := strconv.Unquote(literal.Value)
		return value, err == nil
	}
	return "", false
}
