package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmilojevicc/seshagy/internal/cli"
	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/logging"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
	"github.com/lmilojevicc/seshagy/internal/tui"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	if err := run(args); err != nil {
		if hasJSONFlag(args) {
			if encErr := encodeJSONError(err); encErr != nil {
				cli.Errorf("%v", encErr)
			}
			os.Exit(1)
			return
		}
		cli.Errorf("%v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if handled, err := runCompletion(args); handled {
		return err
	}
	ephemeral := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--ephemeral" {
			ephemeral = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered
	if len(args) == 0 {
		return runTUI(ephemeral)
	}

	// Introspection commands are deliberately resolved before logging config so
	// they can never create, truncate, lock, or prune a diagnostic file.
	switch args[0] {
	case "--help", "-h", "help":
		cli.Help(os.Stdout, helpText())
		return nil
	case "--version", "version":
		rest, jsonOutput := stripJSONFlag(args[1:])
		if len(rest) > 0 {
			return errors.New(joinUsage("--version", "[--json]"))
		}
		if jsonOutput {
			return encodeSuccess(map[string]string{"version": version})
		}
		cli.Println(version)
		return nil
	case "config":
		return runConfig(args[1:])
	case "diagnostics":
		return runDiagnostics(args[1:])
	}

	parsed, err := parseOperationalCommand(args)
	if err != nil {
		return err
	}
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	mux := sessionmgr.Detect()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if parsed.kind == commandDelete {
		item, resolveErr := resolveDeleteItem(ctx, mux, parsed.deleteLine, cfg)
		if resolveErr != nil {
			return resolveErr
		}
		parsed.deleteItem = &item
	}
	resolved, err := logging.Resolve(
		logging.Config{Level: cfg.Log.Level, File: cfg.Log.File},
		os.LookupEnv,
	)
	if err != nil {
		return err
	}
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: version})
	if err != nil {
		return err
	}
	runtime.Activate()
	started := logAppStart(runtime.Logger(), mux.Kind(), resolved.LevelName)
	var operationErr error
	switch parsed.kind {
	case commandGet:
		operationErr = printItemsWithConfig(ctx, mux, parsed.mode, parsed.jsonOutput, cfg)
	case commandReport:
		operationErr = executeReportAgent(ctx, mux, *parsed.report)
	case commandRelease:
		operationErr = executeReleaseAgent(ctx, mux, *parsed.release)
	case commandIntegration:
		operationErr = executeIntegration(*parsed.integration)
	case commandKeybind:
		operationErr = executeKeybind(*parsed.keybind)
	case commandDelete:
		operationErr = deletePreparedItem(ctx, mux, *parsed.deleteItem, parsed.jsonOutput)
	}
	logAppStop(runtime.Logger(), mux.Kind(), started, operationErr)
	return errors.Join(operationErr, runtime.Shutdown())
}

func runTUI(ephemeral bool) error {
	cfg, cfgErr := appconfig.Load()
	if cfgErr != nil {
		return tui.Run(tui.WithConfig(cfg), tui.WithStartupError(cfgErr),
			tui.WithLogger(slog.New(slog.DiscardHandler)), tui.WithEphemeral(ephemeral))
	}
	resolved, err := logging.Resolve(
		logging.Config{Level: cfg.Log.Level, File: cfg.Log.File},
		os.LookupEnv,
	)
	if err != nil {
		return err
	}
	runtime, err := logging.Open(resolved, logging.Metadata{AppVersion: version})
	if err != nil {
		return err
	}
	runtime.Activate()
	mux := sessionmgr.Detect()
	started := logAppStart(runtime.Logger(), mux.Kind(), resolved.LevelName)
	operationErr := tui.Run(tui.WithConfig(cfg), tui.WithMultiplexer(mux),
		tui.WithLogger(runtime.Logger()), tui.WithEphemeral(ephemeral))
	logAppStop(runtime.Logger(), mux.Kind(), started, operationErr)
	return errors.Join(operationErr, runtime.Shutdown())
}

func logAppStart(logger *slog.Logger, backend sessionmgr.BackendKind, level string) time.Time {
	ctx := context.Background()
	if !logger.Enabled(ctx, slog.LevelInfo) {
		return time.Time{}
	}
	started := time.Now()
	logging.LogAttrs(ctx, logger, slog.LevelInfo,
		logging.EventAppStart, logging.ComponentApp,
		slog.String("backend", string(backend)), slog.String("log_level", level))
	return started
}

func logAppStop(
	logger *slog.Logger,
	backend sessionmgr.BackendKind,
	started time.Time,
	operationErr error,
) {
	if started.IsZero() {
		return
	}
	result := "success"
	if operationErr != nil {
		result = "failed"
	}
	logging.LogAttrs(context.Background(), logger, slog.LevelInfo,
		logging.EventAppStop, logging.ComponentApp,
		slog.String("backend", string(backend)), slog.String("result", result),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()))
}

type commandKind uint8

const (
	commandGet commandKind = iota
	commandReport
	commandRelease
	commandIntegration
	commandKeybind
	commandDelete
)

type parsedCommand struct {
	kind        commandKind
	mode        sessionmgr.SourceMode
	jsonOutput  bool
	report      *reportAgentCommand
	release     *releaseAgentCommand
	integration *integrationCommand
	keybind     *keybindCommand
	deleteLine  string
	deleteItem  *sessionmgr.Item
}

type reportAgentCommand struct {
	pane, cwd, agent, state, source, message, sessionID string
	seq                                                 int64
	jsonOutput                                          bool
}

type releaseAgentCommand struct {
	pane, cwd, source string
	seq               int64
	jsonOutput        bool
}

type integrationCommand struct{ action, name string }

type getCommand struct {
	Name        string
	Mode        sessionmgr.SourceMode
	Description string
}

var getCommands = []getCommand{
	{Name: "--get-all", Mode: sessionmgr.ModeAll, Description: "print all configured sources"},
	{
		Name:        "--get-sessions",
		Mode:        sessionmgr.ModeSessions,
		Description: "print sessions or workspaces",
	},
	{Name: "--get-zoxide", Mode: sessionmgr.ModeZoxide, Description: "print zoxide directories"},
	{Name: "--get-fd", Mode: sessionmgr.ModeFD, Description: "print fd directories"},
	{Name: "--get-agents", Mode: sessionmgr.ModeAgents, Description: "print agent panes"},
	{
		Name:        "--get-current-session-agents",
		Mode:        sessionmgr.ModeCurrentAgents,
		Description: "print current-session agents",
	},
}

func getMode(name string) (sessionmgr.SourceMode, bool) {
	for _, command := range getCommands {
		if command.Name == name {
			return command.Mode, true
		}
	}
	return 0, false
}

func parseOperationalCommand(args []string) (parsedCommand, error) {
	if len(args) == 0 {
		return parsedCommand{}, errors.New(joinUsage("<command>"))
	}
	if mode, ok := getMode(args[0]); ok {
		rest, jsonOutput := stripJSONFlag(args[1:])
		if len(rest) > 0 {
			return parsedCommand{}, errors.New(modeUsage(args[0]))
		}
		return parsedCommand{kind: commandGet, mode: mode, jsonOutput: jsonOutput}, nil
	}
	switch args[0] {
	case "--report-agent":
		cmd, err := parseReportAgentCommand(args[1:])
		return parsedCommand{kind: commandReport, report: &cmd}, err
	case "--release-agent":
		cmd, err := parseReleaseAgentCommand(args[1:])
		return parsedCommand{kind: commandRelease, release: &cmd}, err
	case "integration":
		cmd, err := parseIntegrationCommand(args[1:])
		return parsedCommand{kind: commandIntegration, integration: &cmd}, err
	case "keybind":
		cmd, err := parseKeybindCommand(args[1:])
		return parsedCommand{kind: commandKeybind, keybind: &cmd}, err
	case "--delete-item":
		line, jsonOutput := parseDeleteItemArgs(args[1:])
		if line == "" {
			return parsedCommand{}, errors.New("--delete-item requires a rendered item line")
		}
		return parsedCommand{kind: commandDelete, deleteLine: line, jsonOutput: jsonOutput}, nil
	default:
		return parsedCommand{}, unknownCommandError(args)
	}
}

func parseReportAgentCommand(args []string) (reportAgentCommand, error) {
	var cmd reportAgentCommand
	fs := flag.NewFlagSet("--report-agent", flag.ContinueOnError)
	fs.SetOutput(cli.Default.StderrWriter())
	fs.StringVar(&cmd.pane, "pane", "", "target pane id (e.g. %5)")
	fs.StringVar(&cmd.cwd, "cwd", "", "target by working directory (alternative to --pane)")
	fs.StringVar(&cmd.agent, "agent", "", "agent name")
	fs.StringVar(&cmd.state, "state", "", "agent state")
	fs.StringVar(&cmd.source, "source", "", "report source")
	fs.Int64Var(&cmd.seq, "seq", 0, "monotonic sequence number")
	fs.StringVar(&cmd.message, "message", "", "optional status message")
	fs.StringVar(&cmd.sessionID, "session-id", "", "optional agent session id")
	fs.BoolVar(&cmd.jsonOutput, "json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return reportAgentCommand{}, err
	}
	if cmd.state == "" || cmd.source == "" {
		return reportAgentCommand{}, errors.New("--report-agent requires --state, --source")
	}
	if cmd.pane == "" && cmd.cwd == "" {
		return reportAgentCommand{}, errors.New("requires --pane or --cwd")
	}
	return cmd, nil
}

func parseReleaseAgentCommand(args []string) (releaseAgentCommand, error) {
	var cmd releaseAgentCommand
	fs := flag.NewFlagSet("--release-agent", flag.ContinueOnError)
	fs.SetOutput(cli.Default.StderrWriter())
	fs.StringVar(&cmd.pane, "pane", "", "target pane id (e.g. %5)")
	fs.StringVar(&cmd.cwd, "cwd", "", "target by working directory (alternative to --pane)")
	fs.StringVar(&cmd.source, "source", "", "report source")
	fs.Int64Var(&cmd.seq, "seq", 0, "monotonic sequence number")
	fs.BoolVar(&cmd.jsonOutput, "json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return releaseAgentCommand{}, err
	}
	if cmd.source == "" {
		return releaseAgentCommand{}, errors.New("--release-agent requires --source")
	}
	if cmd.pane == "" && cmd.cwd == "" {
		return releaseAgentCommand{}, errors.New("requires --pane or --cwd")
	}
	return cmd, nil
}

func parseIntegrationCommand(args []string) (integrationCommand, error) {
	if len(args) == 0 {
		return integrationCommand{}, errors.New(joinUsage("integration", "install <name>"))
	}
	if args[0] != "install" && args[0] != "uninstall" {
		return integrationCommand{}, fmt.Errorf("unknown integration command: %q", args[0])
	}
	if len(args) < 2 {
		return integrationCommand{}, errors.New(joinUsage("integration", args[0], "<name>"))
	}
	for _, name := range integrations.Available() {
		if args[1] == name {
			return integrationCommand{action: args[0], name: name}, nil
		}
	}
	return integrationCommand{}, fmt.Errorf("unknown integration: %s", args[1])
}

func unknownCommandError(args []string) error {
	rest, jsonOnly := stripJSONFlag(args)
	if len(rest) == 0 {
		if jsonOnly {
			return errors.New(joinUsage("<command>", "[--json]"))
		}
		return errors.New(joinUsage("<command>"))
	}
	return fmt.Errorf("unknown command: %q", rest[0])
}

// resolvePaneTarget resolves a pane from --pane (wins) or --cwd. When only --cwd
// is given, ResolvePaneByCwd maps it to a unique pane via pane_current_path
// (refuses on 0 or >1 matches, returning "" for a silent no-op).
func resolvePaneTarget(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	pane, cwd string,
) (string, error) {
	if pane == "" && cwd == "" {
		return "", errors.New("requires --pane or --cwd")
	}
	if pane != "" {
		return mux.ResolvePane(ctx, pane)
	}
	return mux.ResolvePaneByCwd(ctx, cwd)
}

func executeReportAgent(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	cmd reportAgentCommand,
) error {
	resolved, err := resolvePaneTarget(ctx, mux, cmd.pane, cmd.cwd)
	if err != nil {
		return err
	}
	if resolved == "" {
		return nil // no unique cwd match — silent no-op
	}
	applied, err := mux.ReportAgent(ctx, sessionmgr.AgentReport{
		Pane: resolved, Name: cmd.agent, State: sessionmgr.NormalizeAgentState(cmd.state),
		Source: cmd.source, Seq: cmd.seq, Message: cmd.message, SessionID: cmd.sessionID,
	})
	if err != nil {
		return err
	}
	if cmd.jsonOutput {
		return encodeSuccess(map[string]any{"applied": applied})
	}
	if applied {
		cli.Successf("reported %s %s on %s", cmd.agent, cmd.state, resolved)
	} else if mux.Kind() == sessionmgr.BackendHerdr {
		cli.Info("report ignored (herdr owns agent state)")
	} else {
		cli.Warnf("stale report ignored (seq %d)", cmd.seq)
	}
	return nil
}

func executeReleaseAgent(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	cmd releaseAgentCommand,
) error {
	resolved, err := resolvePaneTarget(ctx, mux, cmd.pane, cmd.cwd)
	if err != nil {
		return err
	}
	if resolved == "" {
		return nil // no unique cwd match — silent no-op
	}
	applied, err := mux.ReleaseAgent(ctx, sessionmgr.AgentRelease{
		Pane: resolved, Source: cmd.source, Seq: cmd.seq,
	})
	if err != nil {
		return err
	}
	if cmd.jsonOutput {
		return encodeSuccess(map[string]any{"applied": applied})
	}
	if applied {
		cli.Successf("released agent state on %s", resolved)
	} else if mux.Kind() == sessionmgr.BackendHerdr {
		cli.Info("release ignored (herdr owns agent state)")
	} else {
		cli.Warnf("stale release ignored (seq %d)", cmd.seq)
	}
	return nil
}

func executeIntegration(cmd integrationCommand) error {
	var (
		path string
		err  error
	)
	if cmd.action == "install" {
		path, err = integrations.Install(cmd.name)
	} else {
		path, err = integrations.Uninstall(cmd.name)
	}
	if err != nil {
		return err
	}
	if cmd.action == "install" {
		cli.Successf("installed %s integration to %s", cmd.name, path)
	} else {
		cli.Successf("uninstalled %s integration from %s", cmd.name, path)
	}
	return nil
}

func runConfig(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 {
		if jsonOutput {
			return encodeSuccess(map[string]string{"path": appconfig.Path()})
		}
		cli.Println(appconfig.Path())
		return nil
	}
	switch rest[0] {
	case "path":
		if len(rest) != 1 {
			return errors.New(joinUsage("config", "path", "[--json]"))
		}
		if jsonOutput {
			return encodeSuccess(map[string]string{"path": appconfig.Path()})
		}
		cli.Println(appconfig.Path())
		return nil
	case "show":
		if len(rest) != 1 {
			return errors.New(joinUsage("config", "show", "[--json]"))
		}
		cfg, err := appconfig.Load()
		if err != nil {
			return err
		}
		if jsonOutput {
			return encodeSuccess(map[string]any{
				"config": cfg,
			})
		}
		data, err := appconfig.Marshal(cfg)
		if err != nil {
			return err
		}
		cli.Println(string(data))
		return nil
	case "init":
		force := len(rest) == 2 && rest[1] == "--force"
		if len(rest) > 2 || (len(rest) == 2 && !force) {
			return errors.New(joinUsage("config", "init", "[--force]", "[--json]"))
		}
		created := true
		if appconfig.Exists() && !force {
			return fmt.Errorf("config already exists: %s", appconfig.Path())
		}
		if appconfig.Exists() {
			created = false
		}
		if err := appconfig.Save(appconfig.Default()); err != nil {
			return err
		}
		if jsonOutput {
			return encodeSuccess(map[string]any{
				"path":    appconfig.Path(),
				"created": created,
				"forced":  force,
			})
		}
		cli.Println(appconfig.Path())
		return nil
	default:
		return errors.New(joinUsage("config", "path|show|init", "[--force]", "[--json]"))
	}
}

func printItems(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	mode sessionmgr.SourceMode,
	jsonOutput bool,
) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	return printItemsWithConfig(ctx, mux, mode, jsonOutput, cfg)
}

func printItemsWithConfig(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	mode sessionmgr.SourceMode,
	jsonOutput bool,
	cfg appconfig.Config,
) error {
	result, err := sessionmgr.LoadWithBackend(ctx, mux, mode, cfg.LoadOptions())
	if err != nil {
		return err
	}
	if result.Warning != "" {
		cli.Warnf("%s", result.Warning)
	}
	if jsonOutput {
		return encodeSuccess(
			sessionmgr.ItemsToJSON(mode, result.Items, cfg.IconSet(), result.Warning),
		)
	}
	icons := cfg.IconSet()
	for _, item := range result.Items {
		cli.Println(sessionmgr.FormatLineWithIcons(item, icons))
	}
	return nil
}

func deleteItem(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	raw string,
	jsonOutput bool,
) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	item, err := resolveDeleteItem(ctx, mux, raw, cfg)
	if err != nil {
		return err
	}
	return deletePreparedItem(ctx, mux, item, jsonOutput)
}

func resolveDeleteItem(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	raw string,
	cfg appconfig.Config,
) (sessionmgr.Item, error) {
	items, err := mux.ListSessions(ctx)
	if err != nil {
		return sessionmgr.Item{}, err
	}
	matches := func(match func(sessionmgr.Item) bool) []sessionmgr.Item {
		var found []sessionmgr.Item
		for _, item := range items {
			if item.Kind == sessionmgr.KindSession && match(item) {
				found = append(found, item)
			}
		}
		return found
	}
	stages := []func(sessionmgr.Item) bool{
		func(item sessionmgr.Item) bool { return item.ActionTarget() == raw },
		func(item sessionmgr.Item) bool {
			line := sessionmgr.FormatLineWithIcons(item, cfg.IconSet())
			return line == raw || sessionmgr.StripANSI(line) == sessionmgr.StripANSI(raw)
		},
		func(item sessionmgr.Item) bool { return item.Name == raw },
	}
	for _, stage := range stages {
		found := matches(stage)
		if len(found) == 1 {
			return found[0], nil
		}
		if len(found) > 1 {
			return sessionmgr.Item{}, fmt.Errorf("--delete-item: ambiguous active item: %q", raw)
		}
	}
	if parsed, ok := sessionmgr.ParseActionLineWithIcons(raw, cfg.IconSet()); ok &&
		parsed.Kind != sessionmgr.KindSession {
		return sessionmgr.Item{}, fmt.Errorf(
			"--delete-item: %s items cannot be deleted",
			parsed.Kind,
		)
	}
	return sessionmgr.Item{}, fmt.Errorf(
		"--delete-item: stale or unrecognized active item: %q",
		raw,
	)
}

func deletePreparedItem(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	item sessionmgr.Item,
	jsonOutput bool,
) error {
	if err := mux.KillSession(ctx, item.ActionTarget()); err != nil {
		return err
	}
	if jsonOutput {
		return encodeSuccess(map[string]any{
			"deleted": true,
			"kind":    item.Kind,
			"name":    item.Name,
		})
	}
	return nil
}

func helpText() string {
	return `seshagy — minimal terminal session manager

Usage:
  seshagy                         open the Bubble Tea dashboard
  seshagy --ephemeral             open the dashboard and exit on focus-loss
                                  (used by the tmux/herdr keybind launchers)
  seshagy --get-all [--json]      print sessions, zoxide dirs, fd dirs
  seshagy --get-sessions [--json] print tmux sessions / herdr workspaces
  seshagy --get-zoxide [--json]   print zoxide directories
  seshagy --get-fd [--json]       print fd directories
  seshagy --get-agents [--json]   print agent panes (all sessions)
  seshagy --get-current-session-agents [--json]
                                  print agent panes (current session)
  seshagy --delete-item <target> [--json]
                                  kill an active session/workspace by exact id,
                                  label, or current rendered line
  seshagy config path [--json]    print config file path
  seshagy config show [--json]    print effective config
  seshagy config init [--force] [--json]
  seshagy diagnostics [--json]   show logging status and bug-report guidance
  seshagy keybind install tmux [--key <key>] [--mode popup|window|pane|pane-zoomed] [--persistent]
                                  bind prefix+<key> (default: s) to launch
                                  seshagy as a tmux window/popup
                                  (default: popup with focus-loss auto-dismiss;
                                  --persistent stays open until explicitly quit)
  seshagy keybind install herdr [--key <key>] [--mode pane|popup] [--persistent]
                                  [--width <cells|percent>] [--height <cells|percent>]
                                  bind prefix+<key> (default: s) to launch
                                  seshagy as a herdr pane/popup
                                  (default: pane with focus-loss auto-dismiss;
                                  --persistent stays open until explicitly quit;
                                  popup needs herdr 0.7.4+ and defaults to 80% × 80%)
  seshagy keybind uninstall tmux
                                  remove the seshagy tmux keybinding
  seshagy keybind uninstall herdr
                                  remove the seshagy herdr keybinding
  seshagy --version [--json]
  seshagy completion bash|zsh|fish
                                  generate a shell completion script

Scripting:
  Append --json to commands marked [--json] for machine-readable JSON on stdout.
  Responses include schema_version and ok; errors also print JSON on stdout.
  Human text output is unchanged when --json is omitted.
  seshagy --report-agent --pane %N --state <state> --source <src> --seq <n>
                                  report agent state to a tmux pane (tmux only;
                                  no-op under herdr since herdr owns state)
                                  (--cwd <dir> may replace --pane; resolved by
                                  working directory when unique)
  seshagy --release-agent --pane %N --source <src> --seq <n>
                                  clear agent state from a tmux pane
                                  (--cwd <dir> may replace --pane)
  seshagy integration install <name>
                                  install an agent hook/extension
  seshagy integration uninstall <name>
                                  remove an agent hook/extension

TUI keys:
  enter attach/create/focus   q quit   / filter   r refresh   R rename
  x kill session/pane         y yazi   m mode     1-5 modes
  o agent scope               p preview   ? help

Config:
  Config lives at $XDG_CONFIG_HOME/seshagy/config.toml, or
  ~/.config/seshagy/config.toml when XDG_CONFIG_HOME is unset. It controls
  source order/default source, fd command, theme colors, Nerd Font icons, text
  label mode, no-icons mode, icon colors, type-first mode, and the action
  prefix key.
  In type-first mode, enter and arrow/page/home/end navigation keys do not need a prefix.
`
}
