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
	Overview   *overviewView
	Details    *detailsView
	Preview    *previewView
}

type frameView struct {
	Width         int
	Height        int
	ContentHeight int
}

type sourceID string

type sourcesView struct {
	Entries        []sourceEntryView
	VisibleCount   int
	TotalCount     int
	ShowTotalCount bool
	Loading        bool
	Refreshing     bool
	SpinnerFrame   int
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
	SessionPlural    string
	Rows             []rowView
	State            collectionState
	VisibleCount     int
	TotalCount       int
	SessionCount     int
	AgentCount       int
	DirectoryCount   int
	ScopeLabel       string
	AgentStateFilter string
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
	Location    string
}

type directoryRowView struct {
	Path string
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
	Mode    searchModeView
	Query   string
	Editing bool
}

type actionsView struct {
	Expanded    bool
	Prefix      string
	PrefixArmed bool
	Hints       []actionHintView
}

type actionHintView struct {
	Key       string
	Label     string
	Available bool
}

type overviewState uint8

const (
	overviewLoading overviewState = iota
	overviewEmpty
	overviewError
	overviewReady
)

type overviewAgentCountsView struct {
	Working int
	Blocked int
	Done    int
	Idle    int
	Unknown int
}

type overviewView struct {
	State    overviewState
	Items    int
	Sessions int
	Agents   overviewAgentCountsView
	Warning  string
	Error    string
}

type detailsState uint8

const (
	detailsNoSelection detailsState = iota
	detailsReady
)

type detailIndicatorKind uint8

const (
	detailIndicatorNone detailIndicatorKind = iota
	detailIndicatorAttached
	detailIndicatorAgentState
)

type detailFieldView struct {
	Label         string
	Value         string
	Indicator     indicatorView
	IndicatorKind detailIndicatorKind
	Attached      bool
	AgentState    string
}

type detailsView struct {
	State  detailsState
	Title  string
	Fields []detailFieldView
}

type previewState uint8

const (
	previewLoading previewState = iota
	previewEmpty
	previewError
	previewReady
)

type previewAnchor uint8

const (
	previewAnchorTop previewAnchor = iota
	previewAnchorBottom
)

type previewView struct {
	State   previewState
	Pending bool
	Title   string
	Content string
	Error   string
	Anchor  previewAnchor
}

// layoutNeeds contains only optional semantic projections or controller-owned
// preparation. Mandatory Sources, Collection, Search, and Actions never become
// requirements.
type layoutNeeds struct {
	Overview bool
	Details  bool
	Preview  bool
}

type renderLayout func(layoutView, layoutRenderTheme) renderedDashboard

type renderLayoutActions func(actionsView, styles, int) string

type renderLayoutInput func(inputChrome, styles, int) string

type layoutInputWidth func(int) int

type layoutID string

const (
	layoutDefault layoutID = "default"
	layoutZen     layoutID = "zen"
)

type layoutSpec struct {
	id                layoutID
	render            renderLayout
	renderActions     renderLayoutActions
	renderInput       renderLayoutInput
	inputContentWidth layoutInputWidth
	needs             layoutNeeds
}

var defaultLayout = layoutSpec{
	id:                layoutDefault,
	render:            renderDefault,
	renderActions:     renderDefaultActionsTile,
	renderInput:       renderCmdlineInput,
	inputContentWidth: defaultInputContentWidth,
	needs: layoutNeeds{
		Overview: true,
		Details:  true,
		Preview:  true,
	},
}

var zenLayout = layoutSpec{
	id:                layoutZen,
	render:            renderZen,
	renderActions:     renderZenActionsLine,
	renderInput:       renderZenInlineInput,
	inputContentWidth: zenInputContentWidth,
	needs: layoutNeeds{
		Overview: true,
	},
}

func resolveLayout(name string) (layoutSpec, bool) {
	switch layoutID(name) {
	case "", layoutDefault:
		return defaultLayout, true
	case layoutZen:
		return zenLayout, true
	default:
		return defaultLayout, false
	}
}
