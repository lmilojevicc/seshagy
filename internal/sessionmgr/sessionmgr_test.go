package sessionmgr

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSessionNameFromDirMatchesScriptConventions(t *testing.T) {
	tests := map[string]string{
		"/Users/milo/Projects/foo.bar": "foo_bar",
		"/tmp/.config":                 "dot_config",
		"/tmp/a:b":                     "a_b",
		"/tmp/a b":                     "a_b",
	}
	for in, want := range tests {
		if got := SessionNameFromDir(in); got != want {
			t.Fatalf("SessionNameFromDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSessions(t *testing.T) {
	raw := []byte("dev\x1f100\x1f120\x1f/Users/milo/dev\x1f1\x1f2\n")
	got := ParseSessions(raw)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name != "dev" || got[0].Path != "/Users/milo/dev" || !got[0].Attached ||
		got[0].Windows != 2 {
		t.Fatalf("parsed unexpected session: %#v", got[0])
	}
	if !got[0].Created.Equal(time.Unix(100, 0)) || !got[0].Activity.Equal(time.Unix(120, 0)) {
		t.Fatalf("timestamps parsed incorrectly: %#v", got[0])
	}
}

func TestListFDirsWithCustomCommand(t *testing.T) {
	got, err := ListFDirsWithCommand(
		context.Background(),
		`printf '%s\n' /tmp/seshagy-fd-b /tmp/seshagy-fd-a`,
	)
	if err != nil {
		t.Fatalf("ListFDirsWithCommand() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Path != "/tmp/seshagy-fd-a" || got[1].Path != "/tmp/seshagy-fd-b" {
		t.Fatalf("custom fd dirs not sorted/parsed: %#v", got)
	}
	for _, item := range got {
		if item.Kind != KindFD || item.Target != item.Path {
			t.Fatalf("custom fd item = %#v", item)
		}
	}
}

func TestDetectTmuxPopup(t *testing.T) {
	tests := []struct {
		name        string
		envPane     string
		currentPane string
		want        bool
	}{
		{name: "normal pane", envPane: "%1", currentPane: "%1", want: false},
		{name: "popup has no pane env", envPane: "", currentPane: "%2", want: true},
		{
			name:        "popup differs from inherited pane env",
			envPane:     "%1",
			currentPane: "%2",
			want:        true,
		},
		{name: "empty current pane is inconclusive", envPane: "%1", currentPane: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectTmuxPopup(tt.envPane, tt.currentPane); got != tt.want {
				t.Fatalf(
					"detectTmuxPopup(%q, %q) = %v, want %v",
					tt.envPane,
					tt.currentPane,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestFormatLineColorsIconsButKeepsParseableText(t *testing.T) {
	line := FormatLine(Item{Kind: KindSession, Name: "demo"})
	if !strings.Contains(line, "\x1b[38;5;10m"+IconSession+" \x1b[0m") {
		t.Fatalf("session icon does not use terminal bright green: %q", line)
	}
	if clean := StripANSI(line); clean != IconSession+" demo" {
		t.Fatalf("StripANSI(%q) = %q", line, clean)
	}
	item, ok := ParseActionLine(line)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLine(%q) = %#v, %v", line, item, ok)
	}
}

func TestFormatLineWithASCIIIcons(t *testing.T) {
	icons := DefaultIconSet()
	icons.ASCII = true
	icons.Session.ASCII = "S"
	icons.Session.Color = "9"
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if !strings.Contains(line, "\x1b[38;5;9mS\x1b[0m demo") {
		t.Fatalf("line does not use configured ascii icon/color: %q", line)
	}
	item, ok := ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", line, item, ok)
	}
}

func TestFormatLineWithHexIconColor(t *testing.T) {
	icons := DefaultIconSet()
	icons.Session.Color = "#a6e3a1"
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if !strings.Contains(line, "\x1b[38;2;166;227;161m"+IconSession+" \x1b[0mdemo") {
		t.Fatalf("line does not use truecolor hex escape: %q", line)
	}
}

func TestDefaultIconsCarryConfiguredDisplaySpacing(t *testing.T) {
	icons := DefaultIconSet()
	for name, style := range map[string]IconStyle{
		"session": icons.Session,
		"zoxide":  icons.Zoxide,
		"fd":      icons.FD,
	} {
		if !strings.HasSuffix(style.Icon, " ") || strings.HasSuffix(style.Icon, "  ") {
			t.Fatalf("%s default icon = %q, want exactly one trailing space", name, style.Icon)
		}
	}

	line := FormatLine(Item{Kind: KindSession, Name: "demo"})
	if clean := StripANSI(line); clean != IconSession+" demo" {
		t.Fatalf("default icon spacing = %q, want one display space", clean)
	}
}

func TestFormatLineWithNoIconsOmitsSourcePrefixes(t *testing.T) {
	icons := DefaultIconSet()
	icons.Enabled = false
	line := FormatLineWithIcons(Item{Kind: KindSession, Name: "demo"}, icons)
	if line != "demo" {
		t.Fatalf("no-icons session line = %q, want demo", line)
	}
	item, ok := ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", line, item, ok)
	}

	line = FormatLineWithIcons(Item{Kind: KindSession, Name: "Sdemo"}, icons)
	item, ok = ParseActionLineWithIcons(line, icons)
	if !ok || item.Kind != KindSession || item.Name != "Sdemo" {
		t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v, want full session name", line, item, ok)
	}
}

func TestParseActionLineWithConfiguredIconsFallsBackToDefaults(t *testing.T) {
	icons := DefaultIconSet()
	icons.Session.Icon = "X"
	item, ok := ParseActionLineWithIcons(IconSession+" demo", icons)
	if !ok || item.Kind != KindSession || item.Name != "demo" {
		t.Fatalf("fallback parse = %#v, %v", item, ok)
	}
}
