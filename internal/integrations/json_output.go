package integrations

// RecommendationJSON is the script-friendly integration scan record.
type RecommendationJSON struct {
	Target         string   `json:"target"`
	Label          string   `json:"label"`
	Commands       []string `json:"commands,omitempty"`
	ConfigDir      string   `json:"config_dir,omitempty"`
	InstallPath    string   `json:"install_path,omitempty"`
	AgentAvailable bool     `json:"agent_available"`
	State          string   `json:"state"`
	Version        int      `json:"version,omitempty"`
	Installable    bool     `json:"installable"`
	Reason         string   `json:"reason,omitempty"`
	Authority      string   `json:"authority"`
}

// RecommendationsJSON wraps integration scan results.
type RecommendationsJSON struct {
	Integrations []RecommendationJSON `json:"integrations"`
}

func RecommendationToJSON(rec Recommendation) RecommendationJSON {
	return RecommendationJSON{
		Target:         string(rec.Target),
		Label:          rec.Label,
		Commands:       append([]string(nil), rec.Commands...),
		ConfigDir:      rec.ConfigDir,
		InstallPath:    rec.InstallPath,
		AgentAvailable: rec.AgentAvailable,
		State:          string(rec.State),
		Version:        rec.Version,
		Installable:    rec.Installable,
		Reason:         rec.Reason,
		Authority:      string(rec.Authority),
	}
}

func ScanToJSON(recs []Recommendation) RecommendationsJSON {
	out := make([]RecommendationJSON, 0, len(recs))
	for _, rec := range recs {
		out = append(out, RecommendationToJSON(rec))
	}
	return RecommendationsJSON{Integrations: out}
}
