package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
	"github.com/lmilojevicc/seshagy/internal/tui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
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
			return encodeJSON(map[string]string{"version": version})
		}
		fmt.Println(version)
		return nil
	case "integration", "integrations", "hook", "hooks":
		return runIntegration(args[1:])
	case "agent", "agents":
		return runAgent(ctx, args[1:])
	case "config":
		return runConfig(args[1:])
	case "manifest", "manifests":
		return runManifest(args[1:])
	case "--get-sessions":
		return runGetItems(ctx, args[1:], sessionmgr.ModeSessions, "--get-sessions")
	case "--get-agents":
		return runGetItems(ctx, args[1:], sessionmgr.ModeAgents, "--get-agents")
	case "--get-current-session-agents", "--get-session-agents":
		return runGetItems(
			ctx,
			args[1:],
			sessionmgr.ModeCurrentAgents,
			"--get-current-session-agents",
		)
	case "--get-zoxide":
		return runGetItems(ctx, args[1:], sessionmgr.ModeZoxide, "--get-zoxide")
	case "--get-fd":
		return runGetItems(ctx, args[1:], sessionmgr.ModeFD, "--get-fd")
	case "--get-all":
		return runGetItems(ctx, args[1:], sessionmgr.ModeAll, "--get-all")
	case "--delete-item":
		rest, jsonOutput := stripJSONFlag(args[1:])
		if len(rest) < 1 {
			return errors.New("--delete-item requires a rendered item line")
		}
		return deleteItem(ctx, strings.Join(rest, " "), jsonOutput)
	case "--report-agent":
		rest, jsonOutput := stripJSONFlag(args[1:])
		report, err := parseReportArgs(rest)
		if err != nil {
			return err
		}
		if err := sessionmgr.ReportAgent(ctx, report); err != nil {
			return err
		}
		if jsonOutput {
			return encodeJSON(map[string]any{
				"applied": true,
				"pane":    report.Pane,
				"agent":   report.Name,
				"state":   report.State,
			})
		}
		return nil
	case "--release-agent":
		rest, jsonOutput := stripJSONFlag(args[1:])
		release, err := parseReleaseArgs(rest)
		if err != nil {
			return err
		}
		if err := sessionmgr.ReleaseAgent(ctx, release); err != nil {
			return err
		}
		if jsonOutput {
			return encodeJSON(map[string]any{
				"released": true,
				"pane":     release.Pane,
			})
		}
		return nil
	default:
		return tui.Run()
	}
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

func runAgent(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(joinUsage("agent", "explain", "<pane-id>", "[--json]"))
	}
	switch args[0] {
	case "explain":
		rest, jsonOutput := stripJSONFlag(args[1:])
		if len(rest) != 1 {
			return errors.New(joinUsage("agent", "explain", "<pane-id>", "[--json]"))
		}
		cfg, err := appconfig.Load()
		if err != nil {
			return err
		}
		if jsonOutput {
			report, err := sessionmgr.ExplainAgentReport(ctx, rest[0], cfg.LoadOptions())
			if err != nil {
				return err
			}
			return encodeJSON(report)
		}
		out, err := sessionmgr.ExplainAgent(ctx, rest[0], cfg.LoadOptions())
		if err != nil {
			return err
		}
		fmt.Print(out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Println()
		}
		return nil
	default:
		return errors.New(joinUsage("agent", "explain", "<pane-id>", "[--json]"))
	}
}

func runManifest(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 {
		return errors.New(joinUsage("manifest", "status|update|reload", "[--json]"))
	}

	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}

	switch rest[0] {
	case "status":
		if len(rest) != 1 {
			return errors.New(joinUsage("manifest", "status", "[--json]"))
		}
		return printManifestStatus(cfg.Agents.ManifestCatalogURL, jsonOutput)
	case "update":
		if len(rest) != 1 {
			return errors.New(joinUsage("manifest", "update", "[--json]"))
		}
		output, err := sessionmgr.CheckAndUpdateManifests(cfg.Agents.ManifestCatalogURL)
		if err != nil {
			return err
		}
		sessionmgr.ReloadManifests()
		if jsonOutput {
			return encodeJSON(sessionmgr.ManifestUpdateOutputToJSON(output))
		}
		printManifestUpdateResult(output)
		return nil
	case "reload":
		if len(rest) != 1 {
			return errors.New(joinUsage("manifest", "reload", "[--json]"))
		}
		summaries := sessionmgr.ReloadManifests()
		if jsonOutput {
			return encodeJSON(map[string]any{
				"reloaded": len(summaries),
				"agents":   sessionmgr.AgentManifestSummariesToJSON(summaries),
			})
		}
		fmt.Printf("reloaded %d agent manifests\n", len(summaries))
		return nil
	default:
		return errors.New(joinUsage("manifest", "status|update|reload", "[--json]"))
	}
}

func printManifestStatus(catalogURL string, jsonOutput bool) error {
	status := sessionmgr.LoadManifestUpdateStatus()
	summaries := sessionmgr.ActiveManifestSummaries()
	resolvedCatalog := sessionmgr.ResolveManifestCatalogURL(catalogURL)
	if jsonOutput {
		return encodeJSON(map[string]any{
			"catalog": resolvedCatalog,
			"status":  sessionmgr.ManifestUpdateStatusToJSON(status),
			"agents":  sessionmgr.AgentManifestSummariesToJSON(summaries),
		})
	}
	fmt.Printf("catalog: %s\n", resolvedCatalog)
	if status.LastCheckUnix != nil {
		fmt.Printf(
			"last check: %s\n",
			time.Unix(int64(*status.LastCheckUnix), 0).UTC().Format(time.RFC3339),
		)
	}
	if status.LastResult != nil {
		fmt.Printf("last result: %s\n", *status.LastResult)
	}
	fmt.Println()
	for _, summary := range summaries {
		line := fmt.Sprintf(
			"%-16s %-10s %s",
			summary.AgentID,
			summary.ActiveSource.KindLabel(),
			summary.ActiveVersion,
		)
		if summary.CachedRemoteVersion != "" &&
			summary.CachedRemoteVersion != summary.ActiveVersion {
			line += fmt.Sprintf(" (cached remote %s)", summary.CachedRemoteVersion)
		}
		fmt.Println(line)
		if agentStatus, ok := status.AgentStatus(
			summary.AgentID,
		); ok &&
			agentStatus.LastResult != "" {
			updateLine := fmt.Sprintf("  update: %s", agentStatus.LastResult)
			if agentStatus.LastError != nil && *agentStatus.LastError != "" {
				updateLine += fmt.Sprintf(" (%s)", *agentStatus.LastError)
			}
			fmt.Println(updateLine)
		}
		if summary.Warning != "" {
			fmt.Printf("  warning: %s\n", summary.Warning)
		}
	}
	return nil
}

func printManifestUpdateResult(output sessionmgr.ManifestUpdateOutput) {
	if output.Status.LastResult != nil {
		fmt.Printf("update result: %s\n", *output.Status.LastResult)
	}
	if len(output.Updated) == 0 {
		fmt.Println("no manifest updates")
		return
	}
	for _, commit := range output.Updated {
		fmt.Printf("updated %s to %s\n", commit.AgentID, commit.Version)
	}
}

func runConfig(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 {
		if jsonOutput {
			return encodeJSON(map[string]string{"path": appconfig.Path()})
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
			return encodeJSON(map[string]string{"path": appconfig.Path()})
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
			return encodeJSON(cfg)
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
			return encodeJSON(map[string]any{
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

func runIntegration(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 || rest[0] == "status" {
		if len(rest) > 1 {
			return errors.New(joinUsage("integration", "status", "[--json]"))
		}
		recs := integrations.Scan()
		if jsonOutput {
			return encodeJSON(integrations.ScanToJSON(recs))
		}
		for _, rec := range recs {
			availability := "not found"
			if rec.AgentAvailable {
				availability = "found"
			}
			state := string(rec.State)
			if rec.State == integrations.StatusCurrent {
				state = fmt.Sprintf("current (v%d)", rec.Version)
			}
			if !rec.Installable && rec.AgentAvailable {
				state += "; " + rec.Reason
			}
			fmt.Printf(
				"%-18s %-9s %-18s %-14s %s\n",
				rec.Target,
				availability,
				state,
				rec.Authority,
				rec.InstallPath,
			)
		}
		return nil
	}
	if len(rest) != 2 || (rest[0] != "install" && rest[0] != "uninstall") {
		return errors.New(joinUsage(
			"integration",
			"status|install <target>|uninstall <target>",
			"[--json]",
		))
	}
	target, err := integrations.ParseTarget(rest[1])
	if err != nil {
		return err
	}
	var messages []string
	if rest[0] == "install" {
		messages, err = integrations.Install(target)
	} else {
		messages, err = integrations.Uninstall(target)
	}
	if err != nil {
		return err
	}
	if jsonOutput {
		return encodeJSON(map[string]any{
			"target":   target,
			"action":   rest[0],
			"messages": messages,
		})
	}
	for _, message := range messages {
		fmt.Println(message)
	}
	return nil
}

func printItems(ctx context.Context, mode sessionmgr.SourceMode, jsonOutput bool) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	items, err := sessionmgr.LoadWithOptions(ctx, mode, cfg.LoadOptions())
	if err != nil {
		return err
	}
	if jsonOutput {
		return encodeJSON(sessionmgr.ItemsToJSON(mode, items, cfg.IconSet()))
	}
	icons := cfg.IconSet()
	for _, item := range items {
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
			return encodeJSON(map[string]any{
				"deleted": true,
				"kind":    item.Kind,
				"name":    item.Name,
			})
		}
		return nil
	case sessionmgr.KindAgent:
		if err := sessionmgr.KillAgentPane(ctx, item.PaneID); err != nil {
			return err
		}
		if jsonOutput {
			return encodeJSON(map[string]any{
				"deleted": true,
				"kind":    item.Kind,
				"pane_id": item.PaneID,
				"agent":   item.AgentName,
			})
		}
		return nil
	default:
		return fmt.Errorf("--delete-item: %s items cannot be deleted", item.Kind)
	}
}

func parseReportArgs(args []string) (sessionmgr.AgentReport, error) {
	var opts sessionmgr.AgentReport
	for i := 0; i < len(args); {
		arg := args[i]
		key, val, hasInline := splitFlag(arg)
		nextValue := func() (string, error) {
			if hasInline {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", arg)
			}
			i++
			return args[i], nil
		}
		switch key {
		case "--pane":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Pane = v
		case "--agent", "--name":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Name = v
		case "--state", "--status":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.State = sessionmgr.NormalizeAgentState(v)
		case "--message":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Message = v
			opts.MessageSeen = true
		case "--source":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Source = v
			opts.SourceSeen = true
		case "--session-id":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.SessionID = v
			opts.SessionIDSeen = true
		case "--seq":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			seq, err := parseSeqFlag(v, key)
			if err != nil {
				return opts, err
			}
			opts.Seq = seq
			opts.SeqSeen = true
		default:
			return opts, fmt.Errorf("unknown --report-agent flag: %s", arg)
		}
		i++
	}
	return opts, nil
}

func parseReleaseArgs(args []string) (sessionmgr.AgentRelease, error) {
	var opts sessionmgr.AgentRelease
	for i := 0; i < len(args); {
		arg := args[i]
		key, val, hasInline := splitFlag(arg)
		nextValue := func() (string, error) {
			if hasInline {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", arg)
			}
			i++
			return args[i], nil
		}
		switch key {
		case "--pane":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Pane = v
		case "--source":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			opts.Source = v
			opts.SourceSeen = true
		case "--seq":
			v, err := nextValue()
			if err != nil {
				return opts, err
			}
			seq, err := parseSeqFlag(v, key)
			if err != nil {
				return opts, err
			}
			opts.Seq = seq
			opts.SeqSeen = true
		default:
			return opts, fmt.Errorf("unknown --release-agent flag: %s", arg)
		}
		i++
	}
	return opts, nil
}

func parseSeqFlag(raw, flag string) (int64, error) {
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq < 0 {
		return 0, fmt.Errorf("%s requires a non-negative integer", flag)
	}
	return seq, nil
}

func splitFlag(arg string) (key, val string, inline bool) {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i], arg[i+1:], true
	}
	return arg, "", false
}

func helpText() string {
	return `seshagy — minimal tmux session manager

Usage:
  seshagy                         open the Bubble Tea dashboard
  seshagy --get-all [--json]      print sessions, agents, zoxide dirs, fd dirs
  seshagy --get-sessions [--json] print tmux sessions
  seshagy --get-agents [--json]   print detected/tracked agent panes
  seshagy --get-current-session-agents [--json]
  seshagy --get-zoxide [--json]   print zoxide directories
  seshagy --get-fd [--json]       print fd directories
  seshagy --delete-item <line> [--json]
                                  kill a rendered session/agent line
  seshagy --report-agent [flags] [--json]
                                  set tmux pane @agent_* metadata
  seshagy --release-agent [flags] [--json]
                                  clear tmux pane @agent_* metadata
  seshagy agent explain <pane> [--json]
                                  show why a pane has its agent state
  seshagy manifest status [--json] show active manifest sources and update status
  seshagy manifest update [--json] fetch remote manifest catalog updates
  seshagy manifest reload [--json] re-read agent manifests from disk
  seshagy integration status [--json]
                                  list detected agents and hook status
  seshagy integration install pi [--json]
  seshagy integration uninstall pi [--json]
  seshagy config path [--json]    print config file path
  seshagy config show [--json]    print effective config
  seshagy config init [--force] [--json]
  seshagy --version [--json]

Scripting:
  Append --json to any command above for machine-readable output on stdout.
  Human text output is unchanged when --json is omitted.

TUI keys:
  enter attach/create/focus   q quit   / filter   r refresh   R rename
  x kill session/pane         y yazi   i hooks    m mode     1-6 modes
  p preview                   V session id expand   ? help

Config:
  Config lives at $XDG_CONFIG_HOME/seshagy/config.toml, or
  ~/.config/seshagy/config.toml when XDG_CONFIG_HOME is unset. It controls
  source order/default source, fd command, theme colors, Nerd Font icons, text
  label mode, no-icons mode, icon colors, type-first mode, and the action
  prefix key.
  In type-first mode, enter and arrow/page/home/end navigation keys do not need a prefix.

Agent flags:
  --pane <pane>               pane id; defaults to current tmux pane
  --agent/--name <name>       agent name when auto-detection is not enough
  --state/--status <state>    working|blocked|aborted|done|idle|unknown
  --message <text>            optional status message; empty clears
  --source <text>             optional owner/source token; empty clears
  --session-id <id>           optional native agent session id
  --seq <integer>             optional monotonic ordering token

Hook integrations:
  Supported targets: pi, claude, codex, copilot, droid, opencode, qodercli, cursor, kimi, grok, kilo, hermes.
  The TUI prompts to install missing hooks when new integrations are available
  (first launch or after upgrading seshagy). Press i anytime or use this
  integration command. Pi, OpenCode, Kimi Code, Kilo Code, and Hermes install
  plugin integrations that report lifecycle state directly. Claude, Codex,
  Copilot, Droid, Qoder CLI, Cursor, and Grok install shell-hook integrations
  that map hook events to working/blocked/idle lifecycle state. Hook-capable
  agents are not listed from process detection alone; install the integration
  so hooks report @agent_* metadata.
`
}
