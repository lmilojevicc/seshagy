package sessionmgr

import "strings"

// manifestRegion selects the sub-slice of the capture buffer a rule matches.
// Only the regions used by the bundled manifests are supported:
// whole_recent, osc_title, bottom_lines(N), bottom_non_empty_lines(N).

func manifestRegion(input manifestDetectionInput, spec string) string {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "osc_title":
		return input.oscTitle
	case "", "whole_recent":
		return input.screen
	default:
		if count, ok := manifestRegionCount(spec, "bottom_lines"); ok {
			return manifestBottomLines(input.screen, count)
		}
		if count, ok := manifestRegionCount(spec, "bottom_non_empty_lines"); ok {
			return manifestBottomNonEmptyLines(input.screen, count)
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
