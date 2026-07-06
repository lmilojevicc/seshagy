package sessionmgr

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestInTmuxPopup(t *testing.T) {
	t.Run("detects pane mismatch", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/tmux-123")
		t.Setenv("HERDR_ENV", "")
		t.Setenv("TMUX_PANE", "%0")
		SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) >= 3 && args[0] == "display-message" && args[2] == "#{pane_id}" {
				return []byte("%1"), nil
			}
			return nil, nil
		}, nil)

		inPopup, err := InTmuxPopup(context.Background())
		if err != nil || !inPopup {
			t.Fatalf("InTmuxPopup() = (%v, %v), want (true, nil)", inPopup, err)
		}
	})

	t.Run("outside tmux", func(t *testing.T) {
		t.Setenv("TMUX", "")
		t.Setenv("HERDR_ENV", "")
		inPopup, err := InTmuxPopup(context.Background())
		if err != nil || inPopup {
			t.Fatalf("InTmuxPopup() = (%v, %v), want (false, nil)", inPopup, err)
		}
	})

	t.Run("propagates display-message error", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/tmux-123")
		t.Setenv("HERDR_ENV", "")
		SetTmuxHooksForTest(t, func(_ context.Context, args ...string) ([]byte, error) {
			if len(args) >= 3 && args[0] == "display-message" {
				return nil, fmt.Errorf("tmux unavailable")
			}
			return nil, nil
		}, nil)

		if _, err := InTmuxPopup(context.Background()); err == nil {
			t.Fatal("InTmuxPopup() expected error")
		} else if !strings.Contains(err.Error(), "tmux unavailable") {
			t.Fatalf("InTmuxPopup() error = %v", err)
		}
	})
}

func TestWithLocalePreservesExistingLang(t *testing.T) {
	env := withLocale([]string{"LANG=en_US.UTF-8", "HOME=/tmp"})
	found := false
	for _, entry := range env {
		if entry == "LANG=en_US.UTF-8" {
			found = true
		}
		if entry == "LC_ALL=C.UTF-8" {
			t.Fatal("withLocale should not append LC_ALL when LANG is set")
		}
	}
	if !found {
		t.Fatalf("withLocale() = %#v", env)
	}
}

func TestWithLocaleAppendsWhenLangMissing(t *testing.T) {
	env := withLocale([]string{"HOME=/tmp"})
	if len(env) != 2 || env[1] != "LC_ALL=C.UTF-8" {
		t.Fatalf("withLocale() = %#v", env)
	}
}
