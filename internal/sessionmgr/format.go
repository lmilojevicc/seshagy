package sessionmgr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes ANSI escape sequences from s.
func StripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

type IconSet struct {
	Enabled        bool
	ASCII          bool
	TmuxStateMode  string
	TmuxStates     TmuxStateStyles
	AgentStateMode string
	AgentStates    AgentStateStyles
	Session        IconStyle
	Workspace      IconStyle
	Zoxide         IconStyle
	FD             IconStyle
}

type TmuxStateStyles struct {
	Attached IconStyle
	Detached IconStyle
}

type AgentStateStyles struct {
	Idle    IconStyle
	Working IconStyle
	Blocked IconStyle
	Done    IconStyle
	Unknown IconStyle
}

type IconStyle struct {
	Icon  string
	ASCII string
	Color string
	Text  string
}

func DefaultIconSet() IconSet {
	return IconSet{
		Enabled:   true,
		ASCII:     false,
		Session:   IconStyle{Icon: IconSession + " ", ASCII: "S", Color: "10"},
		Workspace: IconStyle{Icon: IconWorkspace + " ", ASCII: "W", Color: "10"},
		Zoxide:    IconStyle{Icon: IconZoxide + " ", ASCII: "Z", Color: "14"},
		FD:        IconStyle{Icon: IconFD + " ", ASCII: "F", Color: "11"},
	}
}

func (set IconSet) TmuxStateHidden() bool {
	return set.TmuxStateMode == "none"
}

func (set IconSet) TmuxStateUsesIcons() bool {
	if set.TmuxStateHidden() {
		return false
	}
	switch set.TmuxStateMode {
	case "icons":
		return true
	case "text":
		return false
	default: // inherit
		return set.Enabled && !set.ASCII
	}
}

func (set IconSet) TmuxStateUsesLabels() bool {
	if set.TmuxStateHidden() {
		return false
	}
	return !set.TmuxStateUsesIcons()
}

func (set IconSet) AgentStateHidden() bool {
	return set.AgentStateMode == "none"
}

func (set IconSet) AgentStateUsesIcons() bool {
	if set.AgentStateHidden() {
		return false
	}
	switch set.AgentStateMode {
	case "icons":
		return true
	case "text":
		return false
	default: // inherit
		return set.Enabled && !set.ASCII
	}
}

func (set IconSet) ForAgentState(state AgentState) IconStyle {
	style := set.rawAgentState(state)
	defaults := defaultAgentStateStyle(state)
	if style.Icon == "" {
		style.Icon = defaults.Icon
	}
	if style.ASCII == "" {
		style.ASCII = defaults.ASCII
	}
	return style
}

func (set IconSet) rawAgentState(state AgentState) IconStyle {
	switch state {
	case AgentWorking:
		return set.AgentStates.Working
	case AgentBlocked:
		return set.AgentStates.Blocked
	case AgentDone:
		return set.AgentStates.Done
	case AgentUnknown:
		return set.AgentStates.Unknown
	case AgentIdle:
		return set.AgentStates.Idle
	default:
		return set.AgentStates.Idle
	}
}

func defaultAgentStateStyle(state AgentState) IconStyle {
	switch state {
	case AgentWorking:
		return IconStyle{Icon: "●", ASCII: "working", Color: "10"}
	case AgentBlocked:
		return IconStyle{Icon: "◐", ASCII: "blocked", Color: "11"}
	case AgentDone:
		return IconStyle{Icon: "◉", ASCII: "done", Color: "14"}
	case AgentUnknown:
		return IconStyle{Icon: "?", ASCII: "unknown", Color: "8"}
	case AgentIdle:
		return IconStyle{Icon: "○", ASCII: "idle", Color: "8"}
	default:
		return IconStyle{Icon: "○", ASCII: "idle", Color: "8"}
	}
}

func (set IconSet) ForTmuxState(attached bool) IconStyle {
	style := set.rawTmuxState(attached)
	defaults := defaultTmuxStateStyle(attached)
	if style.Icon == "" {
		style.Icon = defaults.Icon
	}
	if style.ASCII == "" {
		style.ASCII = defaults.ASCII
	}
	return style
}

func defaultTmuxStateStyle(attached bool) IconStyle {
	if attached {
		return IconStyle{Icon: "●", ASCII: "attached"}
	}
	return IconStyle{Icon: "◌", ASCII: "detached"}
}

func (set IconSet) rawTmuxState(attached bool) IconStyle {
	if attached {
		return set.TmuxStates.Attached
	}
	return set.TmuxStates.Detached
}

func (set IconSet) For(kind Kind) IconStyle {
	style := set.raw(kind)
	if !set.Enabled {
		style.Text = ""
	} else if set.ASCII {
		style.Text = style.ASCII
		if style.Text == "" {
			style.Text = defaultIconText(kind, true)
		}
	} else {
		style.Text = style.Icon
		if style.Text == "" {
			style.Text = defaultIconText(kind, false)
		}
	}
	if style.Color == "" {
		style.Color = DefaultIconSet().raw(kind).Color
	}
	return style
}

func (set IconSet) raw(kind Kind) IconStyle {
	switch kind {
	case KindSession:
		return set.Session
	case KindZoxide:
		return set.Zoxide
	case KindFD:
		return set.FD
	case KindAgent:
		return IconStyle{}
	default:
		return IconStyle{}
	}
}

func defaultIconText(kind Kind, ascii bool) string {
	defaults := DefaultIconSet().raw(kind)
	if ascii {
		return defaults.ASCII
	}
	return defaults.Icon
}

func FormatLine(i Item) string {
	return FormatLineWithIcons(i, DefaultIconSet())
}

func FormatLineWithIcons(i Item, icons IconSet) string {
	switch i.Kind {
	case KindSession:
		return joinNonEmpty(colorIcon(KindSession, icons), i.Name)
	case KindZoxide:
		return joinNonEmpty(colorIcon(KindZoxide, icons), i.Path)
	case KindFD:
		return joinNonEmpty(colorIcon(KindFD, icons), i.Path)
	case KindAgent:
		if icons.AgentStateHidden() {
			return i.DisplayName()
		}
		return joinNonEmpty(agentStateGlyph(i.AgentState, icons), i.DisplayName())
	default:
		return i.DisplayName()
	}
}

func colorIcon(kind Kind, icons IconSet) string {
	style := icons.For(kind)
	if style.Text == "" || style.Color == "" {
		return style.Text
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", ansiColorSequence(style.Color), style.Text)
}

func agentStateGlyph(state AgentState, icons IconSet) string {
	style := icons.ForAgentState(state)
	if style.Icon == "" {
		return ""
	}
	if style.Color == "" {
		return style.Icon
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", ansiColorSequence(style.Color), style.Icon)
}

func joinNonEmpty(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	var b strings.Builder
	for i, part := range kept {
		if i > 0 && shouldInsertSeparator(b.String(), part) {
			b.WriteString(" ")
		}
		b.WriteString(part)
	}
	return b.String()
}

func shouldInsertSeparator(left, right string) bool {
	leftClean := StripANSI(left)
	rightClean := StripANSI(right)
	return !strings.HasSuffix(leftClean, " ") && !strings.HasPrefix(rightClean, " ")
}

func ParseActionLine(raw string) (Item, bool) {
	return ParseActionLineWithIcons(raw, DefaultIconSet())
}

func ParseActionLineWithIcons(raw string, icons IconSet) (Item, bool) {
	clean := strings.TrimSpace(StripANSI(raw))
	if clean == "" {
		return Item{}, false
	}
	if !icons.Enabled {
		return parseNoIconActionLine(clean)
	}
	switch {
	case hasIconPrefix(clean, icons, KindSession):
		name := strings.TrimSpace(
			strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindSession)),
		)
		return Item{Kind: KindSession, Name: name, Target: name}, name != ""
	case hasIconPrefix(clean, icons, KindZoxide):
		path := strings.TrimSpace(
			strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindZoxide)),
		)
		return Item{Kind: KindZoxide, Path: path, Target: ExpandHome(path)}, path != ""
	case hasIconPrefix(clean, icons, KindFD):
		path := strings.TrimSpace(
			strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindFD)),
		)
		return Item{Kind: KindFD, Path: path, Target: ExpandHome(path)}, path != ""
	default:
		return Item{}, false
	}
}

func parseNoIconActionLine(clean string) (Item, bool) {
	if looksPathLine(clean) {
		return Item{Kind: KindZoxide, Path: clean, Target: ExpandHome(clean)}, clean != ""
	}
	return Item{Kind: KindSession, Name: clean, Target: clean}, true
}

func looksPathLine(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../")
}

func hasIconPrefix(clean string, icons IconSet, kind Kind) bool {
	return matchedIconPrefix(clean, icons, kind) != ""
}

func matchedIconPrefix(clean string, icons IconSet, kind Kind) string {
	configured := icons.For(kind).Text
	if hasPrefixToken(clean, configured) {
		return configured
	}
	defaults := DefaultIconSet()
	defaultIcon := defaults.For(kind).Text
	if hasPrefixToken(clean, defaultIcon) {
		return defaultIcon
	}
	ascii := defaultIconText(kind, true)
	if hasPrefixToken(clean, ascii) {
		return ascii
	}
	return ""
}

func hasPrefixToken(clean, prefix string) bool {
	if prefix == "" || !strings.HasPrefix(clean, prefix) {
		return false
	}
	if strings.HasSuffix(prefix, " ") || strings.HasSuffix(prefix, "\t") {
		return true
	}
	if len(clean) == len(prefix) {
		return true
	}
	next := clean[len(prefix)]
	return next == ' ' || next == '\t'
}

func ansiColorSequence(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return ""
	}
	if strings.HasPrefix(color, "#") {
		r, g, b, ok := hexToRGB(color)
		if ok {
			return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
		}
	}
	if n, err := strconv.Atoi(color); err == nil {
		if (n >= 30 && n <= 37) || (n >= 90 && n <= 97) {
			return color
		}
		return fmt.Sprintf("38;5;%d", n)
	}
	return color
}

func hexToRGB(hex string) (int, int, int, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(n >> 16), int((n >> 8) & 0xff), int(n & 0xff), true
}
