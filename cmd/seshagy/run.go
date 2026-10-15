package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lmilojevicc/seshagy/internal/cli"
	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/remote"
)

// runCommand is a parsed `seshagy run` invocation.
type runCommand struct {
	host    string
	session string
}

// parseRunCommand parses `seshagy run [--session <name>] [<host>]`. The host
// may appear before the flag, so the flags are not handed to flag.FlagSet
// (which stops at the first positional argument). This is also why the
// completion parity test states the run flags explicitly.
func parseRunCommand(args []string) (runCommand, error) {
	var cmd runCommand
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--session":
			if i+1 >= len(args) {
				return runCommand{}, errors.New("--session requires a value")
			}
			cmd.session = args[i+1]
			i++
		case strings.HasPrefix(arg, "-"):
			return runCommand{}, fmt.Errorf("unknown flag: %s", arg)
		case cmd.host == "":
			cmd.host = arg
		default:
			return runCommand{}, errors.New(joinUsage("run", "[--session <name>]", "[<host>]"))
		}
	}
	return cmd, nil
}

// runRun runs the launcher that owns the terminal and can therefore switch the
// herdr client between the local server and any remote host.
func runRun(args []string) error {
	cmd, err := parseRunCommand(args)
	if err != nil {
		return err
	}
	return executeRun(cmd)
}

func executeRun(cmd runCommand) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	hosts := knownHosts(cfg)
	opts := remote.LaunchOptions{Hosts: hosts}
	if cmd.host == "" || cmd.host == "local" {
		// A bare --session selects the local herdr session, which is also the
		// session the launcher falls back to when a remote client exits.
		opts.Initial = remote.Target{Session: cmd.session}
		opts.LocalSession = cmd.session
		return remote.RunLauncher(context.Background(), opts)
	}
	host, ok := remote.Find(hosts, cmd.host)
	if !ok {
		return fmt.Errorf("unknown host: %s (see `seshagy hosts`)", cmd.host)
	}
	session := cmd.session
	if session == "" {
		session = host.Session
	}
	opts.Initial = remote.Target{Alias: host.Alias(), Session: session}
	return remote.RunLauncher(context.Background(), opts)
}

// knownHosts merges the configured [[hosts]] with the hosts discovered from
// ~/.ssh/config, warning about malformed entries.
func knownHosts(cfg appconfig.Config) []remote.Host {
	for _, problem := range cfg.ValidateRemoteHosts() {
		cli.Warn(problem)
	}
	return remote.Hosts(cfg.RemoteHosts(), cfg.RemoteSSHConfigHosts())
}

// runHosts dispatches `seshagy hosts`.
func runHosts(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) == 0 {
		return listHosts(jsonOutput)
	}
	switch rest[0] {
	case "setup":
		if len(rest) != 1 {
			return errors.New(joinUsage("hosts", "setup", "[--json]"))
		}
		return setupHosts(jsonOutput)
	case "switch":
		if len(rest) != 2 {
			return errors.New(joinUsage("hosts", "switch", "<name|local>", "[--json]"))
		}
		return switchHost(rest[1], jsonOutput)
	default:
		return errors.New(joinUsage("hosts", "[setup | switch <name|local>]", "[--json]"))
	}
}

func listHosts(jsonOutput bool) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	hosts := knownHosts(cfg)
	if jsonOutput {
		entries := make([]map[string]string, 0, len(hosts))
		for _, host := range hosts {
			entries = append(entries, map[string]string{
				"name":      host.Name,
				"target":    host.Target,
				"transport": host.Transport,
				"session":   host.Session,
				"alias":     host.Alias(),
				"source":    hostSource(host),
			})
		}
		return encodeSuccess(map[string]any{"hosts": entries})
	}
	for _, host := range hosts {
		cli.Println(formatHostLine(host))
	}
	return nil
}

// formatHostLine renders one host as the four columns `seshagy hosts` lists.
func formatHostLine(host remote.Host) string {
	line := strings.Join(
		[]string{host.Name, host.Transport, host.Target, hostSource(host)},
		"  ",
	)
	if host.Session != "" {
		line += "  session=" + host.Session
	}
	return line
}

func hostSource(host remote.Host) string {
	if host.FromSSHConfig {
		return "ssh_config"
	}
	return "config"
}

// setupHosts writes the generated ssh config and adds its Include line to
// ~/.ssh/config. It is the only path that touches the user's ssh config.
func setupHosts(jsonOutput bool) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}
	for _, problem := range cfg.ValidateRemoteHosts() {
		cli.Warn(problem)
	}
	changed, err := remote.WriteSSHConfig(cfg.RemoteHosts())
	if err != nil {
		return err
	}
	userConfig := remote.UserSSHConfigPath()
	added, err := remote.EnsureUserSSHConfigInclude()
	if err != nil {
		return err
	}
	if jsonOutput {
		return encodeSuccess(map[string]any{
			"ssh_config":    remote.SSHConfigPath(),
			"user_config":   userConfig,
			"changed":       changed,
			"include_added": added,
			"include_line":  remote.IncludeLine(),
		})
	}
	if changed {
		cli.Successf("wrote %s", remote.SSHConfigPath())
	} else {
		cli.Infof("%s is up to date", remote.SSHConfigPath())
	}
	if added {
		cli.Successf("added %q to %s", remote.IncludeLine(), userConfig)
	} else {
		cli.Infof("%s already includes the generated config", userConfig)
	}
	return nil
}

// switchHost asks the running launcher to switch its herdr client. It is called
// from inside a herdr pane, where HERDR_SOCKET_PATH identifies the server.
func switchHost(name string, jsonOutput bool) error {
	request := remote.SwitchRequest{}
	if name != "local" {
		request.Host = name
	}
	if err := remote.RequestSwitch(
		context.Background(),
		request,
		os.Getenv("HERDR_SOCKET_PATH"),
	); err != nil {
		return err
	}
	if jsonOutput {
		return encodeSuccess(map[string]any{"switched": true, "host": name})
	}
	cli.Successf("switching to %s", name)
	return nil
}
