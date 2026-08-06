// Package completion owns seshagy's canonical command-line completion model.
package completion

import "github.com/lmilojevicc/seshagy/internal/integrations"

type Directive string

const (
	NoFiles Directive = "nofiles"
	Files   Directive = "files"
	Dirs    Directive = "dirs"
)

type ValueKind string

const (
	ValueNone        ValueKind = "none"
	ValueText        ValueKind = "text"
	ValueInteger     ValueKind = "integer"
	ValueDirectory   ValueKind = "directory"
	ValueEnum        ValueKind = "enum"
	ValuePane        ValueKind = "pane"
	ValueAgent       ValueKind = "agent"
	ValueSource      ValueKind = "source"
	ValueSessionLine ValueKind = "session-line"
)

type Value struct {
	Kind           ValueKind
	Values         []string
	ValuesByTarget map[string][]string
}

type Flag struct {
	Name        string
	Description string
	Value       Value
	Targets     []string
}

type Positional struct {
	Name        string
	Description string
	Value       Value
}

type Command struct {
	Name        string
	Description string
	Aliases     []string
	Machine     bool
	Terminal    bool
	Flags       []Flag
	Positionals []Positional
	Children    []Command
}

const (
	TmuxModePopup      = "popup"
	TmuxModeWindow     = "window"
	TmuxModePane       = "pane"
	TmuxModePaneZoomed = "pane-zoomed"
	HerdrModePane      = "pane"
	HerdrModePopup     = "popup"
)

var states = []string{"idle", "working", "blocked", "done", "unknown"}

func boolFlag(name, description string) Flag {
	return Flag{Name: name, Description: description, Value: Value{Kind: ValueNone}}
}

func valueFlag(name, description string, kind ValueKind, values ...string) Flag {
	return Flag{Name: name, Description: description, Value: Value{Kind: kind, Values: values}}
}

var Root = Command{
	Description: "Agent-aware terminal dashboard",
	Flags: []Flag{
		boolFlag("--ephemeral", "Exit when the dashboard loses focus"),
	},
	Children: []Command{
		{Name: "--help", Description: "Show help", Aliases: []string{"-h", "help"}, Terminal: true},
		{
			Name:        "--version",
			Description: "Show version",
			Aliases:     []string{"version"},
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "config",
			Description: "Inspect or initialize configuration",
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
			Children: []Command{
				{
					Name:        "path",
					Description: "Print the config file path",
					Terminal:    true,
					Flags:       []Flag{boolFlag("--json", "Emit JSON")},
				},
				{
					Name:        "show",
					Description: "Print effective configuration",
					Terminal:    true,
					Flags:       []Flag{boolFlag("--json", "Emit JSON")},
				},
				{
					Name:        "init",
					Description: "Initialize configuration",
					Terminal:    true,
					Flags: []Flag{
						boolFlag("--force", "Overwrite an existing file"),
						boolFlag("--json", "Emit JSON"),
					},
				},
			},
		},
		{
			Name:        "diagnostics",
			Description: "Show logging diagnostics",
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{Name: "integration", Description: "Manage agent integrations", Children: []Command{
			{
				Name:        "install",
				Description: "Install an agent integration",
				Terminal:    true,
				Positionals: []Positional{
					{
						Name:        "name",
						Description: "Integration name",
						Value:       Value{Kind: ValueEnum, Values: integrations.Available()},
					},
				},
			},
			{
				Name:        "uninstall",
				Description: "Uninstall an agent integration",
				Terminal:    true,
				Positionals: []Positional{
					{
						Name:        "name",
						Description: "Integration name",
						Value:       Value{Kind: ValueEnum, Values: integrations.Available()},
					},
				},
			},
		}},
		{Name: "keybind", Description: "Manage multiplexer keybindings", Children: []Command{
			{
				Name:        "install",
				Description: "Install a keybinding",
				Terminal:    true,
				Positionals: []Positional{
					{
						Name:        "target",
						Description: "Multiplexer target",
						Value:       Value{Kind: ValueEnum, Values: []string{"tmux", "herdr"}},
					},
				},
				Flags: []Flag{
					valueFlag(
						"--key",
						"Prefix key",
						ValueText,
					),
					{
						Name:        "--mode",
						Description: "Launch mode",
						Value: Value{
							Kind: ValueEnum,
							ValuesByTarget: map[string][]string{
								"tmux": {
									TmuxModePopup,
									TmuxModeWindow,
									TmuxModePane,
									TmuxModePaneZoomed,
								},
								"herdr": {HerdrModePane, HerdrModePopup},
							},
						},
					},
					{
						Name:        "--width",
						Description: "Popup width",
						Value:       Value{Kind: ValueText},
						Targets:     []string{"herdr"},
					},
					{
						Name:        "--height",
						Description: "Popup height",
						Value:       Value{Kind: ValueText},
						Targets:     []string{"herdr"},
					},
					boolFlag("--persistent", "Disable focus-loss dismissal"),
				},
			},
			{
				Name:        "uninstall",
				Description: "Uninstall a keybinding",
				Terminal:    true,
				Positionals: []Positional{
					{
						Name:        "target",
						Description: "Multiplexer target",
						Value:       Value{Kind: ValueEnum, Values: []string{"tmux", "herdr"}},
					},
				},
			},
		}},
		{
			Name:        "completion",
			Description: "Generate a shell completion script",
			Terminal:    true,
			Positionals: []Positional{
				{
					Name:        "shell",
					Description: "Shell name",
					Value:       Value{Kind: ValueEnum, Values: []string{"bash", "zsh", "fish"}},
				},
			},
		},
		{
			Name:        "--get-all",
			Description: "Print all dashboard items",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--get-sessions",
			Description: "Print sessions or workspaces",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--get-zoxide",
			Description: "Print zoxide directories",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--get-fd",
			Description: "Print fd directories",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--get-agents",
			Description: "Print agent panes",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--get-current-session-agents",
			Description: "Print agents in the current session",
			Machine:     true,
			Terminal:    true,
			Flags:       []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--delete-item",
			Description: "Delete a rendered session line",
			Machine:     true,
			Terminal:    true,
			Positionals: []Positional{
				{
					Name:        "line",
					Description: "Rendered session line",
					Value:       Value{Kind: ValueSessionLine},
				},
			},
			Flags: []Flag{boolFlag("--json", "Emit JSON")},
		},
		{
			Name:        "--report-agent",
			Description: "Report agent state",
			Machine:     true,
			Terminal:    true,
			Flags: []Flag{
				valueFlag(
					"--pane",
					"Target pane ID",
					ValuePane,
				),
				valueFlag("--cwd", "Target working directory", ValueDirectory),
				valueFlag(
					"--agent",
					"Agent name",
					ValueAgent,
				),
				valueFlag("--state", "Agent state", ValueEnum, states...),
				valueFlag(
					"--source",
					"Report source",
					ValueSource,
				),
				valueFlag("--seq", "Monotonic sequence number", ValueInteger),
				valueFlag(
					"--message",
					"Optional status message",
					ValueText,
				),
				valueFlag("--session-id", "Optional agent session ID", ValueText),
				boolFlag("--json", "Emit JSON"),
			},
		},
		{
			Name:        "--release-agent",
			Description: "Release agent state",
			Machine:     true,
			Terminal:    true,
			Flags: []Flag{
				valueFlag(
					"--pane",
					"Target pane ID",
					ValuePane,
				),
				valueFlag("--cwd", "Target working directory", ValueDirectory),
				valueFlag(
					"--source",
					"Report source",
					ValueSource,
				),
				valueFlag("--seq", "Monotonic sequence number", ValueInteger),
				boolFlag("--json", "Emit JSON"),
			},
		},
	},
}

func FindChild(command *Command, name string) *Command {
	for i := range command.Children {
		if command.Children[i].Name == name {
			return &command.Children[i]
		}
		for _, alias := range command.Children[i].Aliases {
			if alias == name {
				return &command.Children[i]
			}
		}
	}
	return nil
}
