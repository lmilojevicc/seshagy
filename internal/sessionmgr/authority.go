package sessionmgr

import (
	"strings"

	"github.com/lmilojevicc/seshagy/internal/integrations"
)

func HasLifecycleAuthority(agentName, source string) bool {
	for _, raw := range []string{source, agentName} {
		target, ok := authorityTarget(raw)
		if !ok {
			continue
		}
		return integrations.Authority(target) == integrations.LifecycleAuthority
	}
	return false
}

func authorityTarget(raw string) (integrations.Target, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "", false
	}
	if after, ok := strings.CutPrefix(raw, "seshagy:"); ok {
		raw = after
	}
	target, err := integrations.ParseTarget(raw)
	if err != nil {
		return "", false
	}
	return target, true
}
