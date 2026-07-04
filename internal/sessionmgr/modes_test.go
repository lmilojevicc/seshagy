package sessionmgr

import "testing"

func TestSourceModeDisplayNames(t *testing.T) {
	// Herdr terms: session labels become workspace labels.
	h := HerdrTerms()
	if got := ModeSessions.DisplayNames(
		h,
	); got.Title != "Workspaces" || got.List != "workspaces" ||
		got.ConfigToken != "sessions" {
		t.Fatalf(
			"herdr ModeSessions DisplayNames = %+v, want Title=Workspaces List=workspaces ConfigToken=sessions",
			got,
		)
	}
	if got := ModeCurrentAgents.DisplayNames(
		h,
	); got.Title != "Current Agents" || got.List != "current workspace agents" ||
		got.ConfigToken != "current-agents" {
		t.Fatalf("herdr ModeCurrentAgents DisplayNames = %+v", got)
	}

	// Tmux terms: display names must match Names() exactly.
	for _, mode := range []SourceMode{ModeAll, ModeSessions, ModeZoxide, ModeFD, ModeAgents, ModeCurrentAgents} {
		want := mode.Names()
		got := mode.DisplayNames(TmuxTerms())
		if got != want {
			t.Fatalf("tmux %d DisplayNames = %+v, want %+v", mode, got, want)
		}
	}

	// None terms: match tmux defaults (neutral fallback).
	if got := ModeSessions.DisplayNames(
		NoneTerms(),
	); got.Title != "Sessions" ||
		got.List != "sessions" {
		t.Fatalf("none ModeSessions DisplayNames = %+v", got)
	}
}
