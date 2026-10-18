package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestStartupInstallMenuShowsOnFirstRun(t *testing.T) {
	m := newTestModel(t)
	msg := startupInstallMenuCmd(m.config)()
	im, ok := msg.(installMenuMsg)
	if !ok {
		t.Fatalf("startupInstallMenuCmd = %#v, %T", msg, msg)
	}
	if !im.show {
		t.Fatalf("expected installMenuMsg{show:true} on first run, got %+v", im)
	}
	model, _ := m.Update(im)
	mm := model.(Model)
	if !mm.installMenu.active {
		t.Fatal("Update(installMenuMsg{show:true}) did not open the install menu")
	}
	if mm.installMenu.message == "" {
		t.Fatal("first-run install menu should have a message")
	}
}

func TestStartupInstallMenuSkipsAfterSeen(t *testing.T) {
	m := newTestModel(t)
	m.config.Setup.InstallMenuSeen = true
	im, ok := startupInstallMenuCmd(m.config)().(installMenuMsg)
	if !ok {
		t.Fatal("startupInstallMenuCmd returned the wrong message type")
	}
	if im.show {
		t.Fatal("expected installMenuMsg{show:false} after seen")
	}
}

func TestCloseInstallMenuPersistsSeenFlag(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(true)
	if m.config.Setup.InstallMenuSeen {
		t.Fatal("InstallMenuSeen should be false before closing")
	}
	model, cmd := m.closeInstallMenu()
	mm := model.(Model)
	if mm.installMenu.active {
		t.Fatal("closeInstallMenu did not deactivate the menu")
	}
	if !mm.config.Setup.InstallMenuSeen {
		t.Fatal("closeInstallMenu did not persist InstallMenuSeen=true")
	}
	if cmd == nil {
		t.Fatal("closing the menu did not reload sources")
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after close: %v", err)
	}
	if !loaded.Setup.InstallMenuSeen {
		t.Fatal("InstallMenuSeen was not written to the config file")
	}
}

func TestInstallMenuKeyNavigationAndDispatch(t *testing.T) {
	m := newTestModel(t)
	m.openInstallMenu(false)
	names := integrations.Available()
	if len(names) == 0 {
		t.Fatal("no integrations are registered")
	}

	model, _ := m.handleInstallMenuKey(downMsg())
	m = model.(Model)
	if m.installMenu.cursor != wrapCursorDown(0, len(names)) {
		t.Fatalf("down: cursor = %d", m.installMenu.cursor)
	}
	model, _ = m.handleInstallMenuKey(upMsg())
	m = model.(Model)
	if m.installMenu.cursor != 0 {
		t.Fatalf("up: cursor = %d, want 0", m.installMenu.cursor)
	}

	model, cmd := m.handleInstallMenuKey(enterMsg())
	m = model.(Model)
	if cmd == nil {
		t.Fatal("enter did not dispatch an install")
	}
	if m.installMenu.statuses[names[0]] != "installing" {
		t.Fatalf("statuses = %#v", m.installMenu.statuses)
	}

	model, cmd = m.handleInstallMenuKey(runeMsg("u"))
	m = model.(Model)
	if cmd == nil || m.installMenu.statuses[names[0]] != "uninstalling" {
		t.Fatalf("u: cmd = %v statuses = %#v", cmd, m.installMenu.statuses)
	}

	model, _ = m.handleInstallMenuKey(escMsg())
	m = model.(Model)
	if m.installMenu.active {
		t.Fatal("esc did not close the install menu")
	}
}

func TestInstallMenuInstallAllBatch(t *testing.T) {
	m := newTestModel(t)
	m.openInstallMenu(false)
	names := integrations.Available()
	model, cmd := m.handleInstallMenuKey(runeMsg("a"))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("a (install all): expected a non-nil cmd (tea.Batch)")
	}
	for _, name := range names {
		if m.installMenu.statuses[name] != "installing" {
			t.Fatalf("a: statuses[%s] = %q, want installing", name, m.installMenu.statuses[name])
		}
	}
}

func TestInstallResultMsgUpdatesStatus(t *testing.T) {
	m := newTestModel(t)
	m.openInstallMenu(false)
	names := integrations.Available()

	model, _ := m.Update(installResultMsg{name: names[0], action: "install"})
	m = model.(Model)
	if m.installMenu.statuses[names[0]] != "installed" {
		t.Fatalf("statuses = %#v", m.installMenu.statuses)
	}

	model, _ = m.Update(installResultMsg{name: names[0], action: "uninstall"})
	m = model.(Model)
	if m.installMenu.statuses[names[0]] != "uninstalled" {
		t.Fatalf("statuses = %#v", m.installMenu.statuses)
	}

	model, _ = m.Update(
		installResultMsg{name: names[0], action: "install", err: errSessionsUnavailable},
	)
	m = model.(Model)
	if m.installMenu.statuses[names[0]] != "failed" {
		t.Fatalf("statuses = %#v", m.installMenu.statuses)
	}
	if m.installMenu.message == "" {
		t.Fatal("failed install did not set a message")
	}
}

func TestCtrlGOpensTheInstallMenuFromThePicker(t *testing.T) {
	m := newTestModel(t)
	var cmd tea.Cmd
	m, cmd = press(t, m, "ctrl+g")
	if !m.installMenu.active {
		t.Fatal("ctrl+g did not open the install menu")
	}
	if cmd != nil {
		t.Fatalf("ctrl+g returned a command: %v", cmd)
	}
}

func TestInstallMenuViewRendersIntegrationRows(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.openInstallMenu(true)
	out := m.View()
	for _, want := range []string{"Install agent integrations", integrations.Available()[0]} {
		if !strings.Contains(out, want) {
			t.Fatalf("install menu missing %q:\n%s", want, out)
		}
	}
}

// TestInitSchedulesTheStartupInstallMenu runs Init's batch the way the runtime
// does, so a fresh config pops the menu through the real startup path instead of
// only when the command is called directly.
func TestInitSchedulesTheStartupInstallMenu(t *testing.T) {
	tests := []struct {
		name string
		seen bool
		want bool
	}{
		{"fresh config pops the menu", false, true},
		{"config that has seen the menu stays closed", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.config.Setup.InstallMenuSeen = tt.seen
			// The catalog refresh must not reach the network from a test.
			m.config.Agents.CatalogURL = "https://127.0.0.1:1/manifest.toml"
			msg := firstInitMsg[installMenuMsg](t, m.Init())
			if msg.show != tt.want {
				t.Fatalf("Init's install-menu msg = %+v, want show=%v", msg, tt.want)
			}
		})
	}
}

// TestInstallMenuNamesTheIntegrationsAsTmuxOnly pins the menu's caveat on both
// backends: the hooks report state to tmux only, and the herdr backend keeps the
// menu (and the note) visible because it detects agent state itself.
func TestInstallMenuNamesTheIntegrationsAsTmuxOnly(t *testing.T) {
	tests := []struct {
		name string
		mux  *fakeMux
	}{
		{"tmux", newFakeMux()},
		{"herdr", func() *fakeMux {
			mux := newFakeMux()
			mux.kind = sessionmgr.BackendHerdr
			mux.terms = sessionmgr.HerdrTerms()
			return mux
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWith(t, tt.mux)
			m.width, m.height = 100, 30
			m.openInstallMenu(true)
			note := m.integrationsNote()
			if !strings.HasPrefix(note, "tmux only") {
				t.Fatalf("note = %q, want it to say the integrations are tmux-only", note)
			}
			if !strings.Contains(m.View(), note) {
				t.Fatalf("install menu does not render the note %q:\n%s", note, m.View())
			}
			if !m.installMenu.active {
				t.Fatal("the notice must not close the install menu")
			}
		})
	}
}

// firstInitMsg runs Init's batch the way the tea runtime does — every command
// concurrently — and returns the first message of type T. Commands that block
// (the ticks) are left running; the buffered channel keeps their send from
// leaking.
func firstInitMsg[T any](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	var zero T
	if cmd == nil {
		t.Fatal("Init() returned no commands")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init() produced %T, want tea.BatchMsg", cmd())
	}
	msgs := make(chan tea.Msg, len(batch))
	for _, sub := range batch {
		go func() { msgs <- sub() }()
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-msgs:
			if typed, ok := msg.(T); ok {
				return typed
			}
		case <-deadline:
			t.Fatalf("Init() never produced %T", zero)
		}
	}
}
