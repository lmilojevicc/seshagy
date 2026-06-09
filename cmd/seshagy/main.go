package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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
		fmt.Println(version)
		return nil
	case "integration", "integrations", "hook", "hooks":
		return runIntegration(args[1:])
	case "--get-sessions":
		return printItems(ctx, sessionmgr.ModeSessions)
	case "--get-agents":
		return printItems(ctx, sessionmgr.ModeAgents)
	case "--get-current-session-agents", "--get-session-agents":
		return printItems(ctx, sessionmgr.ModeCurrentAgents)
	case "--get-zoxide":
		return printItems(ctx, sessionmgr.ModeZoxide)
	case "--get-fd":
		return printItems(ctx, sessionmgr.ModeFD)
	case "--get-all":
		return printItems(ctx, sessionmgr.ModeAll)
	case "--delete-item":
		if len(args) < 2 {
			return errors.New("--delete-item requires a rendered item line")
		}
		return deleteItem(ctx, strings.Join(args[1:], " "))
	case "--report-agent":
		report, err := parseReportArgs(args[1:])
		if err != nil {
			return err
		}
		return sessionmgr.ReportAgent(ctx, report)
	case "--release-agent":
		pane, source, sourceSeen, err := parseReleaseArgs(args[1:])
		if err != nil {
			return err
		}
		return sessionmgr.ReleaseAgent(ctx, pane, source, sourceSeen)
	default:
		return tui.Run()
	}
}

func runIntegration(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		for _, rec := range integrations.Scan() {
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
			fmt.Printf("%-18s %-9s %-18s %s\n", rec.Target, availability, state, rec.InstallPath)
		}
		return nil
	}
	if len(args) != 2 || (args[0] != "install" && args[0] != "uninstall") {
		return errors.New("usage: seshagy integration status|install <target>|uninstall <target>")
	}
	target, err := integrations.ParseTarget(args[1])
	if err != nil {
		return err
	}
	var messages []string
	if args[0] == "install" {
		messages, err = integrations.Install(target)
	} else {
		messages, err = integrations.Uninstall(target)
	}
	if err != nil {
		return err
	}
	for _, message := range messages {
		fmt.Println(message)
	}
	return nil
}

func printItems(ctx context.Context, mode sessionmgr.SourceMode) error {
	items, err := sessionmgr.Load(ctx, mode)
	if err != nil {
		return err
	}
	for _, item := range items {
		fmt.Println(sessionmgr.FormatLine(item))
	}
	return nil
}

func deleteItem(ctx context.Context, raw string) error {
	item, ok := sessionmgr.ParseActionLine(raw)
	if !ok {
		return nil
	}
	switch item.Kind {
	case sessionmgr.KindSession:
		return sessionmgr.KillSession(ctx, item.Name)
	case sessionmgr.KindAgent:
		return sessionmgr.KillAgentPane(ctx, item.PaneID)
	default:
		return nil
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
		default:
			return opts, fmt.Errorf("unknown --report-agent flag: %s", arg)
		}
		i++
	}
	return opts, nil
}

func parseReleaseArgs(args []string) (pane, source string, sourceSeen bool, err error) {
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
			pane, err = nextValue()
			if err != nil {
				return "", "", false, err
			}
		case "--source":
			source, err = nextValue()
			if err != nil {
				return "", "", false, err
			}
			sourceSeen = true
		default:
			return "", "", false, fmt.Errorf("unknown --release-agent flag: %s", arg)
		}
		i++
	}
	return pane, source, sourceSeen, nil
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
  seshagy --get-all               print sessions, agents, zoxide dirs, fd dirs
  seshagy --get-sessions          print tmux sessions
  seshagy --get-agents            print detected/tracked agent panes
  seshagy --get-current-session-agents
  seshagy --get-zoxide            print zoxide directories
  seshagy --get-fd                print fd directories
  seshagy --delete-item <line>    kill a rendered session/agent line
  seshagy --report-agent [flags]  set tmux pane @agent_* metadata
  seshagy --release-agent [flags] clear tmux pane @agent_* metadata
  seshagy integration status      list detected agents and hook status
  seshagy integration install pi  install one hook/plugin integration
  seshagy integration uninstall pi

TUI keys:
  enter attach/create/focus   q quit   / filter   r refresh   R rename
  x kill session/pane         y yazi   p preview  ? help      1-5 modes

Agent flags:
  --pane <pane>               pane id; defaults to current tmux pane
  --agent/--name <name>       agent name when auto-detection is not enough
  --state/--status <state>    working|blocked|aborted|done|idle|unknown
  --message <text>            optional status message; empty clears
  --source <text>             optional owner/source token; empty clears

Hook integrations:
  Supported targets: pi, claude, codex, copilot, droid, opencode, qodercli, cursor.
  The TUI asks before installing missing hooks for detected agents. Hooks report
  state directly with --report-agent; seshagy no longer infers agent state by
  inspecting foreground process names or pane text.
`
}
