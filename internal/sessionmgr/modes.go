package sessionmgr

// modeConfigTokens holds the stable token of each SourceMode, persisted in
// config and emitted in JSON.
var modeConfigTokens = map[SourceMode]string{
	ModeAll:           "all",
	ModeSessions:      "sessions",
	ModeZoxide:        "zoxide",
	ModeFD:            "fd",
	ModeAgents:        "agents",
	ModeCurrentAgents: "current-agents",
}

// ConfigToken returns the stable token for mode, falling back to "all" for
// unknown values.
func (m SourceMode) ConfigToken() string {
	if token, ok := modeConfigTokens[m]; ok {
		return token
	}
	return modeConfigTokens[ModeAll]
}
