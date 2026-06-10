package sessionmgr

import (
	"fmt"
	"strconv"
	"strings"
)

type IconSet struct {
	Enabled bool
	ASCII   bool
	Session IconStyle
	Zoxide  IconStyle
	FD      IconStyle
	Agent   IconStyle
}

type IconStyle struct {
	Icon  string
	ASCII string
	Color string
	Text  string
}

func DefaultIconSet() IconSet {
	return IconSet{
		Enabled: true,
		ASCII:   false,
		Session: IconStyle{Icon: IconSession, ASCII: "S", Color: "10"},
		Zoxide:  IconStyle{Icon: IconZoxide, ASCII: "Z", Color: "14"},
		FD:      IconStyle{Icon: IconFD, ASCII: "F", Color: "11"},
		Agent:   IconStyle{Icon: IconAgent, ASCII: "A", Color: "13"},
	}
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
		return set.Agent
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
		return joinNonEmpty(" ", colorIcon(KindSession, icons), i.Name)
	case KindAgent:
		suffix := ""
		if i.AgentMessage != "" {
			suffix = " — " + i.AgentMessage
		} else if i.AgentSource != "" {
			suffix = " — " + i.AgentSource
		} else if i.AgentUpdated != "" {
			suffix = " — updated " + i.AgentUpdated
		}
		prefix := joinNonEmpty(" ", colorIcon(KindAgent, icons), "["+agentStateLabel(i.AgentState)+"]")
		return fmt.Sprintf("%s\t%s\t%s\t%s%s", prefix, i.AgentName, i.Location, i.Path, suffix)
	case KindZoxide:
		return joinNonEmpty(" ", colorIcon(KindZoxide, icons), i.Path)
	case KindFD:
		return joinNonEmpty(" ", colorIcon(KindFD, icons), i.Path)
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

func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}

func iconAndColor(kind Kind) (string, string) {
	style := DefaultIconSet().For(kind)
	return style.Text, style.Color
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
		name := strings.TrimSpace(strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindSession)))
		return Item{Kind: KindSession, Name: name, Target: name}, name != ""
	case hasIconPrefix(clean, icons, KindAgent):
		pane := AgentPaneFromLine(clean)
		return Item{Kind: KindAgent, PaneID: pane, Target: pane}, pane != ""
	case strings.HasPrefix(clean, "["):
		pane := AgentPaneFromLine(clean)
		return Item{Kind: KindAgent, PaneID: pane, Target: pane}, pane != ""
	case hasIconPrefix(clean, icons, KindZoxide):
		path := strings.TrimSpace(strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindZoxide)))
		return Item{Kind: KindZoxide, Path: path, Target: ExpandHome(path)}, path != ""
	case hasIconPrefix(clean, icons, KindFD):
		path := strings.TrimSpace(strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindFD)))
		return Item{Kind: KindFD, Path: path, Target: ExpandHome(path)}, path != ""
	default:
		return Item{}, false
	}
}

func parseNoIconActionLine(clean string) (Item, bool) {
	if strings.HasPrefix(clean, "[") {
		pane := AgentPaneFromLine(clean)
		if pane != "" {
			return Item{Kind: KindAgent, PaneID: pane, Target: pane}, true
		}
	}
	if looksPathLine(clean) {
		return Item{Kind: KindZoxide, Path: clean, Target: ExpandHome(clean)}, clean != ""
	}
	return Item{Kind: KindSession, Name: clean, Target: clean}, true
}

func looksPathLine(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
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

func AgentPaneFromLine(clean string) string {
	for _, field := range strings.Fields(clean) {
		if strings.Contains(field, ":") && strings.Contains(field, ".") {
			parts := strings.Split(field, ":")
			if len(parts) < 2 {
				continue
			}
			wp := parts[len(parts)-1]
			if looksWindowPane(wp) {
				return field
			}
		}
	}
	return ""
}

func looksWindowPane(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return false
	}
	return parts[0] != "" && parts[1] != "" && allDigits(parts[0]) && allDigits(parts[1])
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
