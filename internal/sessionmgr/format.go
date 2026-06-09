package sessionmgr

import (
	"fmt"
	"strings"
)

func FormatLine(i Item) string {
	switch i.Kind {
	case KindSession:
		return fmt.Sprintf("%s %s", colorIcon(KindSession), i.Name)
	case KindAgent:
		suffix := ""
		if i.AgentMessage != "" {
			suffix = " — " + i.AgentMessage
		} else if i.AgentSource != "" {
			suffix = " — " + i.AgentSource
		} else if i.AgentUpdated != "" {
			suffix = " — updated " + i.AgentUpdated
		}
		return fmt.Sprintf("%s [%s]\t%s\t%s\t%s%s", colorIcon(KindAgent), agentStateLabel(i.AgentState), i.AgentName, i.Location, i.Path, suffix)
	case KindZoxide:
		return fmt.Sprintf("%s %s", colorIcon(KindZoxide), i.Path)
	case KindFD:
		return fmt.Sprintf("%s %s", colorIcon(KindFD), i.Path)
	default:
		return i.DisplayName()
	}
}

func colorIcon(kind Kind) string {
	icon, sgr := iconAndColor(kind)
	if icon == "" || sgr == "" {
		return icon
	}
	return fmt.Sprintf("\x1b[%sm%s\x1b[0m", sgr, icon)
}

func iconAndColor(kind Kind) (string, string) {
	switch kind {
	case KindSession:
		return IconSession, "92" // Bright green from the user's terminal palette.
	case KindZoxide:
		return IconZoxide, "96" // Bright cyan pairs cleanly with green for jump history.
	case KindFD:
		return IconFD, "93" // Bright yellow gives fd a warm contrasting source color.
	case KindAgent:
		return IconAgent, "95" // Bright magenta matches the ccmux/seshagy accent.
	default:
		return "", ""
	}
}

func ParseActionLine(raw string) (Item, bool) {
	clean := strings.TrimSpace(StripANSI(raw))
	switch {
	case strings.HasPrefix(clean, IconSession):
		name := strings.TrimSpace(strings.TrimPrefix(clean, IconSession))
		return Item{Kind: KindSession, Name: name, Target: name}, name != ""
	case strings.HasPrefix(clean, IconAgent):
		pane := AgentPaneFromLine(clean)
		return Item{Kind: KindAgent, PaneID: pane, Target: pane}, pane != ""
	case strings.HasPrefix(clean, IconZoxide):
		path := strings.TrimSpace(strings.TrimPrefix(clean, IconZoxide))
		return Item{Kind: KindZoxide, Path: path, Target: ExpandHome(path)}, path != ""
	case strings.HasPrefix(clean, IconFD):
		path := strings.TrimSpace(strings.TrimPrefix(clean, IconFD))
		return Item{Kind: KindFD, Path: path, Target: ExpandHome(path)}, path != ""
	default:
		return Item{}, false
	}
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
