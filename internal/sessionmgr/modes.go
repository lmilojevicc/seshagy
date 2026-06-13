package sessionmgr

// ModeNames holds the config- and human-facing labels for a SourceMode. It is
// the single source of truth for mode naming across config and the TUI.
type ModeNames struct {
	ConfigToken string // stable token persisted in config (e.g. "current-agents")
	Tab         string // short tab label (e.g. "Current agents")
	Title       string // panel title (e.g. "Current session agents")
	List        string // lowercase status phrasing (e.g. "current agents")
}

var modeNames = map[SourceMode]ModeNames{
	ModeAll:      {ConfigToken: "all", Tab: "All", Title: "All", List: "all"},
	ModeSessions: {ConfigToken: "sessions", Tab: "Sessions", Title: "Sessions", List: "sessions"},
	ModeAgents:   {ConfigToken: "agents", Tab: "Agents", Title: "Agents", List: "agents"},
	ModeCurrentAgents: {
		ConfigToken: "current-agents",
		Tab:         "Current agents",
		Title:       "Current session agents",
		List:        "current agents",
	},
	ModeZoxide: {ConfigToken: "zoxide", Tab: "Zoxide", Title: "Zoxide", List: "zoxide"},
	ModeFD:     {ConfigToken: "fd", Tab: "fd", Title: "fd", List: "fd"},
}

// Names returns the label set for mode, falling back to the ModeAll labels for
// unknown values (preserving the previous default-branch behavior).
func (m SourceMode) Names() ModeNames {
	if n, ok := modeNames[m]; ok {
		return n
	}
	return modeNames[ModeAll]
}
