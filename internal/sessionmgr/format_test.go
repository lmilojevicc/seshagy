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

func TestIconSetForState(t *testing.T) {
	set := IconSet{
		AgentStates: AgentStateStyles{
			Working: IconStyle{Icon: "W", ASCII: "work"},
			Blocked: IconStyle{Icon: "B", ASCII: "block"},
		},
	}
	if style := set.ForState(AgentWorking); style.Icon != "W" || style.ASCII != "work" {
		t.Fatalf("working = %#v, want custom working style", style)
	}
	if style := set.ForState(AgentBlocked); style.Icon != "B" || style.ASCII != "block" {
		t.Fatalf("blocked = %#v, want custom blocked style", style)
	}
	if style := DefaultIconSet().ForState(AgentDone); style.Icon != "✓" {
		t.Fatalf("default done icon = %q, want ✓", style.Icon)
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
	hidden := IconSet{AgentStateMode: "none", TmuxStateMode: "none"}
	if !hidden.AgentStateHidden() || !hidden.TmuxStateHidden() {
		t.Fatal("none mode should hide states")
	}
	if hidden.AgentStateUsesIcons() || hidden.AgentStateUsesLabels() ||
		hidden.TmuxStateUsesIcons() || hidden.TmuxStateUsesLabels() {
		t.Fatal("hidden states should not use icons or labels")
	}

	iconMode := IconSet{Enabled: true, AgentStateMode: "icons", TmuxStateMode: "icons"}
	if !iconMode.AgentStateUsesIcons() || iconMode.AgentStateUsesLabels() {
		t.Fatalf("icons mode = agent icons:%v labels:%v",
			iconMode.AgentStateUsesIcons(), iconMode.AgentStateUsesLabels())
	}
	if !iconMode.TmuxStateUsesIcons() || iconMode.TmuxStateUsesLabels() {
		t.Fatalf("icons mode = tmux icons:%v labels:%v",
			iconMode.TmuxStateUsesIcons(), iconMode.TmuxStateUsesLabels())
	}

	textMode := IconSet{Enabled: true, ASCII: true, AgentStateMode: "text", TmuxStateMode: "text"}
	if textMode.AgentStateUsesIcons() || !textMode.AgentStateUsesLabels() {
		t.Fatalf("text mode = agent icons:%v labels:%v",
			textMode.AgentStateUsesIcons(), textMode.AgentStateUsesLabels())
	}
	if textMode.TmuxStateUsesIcons() || !textMode.TmuxStateUsesLabels() {
		t.Fatalf("text mode = tmux icons:%v labels:%v",
			textMode.TmuxStateUsesIcons(), textMode.TmuxStateUsesLabels())
	}

	inherit := IconSet{Enabled: true, AgentStateMode: "inherit", TmuxStateMode: "inherit"}
	if !inherit.AgentStateUsesIcons() || inherit.AgentStateUsesLabels() {
		t.Fatalf("inherit mode = agent icons:%v labels:%v",
			inherit.AgentStateUsesIcons(), inherit.AgentStateUsesLabels())
	}
}

func TestLooksPaneIDAndWindowPane(t *testing.T) {
	if !looksPaneID("%12") || looksPaneID("12") || looksPaneID("%ab") {
		t.Fatal("looksPaneID() misclassified pane ids")
	}
	if !looksWindowPane("1.2") || looksWindowPane("1.") || looksWindowPane("a.2") {
		t.Fatal("looksWindowPane() misclassified locations")
	}
}

func TestParseActionLineBracketAgentLine(t *testing.T) {
	line := "[working]\tclaude\t%3\twork:1.0\t/path"
	item, ok := ParseActionLine(line)
	if !ok || item.Kind != KindAgent || item.PaneID != "%3" || item.AgentName != "claude" {
		t.Fatalf("ParseActionLine() = %#v, %v", item, ok)
	}
}

func TestDefaultIconTextModes(t *testing.T) {
	if got := defaultIconText(KindSession, true); got != "S" {
		t.Fatalf("ascii session = %q", got)
	}
	if got := defaultIconText(KindAgent, false); !strings.HasPrefix(got, IconAgent) {
		t.Fatalf("icon agent = %q", got)
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

func TestRawStateUsesConfiguredStyles(t *testing.T) {
	set := IconSet{
		AgentStates: AgentStateStyles{
			Working: IconStyle{Icon: "W"},
			Blocked: IconStyle{Icon: "B"},
			Aborted: IconStyle{Icon: "A"},
			Done:    IconStyle{Icon: "D"},
			Idle:    IconStyle{Icon: "I"},
			Unknown: IconStyle{Icon: "U"},
		},
	}
	cases := map[AgentState]string{
		AgentWorking: "W",
		AgentBlocked: "B",
		AgentAborted: "A",
		AgentDone:    "D",
		AgentIdle:    "I",
		AgentUnknown: "U",
	}
	for state, want := range cases {
		if got := set.rawState(state).Icon; got != want {
			t.Fatalf("rawState(%s).Icon = %q, want %q", state, got, want)
		}
	}
}

func TestDefaultStateStyleCoversAllStates(t *testing.T) {
	cases := map[AgentState]string{
		AgentWorking: "▶",
		AgentBlocked: "◆",
		AgentAborted: "■",
		AgentDone:    "✓",
		AgentIdle:    "◌",
		AgentUnknown: "?",
	}
	for state, wantIcon := range cases {
		style := defaultStateStyle(state)
		if style.Icon != wantIcon {
			t.Fatalf("defaultStateStyle(%s).Icon = %q, want %q", state, style.Icon, wantIcon)
		}
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
		{Item{Kind: KindAgent, AgentName: "claude", PaneID: "%1"}, "claude"},
		{Item{Kind: KindAgent, PaneID: "%2"}, "%2"},
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
	for _, kind := range []Kind{KindSession, KindZoxide, KindFD, KindAgent} {
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
