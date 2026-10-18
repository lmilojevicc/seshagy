package tui

import (
	"context"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// fakeMux is an in-memory Multiplexer for picker tests: sources return the
// configured lists and actions record what they were asked to do.
type fakeMux struct {
	kind    sessionmgr.BackendKind
	terms   sessionmgr.Terms
	popup   bool
	current string

	sessions    []sessionmgr.Item
	agents      []sessionmgr.Item
	preview     string
	previewErr  error
	sessionsErr error
	agentsErr   error
	createdDir  string
	killed      []string
	renamed     []string
	visited     []string
	focused     sessionmgr.Item

	// agentPID is the pid AgentPID reports for panes that carry none; pidQueries
	// records every pane it was asked about (nil when unset).
	agentPID    int
	agentPIDErr error
	pidQueries  []string
}

func newFakeMux() *fakeMux {
	return &fakeMux{
		kind:  sessionmgr.BackendTmux,
		terms: sessionmgr.TmuxTerms(),
	}
}

func (f *fakeMux) Kind() sessionmgr.BackendKind { return f.kind }
func (f *fakeMux) Terms() sessionmgr.Terms      { return f.terms }
func (f *fakeMux) InMultiplexer() bool          { return true }

func (f *fakeMux) InMultiplexerPopup(context.Context) (bool, error) { return f.popup, nil }

func (f *fakeMux) CurrentSession(context.Context) (string, error) { return f.current, nil }

func (f *fakeMux) ListSessions(context.Context) ([]sessionmgr.Item, error) {
	return f.sessions, f.sessionsErr
}

func (f *fakeMux) HasSession(context.Context, string) (bool, error) { return false, nil }

func (f *fakeMux) CreateSessionFromDir(
	_ context.Context,
	dir string,
) (sessionmgr.Item, bool, error) {
	f.createdDir = dir
	return sessionmgr.Item{Kind: sessionmgr.KindSession, Name: "new", Target: dir}, true, nil
}

func (f *fakeMux) KillSession(_ context.Context, target string) error {
	f.killed = append(f.killed, target)
	return nil
}

func (f *fakeMux) KillPane(_ context.Context, paneID string) error {
	f.killed = append(f.killed, paneID)
	return nil
}

func (f *fakeMux) RenameSession(_ context.Context, target, newName string) error {
	f.renamed = append(f.renamed, target+"="+newName)
	return nil
}

func (f *fakeMux) CaptureSession(context.Context, string, int) (string, error) {
	return f.preview, f.previewErr
}

func (f *fakeMux) AttachOrSwitchCommand(sessionmgr.Item) *exec.Cmd {
	return exec.Command("/bin/true")
}

func (f *fakeMux) ListAgents(context.Context, string) ([]sessionmgr.Item, error) {
	return f.agents, f.agentsErr
}

func (f *fakeMux) AgentPID(_ context.Context, item sessionmgr.Item) (int, error) {
	f.pidQueries = append(f.pidQueries, item.PaneID)
	if item.AgentPID > 0 {
		return item.AgentPID, nil
	}
	return f.agentPID, f.agentPIDErr
}

func (f *fakeMux) CaptureAgentPane(context.Context, string, int) (string, error) {
	return f.preview, f.previewErr
}

func (f *fakeMux) FocusAgentCommand(item sessionmgr.Item) *exec.Cmd {
	f.focused = item
	return exec.Command("/bin/true")
}

func (f *fakeMux) RenameAgent(_ context.Context, item sessionmgr.Item, displayName string) error {
	f.renamed = append(f.renamed, item.PaneID+"="+displayName)
	return nil
}

func (f *fakeMux) ResolvePane(_ context.Context, pane string) (string, error) { return pane, nil }

func (f *fakeMux) ResolvePaneByCwd(context.Context, string) (string, error) { return "", nil }

func (f *fakeMux) ReportAgent(context.Context, sessionmgr.AgentReport) (bool, error) {
	return false, nil
}

func (f *fakeMux) ReleaseAgent(context.Context, sessionmgr.AgentRelease) (bool, error) {
	return false, nil
}

func (f *fakeMux) MarkAgentVisited(_ context.Context, paneID string) (bool, error) {
	f.visited = append(f.visited, paneID)
	return true, nil
}

func (f *fakeMux) MarkActiveDoneAgentsIdle(context.Context, []sessionmgr.Item) {}

// newTestModel returns a Model wired to a fake multiplexer at a fixed size.
func newTestModel(t *testing.T) Model {
	t.Helper()
	return newTestModelWithConfig(t, appconfig.Default())
}

// newTestModelWithConfig returns a Model at a fixed size for one config.
func newTestModelWithConfig(t *testing.T, cfg appconfig.Config) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HERDR_ENV", "")
	t.Setenv("SESHAGY_AGENT_ICONS", "none")
	m := New(WithConfig(cfg), WithMultiplexer(newFakeMux()))
	m.width, m.height = 120, 32
	return m
}

// remappedModel returns a Model whose [tui.keys] overrides are the given ones.
func remappedModel(t *testing.T, keys map[string]appconfig.KeyList) Model {
	t.Helper()
	cfg := appconfig.Default()
	cfg.TUI.Keys = keys
	return newTestModelWithConfig(t, cfg)
}

// defaultKeyTable is the picker's key table with no [tui.keys] overrides.
func defaultKeyTable() KeyTable { return ResolveKeys(nil) }

func newTestModelWith(t *testing.T, mux sessionmgr.Multiplexer) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HERDR_ENV", "")
	t.Setenv("SESHAGY_AGENT_ICONS", "none")
	m := New(WithConfig(appconfig.Default()), WithMultiplexer(mux))
	m.width, m.height = 120, 32
	return m
}

func runeMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyMsg(key tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: key}
}

func enterMsg() tea.KeyMsg     { return keyMsg(tea.KeyEnter) }
func escMsg() tea.KeyMsg       { return keyMsg(tea.KeyEscape) }
func tabMsg() tea.KeyMsg       { return keyMsg(tea.KeyTab) }
func shiftTabMsg() tea.KeyMsg  { return keyMsg(tea.KeyShiftTab) }
func upMsg() tea.KeyMsg        { return keyMsg(tea.KeyUp) }
func downMsg() tea.KeyMsg      { return keyMsg(tea.KeyDown) }
func backspaceMsg() tea.KeyMsg { return keyMsg(tea.KeyBackspace) }
func deleteMsg() tea.KeyMsg    { return keyMsg(tea.KeyDelete) }

// keyStringMsg builds the message bubbletea produces for a named key press.
func keyStringMsg(name string) tea.KeyMsg {
	switch name {
	case "esc":
		return escMsg()
	case "enter":
		return enterMsg()
	case "tab":
		return tabMsg()
	case "shift+tab":
		return shiftTabMsg()
	case "up":
		return upMsg()
	case "down":
		return downMsg()
	case "backspace":
		return backspaceMsg()
	case "delete":
		return deleteMsg()
	case "f1":
		return keyMsg(tea.KeyF1)
	case "f2":
		return keyMsg(tea.KeyF2)
	case "f9":
		return keyMsg(tea.KeyF9)
	case "ctrl+c":
		return keyMsg(tea.KeyCtrlC)
	case "ctrl+r":
		return keyMsg(tea.KeyCtrlR)
	case "ctrl+x":
		return keyMsg(tea.KeyCtrlX)
	case "ctrl+o":
		return keyMsg(tea.KeyCtrlO)
	case "ctrl+y":
		return keyMsg(tea.KeyCtrlY)
	case "ctrl+l":
		return keyMsg(tea.KeyCtrlL)
	case "ctrl+a":
		return keyMsg(tea.KeyCtrlA)
	case "ctrl+s":
		return keyMsg(tea.KeyCtrlS)
	case "ctrl+t":
		return keyMsg(tea.KeyCtrlT)
	case "ctrl+z":
		return keyMsg(tea.KeyCtrlZ)
	case "ctrl+f":
		return keyMsg(tea.KeyCtrlF)
	case "ctrl+e":
		return keyMsg(tea.KeyCtrlE)
	case "ctrl+g":
		return keyMsg(tea.KeyCtrlG)
	case "ctrl+u":
		return keyMsg(tea.KeyCtrlU)
	case "ctrl+w":
		return keyMsg(tea.KeyCtrlW)
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	default:
		return runeMsg(name)
	}
}

// press feeds a named key through Update and returns the resulting model.
func press(t *testing.T, m Model, name string) (Model, tea.Cmd) {
	t.Helper()
	model, cmd := m.Update(keyStringMsg(name))
	return model.(Model), cmd
}

func latestNotificationText(m Model) string {
	if len(m.notifications) == 0 {
		return ""
	}
	return m.notifications[len(m.notifications)-1].text
}

func latestNotificationSeverity(m Model) notifSev {
	if len(m.notifications) == 0 {
		return sevInfo
	}
	return m.notifications[len(m.notifications)-1].sev
}

// agentItem builds an idle agent item with a pane and workspace location.
func agentItem(name, paneID, location string) sessionmgr.Item {
	return sessionmgr.Item{
		Kind:       sessionmgr.KindAgent,
		AgentName:  name,
		PaneID:     paneID,
		Location:   location,
		Session:    location,
		Window:     "w1",
		AgentState: sessionmgr.AgentIdle,
	}
}

// withItems loads a fixed item list into the model as if every source had
// returned it.
func withItems(m Model, items []sessionmgr.Item) Model {
	m.cache = map[sessionmgr.SourceMode]modeCache{
		sessionmgr.ModeSessions: {items: items},
	}
	m.items = m.mergedItems()
	m.loadedOnce = true
	m.syncCursor()
	return m
}

// defaultTestConfig is appconfig.Default() normalized, for tests that tweak a
// single icon or theme field.
func defaultTestConfig() appconfig.Config {
	cfg := appconfig.Default()
	cfg.Normalize()
	return cfg
}

// withColor forces the lipgloss renderer into truecolor for style assertions,
// restoring the detected profile when the test ends.
func withColor(t *testing.T) {
	t.Helper()
	withProfile(t, termenv.TrueColor)
}

// withProfile forces the lipgloss renderer into a color profile, restoring the
// detected one when the test ends.
func withProfile(t *testing.T, profile termenv.Profile) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(profile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}
