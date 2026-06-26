package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
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

func startupSetupCmd(cfg appconfig.Config) tea.Cmd {
	return func() tea.Msg {
		if cfg.Setup.TypeFirstPromptSeen {
			return setupMsg{}
		}
		return setupMsg{prompt: true}
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
			preview = "no preview available"
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
		return actionDoneMsg{status: "killed session " + item.Name, err: err}
	}
}

func renameCmd(mux sessionmgr.Multiplexer, target, displayName, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := mux.RenameSession(ctx, target, newName)
		return actionDoneMsg{
			status: fmt.Sprintf("renamed %s to %s", displayName, newName),
			err:    err,
		}
	}
}

func renameAgentCmd(mux sessionmgr.Multiplexer, agentType, session, displayName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item := sessionmgr.Item{Kind: sessionmgr.KindAgent, AgentName: agentType, Session: session}
		err := mux.RenameAgent(ctx, item, displayName)
		verb := "renamed agent"
		if displayName == "" {
			verb = "cleared agent alias"
		}
		return actionDoneMsg{status: fmt.Sprintf("%s %s", verb, agentType), err: err}
	}
}

const tickInterval = 5 * time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// tickIntervalFor returns the tick interval for a source. Agents mode uses 1s
// for near-instant state-detection; all other modes use the default 5s.
func tickIntervalFor(source sessionmgr.SourceMode) time.Duration {
	if source == sessionmgr.ModeAgents {
		return 1 * time.Second
	}
	return tickInterval
}
