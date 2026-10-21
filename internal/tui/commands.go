package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// Timeouts. A saved machine is reached over ssh, so a command that targets one
// pays a round trip the local socket does not; the remote budgets are the same
// shapes, a little larger. They stay small on purpose: herdr kills its own ssh
// child when the budget runs out, but a forked ssh can outlive it for a moment,
// so the picker never waits on a remote server for long.
const (
	localRefreshTimeout  = 8 * time.Second
	remoteRefreshTimeout = 10 * time.Second
	localPreviewTimeout  = 2 * time.Second
	remotePreviewTimeout = 6 * time.Second
	localActionTimeout   = 5 * time.Second
	remoteActionTimeout  = 8 * time.Second
	// machinesTimeout bounds the saved-machine catalog read: it is a local file
	// read, and the budget only guards against a wedged herdr binary.
	machinesTimeout = 2 * time.Second
)

// itemTimeout picks the budget for an item's server: a remote herdr command pays
// an ssh round trip the local socket does not.
func itemTimeout(local, remote time.Duration, machine string) time.Duration {
	if machine == "" {
		return local
	}
	return remote
}

func refreshCmd(
	mux sessionmgr.Multiplexer,
	source sessionmgr.SourceMode,
	gen uint64,
	machine string,
	opts sessionmgr.LoadOptions,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localRefreshTimeout, remoteRefreshTimeout, machine),
		)
		defer cancel()
		result, err := sessionmgr.LoadWithBackend(ctx, mux, source, opts)
		// Backstop: flip done→idle for agent panes the user has navigated to
		// directly in tmux (bypassing seshagy's Enter-focus path). Runs only in
		// agents mode and only issues a tmux call when a done agent exists.
		if source == sessionmgr.ModeAgents && err == nil {
			mux.MarkActiveDoneAgentsIdle(ctx, result.Items)
		}
		msg := refreshMsg{
			source:  source,
			gen:     gen,
			machine: machine,
			items:   result.Items,
			warning: result.Warning,
			err:     err,
		}
		return msg
	}
}

// machinesCmd reads the saved herdr machines. It is a local file read (~2ms, no
// server, no ssh), so it runs at startup and on every explicit reload; the
// catalog only changes when the user runs `herdr machine add`.
func machinesCmd(mux sessionmgr.Multiplexer) tea.Cmd {
	return func() tea.Msg {
		selector, ok := mux.(sessionmgr.MachineSelector)
		if !ok {
			return machinesMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), machinesTimeout)
		defer cancel()
		machines, err := selector.Machines(ctx)
		return machinesMsg{machines: machines, err: err}
	}
}

type installMenuMsg struct {
	show bool
}

// agentStartTimesCmd resolves when the agents in items started: the agent pid
// per pane (a CLI call under herdr, already known under tmux) and then ONE
// batched ps call for every pid. Failures resolve nothing rather than erroring:
// the row simply shows no age.
func agentStartTimesCmd(mux sessionmgr.Multiplexer, items []sessionmgr.Item) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		panePIDs := make(map[string]int, len(items))
		pids := make([]int, 0, len(items))
		for _, item := range items {
			pid := item.AgentPID
			if pid <= 0 {
				resolved, err := mux.AgentPID(ctx, item)
				if err != nil || resolved <= 0 {
					continue
				}
				pid = resolved
			}
			panePIDs[agentStartKey(item)] = pid
			pids = append(pids, pid)
		}
		if len(pids) == 0 {
			return agentStartedMsg{}
		}
		starts, err := sessionmgr.ProcessStartTimes(ctx, pids, time.Now())
		if err != nil {
			return agentStartedMsg{}
		}
		resolved := make(map[string]agentStart, len(panePIDs))
		for pane, pid := range panePIDs {
			if started, ok := starts[pid]; ok {
				resolved[pane] = agentStart{pid: pid, started: started}
			}
		}
		return agentStartedMsg{starts: resolved}
	}
}

type installResultMsg struct {
	name   string
	action string
	err    error
}

// catalogRefreshMsg carries the result of the async manifest-catalog refresh.
type catalogRefreshMsg struct {
	result sessionmgr.ManifestFetchResult
	err    error
}

// refreshCatalogsCmd fetches manifest updates from the herdr catalog in the
// background. Non-blocking: the TUI renders immediately with bundled/cached
// manifests; when this completes the in-memory cache is reloaded.
func refreshCatalogsCmd(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := sessionmgr.FetchManifestUpdates(ctx, cfg.CatalogURL())
		return catalogRefreshMsg{result: result, err: err}
	}
}

func startupInstallMenuCmd(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg {
		return installMenuMsg{show: !cfg.Setup.InstallMenuSeen}
	}
}

func installIntegrationCmd(name, action string) tea.Cmd {
	return func() tea.Msg {
		if action == "uninstall" {
			_, err := integrations.Uninstall(name)
			return installResultMsg{name: name, action: action, err: err}
		}
		_, err := integrations.Install(name)
		return installResultMsg{name: name, action: action, err: err}
	}
}

const noPreviewAvailableText = "no preview available"

// loadRemoteHosts resolves the picker's host list: the configured [[hosts]]
// first, then the hosts already defined in ~/.ssh/config. It reads local files
// only. It is a seam so tests do not depend on the developer's ssh config.
var loadRemoteHosts = func(cfg appconfig.Config) []remote.Host {
	return remote.Hosts(cfg.RemoteHosts(), cfg.RemoteSSHConfigHosts())
}

// hostsCmd loads the remote hosts the picker offers. Hosts are read once at
// startup and again on an explicit reload; there is no poll because they cannot
// change without an edit to config.toml or ~/.ssh/config.
func hostsCmd(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg { return hostsMsg{hosts: loadRemoteHosts(cfg)} }
}

// hostItem is the picker's row for a remote host: the name is the title, the
// ssh alias is the target herdr is handed, and the transport plus the raw target
// fill the meta columns and the fuzzy-search fields.
func hostItem(host remote.Host) sessionmgr.Item {
	return sessionmgr.Item{
		Kind:     sessionmgr.KindHost,
		Name:     host.Name,
		Target:   host.Alias(),
		Location: host.Transport,
		Path:     host.Target,
	}
}

func hostItems(hosts []remote.Host) []sessionmgr.Item {
	if len(hosts) == 0 {
		return nil
	}
	items := make([]sessionmgr.Item, 0, len(hosts))
	for _, host := range hosts {
		items = append(items, hostItem(host))
	}
	return items
}

// switchHostCmd asks the `seshagy run` launcher attached to this herdr server to
// switch its client to host. It must run inside a herdr pane, where
// HERDR_SOCKET_PATH identifies the server the launcher is attached to.
func switchHostCmd(host remote.Host) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := requestSwitch(ctx, remote.SwitchRequest{
			Host:    host.Name,
			Session: host.Session,
		}, os.Getenv("HERDR_SOCKET_PATH"))
		return hostSwitchMsg{err: err}
	}
}

// requestSwitch asks a running launcher to switch servers; tests replace it so
// they need no launcher socket.
var requestSwitch = remote.RequestSwitch

// runSSHWindow opens the tmux window that reaches a host; tests replace it.
var runSSHWindow = func(name, alias string) error {
	return sessionmgr.SSHWindowCommand(name, alias).Run()
}

// openSSHWindowCmd opens a new tmux window running `ssh <alias>` on the tmux
// backend, where there is no launcher that can switch servers in place.
func openSSHWindowCmd(host remote.Host) tea.Cmd {
	return func() tea.Msg {
		return hostWindowMsg{err: runSSHWindow(host.Name, host.Alias())}
	}
}

// hostPreviewCmd renders a host's connection details from local config, so the
// preview is instant and needs no network access.
func hostPreviewCmd(host remote.Host) tea.Cmd {
	key := hostItem(host).Key()
	preview := hostPreviewText(host)
	return func() tea.Msg { return previewMsg{key: key, preview: preview} }
}

func hostPreviewText(host remote.Host) string {
	line := func(label, value string) string { return fmt.Sprintf("%-11s%s", label, value) }
	lines := []string{
		line("transport", host.Transport),
		line("target", host.Target),
	}
	if host.Session != "" {
		lines = append(lines, line("session", host.Session))
	}
	lines = append(lines, line("alias", host.Alias()))
	if host.FromSSHConfig {
		return strings.Join(
			append(lines, "", "# defined in ~/.ssh/config"), "\n")
	}
	block, err := remote.RenderSSHConfig([]remote.Host{host})
	if err != nil {
		return strings.Join(
			append(lines, "", "cannot render ssh config: "+err.Error()), "\n")
	}
	return strings.Join(lines, "\n") + "\n\n" + block
}

func previewCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	key := item.Key()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localPreviewTimeout, remotePreviewTimeout, item.Machine),
		)
		defer cancel()
		var (
			preview string
			err     error
		)
		switch item.Kind {
		case sessionmgr.KindSession:
			preview, err = mux.CaptureSession(ctx, item.ActionTarget(), 160)
		case sessionmgr.KindAgent:
			if item.PaneID != "" {
				preview, err = mux.CaptureAgentPane(ctx, item.PaneID, 160)
			}
		case sessionmgr.KindZoxide, sessionmgr.KindFD:
			preview, err = sessionmgr.ListDirectoryPreview(ctx, item.Path, 160)
		}
		if strings.TrimSpace(preview) == "" && err == nil {
			preview = noPreviewAvailableText
		}
		return previewMsg{key: key, preview: preview, err: err}
	}
}

func attachExecCallback(err error) tea.Msg {
	return attachDoneMsg{err: err}
}

// errCommandTimeout is what a remote action reports when its budget runs out.
var errCommandTimeout = errors.New("timed out on the saved machine")

// runBounded starts cmd and waits for it at most timeout, killing the child when
// the budget runs out so a wedged ssh cannot pin a picker command forever.
func runBounded(cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		return errCommandTimeout
	}
}

// attachCmd opens an item. A local item hands the terminal to the multiplexer
// and the picker comes back when the user detaches; a remote item runs its
// `herdr --machine <id> focus` as a bounded, non-interactive call, because that
// command updates the remote server's focus and returns rather than moving this
// client.
func attachCmd(m Model, item sessionmgr.Item) tea.Cmd {
	cmd := m.itemMux(item).AttachOrSwitchCommand(item)
	if item.Machine != "" {
		return remoteActionCmd(cmd, item)
	}
	return tea.ExecProcess(cmd, attachExecCallback)
}

// focusAgentCmd focuses an agent pane. Locally it suspends the picker (the user
// lands on the focused pane); on a saved machine it is a plain bounded API call.
func focusAgentCmd(m Model, item sessionmgr.Item) tea.Cmd {
	cmd := m.itemMux(item).FocusAgentCommand(item)
	if item.Machine != "" {
		return remoteActionCmd(cmd, item)
	}
	return tea.ExecProcess(cmd, attachExecCallback)
}

// remoteActionCmd runs a herdr command against a saved machine without handing
// the terminal over, and reports which server it targeted so the toast can say
// that this client did not move.
func remoteActionCmd(cmd *exec.Cmd, item sessionmgr.Item) tea.Cmd {
	return func() tea.Msg {
		err := runBounded(cmd, remoteActionTimeout)
		return attachDoneMsg{err: err, machine: item.Machine, name: item.DisplayName()}
	}
}

func createSessionCmd(mux sessionmgr.Multiplexer, dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), localActionTimeout)
		defer cancel()
		item, created, err := mux.CreateSessionFromDir(ctx, dir)
		return createDoneMsg{item: item, created: created, err: err}
	}
}

func deleteSessionCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localActionTimeout, remoteActionTimeout, item.Machine),
		)
		defer cancel()
		err := mux.KillSession(ctx, item.ActionTarget())
		verbPast := mux.Terms().KillVerbPast
		noun := mux.Terms().SessionNoun
		return actionDoneMsg{
			kind:   actionKill,
			status: verbPast + " " + noun + " " + item.Name,
			err:    err,
		}
	}
}

func deleteAgentCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localActionTimeout, remoteActionTimeout, item.Machine),
		)
		defer cancel()
		err := mux.KillPane(ctx, item.PaneID)
		terms := mux.Terms()
		return actionDoneMsg{
			kind:   actionKill,
			status: terms.KillVerbPast + " " + terms.PaneNoun + " " + item.DisplayName(),
			err:    err,
		}
	}
}

func renameCmd(
	mux sessionmgr.Multiplexer,
	machine, target, displayName, newName string,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localActionTimeout, remoteActionTimeout, machine),
		)
		defer cancel()
		err := mux.RenameSession(ctx, target, newName)
		return actionDoneMsg{
			kind:   actionRename,
			status: fmt.Sprintf("renamed %s to %s", displayName, newName),
			err:    err,
		}
	}
}

func renameAgentCmd(
	mux sessionmgr.Multiplexer,
	machine, paneID, agentType, session, displayName string,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			itemTimeout(localActionTimeout, remoteActionTimeout, machine),
		)
		defer cancel()
		item := sessionmgr.Item{
			Kind:      sessionmgr.KindAgent,
			PaneID:    paneID,
			AgentName: agentType,
			Session:   session,
			Machine:   machine,
		}
		err := mux.RenameAgent(ctx, item, displayName)
		verb := "renamed agent"
		if displayName == "" {
			verb = "cleared agent alias"
		}
		return actionDoneMsg{
			kind:   actionAgentRename,
			status: fmt.Sprintf("%s %s", verb, agentType),
			err:    err,
		}
	}
}

type tickMsg time.Time

// tickCmd drives the source-refresh poll; every tick refreshes whichever
// sources are stale and not already in flight.
func tickCmd() tea.Cmd {
	return tea.Tick(refreshTickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
