package tui

import (
	"testing"

	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/integrations"
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
	msg := startupInstallMenuCmd(m.config)()
	im, ok := msg.(installMenuMsg)
	if !ok {
		t.Fatalf("startupInstallMenuCmd = %#v, %T", msg, msg)
	}
	if im.show {
		t.Fatalf("expected installMenuMsg{show:false} after seen, got %+v", im)
	}
}

func TestCloseInstallMenuPersistsSeenFlag(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(true)
	if m.config.Setup.InstallMenuSeen {
		t.Fatal("InstallMenuSeen should be false before closing")
	}
	model, _ := m.closeInstallMenu()
	mm := model.(Model)
	if mm.installMenu.active {
		t.Fatal("closeInstallMenu did not deactivate the menu")
	}
	if !mm.config.Setup.InstallMenuSeen {
		t.Fatal("closeInstallMenu did not persist InstallMenuSeen=true")
	}
	loaded, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load() after close: %v", err)
	}
	if !loaded.Setup.InstallMenuSeen {
		t.Fatal("persisted config does not have InstallMenuSeen=true")
	}
}

func TestInstallMenuKeyNavigation(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(false)
	names := integrations.Available()
	if len(names) < 2 {
		t.Skip("need at least 2 integrations for navigation test")
	}
	model, _ := m.handleInstallMenuKey(keyMsg("down"))
	mm := model.(Model)
	if mm.installMenu.cursor != 1 {
		t.Fatalf("down: expected cursor 1, got %d", mm.installMenu.cursor)
	}
	// Navigate to the last item then wrap.
	for i := 1; i < len(names)-1; i++ {
		model, _ = mm.handleInstallMenuKey(keyMsg("down"))
		mm = model.(Model)
	}
	if mm.installMenu.cursor != len(names)-1 {
		t.Fatalf("down: expected cursor %d, got %d", len(names)-1, mm.installMenu.cursor)
	}
	model, _ = mm.handleInstallMenuKey(keyMsg("down"))
	mm = model.(Model)
	if mm.installMenu.cursor != 0 {
		t.Fatalf("down wrap: expected cursor 0, got %d", mm.installMenu.cursor)
	}
	model, _ = mm.handleInstallMenuKey(keyMsg("k"))
	mm = model.(Model)
	if mm.installMenu.cursor != len(names)-1 {
		t.Fatalf("k (up wrap): expected cursor %d, got %d", len(names)-1, mm.installMenu.cursor)
	}
}

func TestInstallMenuEnterDispatchesInstall(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(false)
	names := integrations.Available()
	name := names[m.installMenu.cursor]
	model, cmd := m.handleInstallMenuKey(keyMsg("enter"))
	mm := model.(Model)
	if mm.installMenu.statuses[name] != "installing" {
		t.Fatalf(
			"enter: expected statuses[%s]=installing, got %q",
			name,
			mm.installMenu.statuses[name],
		)
	}
	if cmd == nil {
		t.Fatal("enter: expected a non-nil command (installIntegrationCmd)")
	}
}

func TestInstallMenuEscClosesMenu(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(false)
	model, _ := m.handleInstallMenuKey(keyMsg("esc"))
	mm := model.(Model)
	if mm.installMenu.active {
		t.Fatal("esc did not close the install menu")
	}
}

func TestInstallResultMsgUpdatesStatus(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(false)
	m.installMenu.statuses["codex"] = "installing"
	model, _ := m.Update(installResultMsg{name: "codex", action: "install", err: nil})
	mm := model.(Model)
	if mm.installMenu.statuses["codex"] != "installed" {
		t.Fatalf(
			"installResultMsg success: expected installed, got %q",
			mm.installMenu.statuses["codex"],
		)
	}
	model, _ = mm.Update(installResultMsg{name: "codex", action: "install", err: errSentinel})
	mm = model.(Model)
	if mm.installMenu.statuses["codex"] != "failed" {
		t.Fatalf(
			"installResultMsg failure: expected failed, got %q",
			mm.installMenu.statuses["codex"],
		)
	}
}

var errSentinel = sentinelErr{}

type sentinelErr struct{}

func (sentinelErr) Error() string { return "boom" }

func TestHandleActionKeyHOpensMenu(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = false
	model, cmd := m.handleActionKey(keyMsg("h"))
	mm := model.(Model)
	if !mm.installMenu.active {
		t.Fatal("h: expected install menu to open")
	}
	if cmd != nil {
		t.Fatalf("h: expected nil cmd, got %v", cmd)
	}
}

func TestHDoesNotToggleHelp(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = false
	m.showHelp = false
	model, _ := m.handleActionKey(keyMsg("h"))
	mm := model.(Model)
	if mm.showHelp {
		t.Fatal("h should NOT toggle showHelp anymore")
	}
	if !mm.installMenu.active {
		t.Fatal("h should open the install menu")
	}
}

func TestQuestionMarkStillTogglesHelp(t *testing.T) {
	m := newTestModel(t)
	m.config.TypeFirst.Enabled = false
	m.showHelp = false
	model, _ := m.handleActionKey(keyMsg("?"))
	mm := model.(Model)
	if !mm.showHelp {
		t.Fatal("? should still toggle showHelp on")
	}
}

func TestInstallMenuFirstRunDeferredUntilSetupCloses(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	// Simulate setup active when installMenuMsg arrives.
	m.setup.active = true
	model, cmd := m.Update(installMenuMsg{show: true})
	mm := model.(Model)
	if mm.installMenu.active {
		t.Fatal("install menu should not open while setup is active")
	}
	if !mm.pendingInstall {
		t.Fatal("pendingInstall should be set when installMenuMsg arrives during setup")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd during setup deferral, got %v", cmd)
	}
}

func TestInstallMenuDeferredOpenFiresAfterSetupDismiss(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	// Set up: setup active, pendingInstall true.
	m.setup.active = true
	m.pendingInstall = true
	// Simulate a setup-dismiss key (esc cancels the type-first setup prompt).
	model, _ := m.Update(keyMsg("esc"))
	mm := model.(Model)
	if mm.setup.active {
		t.Fatal("setup should be deactivated after esc")
	}
	if !mm.installMenu.active {
		t.Fatal("install menu should open after setup dismisses when pendingInstall is set")
	}
	if mm.pendingInstall {
		t.Fatal("pendingInstall should be cleared after opening")
	}
}

func TestInstallMenuInstallAllBatch(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 32
	m.openInstallMenu(false)
	names := integrations.Available()
	model, cmd := m.handleInstallMenuKey(keyMsg("a"))
	mm := model.(Model)
	if cmd == nil {
		t.Fatal("a (install all): expected a non-nil cmd (tea.Batch)")
	}
	for _, name := range names {
		if mm.installMenu.statuses[name] != "installing" {
			t.Fatalf(
				"a: expected statuses[%s]=installing, got %q",
				name,
				mm.installMenu.statuses[name],
			)
		}
	}
}
