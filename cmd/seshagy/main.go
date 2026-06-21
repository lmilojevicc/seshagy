package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
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
		return runGetItems(ctx, args[1:], sessionmgr.ModeSessions, "--get-sessions")
	case "--get-zoxide":
		return runGetItems(ctx, args[1:], sessionmgr.ModeZoxide, "--get-zoxide")
	case "--get-fd":
		return runGetItems(ctx, args[1:], sessionmgr.ModeFD, "--get-fd")
	case "--get-all":
		return runGetItems(ctx, args[1:], sessionmgr.ModeAll, "--get-all")
	case "--delete-item":
		line, jsonOutput := parseDeleteItemArgs(args[1:])
		if line == "" {
			return errors.New("--delete-item requires a rendered item line")
		}
		return deleteItem(ctx, line, jsonOutput)
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
	args []string,
	mode sessionmgr.SourceMode,
	flag string,
) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) > 0 {
		return errors.New(modeUsage(flag))
	}
	return printItems(ctx, mode, jsonOutput)
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

func printItems(ctx context.Context, mode sessionmgr.SourceMode, jsonOutput bool) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	result, err := sessionmgr.LoadWithOptions(ctx, mode, cfg.LoadOptions())
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

func deleteItem(ctx context.Context, raw string, jsonOutput bool) error {
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
		if err := sessionmgr.KillSession(ctx, item.Name); err != nil {
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
	return `seshagy — minimal tmux session manager

Usage:
  seshagy                         open the Bubble Tea dashboard
  seshagy --get-all [--json]      print sessions, zoxide dirs, fd dirs
  seshagy --get-sessions [--json] print tmux sessions
  seshagy --get-zoxide [--json]   print zoxide directories
  seshagy --get-fd [--json]       print fd directories
  seshagy --delete-item <line> [--json]
                                  kill a rendered session line
  seshagy config path [--json]    print config file path
  seshagy config show [--json]    print effective config
  seshagy config init [--force] [--json]
  seshagy --version [--json]

Scripting:
  Append --json to any command above for machine-readable JSON on stdout.
  Responses include schema_version and ok; errors also print JSON on stdout.
  Human text output is unchanged when --json is omitted.

TUI keys:
  enter attach/create/focus   q quit   / filter   r refresh   R rename
  x kill session/pane         y yazi   m mode     1-6 modes
  p preview                   ? help

Config:
  Config lives at $XDG_CONFIG_HOME/seshagy/config.toml, or
  ~/.config/seshagy/config.toml when XDG_CONFIG_HOME is unset. It controls
  source order/default source, fd command, theme colors, Nerd Font icons, text
  label mode, no-icons mode, icon colors, type-first mode, and the action
  prefix key.
  In type-first mode, enter and arrow/page/home/end navigation keys do not need a prefix.
`
}
