package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/remote"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func refreshCmd(
	mux sessionmgr.Multiplexer,
	source sessionmgr.SourceMode,
	gen uint64,
	opts sessionmgr.LoadOptions,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
			items:   result.Items,
			warning: result.Warning,
			err:     err,
		}
		// Resolve the current multiplexer session once per agent refresh so
		// the current-session scope filter (toggled with 'o') can match
		// without a per-render call. Runs in this background goroutine; a
		// missing session (not in tmux/herdr) leaves currentSession empty.
		if source == sessionmgr.ModeAgents {
			if session, err := mux.CurrentSession(ctx); err == nil {
				msg.currentSession = session
			}
		}
		return msg
	}
}

type installMenuMsg struct {
	show bool
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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

func attachCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	return tea.ExecProcess(mux.AttachOrSwitchCommand(item), attachExecCallback)
}

// focusAgentCmd focuses an agent pane in the user's already-attached session.
// It runs switch-window + select-pane via tea.ExecProcess, mirroring attachCmd
// (the TUI exits so the user lands on the focused pane).
func focusAgentCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	return tea.ExecProcess(
		mux.FocusAgentCommand(item),
		attachExecCallback,
	)
}

func createSessionCmd(mux sessionmgr.Multiplexer, dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item, created, err := mux.CreateSessionFromDir(ctx, dir)
		return createDoneMsg{item: item, created: created, err: err}
	}
}

func deleteSessionCmd(mux sessionmgr.Multiplexer, item sessionmgr.Item) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func renameCmd(mux sessionmgr.Multiplexer, target, displayName, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	paneID, agentType, session, displayName string,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item := sessionmgr.Item{
			Kind:      sessionmgr.KindAgent,
			PaneID:    paneID,
			AgentName: agentType,
			Session:   session,
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
