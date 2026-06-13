package integrations

import "strings"

func HookCapableAgent(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, spec := range specs() {
		if name == AgentNameForTarget(spec.target) {
			return true
		}
	}
	return false
}

func AgentNameForTarget(target Target) string {
	return string(target)
}
