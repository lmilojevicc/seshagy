# seshagy

`seshagy` is an agent-aware terminal dashboard for jumping between project
sessions, discovered directories, and coding-agent panes. It supports two
multiplexers — [tmux](https://github.com/tmux/tmux) and
[herdr](https://herdr.dev) — and auto-detects which one it is running in.

Run one command to get a keyboard-first view where you can:

- jump to existing tmux sessions or herdr workspaces,
- create or switch sessions/workspaces from directories found by `zoxide` and `fd`, and
- see coding-agent panes, their current state, and where they are running.

Under tmux, agent state is reported through the `@seshagy_agent_*`
pane-option namespace (tmux backend only). Near-instant detection comes from
installed hook/plugin integrations; a capture-pane screen-rule backstop
(hot-updated from [herdr](https://herdr.dev/docs/agents)) covers agents
without usable hooks.

Under herdr (`$HERDR_ENV=1`), seshagy speaks herdr's vocabulary
(workspaces/tabs/panes) instead of tmux's (sessions/windows/panes). Agent
state is read-only from herdr's `agent_status` — seshagy writes no state,
since herdr owns detection.

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

1. Start tmux (or herdr), or run `seshagy` from inside an existing client.
2. The install menu pops up on first launch — choose which agent integrations
   to install, or press `esc` to skip.
3. Press `z` or `f` to browse project directories from `zoxide` or `fd`.
4. Press `enter` on a directory to create/switch to a session or workspace for it.
5. Press `a`/`t`/`z`/`f` to switch sources (all / sessions-workspaces / zoxide /
   fd directories); on the Agents source, `enter` focuses an agent pane (and
   clears `done` → `idle`).
6. Press `h` to reopen the install menu at any time.

## Launch with a keybinding

seshagy ships a launcher script, `seshagy-focus-kill`, that opens the dashboard
in an ephemeral overlay (herdr) or dedicated window (tmux) and dismisses it
the moment you switch away — so you get a one-keystroke in / one-keystroke out
jump launcher, without leftover panes.

The script auto-detects the active multiplexer from the environment
(`HERDR_ENV=1` → herdr; `$TMUX` → tmux) and is bundled with every release and
`go install` / `make install`.

### tmux

Install the keybinding into `~tmux.conf` idempotently (default key `s`):

```sh
seshagy keybind install tmux            # prefix + s
seshagy keybind install tmux --key f    # prefix + f
tmux source-file ~/.tmux.conf           # reload
```

Remove it with `seshagy keybind uninstall tmux`.

To wire it manually instead, add this line to `~/.tmux.conf`:

```tmux
bind-key s run-shell 'seshagy-focus-kill seshagy'
```

### herdr

Install the seshagy herdr plugin to get the overlay action + keybinding wiring
(see [herdr.dev/plugins](https://herdr.dev/plugins/) for the marketplace
listing). The binary must already be on `PATH` (`brew install` / `go install` /
mise / Nix); the plugin only connects the keybinding:

```sh
brew install seshagy        # or: go install ./cmd/seshagy
herdr plugin install lmilojevicc/seshagy
```

Under the hood the plugin runs `seshagy-focus-kill seshagy` as an overlay pane
— the same wrapper script as the tmux path.

## Requirements

Required:

- `tmux` **or** [herdr](https://herdr.dev) — seshagy auto-detects which
  multiplexer it is running in (`$HERDR_ENV=1` → herdr; else `$TMUX` → tmux;
  else no backend). All session/agent operations are available under either.

Optional, but useful:

- `zoxide` for frecency-ranked directory history
- `fd` for filesystem directory discovery
- `yazi` for choosing a directory interactively
- `eza` for richer directory previews

## Multiplexer support

seshagy supports two multiplexer backends with a shared vocabulary:

| concept           | tmux    | herdr     |
| ----------------- | ------- | --------- |
| project container | session | workspace |
| layout group      | window  | tab       |
| terminal          | pane    | pane      |

Backend is selected automatically from the environment — there is no config
key to set. The TUI and CLI adapt their terminology to the active backend
(sessions/windows under tmux; workspaces/tabs under herdr).

**Agent state under herdr is read-only.** herdr owns agent detection and
exposes it via the `agent_status` field (`idle`/`working`/`blocked`/`done`/
`unknown`). seshagy reads this directly; it does **not** call
`herdr pane report-agent`, run the capture-pane manifest backstop, or write
`@seshagy_agent_*` options under herdr. The state-reporting hooks (shell,
Pi extension, OpenCode plugin) early-exit when `$HERDR_ENV=1` is set.

Builds from source require Go 1.26, matching `go.mod`. Shell-hook integrations
may use `bash` and `python3`; the OpenCode plugin runs on Bun/Node.

## What seshagy manages

| Area                   | What you can do                                                                          |
| ---------------------- | ---------------------------------------------------------------------------------------- |
| tmux sessions / herdr workspaces | list, attach/focus, rename, kill, and preview                                  |
| project directories    | create/switch sessions or workspaces from `zoxide` or a configurable `fd` command        |
| agent panes            | list, filter, focus, or kill detected agent panes                                        |
| current session agents | narrow the agent view to the current session/workspace (`o`)                             |
| input flow             | use classic action keys or type-first filtering with a prefix key                        |

When a directory becomes a session, the session name is derived from the
basename: `.config` becomes `dot_config`, and unsupported characters collapse to
`_` (`foo.bar` becomes `foo_bar`).

## Agent state detection

seshagy detects four states per agent pane:

| State     | Meaning                                                           |
| --------- | ----------------------------------------------------------------- |
| `idle`    | not working, or a `done` pane that has been visited               |
| `working` | agent actively running a turn                                     |
| `blocked` | asking permission or asking the user a question                   |
| `done`    | finished a turn, pane not yet visited (clears to `idle` on focus) |

Detection uses a layered authority model mirroring
[herdr](https://herdr.dev/docs/agents):

**Tier A — lifecycle authority (hooks/plugins own state).** Pi and OpenCode
emit the full lifecycle (idle/working/blocked/done) through their
integrations. When the integration is installed and actively reporting,
seshagy uses those reports and suppresses the screen-rule backstop for that
pane. Near-instant, event-driven (~0ms).

**Tier B — partial hooks + screen rules (screen owns state).** Claude Code,
Codex, and Factory Droid have shell hooks, but their hooks miss approval-result
and ESC-interrupt transitions. For these agents seshagy **always** runs the
capture-pane screen-rule classifier in parallel with the hooks; on a positive
rule match, the screen result overwrites the hook state (so a stale `working`
after ESC is corrected to `idle` within one poll). Hooks still provide
near-instant `working`/`done` signals between screen captures.

**Tier C — screen rules only.** Cursor Agent CLI, Antigravity, Grok Build, and
the other discovered agents have no usable hooks. Their state comes entirely
from the capture-pane classifier (~1s poll cadence in the Agents source).

The screen-rule backstop captures the last 30 pane lines via
`tmux capture-pane` and matches them against per-agent TOML rules (regions,
`contains`/`regex`/`line_regex`, nested `all`/`any`/`not` gates). `blocked` is
strict: it is only set when a rule explicitly matches a known permission or
question UI. No match leaves the existing state unchanged.

### Manifest hot-update

Bundled manifests ship as an offline fallback. On launch, seshagy fetches the
latest manifests from the [herdr public
catalog](https://herdr.dev/agent-detection/) (async, non-blocking) and caches
them locally. Precedence: local override (`$XDG_CONFIG_HOME/seshagy/agent-detection/<id>.toml`)

> cached remote > bundled embed. Remote manifests are version-guarded against
> downgrade and compile-validated before caching. This keeps screen rules current
> without a seshagy release.

### Supported agents

Discovered via process-name matching plus a process-tree descendant walk (for
node-wrapped CLIs that report as `node`):

| Agent              | Process name(s)          | Detection              |
| ------------------ | ------------------------ | ---------------------- |
| Pi                 | `pi`                     | lifecycle (extension)  |
| OpenCode           | `opencode`               | lifecycle (plugin)     |
| Claude Code        | `claude`                 | partial hooks + screen |
| Codex              | `codex`                  | partial hooks + screen |
| Factory Droid      | `droid`, `factory`       | partial hooks + screen |
| Cursor Agent       | `cursor-agent`, `cursor` | screen only            |
| Antigravity        | `agy`, `antigravity`     | screen only            |
| Grok Build         | `grok`                   | screen only            |
| GitHub Copilot CLI | `copilot`                | screen only            |
| Amp                | `amp`                    | screen only            |
| Cline              | `cline`                  | screen only            |
| Devin              | `devin`                  | screen only            |
| Gemini             | `gemini`                 | screen only            |
| Hermes             | `hermes`, `hermes-agent` | screen only            |
| Kilo Code          | `kilo`, `kilocode`       | screen only            |
| Kimi Code          | `kimi`                   | screen only            |
| Kiro               | `kiro-cli`               | screen only            |
| Qoder CLI          | `qodercli`, `qoderclicn` | screen only            |

Architecture-suffixed binary names (e.g. `codex-aarch64-a`) are matched via a
prefix fallback.

### Install menu

Press `h` in the TUI to open the integration install menu (it also auto-pops on
first launch). Select an integration and press `enter` to install, `u` to
uninstall, or `a` to install all. Installs run off the UI thread and never
block the dashboard. The available integrations are: `pi`, `codex`, `claude`,
`droid`, `opencode`.

You can also install/uninstall from the CLI:

```sh
seshagy integration install pi
seshagy integration uninstall pi
```

Shell-hook integrations (codex/claude/droid) merge their entries into the
agent's settings/hooks JSON idempotently and preserve existing user and herdr
entries. The Pi extension installs to `~/.pi/agent/extensions/` (or
`$PI_CODING_AGENT_DIR`). The OpenCode plugin installs to opencode's
auto-discovered `plugin/` directory under its config dir.

## TUI keys

| Key                    | Action                                                                             |
| ---------------------- | ---------------------------------------------------------------------------------- |
| `enter`                | attach/switch to a session, create/switch from a directory, or focus an agent pane |
| `j`/`k`, arrows        | move selection                                                                     |
| `1`..`5`               | select source by configured order                                                  |
| `a`                    | all sources                                                                        |
| `t`                    | tmux sessions                                                                      |
| `o`                    | toggle agents scope (current session vs all)                                       |
| `z`                    | zoxide directories                                                                 |
| `f`                    | fd directories                                                                     |
| `/`                    | filter visible rows                                                                |
| `backspace`            | clear filter when not editing                                                      |
| `r`                    | refresh                                                                            |
| `R`                    | rename selected session                                                            |
| `x`                    | kill selected session or agent pane                                                |
| `y`                    | open `yazi`, then create/switch from its exit directory                            |
| `h`                    | open the integration install menu                                                  |
| `m`                    | change classic/type-first input mode                                               |
| `p`                    | toggle preview pane                                                                |
| `?`                    | toggle help                                                                        |
| `q` / `esc` / `ctrl-c` | quit                                                                               |

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
seshagy --get-zoxide
seshagy --get-fd
seshagy --delete-item '<rendered line from --get-all>'
```

All commands above (plus `config` and `--version`) support a trailing `--json`
flag for machine-readable JSON on stdout. Human text output is unchanged when
`--json` is omitted.

Successful responses include top-level `schema_version` and `ok` fields. With
`--json`, errors also print JSON on stdout (not stderr), for example
`{"schema_version":1,"ok":false,"error":"...","code":"usage|error"}`.
Scripts should check the exit code and the `ok` field.

`--get-* --json` returns structured fields per item (`kind`, `pane_id`, `state`,
and so on). Use `line_plain` for ANSI-free text suitable for parsing; `line`
keeps TUI styling for display.

Agent metadata helpers (used by the installed integrations to report state):

```sh
seshagy --report-agent \
  --pane %1 \
  --agent pi \
  --state working \
  --source seshagy:pi \
  --seq 42

seshagy --release-agent --pane %1 --source seshagy:pi --seq 43
```

`--cwd <dir>` may replace `--pane`; the pane is resolved by a unique
working-directory match across all panes (used by the OpenCode plugin, which
runs in a server process without a reliable `$TMUX_PANE`).

`--seq` is a monotonic ordering token. Reports with a sequence number `<=` the
last applied sequence are rejected (strict-greater), so stale hook reports
cannot resurrect cleared state.

Other commands:

```sh
seshagy integration install <name>
seshagy integration uninstall <name>

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

[type_first]
enabled = false
prefix = "ctrl+x"

[agents]
manifest_fallback = true   # capture-pane screen-rule backstop (default on)
catalog_url = ""           # defaults to the herdr public catalog when empty
```

### Theme colors

`[theme.colors]` controls TUI accents. Values can be:

- an ANSI palette index (`"8"`, `"13"`, …),
- a hex color (`"#cba6f7"`),
- or `"default"` to inherit the terminal foreground (used by `active_tab` by default).

| Key              | Used for                             |
| ---------------- | ------------------------------------ |
| `focused_border` | border on the focused pane           |
| `active_tab`     | selected source tab label            |
| `inactive_tab`   | unselected source tabs               |
| `border`         | pane borders                         |
| `title`          | pane titles and headings             |
| `accent`         | emphasis text and the top accent bar |
| `key`            | key names in help/footer             |
| `muted`          | subtitles and secondary text         |
| `success`        | success status messages              |
| `info`           | informational status messages        |
| `warning`        | warning status messages              |
| `danger`         | error/danger status messages         |

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
follow `mode`. CLI output always prints the state name in brackets and ignores
these settings.

- `"inherit"` — follow `mode` (default)
- `"icons"` — per-state glyphs from `[icons.agent_state.*]`
- `"text"` — per-state labels in brackets
- `"none"` — hide state indicators in list rows

Each source kind can be customized under `[icons.session]`, `[icons.zoxide]`,
`[icons.fd]`:

| Key     | Purpose                                                         |
| ------- | --------------------------------------------------------------- |
| `icon`  | glyph shown in `icons` mode (include trailing space if desired) |
| `label` | text shown in `text` mode                                       |
| `color` | ANSI index or hex color for that icon                           |

Agent state appearance is customized per state under
`[icons.agent_state.working]`, `[icons.agent_state.blocked]`,
`[icons.agent_state.done]`, and `[icons.agent_state.idle]`:

| Key     | Purpose                                                                                |
| ------- | -------------------------------------------------------------------------------------- |
| `icon`  | glyph shown when `agent_state_mode` resolves to icons                                  |
| `label` | text shown when `agent_state_mode` resolves to text (wrapped in `[…]` in list rows)    |
| `color` | optional ANSI index or hex color; when empty, the TUI uses theme colors for that state |

Default state glyphs and labels:

| State   | `icon` | `label`   |
| ------- | ------ | --------- |
| working | `▶`    | `working` |
| blocked | `◆`    | `blocked` |
| done    | `✓`    | `done`    |
| idle    | `◌`    | `idle`    |

Example (defaults from `seshagy config init`):

```toml
[icons]
mode = "icons"
agent_state_mode = "inherit"

  [icons.session]
    icon = " "
    label = "S"
    color = "10"

  [icons.agent_state.working]
    icon = "▶"
    label = "working"

  [icons.agent_state.blocked]
    icon = "◆"
    label = "blocked"
```

Run `seshagy config init` to write the full default `config.toml`, then edit
colors and icons there. `seshagy config show` prints the resolved config.

## Limits and expectations

- a multiplexer is required for session and agent operations: tmux (`$TMUX`) or herdr (`$HERDR_ENV=1`). Without one, seshagy shows directory sources only.
- Hook/plugin integrations report state through the `@seshagy_agent_*`
  namespace. Agents without integrations are still discovered (process name +
  descendant walk) and classified by the capture-pane screen-rule backstop when
  `[agents] manifest_fallback` is enabled (default).
- The screen-rule backstop captures pane content; this is the sanctioned
  `manifest_fallback` exception to the "no pane scraping" rule.
- Directory results depend on your `zoxide` database and configured `fd` command.
- `yazi` directory picking is blocked when seshagy is running inside a tmux popup.

## Build, test, and release

- `mise run verify` runs the CI gate (`fmt:check`, `lint`, `vet`, `test`, `build`).
- `mise run fmt` formats Go and YAML files.
- `mise run vuln` runs `govulncheck ./...`.
- `make build` builds the local `./seshagy` binary from `./cmd/seshagy`.

Releases are tag-driven: after `mise run verify`, `mise run vuln`, and
`mise run release:check` pass on a clean tree, push a `v*` tag to run
GoReleaser.
