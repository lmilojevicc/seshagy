package tui

import (
	"strings"

	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

// queryTokens is the prompt query split into herdr-navigator's prefix grammar
// plus the plain text the fuzzy matcher ranks. Every token is lowercased and
// matched as a case-insensitive substring, and the tokens are ANDed with the
// ctrl scope and the agent-state filter.
type queryTokens struct {
	// plain is the unprefixed tokens joined with single spaces. It is the
	// pattern the fuzzy matcher runs, and the only source of row highlighting.
	plain string
	// agent holds the values of the !x tokens, matched against the agent's name
	// and its kind.
	agent []string
	// workspaceState holds the values of the @x tokens, matched against the
	// agent's workspace label or its state.
	workspaceState []string
	// path holds the values of the /x tokens, matched against the item's path.
	path []string
	// state holds the values of the #x tokens, matched against the agent state.
	state []string
	// allAgents records a bare @: every agent, whatever its workspace or state.
	allAgents bool
}

// parseQuery splits the query on whitespace and reads each token's prefix. A
// prefix with nothing behind it contributes no value, so it only marks the
// query as tokenised — it never falls back to plain text.
func parseQuery(query string) queryTokens {
	var tokens queryTokens
	plain := make([]string, 0, 4)
	for _, raw := range strings.Fields(query) {
		token := strings.ToLower(raw)
		switch {
		case strings.HasPrefix(token, "!"):
			tokens.agent = appendToken(tokens.agent, token[1:])
		case strings.HasPrefix(token, "@"):
			if token[1:] == "" {
				tokens.allAgents = true
				continue
			}
			tokens.workspaceState = appendToken(tokens.workspaceState, token[1:])
		case strings.HasPrefix(token, "/"):
			tokens.path = appendToken(tokens.path, token[1:])
		case strings.HasPrefix(token, "#"):
			tokens.state = appendToken(tokens.state, token[1:])
		default:
			plain = append(plain, token)
		}
	}
	tokens.plain = strings.Join(plain, " ")
	return tokens
}

// appendToken records one token's value, dropping the empty value a bare prefix
// leaves behind.
func appendToken(values []string, value string) []string {
	if value == "" {
		return values
	}
	return append(values, value)
}

// agentOnly reports whether a token narrowed the query to agents. Every agent
// token does: !x, @x, #x, and a bare @.
func (t queryTokens) agentOnly() bool {
	return t.allAgents || len(t.agent) > 0 || len(t.workspaceState) > 0 || len(t.state) > 0
}

// matches reports whether an item survives every token. The plain part is not
// tested here: it is the pattern the fuzzy matcher ranks.
func (t queryTokens) matches(item sessionmgr.Item) bool {
	if t.agentOnly() && item.Kind != sessionmgr.KindAgent {
		return false
	}
	return allContained(t.agent, agentQueryText(item)) &&
		allContainedEither(t.workspaceState, workspaceQueryText(item), agentStateQueryText(item)) &&
		allContained(t.path, item.Path) &&
		allContained(t.state, agentStateQueryText(item))
}

// allContained reports whether every needle is a substring of the haystack,
// ignoring case.
func allContained(needles []string, haystack string) bool {
	if len(needles) == 0 {
		return true
	}
	haystack = strings.ToLower(haystack)
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

// allContainedEither is allContained against two haystacks: a token matches when
// either one contains it.
func allContainedEither(needles []string, left, right string) bool {
	if len(needles) == 0 {
		return true
	}
	left, right = strings.ToLower(left), strings.ToLower(right)
	for _, needle := range needles {
		if !strings.Contains(left, needle) && !strings.Contains(right, needle) {
			return false
		}
	}
	return true
}

// agentQueryText is what a !token is matched against: the name the row shows and
// the detected agent kind, so "claude" reaches a renamed claude pane.
func agentQueryText(item sessionmgr.Item) string {
	return item.DisplayName() + " " + item.AgentName
}

// workspaceQueryText is what the workspace half of an @token is matched against:
// the label the row shows for the agent's workspace (a herdr workspace label, a
// tmux session:window.pane location).
func workspaceQueryText(item sessionmgr.Item) string { return item.Location }

// agentStateQueryText is the normalized agent state a #token (and the state half
// of an @token) is matched against.
func agentStateQueryText(item sessionmgr.Item) string { return string(item.AgentState) }
