package cli

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// helpHeaderRe matches a non-indented section header: a Title-case label
	// ending in ":" (Usage:, Scripting:, TUI keys:, Config:).
	helpHeaderRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9 ]*:[ \t]*$`)
	// helpFlagRe matches a long flag token (--json, --get-all, --report-agent).
	helpFlagRe = regexp.MustCompile(`--[A-Za-z][A-Za-z0-9-]*`)
	// helpBinRe matches the leading binary name on an indented usage line.
	helpBinRe = regexp.MustCompile(`^([ \t]+)(seshagy)\b`)
	// helpMetavarRe matches a metavar placeholder (<name>, <key>, <cells|percent>).
	helpMetavarRe = regexp.MustCompile(`<[^>\n]+>`)
)

// renderHelp applies a two-tone accent theme to a hand-written help string:
// section headers (non-indented "Title:" lines) become bold+underlined+cyan,
// the leading binary name on usage lines and long flags become bold, metavars
// (<name>, <key>, …) become dim, and descriptions stay plain. When the styles
// are zero-valued (color disabled) every Render call is a no-op and the input
// is returned unchanged, byte for byte.
func renderHelp(header, literal, metavar lipgloss.Style, text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = renderHelpLine(header, literal, metavar, line)
	}
	return strings.Join(lines, "\n")
}

func renderHelpLine(header, literal, metavar lipgloss.Style, line string) string {
	if line == "" {
		return line
	}
	trimmed := strings.TrimRight(line, " \t")
	// Section header: no leading indentation and a title-case label ending in ":".
	if !startsWithSpace(line) && helpHeaderRe.MatchString(trimmed) {
		// Render the full line (not trimmed): trailing whitespace is invisible
		// inside a style run and preserved verbatim when color is off.
		return header.Render(line)
	}
	// Bold the leading binary name on usage synopsis lines ("  seshagy ...").
	out := helpBinRe.ReplaceAllStringFunc(line, func(m string) string {
		sub := helpBinRe.FindStringSubmatch(m)
		return sub[1] + literal.Render(sub[2])
	})
	// Bold long flags anywhere on the line.
	out = helpFlagRe.ReplaceAllStringFunc(out, func(m string) string {
		return literal.Render(m)
	})
	// Dim metavar placeholders (<name>, <key>, <cells|percent>, …).
	out = helpMetavarRe.ReplaceAllStringFunc(out, func(m string) string {
		return metavar.Render(m)
	})
	return out
}

func startsWithSpace(s string) bool {
	return len(s) > 0 && (s[0] == ' ' || s[0] == '\t')
}
