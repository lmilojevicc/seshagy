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
scrape pane text. Agents without hook support may still appear via process
detection, but their state stays `unknown` unless explicitly reported.

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
`config.toml`) adds a conservative second pass when hooks are silent and state
is still `unknown`. Seshagy captures the last 20 pane lines and matches bundled
per-agent TOML rules (Claude, Codex, and Grok to start). Hook-reported
`working`, `blocked`, and `idle` are never overridden by title inference or
manifest fallback.
Bundled rules currently cover common blocked prompts and a few working signals;
remote manifest fetch is not implemented yet.

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
| `V` | expand/collapse native agent session id on selected pane |
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

[icons]
mode = "icons" # "icons", "text", or "none"

[type_first]
enabled = false
prefix = "ctrl+x"
```

The config also controls theme colors and icon colors. The TUI uses terminal
default foreground/background colors with ANSI accents, so changing your
terminal theme usually rethemes seshagy without extra config.

## Limits and expectations

- tmux is required for session and agent operations.
- Agents appear only after a hook/plugin reports `@agent_*` metadata for
  hook-capable agents, or after process detection for agents without hook
  integrations.
- Agents without hook integrations may still appear via process detection, but
  their lifecycle state stays `unknown` unless explicitly reported.
- Directory results depend on your `zoxide` database and configured `fd` command.
- `yazi` directory picking is blocked when seshagy is running inside a tmux popup.
