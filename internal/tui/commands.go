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

func refreshCmd(source sessionmgr.SourceMode, gen uint64, opts sessionmgr.LoadOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		result, err := sessionmgr.LoadWithOptions(ctx, source, opts)
		return refreshMsg{
			source:  source,
			gen:     gen,
			items:   result.Items,
			warning: result.Warning,
			err:     err,
		}
	}
}

func integrationsCmd() tea.Cmd {
	return func() tea.Msg {
		return integrationsMsg{recs: integrations.RecommendedForPrompt()}
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

func startupIntegrationsCmd() tea.Cmd {
	return func() tea.Msg {
		shouldPrompt, err := shouldStartupIntegrationPrompt()
		if err != nil {
			return integrationsMsg{err: fmt.Errorf("check startup hook prompt: %w", err)}
		}
		if !shouldPrompt {
			return integrationsMsg{}
		}
		return integrationsMsg{
			recs:    integrations.RecommendedForPrompt(),
			startup: true,
		}
	}
}

func installIntegrationsCmd(targets []integrations.Target) tea.Cmd {
	return func() tea.Msg {
		var messages []string
		for _, target := range targets {
			installed, err := integrations.Install(target)
			if err != nil {
				return integrationsInstalledMsg{messages: messages, err: err}
			}
			messages = append(messages, installed...)
		}
		return integrationsInstalledMsg{messages: messages}
	}
}

func previewCmd(item sessionmgr.Item) tea.Cmd {
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
			preview, err = sessionmgr.CaptureSession(ctx, item.Name, 160)
		case sessionmgr.KindAgent:
			preview, err = sessionmgr.CaptureAgentPane(ctx, item.PaneID, 160)
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

func attachCmd(name string) tea.Cmd {
	return tea.ExecProcess(sessionmgr.AttachOrSwitchCommand(name), attachExecCallback)
}

func focusAgentCmd(pane string) tea.Cmd {
	return tea.ExecProcess(sessionmgr.FocusAgentCommand(pane), attachExecCallback)
}

func createSessionCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name, created, err := sessionmgr.CreateSessionFromDir(ctx, dir)
		return createDoneMsg{name: name, created: created, err: err}
	}
}

func deleteSessionCmd(name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.KillSession(ctx, name)
		return actionDoneMsg{status: "killed session " + name, err: err}
	}
}

func deleteAgentCmd(pane string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.KillAgentPane(ctx, pane)
		return actionDoneMsg{status: "killed pane " + pane, err: err}
	}
}

func renameCmd(oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessionmgr.RenameSession(ctx, oldName, newName)
		return actionDoneMsg{status: fmt.Sprintf("renamed %s to %s", oldName, newName), err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}
