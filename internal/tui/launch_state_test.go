package tui

import (
	"os"
	"testing"
)

func TestClaimStartupIntegrationPromptOnlyOnce(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	first, err := claimStartupIntegrationPrompt()
	if err != nil {
		t.Fatalf("first claim error: %v", err)
	}
	if !first {
		t.Fatal("first launch should claim the startup prompt")
	}
	if _, err := os.Stat(integrationPromptSeenPath()); err != nil {
		t.Fatalf("seen file was not written: %v", err)
	}

	second, err := claimStartupIntegrationPrompt()
	if err != nil {
		t.Fatalf("second claim error: %v", err)
	}
	if second {
		t.Fatal("second launch should not claim the startup prompt")
	}
}

func TestStartupIntegrationsCmdSkipsAfterSeenFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if first, err := claimStartupIntegrationPrompt(); err != nil || !first {
		t.Fatalf("claimStartupIntegrationPrompt() = %v, %v; want true, nil", first, err)
	}

	msg, ok := startupIntegrationsCmd()().(integrationsMsg)
	if !ok {
		t.Fatalf("startupIntegrationsCmd returned %T", msg)
	}
	if msg.err != nil {
		t.Fatalf("startupIntegrationsCmd error: %v", msg.err)
	}
	if len(msg.recs) != 0 {
		t.Fatalf("startup integrations should be skipped after first launch, got %#v", msg.recs)
	}
}
