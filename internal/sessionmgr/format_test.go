package sessionmgr

import (
	"strings"
	"testing"
)

func TestIconSetForTmuxState(t *testing.T) {
	set := IconSet{
		TmuxStates: TmuxStateStyles{
			Attached: IconStyle{Icon: "A", ASCII: "on"},
			Detached: IconStyle{Icon: "D", ASCII: "off"},
		},
	}
	attached := set.ForTmuxState(true)
	if attached.Icon != "A" || attached.ASCII != "on" {
		t.Fatalf("attached = %#v, want custom attached style", attached)
	}
	detached := set.ForTmuxState(false)
	if detached.Icon != "D" || detached.ASCII != "off" {
		t.Fatalf("detached = %#v, want custom detached style", detached)
	}

	defaults := DefaultIconSet()
	if style := defaults.ForTmuxState(true); style.Icon != "●" {
		t.Fatalf("default attached icon = %q, want ●", style.Icon)
	}
	if style := defaults.ForTmuxState(false); style.Icon != "◌" {
		t.Fatalf("default detached icon = %q, want ◌", style.Icon)
	}
}

func TestIconSetForUnknownAgentState(t *testing.T) {
	// ForAgentState backfills only Icon and ASCII from defaults; Color comes
	// from the configured IconSet (tested via config projection in config_test).
	style := IconSet{}.ForAgentState(AgentUnknown)
	if style.Icon != "?" || style.ASCII != "unknown" {
		t.Fatalf("default unknown state style = %#v, want icon ?, label unknown", style)
	}

	// In icon mode (the default icon set) the unknown glyph prefixes the line.
	line := FormatLineWithIcons(Item{
		Kind:       KindAgent,
		AgentName:  "pi",
		AgentState: AgentUnknown,
	}, DefaultIconSet())
	clean := StripANSI(line)
	if !strings.HasPrefix(clean, "? pi") {
		t.Fatalf("unknown agent line = %q, want ? prefix", clean)
	}
}

func TestParseActionLineWithIcons(t *testing.T) {
	icons := DefaultIconSet()
	tests := []struct {
		name string
		line string
		kind Kind
		want string
	}{
		{
			name: "session",
			line: IconSession + " demo",
			kind: KindSession,
			want: "demo",
		},
		{
			name: "zoxide",
			line: IconZoxide + " ~/Projects/x",
			kind: KindZoxide,
			want: "~/Projects/x",
		},
		{
			name: "fd",
			line: IconFD + " /tmp",
			kind: KindFD,
			want: "/tmp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, ok := ParseActionLineWithIcons(tt.line, icons)
			if !ok || item.Kind != tt.kind {
				t.Fatalf("ParseActionLineWithIcons(%q) = %#v, %v", tt.line, item, ok)
			}
			switch tt.kind {
			case KindSession:
				if item.Name != tt.want {
					t.Fatalf("Name = %q, want %q", item.Name, tt.want)
				}
			case KindZoxide, KindFD:
				if item.Path != tt.want {
					t.Fatalf("Path = %q, want %q", item.Path, tt.want)
				}
			}
		})
	}
}

func TestIconSetStateDisplayModes(t *testing.T) {
	hidden := IconSet{TmuxStateMode: "none"}
	if !hidden.TmuxStateHidden() {
		t.Fatal("none mode should hide states")
	}
	if hidden.TmuxStateUsesIcons() || hidden.TmuxStateUsesLabels() {
		t.Fatal("hidden states should not use icons or labels")
	}

	iconMode := IconSet{Enabled: true, TmuxStateMode: "icons"}
	if !iconMode.TmuxStateUsesIcons() || iconMode.TmuxStateUsesLabels() {
		t.Fatalf("icons mode = tmux icons:%v labels:%v",
			iconMode.TmuxStateUsesIcons(), iconMode.TmuxStateUsesLabels())
	}

	textMode := IconSet{Enabled: true, ASCII: true, TmuxStateMode: "text"}
	if textMode.TmuxStateUsesIcons() || !textMode.TmuxStateUsesLabels() {
		t.Fatalf("text mode = tmux icons:%v labels:%v",
			textMode.TmuxStateUsesIcons(), textMode.TmuxStateUsesLabels())
	}

	inherit := IconSet{Enabled: true, TmuxStateMode: "inherit"}
	if !inherit.TmuxStateUsesIcons() || inherit.TmuxStateUsesLabels() {
		t.Fatalf("inherit mode = tmux icons:%v labels:%v",
			inherit.TmuxStateUsesIcons(), inherit.TmuxStateUsesLabels())
	}
}

func TestDefaultIconTextModes(t *testing.T) {
	if got := defaultIconText(KindSession, true); got != "S" {
		t.Fatalf("ascii session = %q", got)
	}
}

func TestAnsiColorSequenceHexAndNumeric(t *testing.T) {
	if got := ansiColorSequence("#ff00aa"); got != "38;2;255;0;170" {
		t.Fatalf("hex color = %q", got)
	}
	if got := ansiColorSequence("12"); got != "38;5;12" {
		t.Fatalf("numeric color = %q", got)
	}
	if got := ansiColorSequence("31"); got != "31" {
		t.Fatalf("standard fg = %q", got)
	}
	if got := ansiColorSequence("bad"); got != "bad" {
		t.Fatalf("passthrough = %q", got)
	}
}

func TestHexToRGBRejectsInvalid(t *testing.T) {
	if _, _, _, ok := hexToRGB("#abc"); !ok {
		return
	}
	t.Fatal("hexToRGB should reject short hex")
}

// TestIconSetForAgentStateResolvesMode asserts ForAgentState resolves the
// configured display mode into the shown value (.Text), mirroring IconSet.For
// for kind icons. This is the central fix for agent-state icons ignoring
// mode="text" / agent_state_mode="inherit".
func TestIconSetForAgentStateResolvesMode(t *testing.T) {
	states := []AgentState{AgentIdle, AgentWorking, AgentBlocked, AgentDone, AgentUnknown}

	cases := []struct {
		name  string
		set   IconSet
		want  func(style IconStyle) string // expected .Text derivation
		label string
	}{
		{
			name:  "global text + agent inherit",
			set:   IconSet{Enabled: true, ASCII: true, AgentStateMode: "inherit"},
			want:  func(s IconStyle) string { return s.ASCII },
			label: "ASCII label",
		},
		{
			name:  "global icons + agent text explicit",
			set:   IconSet{Enabled: true, ASCII: false, AgentStateMode: "text"},
			want:  func(s IconStyle) string { return s.ASCII },
			label: "ASCII label",
		},
		{
			name:  "global text + agent icons override",
			set:   IconSet{Enabled: true, ASCII: true, AgentStateMode: "icons"},
			want:  func(s IconStyle) string { return s.Icon },
			label: "glyph",
		},
		{
			name:  "global icons + agent inherit",
			set:   IconSet{Enabled: true, ASCII: false, AgentStateMode: "inherit"},
			want:  func(s IconStyle) string { return s.Icon },
			label: "glyph",
		},
	}
	for _, tc := range cases {
		for _, st := range states {
			style := tc.set.ForAgentState(st)
			want := tc.want(style)
			if style.Text != want {
				t.Fatalf("%s %s: Text = %q, want %s %q",
					tc.name, st, style.Text, tc.label, want)
			}
		}
	}

	// agent_state_mode="none" hides the category entirely (no display value).
	hidden := IconSet{Enabled: true, ASCII: false, AgentStateMode: "none"}
	for _, st := range states {
		if style := hidden.ForAgentState(st); style.Text != "" {
			t.Fatalf("none %s: Text = %q, want empty", st, style.Text)
		}
	}
}

// TestFormatLineWithIconsAgentStateHonorsMode asserts the action-line/JSON row
// representation honors the resolved mode (was always the glyph before).
func TestFormatLineWithIconsAgentStateHonorsMode(t *testing.T) {
	item := Item{Kind: KindAgent, AgentName: "pi", AgentState: AgentWorking}

	iconLine := StripANSI(FormatLineWithIcons(item,
		IconSet{Enabled: true, ASCII: false, AgentStateMode: "inherit"}))
	if !strings.HasPrefix(iconLine, "\u25cf pi") {
		t.Fatalf("icon mode agent line = %q, want \u25cf prefix", iconLine)
	}

	textLine := StripANSI(FormatLineWithIcons(item,
		IconSet{Enabled: true, ASCII: true, AgentStateMode: "inherit"}))
	if !strings.HasPrefix(textLine, "working pi") {
		t.Fatalf("text mode agent line = %q, want working prefix", textLine)
	}
	if strings.Contains(textLine, "\u25cf") {
		t.Fatalf("text mode agent line = %q, should not use glyph", textLine)
	}
}

func TestFormatLineDefaultKindUsesDisplayName(t *testing.T) {
	item := Item{Kind: Kind("custom"), Name: "fallback-name"}
	if got := FormatLine(item); got != "fallback-name" {
		t.Fatalf("FormatLine() = %q, want fallback-name", got)
	}
}

func TestItemDisplayName(t *testing.T) {
	cases := []struct {
		item Item
		want string
	}{
		{},
		{Item{Kind: KindZoxide, Path: "~/Projects"}, "~/Projects"},
		{Item{Kind: KindSession, Name: "work"}, "work"},
	}
	for _, tc := range cases {
		if got := tc.item.DisplayName(); got != tc.want {
			t.Fatalf("DisplayName(%#v) = %q, want %q", tc.item, got, tc.want)
		}
	}
}

func TestIconSetForKindDisabled(t *testing.T) {
	set := IconSet{Enabled: false}
	for _, kind := range []Kind{KindSession, KindZoxide, KindFD} {
		style := set.For(kind)
		if style.Text != "" {
			t.Fatalf("For(%q).Text = %q, want empty when icons disabled", kind, style.Text)
		}
	}

	enabled := DefaultIconSet()
	if style := enabled.For(KindSession); !strings.HasPrefix(style.Text, IconSession) {
		t.Fatalf("enabled session text = %q, want session icon prefix", style.Text)
	}
}
