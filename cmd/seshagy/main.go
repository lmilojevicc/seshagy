package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
	"github.com/lmilojevicc/seshagy/internal/tui"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	if err := run(args); err != nil {
		if hasJSONFlag(args) {
			if encErr := encodeJSONError(err); encErr != nil {
				fmt.Fprintf(os.Stderr, "seshagy: %v\n", encErr)
			}
			os.Exit(1)
			return
		}
		fmt.Fprintf(os.Stderr, "seshagy: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return tui.Run()
	}
	mux := sessionmgr.Detect()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch args[0] {
	case "--help", "-h", "help":
		fmt.Print(helpText())
		return nil
	case "--version", "version":
		rest, jsonOutput := stripJSONFlag(args[1:])
		if len(rest) > 0 {
			return errors.New(joinUsage("--version", "[--json]"))
		}
		if jsonOutput {
			return encodeSuccess(map[string]string{"version": version})
		}
		fmt.Println(version)
		return nil
	case "config":
		return runConfig(args[1:])
	case "--get-sessions":
		return runGetItems(ctx, mux, args[1:], sessionmgr.ModeSessions, "--get-sessions")
	case "--get-zoxide":
		return runGetItems(ctx, mux, args[1:], sessionmgr.ModeZoxide, "--get-zoxide")
	case "--get-fd":
		return runGetItems(ctx, mux, args[1:], sessionmgr.ModeFD, "--get-fd")
	case "--get-agents":
		return runGetItems(ctx, mux, args[1:], sessionmgr.ModeAgents, "--get-agents")
	case "--get-current-session-agents":
		return runGetItems(
			ctx,
			mux,
			args[1:],
			sessionmgr.ModeCurrentAgents,
			"--get-current-session-agents",
		)
	case "--get-all":
		return runGetItems(ctx, mux, args[1:], sessionmgr.ModeAll, "--get-all")
	case "--report-agent":
		return runReportAgent(ctx, mux, args[1:])
	case "--release-agent":
		return runReleaseAgent(ctx, mux, args[1:])
	case "integration":
		return runIntegration(ctx, args[1:])
	case "keybind":
		return runKeybind(args[1:])
	case "--delete-item":
		line, jsonOutput := parseDeleteItemArgs(args[1:])
		if line == "" {
			return errors.New("--delete-item requires a rendered item line")
		}
		return deleteItem(ctx, mux, line, jsonOutput)
	default:
		return unknownCommandError(args)
	}
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

func runGetItems(
	ctx context.Context,
	mux sessionmgr.Multiplexer,
	args []string,
	mode sessionmgr.SourceMode,
	flag string,
) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) > 0 {
		return errors.New(modeUsage(flag))
	}
	return printItems(ctx, mux, mode, jsonOutput)
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

func runReportAgent(ctx context.Context, mux sessionmgr.Multiplexer, args []string) error {
	fs := flag.NewFlagSet("--report-agent", flag.ContinueOnError)
	pane := fs.String("pane", "", "target pane id (e.g. %5)")
	cwd := fs.String("cwd", "", "target by working directory (alternative to --pane)")
	agent := fs.String("agent", "", "agent name")
	state := fs.String("state", "", "agent state")
	source := fs.String("source", "", "report source")
	seq := fs.Int64("seq", 0, "monotonic sequence number")
	message := fs.String("message", "", "optional status message")
	sessionID := fs.String("session-id", "", "optional agent session id")
	jsonOutput := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *state == "" || *source == "" {
		return errors.New("--report-agent requires --state, --source")
	}
	resolved, err := resolvePaneTarget(ctx, mux, *pane, *cwd)
	if err != nil {
		return err
	}
	if resolved == "" {
		return nil // no unique cwd match — silent no-op
	}
	applied, err := mux.ReportAgent(ctx, sessionmgr.AgentReport{
		Pane:      resolved,
		Name:      *agent,
		State:     sessionmgr.NormalizeAgentState(*state),
		Source:    *source,
		Seq:       *seq,
		Message:   *message,
		SessionID: *sessionID,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return encodeSuccess(map[string]any{"applied": applied})
	}
	if applied {
		fmt.Printf("reported %s %s on %s\n", *agent, *state, resolved)
	} else if mux.Kind() == sessionmgr.BackendHerdr {
		fmt.Printf("report ignored (herdr owns agent state)\n")
	} else {
		fmt.Printf("stale report ignored (seq %d)\n", *seq)
	}
	return nil
}

func runReleaseAgent(ctx context.Context, mux sessionmgr.Multiplexer, args []string) error {
	fs := flag.NewFlagSet("--release-agent", flag.ContinueOnError)
	pane := fs.String("pane", "", "target pane id (e.g. %5)")
	cwd := fs.String("cwd", "", "target by working directory (alternative to --pane)")
	source := fs.String("source", "", "report source")
	seq := fs.Int64("seq", 0, "monotonic sequence number")
	jsonOutput := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source == "" {
		return errors.New("--release-agent requires --source")
	}
	resolved, err := resolvePaneTarget(ctx, mux, *pane, *cwd)
	if err != nil {
		return err
	}
	if resolved == "" {
		return nil // no unique cwd match — silent no-op
	}
	applied, err := mux.ReleaseAgent(ctx, sessionmgr.AgentRelease{
		Pane:   resolved,
		Source: *source,
		Seq:    *seq,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return encodeSuccess(map[string]any{"applied": applied})
	}
	if applied {
		fmt.Printf("released agent state on %s\n", resolved)
	} else if mux.Kind() == sessionmgr.BackendHerdr {
		fmt.Printf("release ignored (herdr owns agent state)\n")
	} else {
		fmt.Printf("stale release ignored (seq %d)\n", *seq)
	}
	return nil
}

func runIntegration(_ context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(joinUsage("integration", "install <name>"))
	}
	switch args[0] {
	case "install":
		if len(args) < 2 {
			return errors.New(joinUsage("integration", "install", "<name>"))
		}
		path, err := integrations.Install(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("installed %s integration to %s\n", args[1], path)
		return nil
	case "uninstall":
		if len(args) < 2 {
			return errors.New(joinUsage("integration", "uninstall", "<name>"))
		}
		path, err := integrations.Uninstall(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("uninstalled %s integration from %s\n", args[1], path)
		return nil
	default:
		return fmt.Errorf("unknown integration command: %q", args[0])
	}
}

func runConfig(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 {
		if jsonOutput {
			return encodeSuccess(map[string]string{"path": appconfig.Path()})
		}
		fmt.Println(appconfig.Path())
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
		fmt.Println(appconfig.Path())
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
		fmt.Println(string(data))
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
		fmt.Println(appconfig.Path())
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
	result, err := sessionmgr.LoadWithBackend(ctx, mux, mode, cfg.LoadOptions())
	if err != nil {
		return err
	}
	if result.Warning != "" {
		fmt.Fprintf(os.Stderr, "seshagy: warning: %s\n", result.Warning)
	}
	if jsonOutput {
		return encodeSuccess(
			sessionmgr.ItemsToJSON(mode, result.Items, cfg.IconSet(), result.Warning),
		)
	}
	icons := cfg.IconSet()
	for _, item := range result.Items {
		fmt.Println(sessionmgr.FormatLineWithIcons(item, icons))
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
	item, ok := sessionmgr.ParseActionLineWithIcons(raw, cfg.IconSet())
	if !ok {
		return fmt.Errorf("--delete-item: unrecognized item line: %q", raw)
	}
	switch item.Kind {
	case sessionmgr.KindSession:
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
	default:
		return fmt.Errorf("--delete-item: %s items cannot be deleted", item.Kind)
	}
}

func helpText() string {
	return `seshagy — minimal terminal session manager

Usage:
  seshagy                         open the Bubble Tea dashboard
  seshagy --get-all [--json]      print sessions, zoxide dirs, fd dirs
  seshagy --get-sessions [--json] print tmux sessions / herdr workspaces
  seshagy --get-zoxide [--json]   print zoxide directories
  seshagy --get-fd [--json]       print fd directories
  seshagy --get-agents [--json]   print agent panes (all sessions)
  seshagy --get-current-session-agents [--json]
                                  print agent panes (current session)
  seshagy --delete-item <line> [--json]
                                  kill a rendered session line
  seshagy config path [--json]    print config file path
  seshagy config show [--json]    print effective config
  seshagy config init [--force] [--json]
  seshagy keybind install tmux [--key <key>] [--mode popup|window|pane|pane-zoomed]
                                  bind prefix+<key> (default: s) to launch
                                  seshagy as an ephemeral tmux window/popup
                                  (default mode: popup)
  seshagy keybind install herdr [--key <key>]
                                  bind prefix+<key> (default: s) to launch
                                  seshagy as an ephemeral herdr pane
  seshagy keybind uninstall tmux
                                  remove the seshagy tmux keybinding
  seshagy keybind uninstall herdr
                                  remove the seshagy herdr keybinding
  seshagy --version [--json]

Scripting:
  Append --json to any command above for machine-readable JSON on stdout.
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
