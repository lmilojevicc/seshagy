package sessionmgr

import (
	"time"
)

// ItemJSON is the script-friendly representation of a list item.
type ItemJSON struct {
	Kind      string `json:"kind"`
	Key       string `json:"key,omitempty"`
	Name      string `json:"name,omitempty"`
	Target    string `json:"target,omitempty"`
	Path      string `json:"path,omitempty"`
	Line      string `json:"line,omitempty"`
	LinePlain string `json:"line_plain,omitempty"`
	Attached  bool   `json:"attached"`
	Windows   int    `json:"windows,omitempty"`

	CreatedAt  *time.Time `json:"created_at,omitempty"`
	ActivityAt *time.Time `json:"activity_at,omitempty"`

	PaneID   string `json:"pane_id,omitempty"`
	Session  string `json:"session,omitempty"`
	Window   string `json:"window,omitempty"`
	Pane     string `json:"pane,omitempty"`
	Location string `json:"location,omitempty"`

	AgentName        string `json:"agent_name,omitempty"`
	AgentDisplayName string `json:"agent_display_name,omitempty"`
	AgentState       string `json:"agent_state,omitempty"`
}

// ItemsJSON wraps a mode query result.
type ItemsJSON struct {
	SchemaVersion int        `json:"schema_version"`
	Ok            bool       `json:"ok"`
	Mode          string     `json:"mode"`
	Warning       string     `json:"warning,omitempty"`
	Items         []ItemJSON `json:"items"`
}

func ItemToJSON(item Item, icons IconSet) ItemJSON {
	formattedLine := FormatLineWithIcons(item, icons)
	out := ItemJSON{
		Kind:             string(item.Kind),
		Key:              item.Key(),
		Name:             item.Name,
		Target:           item.Target,
		Path:             item.Path,
		Line:             formattedLine,
		LinePlain:        StripANSI(formattedLine),
		Attached:         item.Attached,
		Windows:          item.Windows,
		PaneID:           item.PaneID,
		Session:          item.Session,
		Window:           item.Window,
		Pane:             item.Pane,
		Location:         item.Location,
		AgentName:        item.AgentName,
		AgentDisplayName: item.AgentDisplayName,
		AgentState:       string(item.AgentState),
	}
	if !item.Created.IsZero() {
		created := item.Created.UTC()
		out.CreatedAt = &created
	}
	if !item.Activity.IsZero() {
		activity := item.Activity.UTC()
		out.ActivityAt = &activity
	}
	return out
}

func ItemsToJSON(mode SourceMode, items []Item, icons IconSet, warning string) ItemsJSON {
	out := make([]ItemJSON, 0, len(items))
	for _, item := range items {
		out = append(out, ItemToJSON(item, icons))
	}
	return ItemsJSON{
		SchemaVersion: 1,
		Ok:            true,
		Mode:          mode.Names().ConfigToken,
		Warning:       warning,
		Items:         out,
	}
}

const JSONSchemaVersion = 1
