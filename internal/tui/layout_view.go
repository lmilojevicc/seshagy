package tui

// layoutView is the value-only semantic input to normal dashboard rendering.
// It deliberately excludes controller state, effects, backend objects, and
// actionable targets.
type layoutView struct {
	Frame      frameView
	Sources    sourcesView
	Collection collectionView
	Search     searchView
	Actions    actionsView
}

type frameView struct {
	Width  int
	Height int
}

type sourceID string

type sourcesView struct {
	Entries      []sourceEntryView
	VisibleCount int
	TotalCount   int
	Loading      bool
	Refreshing   bool
	SpinnerFrame int
}

type sourceEntryView struct {
	ID         sourceID
	Key        string
	Label      string
	Selected   bool
	Count      int
	CountKnown bool
	Refreshing bool
}

type collectionState uint8

const (
	collectionReady collectionState = iota
	collectionLoading
	collectionEmpty
	collectionFilteredEmpty
	collectionError
)

type collectionView struct {
	Source           sourceID
	Title            string
	Rows             []rowView
	State            collectionState
	VisibleCount     int
	TotalCount       int
	SessionCount     int
	AgentCount       int
	DirectoryCount   int
	ScopeLabel       string
	StateFilterLabel string
	EmptyMessage     string
	Warning          string
	Error            string
}

type rowKind string

type displayModeView uint8

const (
	displayHidden displayModeView = iota
	displayIcon
	displayLabel
)

type indicatorView struct {
	Mode  displayModeView
	Icon  string
	Label string
	Color string
}

type sessionRowView struct {
	Attached bool
	State    indicatorView
	Activity string
}

type agentRowView struct {
	Name        string
	DisplayName string
	State       string
	Indicator   indicatorView
	Activity    string
	Location    string
}

type directoryRowView struct {
	Source sourceID
	Path   string
}

type rowView struct {
	Kind      rowKind
	Label     string
	Icon      indicatorView
	Session   sessionRowView
	Agent     agentRowView
	Directory directoryRowView
	Selected  bool
}

type searchModeView uint8

const (
	searchClassic searchModeView = iota
	searchTypeFirst
)

type searchView struct {
	Mode        searchModeView
	Query       string
	Editing     bool
	Prompt      string
	Placeholder string
	Prefix      string
}

type actionsView struct {
	Expanded    bool
	PrefixArmed bool
	Prefix      string
	Hints       []actionHintView
}

type actionHintView struct {
	Key       string
	Label     string
	Available bool
}

// layoutNeeds contains only optional semantic projections or controller-owned
// preparation. Mandatory Sources, Collection, Search, and Actions never become
// requirements.
type layoutNeeds struct {
	Overview bool
	Details  bool
	Preview  bool
}
