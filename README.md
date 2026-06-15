# seshagy

`seshagy` is an agent-aware tmux dashboard for jumping between project
sessions, discovered directories, and coding-agent panes.

Run one command to get a keyboard-first view where you can:

- jump to existing tmux sessions,
- create or switch sessions from directories found by `zoxide` and `fd`, and
- see hook-reported AI coding agent panes, their current state, and where they
  are running.

It is intentionally tmux-native. Hook-capable agents report identity and state
through `@agent_*` pane metadata from installed integrations; seshagy does not
scrape pane text. Hook-capable agents detected without integrations appear as
`unhooked` with an install hint. Agents without hook support may still appear
via process detection, but their state stays `unknown` unless explicitly reported.

## Quick start

Install from GitHub:

```sh
go install github.com/lmilojevicc/seshagy/cmd/seshagy@latest
```

Or from a local checkout:

```sh
git clone https://github.com/lmilojevicc/seshagy.git
cd seshagy
go install ./cmd/seshagy
# or: make install
```

Open the dashboard:

```sh
seshagy
```

Typical first run:

1. Start tmux or run `seshagy` from inside an existing tmux client.
2. Press `z` or `f` to browse project directories from `zoxide` or `fd`.
3. Press `enter` on a directory to create/switch to a tmux session for it.
4. Press `g` to view tracked agent panes, then `enter` to focus one.
5. Press `i` if you want to install detected agent hook integrations later.

First launch asks about detected missing integrations. After a seshagy upgrade,
the startup prompt appears again when installed hook versions are behind the
current release. Once you complete or skip that prompt, use `i` in the TUI or
`seshagy integration install <target>` manually.
The prompt is toggle-based, so each detected integration can be enabled or
skipped independently.

## Requirements

Required:

- `tmux`

Optional, but useful:

- `zoxide` for frecency-ranked directory history
- `fd` for filesystem directory discovery
- `yazi` for choosing a directory interactively
- `eza` for richer directory previews

Builds from source require Go 1.26, matching `go.mod`. Shell hook integrations
may use `bash`, `python3`, or `date` depending on the agent integration context.

## What seshagy manages

| Area | What you can do |
| --- | --- |
| tmux sessions | list, attach outside tmux, switch-client inside tmux, rename, kill, and preview sessions |
| project directories | create/switch tmux sessions from `zoxide` or a configurable `fd` command |
| agent panes | list, filter, focus, or kill panes that reported `@agent_*` metadata |
| current session agents | narrow the agent view to the current tmux session |
| input flow | use classic action keys or type-first filtering with a prefix key |

When a directory becomes a session, the session name is derived from the
basename: `.config` becomes `dot_config`, and unsupported characters collapse to
`_` (`foo.bar` becomes `foo_bar`).

## Agent tracking

seshagy tracks only agents that report metadata through supported hooks/plugins.
That keeps the model predictable: if an agent pane appears, something explicitly
reported it to tmux. Hook-capable agents (those with installable integrations)
require an installed integration; process-name fallback applies only to agents
without hook support, such as Gemini, Antigravity (`agy`), and similar tools.

Integrations use two authority tiers. **Lifecycle authority** means hooks own
idle/working/blocked state. Shell-hook integrations for Claude Code, Codex,
GitHub Copilot CLI, Factory Droid, Qoder CLI, Cursor Agent, and Grok Build report lifecycle
state through the shared `seshagy-agent-state.sh` hook. Plugin integrations for
Pi, OpenCode, Kimi Code, Kilo Code, and Hermes Agent do the same through their
respective plugins.

Supported integration targets:

| Target | Label | Tracking behavior |
| --- | --- | --- |
| `pi` | Pi | lifecycle state |
| `opencode` | OpenCode | lifecycle state |
| `kimi` | Kimi Code | lifecycle state |
| `claude` | Claude Code | lifecycle state |
| `codex` | Codex | lifecycle state |
| `copilot` | GitHub Copilot CLI | lifecycle state |
| `droid` | Factory Droid | lifecycle state |
| `qodercli` | Qoder CLI | lifecycle state |
| `cursor` | Cursor Agent | lifecycle state |
| `grok` | Grok Build | lifecycle state |
| `kilo` | Kilo Code | lifecycle state |
| `hermes` | Hermes Agent | lifecycle state |

Cursor Agent detection requires the `cursor-agent` command so the generic
Cursor editor CLI is not treated as a hook-capable agent. Grok Build also
recognizes the xAI `agent` symlink for availability scanning; process-name
fallback for `agent` requires a grok-related pane title.

States normalize to:

```text
working  blocked  aborted  done  idle  unknown
```

When hooks are silent, session-only and process-detected agents can infer
`working` or `blocked` from OSC pane titles (for example Braille spinners or
"Action Required") without scraping pane contents.

Optional screen manifest fallback (`[agents] manifest_fallback = true` in
`config.toml`) adds a Herdr-derived second pass when hooks are silent and state
is still `unknown`. Seshagy captures the last 30 pane lines and matches bundled
per-agent TOML rules for 17 agents (Claude, Codex, Cursor, Gemini, Grok,
Copilot, Antigravity, and others). Rules support screen regions, OSC title
matching, nested gates, `line_regex`, and `skip_state_update` for neutral UI
states such as transcript viewers. Known agents with no matching rule fall back
to `idle`. Hook-reported `working`, `blocked`, and `idle` are never overridden
by title inference or manifest fallback. When manifest fallback is enabled,
OSC title inference is deferred to manifest `osc_title` rules instead of the
legacy title heuristics. Remote manifest rules can be fetched from a catalog
URL; use `seshagy manifest status|update|reload [--json]` to inspect or refresh
them. When hooks stop reporting, stale hook state is cleared after a 5-minute
TTL so title inference and manifest fallback can recover.

Check and install integrations:

```sh
seshagy integration status
seshagy integration install pi
seshagy integration uninstall pi
```

After upgrading seshagy, run `seshagy integration install <target>` again for each
installed agent so hook versions stay current.

## TUI keys

| Key | Action |
| --- | --- |
| `enter` | attach/switch to a session, create/switch from a directory, or focus an agent pane |
| `j`/`k`, arrows | move selection |
| `1`..`6` | select source by configured order |
| `a` | all sources |
| `t` | tmux sessions |
| `g` | all tracked agents |
| `o` | tracked agents in the current tmux session |
| `z` | zoxide directories |
| `f` | fd directories |
| `s` / `S` | cycle / clear agent state filter |
| `/` | filter visible rows |
| `backspace` | clear filter when not editing |
| `r` | refresh |
| `R` | rename selected session |
| `x` | kill selected session or agent pane |
| `y` | open `yazi`, then create/switch from its exit directory |
| `i` | open integration installer prompt |
| `m` | change classic/type-first input mode |
| `p` | toggle preview pane |
| `?` / `h` | toggle help |
| `q` / `esc` / `ctrl-c` | quit |

In type-first mode, typing edits the filter immediately. Most action keys then
require the configured prefix first (`ctrl+x` by default). `enter` and movement
keys stay unprefixed.

## CLI helpers

The TUI is the main interface. The CLI helpers are useful for scripts, fzf-style
menus, and agent hooks.

```sh
seshagy --get-all
seshagy --get-sessions
seshagy --get-agents
seshagy --get-current-session-agents
seshagy --get-session-agents        # alias
seshagy --get-zoxide
seshagy --get-fd
seshagy --delete-item '<rendered line from --get-all>'
seshagy agent explain <pane-id>   # show why a pane has its agent state
```

All commands above (plus `config`, `integration`, `manifest`, and `--version`)
support a trailing `--json` flag for machine-readable JSON on stdout. Human text
output is unchanged when `--json` is omitted.

Successful responses include top-level `schema_version` and `ok` fields. With
`--json`, errors also print JSON on stdout (not stderr), for example
`{"schema_version":1,"ok":false,"error":"...","code":"usage|error"}`.
Scripts should check the exit code and the `ok` field.

`--get-* --json` returns structured fields per item (`kind`, `pane_id`, `state`,
and so on). Use `line_plain` for ANSI-free text suitable for parsing; `line`
keeps TUI styling for display.

Manifest `--json` keys are snake_case (`agent_id`, `last_check_unix`, …).
Earlier releases used PascalCase (`AgentID`, `LastCheckUnix`); update payloads
once used `{}` for `version`, which is now a string.

`config show --json` returns normalized JSON config, not TOML.

Example:

```sh
seshagy --get-agents --json
seshagy integration status --json
seshagy agent explain %13 --json
seshagy config show --json
```

Agent metadata helpers:

```sh
seshagy --report-agent \
  --pane %1 \
  --agent pi \
  --state working \
  --message 'running tests' \
  --source hook \
  --session-id native-123 \
  --seq 42

seshagy --release-agent --pane %1 --source hook --seq 43
```

`--seq` is a monotonic ordering token used by hooks to avoid older updates
winning over newer state. `--session-id` stores native agent session identity
when the integration has one.

Other commands:

```sh
seshagy integration status
seshagy integration install <target>
seshagy integration uninstall <target>

seshagy config path
seshagy config show
seshagy config init [--force]

seshagy manifest status [--json]
seshagy manifest update [--json]
seshagy manifest reload [--json]
```

## Configuration

Config is TOML at:

```text
$XDG_CONFIG_HOME/seshagy/config.toml
```

If `XDG_CONFIG_HOME` is unset, seshagy uses:

```text
~/.config/seshagy/config.toml
```

Inspect or create it with:

```sh
seshagy config path
seshagy config show
seshagy config init
```

Common settings:

```toml
[sources]
default = "all"
order = ["all", "sessions", "agents", "current-agents", "zoxide", "fd"]

[directories]
fd_command = 'fd -H -a -d 2 -t d -E .Trash . "$HOME"'

[type_first]
enabled = false
prefix = "ctrl+x"

[agents]
manifest_fallback = false   # opt-in: capture last 30 pane lines for screen rules
manifest_auto_update = true # background catalog fetch on TUI startup
manifest_catalog_url = ""   # defaults to Herdr catalog when empty
```

### Theme colors

`[theme.colors]` controls TUI accents. Values can be:

- an ANSI palette index (`"8"`, `"13"`, …),
- a hex color (`"#cba6f7"`),
- or `"default"` to inherit the terminal foreground (used by `active_tab` by default).

| Key | Used for |
| --- | --- |
| `focused_border` | border on the focused pane |
| `active_tab` | selected source tab label |
| `inactive_tab` | unselected source tabs |
| `border` | pane borders |
| `title` | pane titles and headings |
| `accent` | emphasis text and the top accent bar |
| `key` | key names in help/footer |
| `muted` | subtitles and secondary text |
| `success` | success status messages |
| `info` | informational status messages |
| `warning` | warning status messages |
| `danger` | error/danger status messages |

Example:

```toml
[theme]
  [theme.colors]
    focused_border = "#cba6f7"
    active_tab = "default"
    border = "#313244"
    inactive_tab = "#6c7086"
    title = "#b4befe"
    accent = "#cba6f7"
    key = "#f9e2af"
    muted = "#7f849c"
    success = "#a6e3a1"
    info = "#89dceb"
    warning = "#f9e2af"
    danger = "#f38ba8"
```

The TUI keeps the terminal's default foreground/background for list text and
selection (reverse video), so changing your terminal theme still rethemes most
of seshagy without extra config.

### Icons

`[icons]` controls row prefixes in the list pane.

`mode` selects how source-kind prefixes render:

- `"icons"` — Nerd Font glyphs (default)
- `"text"` — single-letter labels
- `"none"` — no prefix

`agent_state_mode` selects how agent pane state is shown in the TUI list and
detail views. It overrides `mode` for state display only; source icons still
follow `mode`. CLI output (`seshagy list`, fzf actions, and similar) always
prints the state name in brackets and ignores these settings.

- `"inherit"` — follow `mode` (default): glyphs when `mode = "icons"`, bracketed
  labels when `mode = "text"` or `"none"`
- `"icons"` — per-state glyphs from `[icons.agent_state.*]` regardless of `mode`
- `"text"` — per-state labels in brackets regardless of `mode`
- `"none"` — hide state indicators in list rows; detail pane shows the raw state
  string without glyphs or brackets

`tmux_state_mode` selects how session attachment is shown in the TUI list and
detail views. It overrides `mode` for attachment display only; source icons
still follow `mode`. CLI output is unchanged.

- `"inherit"` — follow `mode` (default): glyphs when `mode = "icons"`, bracketed
  labels when `mode = "text"` or `"none"`
- `"icons"` — per-state glyphs from `[icons.tmux_state.*]` regardless of `mode`
- `"text"` — per-state labels in brackets regardless of `mode`
- `"none"` — hide attachment indicators in list rows; detail pane shows plain
  `yes` / `no` for the attached field

Each source kind can be customized under `[icons.session]`, `[icons.zoxide]`,
`[icons.fd]`, and `[icons.agent]`:

| Key | Purpose |
| --- | --- |
| `icon` | glyph shown in `icons` mode (include trailing space if desired) |
| `label` | text shown in `text` mode |
| `color` | ANSI index or hex color for that icon |

Agent state appearance is customized per state under
`[icons.agent_state.working]`, `[icons.agent_state.blocked]`,
`[icons.agent_state.aborted]`, `[icons.agent_state.done]`,
`[icons.agent_state.idle]`, and `[icons.agent_state.unknown]`:

| Key | Purpose |
| --- | --- |
| `icon` | glyph shown when `agent_state_mode` resolves to icons |
| `label` | text shown when `agent_state_mode` resolves to text (wrapped in `[…]` in list rows) |
| `color` | optional ANSI index or hex color; when empty, the TUI uses theme colors for that state |

Session attachment appearance is customized under `[icons.tmux_state.attached]`
and `[icons.tmux_state.detached]`:

| Key | Purpose |
| --- | --- |
| `icon` | glyph shown when `tmux_state_mode` resolves to icons |
| `label` | text shown when `tmux_state_mode` resolves to text (wrapped in `[…]` in list rows) |
| `color` | optional ANSI index or hex color; when empty, the TUI uses theme success (attached) or muted (detached) |

Default state glyphs and labels (from `seshagy config init`):

| State | `icon` | `label` |
| --- | --- | --- |
| working | `▶` | `working` |
| blocked | `◆` | `blocked` |
| aborted | `■` | `aborted` |
| done | `✓` | `done` |
| idle | `◌` | `idle` |
| unknown | `?` | `unknown` |

Default attachment glyphs, labels, and colors:

| State | `icon` | `label` | `color` |
| --- | --- | --- | --- |
| attached | `●` | `attached` | `10` |
| detached | `◌` | `detached` | `8` |

Example (defaults from `seshagy config init`):

```toml
[icons]
mode = "icons"
agent_state_mode = "inherit"
tmux_state_mode = "inherit"

  [icons.session]
    icon = " "
    label = "S"
    color = "10"

  [icons.zoxide]
    icon = "󰉖 "
    label = "Z"
    color = "14"

  [icons.fd]
    icon = "󰥩 "
    label = "F"
    color = "11"

  [icons.agent]
    icon = "  "
    label = "A"
    color = "13"

  [icons.agent_state.working]
    icon = "▶"
    label = "working"

  [icons.agent_state.blocked]
    icon = "◆"
    label = "blocked"

  [icons.tmux_state.attached]
    icon = "●"
    label = "attached"

  [icons.tmux_state.detached]
    icon = "◌"
    label = "detached"
    color = "8"
```

Nerd Font source icons with text state labels:

```toml
[icons]
mode = "icons"
agent_state_mode = "text"
```

Custom per-state icons while keeping Nerd Font source prefixes:

```toml
[icons]
mode = "icons"
agent_state_mode = "icons"

  [icons.agent_state.working]
    icon = "󰄬 "
    color = "10"

  [icons.agent_state.blocked]
    icon = "󰀦 "
    color = "11"
```

Run `seshagy config init` to write the full default `config.toml`, then edit
colors and icons there. `seshagy config show` prints the resolved config.

## Limits and expectations

- tmux is required for session and agent operations.
- Agents appear only after a hook/plugin reports `@agent_*` metadata for
  hook-capable agents, or after process detection for agents without hook
  integrations.
- Agents without hook integrations may still appear via process detection, but
  their lifecycle state stays `unknown` unless explicitly reported.
- Directory results depend on your `zoxide` database and configured `fd` command.
- `yazi` directory picking is blocked when seshagy is running inside a tmux popup.
