package sessionmgr

import (
	"context"
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
	strict.Install(t)

	if len(strict.OutputCalls) != 0 {
		t.Fatalf("unexpected output calls before display-message: %#v", strict.OutputCalls)
	}

	got, err := tmuxOutput(context.Background(), "display-message", "-p", "-t", pane, "#{pane_id}")
	if err != nil || string(got) != pane {
		t.Fatalf("display-message = (%q, %v)", got, err)
	}
	if len(strict.OutputCalls) != 1 {
		t.Fatalf("output calls = %#v", strict.OutputCalls)
	}
}

func TestFakeTmuxShowOptionMissingErrors(t *testing.T) {
	const pane = "%77"
	f := NewFakeTmux()
	NewStrictFakeTmux(t, f).AllowPaneOptions().Install(t)

	_, err := tmuxOutput(context.Background(), "show-option", "-qpt", pane, "@nonexistent")
	if err == nil {
		t.Fatal("show-option for missing option expected error")
	}
}
