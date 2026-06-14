package tui

import (
	"os"
	"testing"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

func TestPromptVersionStateFirstLaunch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	stored, firstLaunch, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != 0 {
		t.Fatalf("stored = %d, want 0", stored)
	}
	if !firstLaunch {
		t.Fatal("firstLaunch = false, want true")
	}
}

func TestPromptVersionStateLegacySeenFile(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	if err := os.MkdirAll(stateHome+"/seshagy", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(integrationPromptSeenPath(), []byte("seen\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stored, firstLaunch, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != 0 {
		t.Fatalf("stored = %d, want 0 for legacy seen file", stored)
	}
	if firstLaunch {
		t.Fatal("firstLaunch = true, want false for legacy seen file")
	}
}

func TestWriteIntegrationPromptVersionRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := writeIntegrationPromptVersion(3); err != nil {
		t.Fatalf("writeIntegrationPromptVersion error: %v", err)
	}

	stored, firstLaunch, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != 3 {
		t.Fatalf("stored = %d, want 3", stored)
	}
	if firstLaunch {
		t.Fatal("firstLaunch = true, want false after writing version")
	}
}

func TestShouldStartupIntegrationPromptFirstLaunchRecordsCurrentVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	should, err := shouldStartupIntegrationPrompt()
	if err != nil {
		t.Fatalf("shouldStartupIntegrationPrompt error: %v", err)
	}
	recs := integrations.RecommendedForPrompt()
	if len(recs) > 0 {
		if !should {
			t.Fatal("first launch with recommendations should prompt")
		}
		return
	}
	if should {
		t.Fatal("first launch without recommendations should not prompt")
	}

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != integrations.CurrentInstallVersion() {
		t.Fatalf("stored = %d, want %d", stored, integrations.CurrentInstallVersion())
	}
}

func TestShouldStartupIntegrationPromptSkipsAtCurrentVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := writeIntegrationPromptVersion(integrations.CurrentInstallVersion()); err != nil {
		t.Fatal(err)
	}

	should, err := shouldStartupIntegrationPrompt()
	if err != nil {
		t.Fatalf("shouldStartupIntegrationPrompt error: %v", err)
	}
	if should {
		t.Fatal("startup prompt should be skipped when already recorded for current version")
	}
}

func TestShouldStartupIntegrationPromptUpgradeDoesNotBumpWithoutPrompt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	oldVersion := integrations.CurrentInstallVersion() - 1
	if oldVersion < 1 {
		t.Fatalf(
			"CurrentInstallVersion() = %d, need at least 2 for upgrade test",
			integrations.CurrentInstallVersion(),
		)
	}
	if err := writeIntegrationPromptVersion(oldVersion); err != nil {
		t.Fatal(err)
	}

	should, err := shouldStartupIntegrationPrompt()
	if err != nil {
		t.Fatalf("shouldStartupIntegrationPrompt error: %v", err)
	}
	recs := integrations.RecommendedForPrompt()
	if len(recs) > 0 && !should {
		t.Fatal("upgrade with outdated integrations should allow startup prompt")
	}
	if len(recs) == 0 && should {
		t.Fatal("upgrade without outdated integrations should not prompt")
	}

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != oldVersion {
		t.Fatalf("stored = %d, want %d without completing prompt", stored, oldVersion)
	}
}

func TestDismissStartupIntegrationPromptWithoutRecordingVersion(t *testing.T) {
	for _, key := range []string{"esc", "s"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			oldVersion := integrations.CurrentInstallVersion() - 1
			if oldVersion < 1 {
				t.Fatalf(
					"CurrentInstallVersion() = %d, need at least 2 for upgrade test",
					integrations.CurrentInstallVersion(),
				)
			}
			if err := writeIntegrationPromptVersion(oldVersion); err != nil {
				t.Fatal(err)
			}

			m := New()
			m.integration.active = true
			m.integration.startupPrompt = true

			model, _ := m.handleIntegrationKey(keyMsg(key))
			m = model.(Model)
			if m.integration.startupPrompt {
				t.Fatal("startupPrompt should be cleared after temporary skip")
			}
			if m.integration.active {
				t.Fatal("integration prompt should close after temporary skip")
			}

			stored, _, err := promptVersionState()
			if err != nil {
				t.Fatalf("promptVersionState error: %v", err)
			}
			if stored != oldVersion {
				t.Fatalf("stored = %d, want %d without completing install", stored, oldVersion)
			}
		})
	}
}

func TestSkipStartupIntegrationPromptThenManualQuitDoesNotRecordVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	oldVersion := integrations.CurrentInstallVersion() - 1
	if oldVersion < 1 {
		t.Fatalf(
			"CurrentInstallVersion() = %d, need at least 2 for upgrade test",
			integrations.CurrentInstallVersion(),
		)
	}
	if err := writeIntegrationPromptVersion(oldVersion); err != nil {
		t.Fatal(err)
	}

	m := New()
	m.integration.active = true
	m.integration.startupPrompt = true

	model, _ := m.handleIntegrationKey(keyMsg("esc"))
	m = model.(Model)

	model, _ = m.handleKey(keyMsg("i"))
	m = model.(Model)
	if !m.integration.active {
		t.Fatal("integration prompt should open after manual i")
	}
	if m.integration.startupPrompt {
		t.Fatal("startupPrompt should stay false after temporary skip")
	}

	_, _ = m.handleIntegrationKey(keyMsg("q"))

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != oldVersion {
		t.Fatalf(
			"stored = %d, want %d after manual quit following temporary skip",
			stored,
			oldVersion,
		)
	}
}

func TestRecordIntegrationPromptDismissed(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := writeIntegrationPromptVersion(1); err != nil {
		t.Fatal(err)
	}
	if err := recordIntegrationPromptDismissed(); err != nil {
		t.Fatalf("recordIntegrationPromptDismissed error: %v", err)
	}

	stored, _, err := promptVersionState()
	if err != nil {
		t.Fatalf("promptVersionState error: %v", err)
	}
	if stored != integrations.CurrentInstallVersion() {
		t.Fatalf("stored = %d, want %d", stored, integrations.CurrentInstallVersion())
	}
}

func TestStartupIntegrationsCmdSkipsAfterCurrentVersion(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := writeIntegrationPromptVersion(integrations.CurrentInstallVersion()); err != nil {
		t.Fatal(err)
	}

	msg, ok := startupIntegrationsCmd()().(integrationsMsg)
	if !ok {
		t.Fatalf("startupIntegrationsCmd returned %T", msg)
	}
	if msg.err != nil {
		t.Fatalf("startupIntegrationsCmd error: %v", msg.err)
	}
	if msg.startup {
		t.Fatal(
			"startup integrations should not be marked as startup prompt after current version is recorded",
		)
	}
	if len(msg.recs) != 0 {
		t.Fatalf(
			"startup integrations should be skipped after current version is recorded, got %#v",
			msg.recs,
		)
	}
}
