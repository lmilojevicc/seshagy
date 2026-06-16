package sessionmgr

import (
	"context"
	"strings"
	"testing"
)

func TestStrictFakeTmuxRecordsAllowedCalls(t *testing.T) {
	const pane = "%99"
	strict := NewStrictFakeTmux(t, nil).
		AllowPaneOptions().
		AllowOutput(MatchDisplayMessage(pane, "#{pane_id}")).
		HandleOutput(MatchDisplayMessage(pane, "#{pane_id}"), func(_ context.Context, _ ...string) ([]byte, error) {
			return []byte(pane), nil
		})
	f := strict.Install(t)

	if err := setPaneOption(context.Background(), pane, "@agent_name", "pi"); err != nil {
		t.Fatalf("setPaneOption() err = %v", err)
	}
	if got := f.Get(pane, "@agent_name"); got != "pi" {
		t.Fatalf("@agent_name = %q, want pi", got)
	}
	if len(strict.RunCalls) != 1 || strict.RunCalls[0].Args[0] != "set-option" {
		t.Fatalf("run calls = %#v", strict.RunCalls)
	}

	got, err := tmuxOutput(context.Background(), "display-message", "-p", "-t", pane, "#{pane_id}")
	if err != nil || string(got) != pane {
		t.Fatalf("display-message = (%q, %v)", got, err)
	}
	if len(strict.OutputCalls) != 1 {
		t.Fatalf("output calls = %#v", strict.OutputCalls)
	}
}

func TestInstallAgentCLIFakeTmuxSupportsListAndExplain(t *testing.T) {
	const pane = "%55"
	listOut := []byte(strings.Join(agentExplainFields(pane, nil), paneSep) + "\n")
	f := InstallAgentCLIFakeTmux(t, pane, listOut)

	out, err := tmuxOutput(context.Background(), "list-panes", "-a", "-F", agentFormat)
	if err != nil || string(out) != string(listOut) {
		t.Fatalf("list-panes = (%q, %v)", out, err)
	}
	got, err := ExplainAgent(context.Background(), pane, LoadOptions{})
	if err != nil {
		t.Fatalf("ExplainAgent() err = %v", err)
	}
	if !strings.Contains(got, "claude") {
		t.Fatalf("ExplainAgent() = %q, want claude in output", got)
	}
	_ = f
}

func TestFakeTmuxShowOptionMissingErrors(t *testing.T) {
	const pane = "%77"
	f := NewFakeTmux()
	InstallTrackFakeTmux(t, f)

	_, err := tmuxOutput(context.Background(), "show-option", "-qvpt", pane, "@agent_name")
	if err == nil {
		t.Fatal("show-option for missing option expected error")
	}
}

func TestFakeTmuxUnsetOption(t *testing.T) {
	const pane = "%12"
	f := NewFakeTmux()
	f.Set(pane, "@agent_name", "pi")
	InstallTrackFakeTmux(t, f)

	if err := unsetPaneOption(context.Background(), pane, "@agent_name"); err != nil {
		t.Fatalf("unsetPaneOption() err = %v", err)
	}
	if got := f.Get(pane, "@agent_name"); got != "" {
		t.Fatalf("@agent_name = %q, want cleared", got)
	}
}
