package tui

import (
	"errors"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

func TestStartupIntegrationPromptPreservesThroughFailedInstall(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := New()
	recs := []integrations.Recommendation{
		{
			Target:         integrations.TargetPi,
			AgentAvailable: true,
			Installable:    true,
			State:          integrations.StatusNotInstalled,
		},
	}

	model, _ := m.Update(integrationsMsg{recs: recs, startup: true})
	m = model.(Model)
	if !m.integration.startupPrompt {
		t.Fatal("startupPrompt = false, want true")
	}
	if !m.integration.active {
		t.Fatal("integration active = false, want true")
	}

	model, cmd := m.Update(integrationsInstalledMsg{err: errors.New("install failed")})
	m = model.(Model)
	if !m.integration.startupPrompt {
		t.Fatal("startupPrompt cleared after failed install")
	}
	if cmd == nil {
		t.Fatal("expected integrationsCmd after failed install")
	}
	rescanMsg, ok := cmd().(integrationsMsg)
	if !ok {
		t.Fatalf("integrationsCmd returned %T", cmd())
	}

	model, _ = m.Update(rescanMsg)
	m = model.(Model)
	if !m.integration.startupPrompt {
		t.Fatal("startupPrompt cleared after rescan following failed install")
	}

	model, _ = m.Update(integrationsInstalledMsg{messages: []string{"installed pi"}})
	m = model.(Model)
	if m.integration.startupPrompt {
		t.Fatal("startupPrompt should be cleared after success")
	}

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != integrations.CurrentInstallVersion() {
		t.Fatalf("stored = %d, want %d", stored, integrations.CurrentInstallVersion())
	}
}

func TestQuitDuringStartupIntegrationPromptRecordsDismissal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	m := New()
	m.integration.active = true
	m.integration.startupPrompt = true

	model, cmd := m.handleIntegrationKey(keyMsg("q"))
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected tea.Quit command")
	}
	if m.integration.startupPrompt {
		t.Fatal("startupPrompt should be cleared after quit")
	}

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != integrations.CurrentInstallVersion() {
		t.Fatalf("stored = %d, want %d", stored, integrations.CurrentInstallVersion())
	}
}
