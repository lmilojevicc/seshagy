package sessionmgr

import "strings"

// manifestRegion selects the sub-slice of the capture buffer a rule matches.
// Supported regions: whole_recent, osc_title, osc_progress,
// bottom_lines(N), bottom_non_empty_lines(N), after_last_prompt_marker,
// after_last_horizontal_rule, prompt_box_body. The structural helpers
// (after_last_prompt_marker, prompt_box_body, after_last_horizontal_rule) are
// ported verbatim from the prior implementation so herdr's codex/claude
// manifests compile and classify identically.

func manifestRegion(input manifestDetectionInput, spec string) string {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "osc_title":
		return input.oscTitle
	case "osc_progress":
		return input.oscProgress
	case "", "whole_recent":
		return input.screen
	}

	content := input.screen
	switch spec {
	case "after_last_prompt_marker":
		return afterLastPromptMarker(content)
	case "after_last_horizontal_rule":
		return afterLastHorizontalRule(content)
	case "prompt_box_body":
		body, _ := promptBoxBody(content)
		return body
	default:
		if count, ok := manifestRegionCount(spec, "bottom_lines"); ok {
			return manifestBottomLines(content, count)
		}
		if count, ok := manifestRegionCount(spec, "bottom_non_empty_lines"); ok {
			return manifestBottomNonEmptyLines(content, count)
		}
		return ""
	}
}

func manifestLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineStartOffset(content string, lines []string, index int) int {
	if index < 0 {
		index = 0
	}
	if index > len(lines) {
		index = len(lines)
	}
	offset := 0
	for i := 0; i < index; i++ {
		offset += len(lines[i]) + 1
	}
	if offset > len(content) {
		offset = len(content)
	}
	return offset
}

// afterLastPromptMarker returns the content after the last codex-style `›`
// prompt line. When no marker is present, the whole buffer is returned.
func afterLastPromptMarker(content string) string {
	lines := manifestLines(content)
	index := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if codexPromptLine(lines[i]) {
			index = i
			break
		}
	}
	if index < 0 {
		return content
	}
	return manifestSliceFromLineIndex(content, lines, index+1)
}

func codexPromptLine(line string) bool {
	return line == "›" || strings.HasPrefix(line, "› ")
}

// afterLastHorizontalRule returns the content after the last `---`/box-rule
// line in the buffer.
func afterLastHorizontalRule(content string) string {
	lastRuleEnd := 0
	offset := 0
	for _, line := range manifestLines(content) {
		nextOffset := offset + len(line) + 1
		if isHorizontalRule(line) {
			if nextOffset > len(content) {
				lastRuleEnd = len(content)
			} else {
				lastRuleEnd = nextOffset
			}
		}
		offset = nextOffset
	}
	if lastRuleEnd > len(content) {
		lastRuleEnd = len(content)
	}
	return content[lastRuleEnd:]
}

// promptBoxBody returns the text inside the claude `❯` prompt box (delimited by
// two horizontal-rule borders). ok is false when no box is present.
func promptBoxBody(content string) (string, bool) {
	lines := manifestLines(content)
	top, ok := promptBoxTopBorderIndex(lines)
	if !ok {
		return "", false
	}
	start := lineStartOffset(content, lines, top+1)
	endIndex := len(lines)
	for i := top + 1; i < len(lines); i++ {
		if isHorizontalRule(lines[i]) {
			endIndex = i
			break
		}
	}
	end := lineStartOffset(content, lines, endIndex)
	if start > len(content) {
		start = len(content)
	}
	if end > len(content) {
		end = len(content)
	}
	return content[start:end], true
}

func promptBoxTopBorderIndex(lines []string) (int, bool) {
	borderCount := 0
	for index := len(lines) - 1; index >= 0; index-- {
		if isHorizontalRule(lines[index]) {
			borderCount++
			if borderCount == 2 {
				return index, true
			}
		}
	}
	return 0, false
}

func isHorizontalRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	ruleChars := 0
	ruleBytes := len(trimmed)
	for idx, ch := range trimmed {
		if ch != '─' {
			ruleBytes = idx
			break
		}
		ruleChars++
	}
	if ruleChars == 0 {
		return false
	}

	suffix := strings.TrimLeft(trimmed[ruleBytes:], " ")
	return suffix == "" || ruleChars >= 3
}
