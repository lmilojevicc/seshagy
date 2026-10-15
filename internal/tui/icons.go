package tui

// Agent logos and label colors for the picker.
//
// The logo glyphs come from the "Herdr Agent Icons Max" font by herdr-radar
// (https://github.com/hhdebb/herdr-radar, MIT). The font maps vendor marks to
// the private-use block starting at U+E1A0 in the vendor order below; the
// vendor marks themselves belong to their owners. The font is never installed
// or configured by seshagy: when it is not detected locally, agents render
// with their state glyph only.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// agentLogoBase is the first codepoint of the vendor block; logos follow in
// agentLogoVendors order.
const agentLogoBase = 0xE1A0

// agentLogoVendors is the vendor order the font was built with. Do not reorder.
var agentLogoVendors = []string{
	"claude", "codex", "opencode", "omp", "cline", "mastracode", "kimi", "kilo",
	"maki", "pi", "hermes", "cursor", "copilot", "deepseek", "gemini", "gpt",
	"qwen", "grok", "agy", "kiro", "amp", "devin", "qodercli", "glm",
}

// agentAliases maps seshagy/herdr agent names onto their vendor logo key.
var agentAliases = map[string]string{
	"antigravity":  "agy",
	"cursor-agent": "cursor",
	"kiro-cli":     "kiro",
	"open-code":    "opencode",
}

// agentLabelColors are truecolor label colors for the agents whose brands have
// a recognizable hue; every other agent keeps the terminal's default fg.
var agentLabelColors = map[string]string{
	"claude":   "#D97757",
	"codex":    "#10A37F",
	"kimi":     "#9B8AFB",
	"gemini":   "#4285F4",
	"cursor":   "#A78BFA",
	"opencode": "#38BDF8",
}

// agentKey normalizes an agent name to its short vendor key: lowercase, the
// part before any ":" qualifier, a trailing " code" stripped, and known
// aliases resolved.
func agentKey(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if i := strings.Index(key, ":"); i >= 0 {
		key = key[:i]
	}
	key = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(key), " code"))
	if alias, ok := agentAliases[key]; ok {
		return alias
	}
	return key
}

// agentLogo returns the private-use logo rune for an agent name, or "" when the
// font has no mark for it.
func agentLogo(name string) string {
	key := agentKey(name)
	for index, vendor := range agentLogoVendors {
		if vendor == key {
			return string(rune(agentLogoBase + index))
		}
	}
	return ""
}

// agentColor returns the label color for an agent name, or NoColor when the
// agent has no configured brand color.
func agentColor(name string) lipgloss.TerminalColor {
	if hex, ok := agentLabelColors[agentKey(name)]; ok {
		return lipgloss.Color(hex)
	}
	return lipgloss.NoColor{}
}

// iconMode is the resolved agent-logo display mode.
type iconMode int

const (
	iconModeAuto iconMode = iota
	iconModeFont
	iconModeNone
)

// agentIconsMode reads the SESHAGY_AGENT_ICONS override (auto|font|none).
func agentIconsMode() iconMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SESHAGY_AGENT_ICONS"))) {
	case "font":
		return iconModeFont
	case "none":
		return iconModeNone
	default:
		return iconModeAuto
	}
}

var (
	agentFontOnce sync.Once
	agentFontOK   bool
)

// agentFontAvailable reports whether the Herdr Agent Icons Max font file is
// installed locally, checking the platform font directories once per process.
func agentFontAvailable() bool {
	agentFontOnce.Do(func() {
		var dirs []string
		home, err := os.UserHomeDir()
		if err == nil {
			if runtime.GOOS == "darwin" {
				dirs = []string{
					filepath.Join(home, "Library", "Fonts"),
					"/Library/Fonts",
				}
			} else {
				data := os.Getenv("XDG_DATA_HOME")
				if data == "" {
					data = filepath.Join(home, ".local", "share")
				}
				dirs = []string{filepath.Join(data, "fonts"), filepath.Join(home, ".fonts")}
			}
		}
		for _, dir := range dirs {
			matches, err := filepath.Glob(filepath.Join(dir, "HerdrAgentIconsMax*.ttf"))
			if err == nil && len(matches) > 0 {
				agentFontOK = true
				return
			}
		}
	})
	return agentFontOK
}

// showAgentLogos reports whether agent logo glyphs are drawn. Private-use
// glyphs are printed only when the configured icon set renders icons (not
// text/none) and the env-driven font mode resolves to "font".
func (m Model) showAgentLogos() bool {
	icons := m.config.IconSet()
	if !icons.Enabled || icons.ASCII {
		return false
	}
	switch agentIconsMode() {
	case iconModeNone:
		return false
	case iconModeFont:
		return true
	default:
		return agentFontAvailable()
	}
}
