package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

func TestOverlayPlacesForegroundAndPreservesDimmedBackground(t *testing.T) {
	bg := "aaaa\nbbbb\ncccc"
	fg := "XX\nYY"
	out := overlay(bg, fg, 1, 1)
	want := "aaaa\nbXXb\ncYYc"
	if out != want {
		t.Fatalf("overlay = %q, want %q", out, want)
	}
}

func TestOverlayYOffsetAndWideRunes(t *testing.T) {
	bg := "000000\n111111"
	out := overlay(bg, "日本", 2, 0)
	if !strings.HasPrefix(out, "00日本") {
		t.Fatalf("overlay with wide runes = %q", out)
	}
	if !strings.HasSuffix(out, "111111") {
		t.Fatalf("overlay clobbered the untouched row: %q", out)
	}
}

func TestOverlayPadsSparseBackgroundToRequestedColumn(t *testing.T) {
	out := overlay("ab", "X", 4, 0)
	if got := strings.Index(out, "X"); got != 4 {
		t.Fatalf("foreground column = %d, want 4 (%q)", got, out)
	}
}

func TestOverlayPositionsAtBoundaryAndPastBackground(t *testing.T) {
	if got := overlay("abc", "Z", 0, 0); got != "Zbc" {
		t.Fatalf("overlay at origin = %q", got)
	}
	if got := overlay("abc", "Z", 9, 3); got != "abc" {
		t.Fatalf("overlay past the background = %q", got)
	}
}

func TestOverlayKeepsANSIOnUncoveredCells(t *testing.T) {
	withColor(t)
	bg := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("aaaaa")
	out := overlay(bg, "X", 2, 0)
	plain := sessionmgr.StripANSI(out)
	if plain != "aaXaa" {
		t.Fatalf("overlay plain text = %q, want aaXaa", plain)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("overlay dropped the background's colors: %q", out)
	}
}

func TestViewRendersBottomRightToast(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 60, 12
	m = withItems(m, []sessionmgr.Item{
		{Kind: sessionmgr.KindSession, Name: "demo", Target: "demo"},
	})
	m.notify("killed session demo", sevInfo)

	frame := sessionmgr.StripANSI(m.View())
	lines := strings.Split(frame, "\n")
	if len(lines) != m.height {
		t.Fatalf("toast changed the frame height: %d lines", len(lines))
	}
	bottom := strings.Join(lines[m.height/2:], "\n")
	if !strings.Contains(bottom, "killed session demo") {
		t.Fatalf("toast missing from the bottom-right:\n%s", frame)
	}
	if strings.Contains(lines[0], "killed session demo") {
		t.Fatal("toast rendered in the header row")
	}
	if col := strings.Index(lines[m.height-3], "•"); col < m.width/2 {
		t.Fatalf("toast is not right-aligned (column %d):\n%s", col, frame)
	}
}

func TestRenderNotificationToastStacksSeverities(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 20
	m.notify("info text", sevInfo)
	m.notify("warning text", sevWarning)
	m.notify("error text", sevError)

	toast := sessionmgr.StripANSI(m.renderNotificationToast(time.Now()))
	for _, want := range []string{"• info text", "! warning text", "× error text"} {
		if !strings.Contains(toast, want) {
			t.Fatalf("toast missing %q:\n%s", want, toast)
		}
	}
}
