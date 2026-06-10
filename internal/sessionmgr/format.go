package sessionmgr

import (
	"fmt"
	"strconv"
	"strings"
)

type IconSet struct {
	Enabled bool
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
		Session: IconStyle{Icon: IconSession, ASCII: "S", Color: "10"},
		Zoxide:  IconStyle{Icon: IconZoxide, ASCII: "Z", Color: "14"},
		FD:      IconStyle{Icon: IconFD, ASCII: "F", Color: "11"},
		Agent:   IconStyle{Icon: IconAgent, ASCII: "A", Color: "13"},
	}
}

func (set IconSet) For(kind Kind) IconStyle {
	style := set.raw(kind)
	text := style.Icon
	if !set.Enabled {
		text = style.ASCII
	}
	if text == "" {
		text = defaultIconText(kind, set.Enabled)
	}
	style.Text = text
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

func defaultIconText(kind Kind, enabled bool) string {
	defaults := DefaultIconSet().raw(kind)
	if enabled {
		return defaults.Icon
	}
	return defaults.ASCII
}

func FormatLine(i Item) string {
	return FormatLineWithIcons(i, DefaultIconSet())
}

func FormatLineWithIcons(i Item, icons IconSet) string {
	switch i.Kind {
	case KindSession:
		return fmt.Sprintf("%s %s", colorIcon(KindSession, icons), i.Name)
	case KindAgent:
		suffix := ""
		if i.AgentMessage != "" {
			suffix = " — " + i.AgentMessage
		} else if i.AgentSource != "" {
			suffix = " — " + i.AgentSource
		} else if i.AgentUpdated != "" {
			suffix = " — updated " + i.AgentUpdated
		}
		return fmt.Sprintf("%s [%s]\t%s\t%s\t%s%s", colorIcon(KindAgent, icons), agentStateLabel(i.AgentState), i.AgentName, i.Location, i.Path, suffix)
	case KindZoxide:
		return fmt.Sprintf("%s %s", colorIcon(KindZoxide, icons), i.Path)
	case KindFD:
		return fmt.Sprintf("%s %s", colorIcon(KindFD, icons), i.Path)
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

func iconAndColor(kind Kind) (string, string) {
	style := DefaultIconSet().For(kind)
	return style.Text, style.Color
}

func ParseActionLine(raw string) (Item, bool) {
	return ParseActionLineWithIcons(raw, DefaultIconSet())
}

func ParseActionLineWithIcons(raw string, icons IconSet) (Item, bool) {
	clean := strings.TrimSpace(StripANSI(raw))
	switch {
	case hasIconPrefix(clean, icons, KindSession):
		name := strings.TrimSpace(strings.TrimPrefix(clean, matchedIconPrefix(clean, icons, KindSession)))
		return Item{Kind: KindSession, Name: name, Target: name}, name != ""
	case hasIconPrefix(clean, icons, KindAgent):
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

func hasIconPrefix(clean string, icons IconSet, kind Kind) bool {
	return matchedIconPrefix(clean, icons, kind) != ""
}

func matchedIconPrefix(clean string, icons IconSet, kind Kind) string {
	if strings.HasPrefix(clean, icons.For(kind).Text) {
		return icons.For(kind).Text
	}
	defaults := DefaultIconSet()
	if strings.HasPrefix(clean, defaults.For(kind).Text) {
		return defaults.For(kind).Text
	}
	ascii := defaultIconText(kind, false)
	if strings.HasPrefix(clean, ascii) {
		return ascii
	}
	return ""
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
