package integrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	hermesPluginName         = "seshagy-agent-state"
	hermesPluginManifestName = "plugin.yaml"
	hermesPluginInitName     = "__init__.py"
)

func installHermes(binaryPath string) ([]string, error) {
	dir := hermesDir()
	if !configDirExists(dir) {
		return nil, fmt.Errorf("hermes config directory not found at %s", dir)
	}
	pluginDir := filepath.Join(dir, "plugins", hermesPluginName)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(pluginDir, hermesPluginManifestName)
	if err := os.WriteFile(manifestPath, []byte(hermesPluginManifestAsset()), 0o644); err != nil {
		return nil, err
	}
	initPath := filepath.Join(pluginDir, hermesPluginInitName)
	if err := os.WriteFile(initPath, []byte(hermesPluginInitAsset(binaryPath)), 0o644); err != nil {
		return nil, err
	}

	configPath := filepath.Join(dir, "config.yaml")
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	updated := ensureHermesPluginEnabled(string(existing))
	messages := []string{
		fmt.Sprintf("installed Hermes Agent plugin to %s", pluginDir),
	}
	if updated != string(existing) {
		if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
			return nil, err
		}
		messages = append(messages, fmt.Sprintf("enabled Hermes plugin in %s", configPath))
	}
	return messages, nil
}

func uninstallHermes() ([]string, error) {
	dir := hermesDir()
	pluginDir := filepath.Join(dir, "plugins", hermesPluginName)
	removedDir, err := removeDir(pluginDir)
	if err != nil {
		return nil, err
	}
	messages := removalMessages("Hermes Agent plugin", pluginDir, removedDir)

	configPath := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return messages, nil
		}
		return nil, err
	}
	updated := removeHermesPluginEnabled(string(data))
	if updated == string(data) {
		messages = append(messages, fmt.Sprintf("no Hermes plugin entry found in %s", configPath))
		return messages, nil
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return nil, err
	}
	messages = append(messages, fmt.Sprintf("disabled Hermes plugin in %s", configPath))
	return messages, nil
}

func ensureHermesPluginEnabled(content string) string {
	return updateHermesEnabledPlugin(content, true)
}

func removeHermesPluginEnabled(content string) string {
	return updateHermesEnabledPlugin(content, false)
}

func updateHermesEnabledPlugin(content string, enabled bool) string {
	trailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	pluginsIndex := topLevelYAMLKeyIndex(lines, "plugins")
	if pluginsIndex == -1 {
		if !enabled {
			return content
		}
		result := strings.TrimRight(content, "\n")
		if result != "" {
			result += "\n"
		}
		return result + "plugins:\n  enabled:\n    - " + hermesPluginName + "\n"
	}

	pluginsEnd := nextTopLevelYAMLKeyIndex(lines, pluginsIndex+1)
	if pluginsEnd == -1 {
		pluginsEnd = len(lines)
	}

	if inlineItems, ok := yamlFlowSequenceItems(
		yamlKeyValueAtIndent(lines[pluginsIndex], 0, "plugins"),
	); ok {
		return joinYAMLLines(
			updateHermesInlinePluginList(lines, pluginsIndex, pluginsEnd, inlineItems, enabled),
			trailingNewline,
		)
	}

	enabledIndex := yamlSubkeyIndex(lines, pluginsIndex+1, pluginsEnd, 2, "enabled")
	if enabledIndex != -1 {
		return joinYAMLLines(
			updateHermesEnabledBlock(lines, pluginsEnd, enabledIndex, enabled),
			trailingNewline,
		)
	}

	flatListStart := yamlFlatListStartIndex(lines, pluginsIndex+1, pluginsEnd, 2)
	if flatListStart != -1 {
		return joinYAMLLines(
			updateHermesFlatPluginList(lines, pluginsIndex, pluginsEnd, flatListStart, enabled),
			trailingNewline,
		)
	}

	if enabled {
		insertAt := pluginsIndex + 1
		lines = append(lines[:insertAt], append([]string{
			"  enabled:",
			"    - " + hermesPluginName,
		}, lines[insertAt:]...)...)
	}
	return joinYAMLLines(lines, trailingNewline)
}

func updateHermesEnabledBlock(
	lines []string,
	pluginsEnd, enabledIndex int,
	enabled bool,
) []string {
	line := strings.TrimSpace(lines[enabledIndex])
	if line == "enabled: []" || line == "enabled: [] # seshagy" {
		if enabled {
			lines[enabledIndex] = "  enabled:"
			lines = append(
				lines[:enabledIndex+1],
				append([]string{"    - " + hermesPluginName}, lines[enabledIndex+1:]...)...)
		}
		return lines
	}

	listStart := enabledIndex + 1
	listEnd := yamlBlockEndIndex(lines, listStart, pluginsEnd, 2)
	existingIndex := yamlListItemIndex(lines, listStart, listEnd, hermesPluginName, 4)
	switch {
	case enabled && existingIndex == -1:
		lines = append(
			lines[:listStart],
			append([]string{"    - " + hermesPluginName}, lines[listStart:]...)...)
	case !enabled && existingIndex != -1:
		lines = append(lines[:existingIndex], lines[existingIndex+1:]...)
	}
	return lines
}

func updateHermesFlatPluginList(
	lines []string,
	pluginsIndex, pluginsEnd, flatListStart int,
	enabled bool,
) []string {
	existingIndex := yamlListItemIndexAtIndent(
		lines,
		pluginsIndex+1,
		pluginsEnd,
		hermesPluginName,
		2,
	)
	switch {
	case enabled && existingIndex == -1:
		lines = append(
			lines[:flatListStart],
			append([]string{"  - " + hermesPluginName}, lines[flatListStart:]...)...)
	case !enabled && existingIndex != -1:
		lines = append(lines[:existingIndex], lines[existingIndex+1:]...)
	}
	return lines
}

func updateHermesInlinePluginList(
	lines []string,
	pluginsIndex, pluginsEnd int,
	items []string,
	enabled bool,
) []string {
	existingIndex := -1
	for i, item := range items {
		if item == hermesPluginName {
			existingIndex = i
			break
		}
	}
	switch {
	case enabled && existingIndex == -1:
		items = append([]string{hermesPluginName}, items...)
	case !enabled && existingIndex != -1:
		items = append(items[:existingIndex], items[existingIndex+1:]...)
	default:
		return lines
	}
	replacement := hermesFlatPluginLines(items)
	return append(append(lines[:pluginsIndex], replacement...), lines[pluginsEnd:]...)
}

func hermesFlatPluginLines(items []string) []string {
	if len(items) == 0 {
		return []string{"plugins: []"}
	}
	lines := []string{"plugins:"}
	for _, item := range items {
		lines = append(lines, "  - "+item)
	}
	return lines
}

func joinYAMLLines(lines []string, trailingNewline bool) string {
	result := strings.Join(lines, "\n")
	if trailingNewline {
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		return result
	}
	return strings.TrimRight(result, "\n")
}

func topLevelYAMLKeyIndex(lines []string, key string) int {
	for i, line := range lines {
		if yamlKeyAtIndent(line, 0) == key {
			return i
		}
	}
	return -1
}

func nextTopLevelYAMLKeyIndex(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if yamlIndent(lines[i]) == 0 && yamlKeyName(lines[i]) != "" {
			return i
		}
	}
	return -1
}

func yamlSubkeyIndex(lines []string, start, end, indent int, key string) int {
	for i := start; i < end; i++ {
		if yamlKeyAtIndent(lines[i], indent) == key {
			return i
		}
	}
	return -1
}

func yamlFlatListStartIndex(lines []string, start, end, indent int) int {
	for i := start; i < end; i++ {
		if yamlListItemValueAtIndent(lines[i], indent) != "" {
			return i
		}
	}
	return -1
}

func yamlBlockEndIndex(lines []string, start, end, indent int) int {
	for i := start; i < end; i++ {
		lineIndent := yamlIndent(lines[i])
		if lineIndent != -1 && lineIndent <= indent && yamlKeyName(lines[i]) != "" {
			return i
		}
	}
	return end
}

func yamlListItemIndex(lines []string, start, end int, value string, itemIndent int) int {
	for i := start; i < end; i++ {
		if yamlListItemMatchesAtIndent(lines[i], itemIndent, value) {
			return i
		}
	}
	return -1
}

func yamlListItemIndexAtIndent(lines []string, start, end int, value string, itemIndent int) int {
	return yamlListItemIndex(lines, start, end, value, itemIndent)
}

func yamlKeyAtIndent(line string, indent int) string {
	if yamlIndent(line) != indent {
		return ""
	}
	return yamlKeyName(line)
}

func yamlKeyName(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
		return ""
	}
	if idx := strings.Index(trimmed, ":"); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return ""
}

func yamlKeyValueAtIndent(line string, indent int, key string) string {
	if yamlKeyAtIndent(line, indent) != key {
		return ""
	}
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func yamlIndent(line string) int {
	if strings.TrimSpace(line) == "" {
		return -1
	}
	return len(line) - len(strings.TrimLeft(line, " "))
}

func yamlListItemValueAtIndent(line string, indent int) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "-") {
		return ""
	}
	if yamlIndent(line) != indent {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
}

func yamlListItemMatchesAtIndent(line string, indent int, value string) bool {
	item := yamlListItemValueAtIndent(line, indent)
	if item == "" {
		return false
	}
	item = strings.Trim(item, `"'`)
	return item == value
}

var yamlFlowSequencePattern = regexp.MustCompile(`^\[(.*)\]$`)

func yamlFlowSequenceItems(value string) ([]string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	match := yamlFlowSequencePattern.FindStringSubmatch(value)
	if match == nil {
		return nil, false
	}
	inner := strings.TrimSpace(match[1])
	if inner == "" {
		return []string{}, true
	}
	items := strings.Split(inner, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out, true
}

func removeDir(path string) (bool, error) {
	if err := os.RemoveAll(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
