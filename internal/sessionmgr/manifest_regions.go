package sessionmgr

import "strings"

type manifestDetectionInput struct {
	screen      string
	oscTitle    string
	oscProgress string
}

func manifestRegion(input manifestDetectionInput, spec string) string {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "osc_title":
		return input.oscTitle
	case "osc_progress":
		return input.oscProgress
	}

	content := input.screen
	switch spec {
	case "", "whole_recent":
		return content
	case "after_last_prompt_marker":
		return afterLastPromptMarker(content)
	case "before_current_prompt_marker":
		return beforeCurrentPromptMarker(content)
	case "whole_recent_without_current_prompt_marker":
		return wholeRecentWithoutCurrentPromptMarker(content)
	case "after_last_horizontal_rule":
		return afterLastHorizontalRule(content)
	case "above_prompt_box":
		return abovePromptBox(content)
	case "last_non_empty_above_prompt_box":
		return lastNonEmptyLine(abovePromptBox(content))
	case "prompt_box_body":
		body, _ := promptBoxBody(content)
		return body
	case "current_prompt_block_marker":
		marker, _ := currentPromptBlockMarker(content)
		return marker
	case "after_current_prompt_block_marker":
		after, _ := afterCurrentPromptBlockMarker(content)
		return after
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

func beforeCurrentPromptMarker(content string) string {
	lines := manifestLines(content)
	index, ok := currentCodexPromptIndex(lines)
	if !ok {
		return content
	}
	return content[:lineStartOffset(content, lines, index)]
}

func wholeRecentWithoutCurrentPromptMarker(content string) string {
	lines := manifestLines(content)
	if _, ok := currentCodexPromptIndex(lines); ok {
		return ""
	}
	return content
}

func currentPromptBlockMarker(content string) (string, bool) {
	lines := manifestLines(content)
	promptIndex, ok := currentCodexPromptIndex(lines)
	if !ok {
		return "", false
	}
	for i := promptIndex - 1; i >= 0; i-- {
		if codexBlockMarkerLine(lines[i]) {
			return lines[i], true
		}
	}
	return "", false
}

func afterCurrentPromptBlockMarker(content string) (string, bool) {
	lines := manifestLines(content)
	promptIndex, ok := currentCodexPromptIndex(lines)
	if !ok {
		return "", false
	}
	blockIndex := -1
	for i := promptIndex - 1; i >= 0; i-- {
		if codexBlockMarkerLine(lines[i]) {
			blockIndex = i
			break
		}
	}
	if blockIndex < 0 {
		return "", false
	}
	return manifestSliceFromLineIndex(content, lines, blockIndex), true
}

func currentCodexPromptIndex(lines []string) (int, bool) {
	index := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if codexPromptLine(lines[i]) {
			index = i
			break
		}
	}
	if index < 0 {
		return 0, false
	}
	for i := index + 1; i < len(lines); i++ {
		if codexBlockMarkerLine(lines[i]) {
			return 0, false
		}
	}
	return index, true
}

func codexPromptLine(line string) bool {
	return line == "›" || strings.HasPrefix(line, "› ")
}

func codexBlockMarkerLine(line string) bool {
	return strings.HasPrefix(line, "•") ||
		strings.HasPrefix(line, "■") ||
		strings.HasPrefix(line, "✗") ||
		strings.HasPrefix(line, "✓")
}

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

func abovePromptBox(content string) string {
	lines := manifestLines(content)
	top, ok := promptBoxTopBorderIndex(lines)
	if !ok {
		return content
	}
	end := lineStartOffset(content, lines, top)
	if end > len(content) {
		end = len(content)
	}
	return content[:end]
}

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

func lastNonEmptyLine(content string) string {
	lines := manifestLines(content)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
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
