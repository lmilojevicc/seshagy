# Configuration and Keybindings

## Launch with a keybinding

seshagy has a built-in `--ephemeral` mode that opens the dashboard in an
ephemeral overlay (herdr) or dedicated window (tmux) and dismisses it the
moment you switch away — so you get a one-keystroke in / one-keystroke out jump
launcher, without leftover panes. It auto-detects the active multiplexer from
the environment (`HERDR_ENV=1` → herdr; `$TMUX` → tmux).

### tmux

Install the keybinding idempotently (default key `s`). seshagy writes to the
config tmux actually reads, in this order: `$TMUX_CONF_PATH` if set; else
`$TMUX_CONFIG_DIR/tmux.conf`; else the existing XDG config
(`$XDG_CONFIG_HOME/tmux/tmux.conf` or `~/.config/tmux/tmux.conf`); else
`~/.tmux.conf`.

```sh
seshagy keybind install tmux            # prefix + s, popup (default)
seshagy keybind install tmux --key f    # custom key
seshagy keybind install tmux --mode window        # new full window
seshagy keybind install tmux --mode pane          # split, unzoomed
seshagy keybind install tmux --mode pane-zoomed   # split + zoomed
tmux source-file ~/.config/tmux/tmux.conf   # reload (path may differ)
```

Remove it with `seshagy keybind uninstall tmux`.

All four modes launch `seshagy --ephemeral` inside a real tmux pane
(`display-popup` / `new-window` / `split-window`) so seshagy gets a controlling
TTY — `--ephemeral` then dismisses the pane the moment you switch to
another session/workspace. To wire a mode manually instead, add the matching
line to your tmux config:

```tmux
bind-key s display-popup -E -w 80% -h 80% 'seshagy --ephemeral'
bind-key s new-window  -c '#{pane_current_path}' 'seshagy --ephemeral'
bind-key s split-window -c '#{pane_current_path}' 'seshagy --ephemeral'
bind-key s split-window -Z -c '#{pane_current_path}' 'seshagy --ephemeral'
```

### herdr

Install the keybinding idempotently into the herdr config
(`~/.config/herdr/config.toml`, or `$HERDR_CONFIG_PATH` / `$XDG_CONFIG_HOME` if
set). The keybind opens `seshagy --ephemeral` as a temporary pane that
herdr closes when the command exits; `--ephemeral` extends that to also dismiss
on focus-loss:

```sh
seshagy keybind install herdr            # prefix+s
seshagy keybind install herdr --key f    # prefix+f
herdr server reload-config               # reload
```

Remove it with `seshagy keybind uninstall herdr`.

To wire it manually instead, add this block to your herdr config:

```toml
[keys]
  [[keys.command]]
    key = "prefix+s"
    type = "pane"
    command = "seshagy --ephemeral"
    description = "seshagy session manager"
```

## Configuration File

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
[directories]
fd_command = 'fd -H -a -d 2 -t d -E .Trash . "$HOME"'

[agents]
manifest_fallback = true   # tmux only: capture-pane screen-rule backstop (default on)
catalog_url = ""           # defaults to the herdr public catalog when empty

[tui]
layout = "horizontal"          # horizontal | vertical | dropdown
preview = true                 # the preview window (dropdown starts hidden)

[remote]
ssh_config_hosts = true        # offer the hosts of ~/.ssh/config too (default)

[[hosts]]
name = "homelab"
target = "milo@192.168.1.10"

[log]
level = "off"                 # off | debug | info | warn | error
file = ""                     # empty: create a per-run file under XDG state
```

### TUI

The picker is always type-first: printable keys fuzzy-filter the merged list of
sessions, agents, directories, and hosts, and `ctrl` keys select a filter
(`ctrl+a` agents, `ctrl+s` agent state, `ctrl+t` sessions, `ctrl+z` zoxide,
`ctrl+f` find, `ctrl+e` remotes; the active filter's key returns to all, and
`tab` / `shift+tab` cycle). The prompt also reads herdr-navigator style prefix
tokens, ANDed with that filter: `!` agent name or kind, `@` workspace or state
(`@` alone is every agent), `#` agent state, `/` path. `[tui] preview` and
`[tui] layout` are the only `[tui]` settings the picker reads. See
the README's TUI section for the full keymap.

```toml
[tui]
layout = "horizontal"   # horizontal (default) | vertical | dropdown
preview = true          # the preview window (dropdown starts hidden)
```

| Value        | Arrangement                                                                 |
| ------------ | --------------------------------------------------------------------------- |
| `horizontal` | Search prompt and results on the left, preview on the right (default).      |
| `vertical`   | Prompt on top, results below it, preview at the bottom.                     |
| `dropdown`   | Prompt on top, results full width below; the preview starts hidden (`ctrl+o` shows it). |

Frames narrower than 100 columns always stack the windows (prompt, results,
preview) instead of putting them side by side, whatever the layout says.

Legacy and unknown values (including the old `default` and `zen` tokens)
normalize to `horizontal` without an error, so an existing `config.toml` keeps
working.

### Remote hosts

`[remote]` and `[[hosts]]` describe remote herdr servers that `seshagy run` can
switch the attached client to. See the README's Remote Hosts section for the
workflow.

```toml
[remote]
ssh_config_hosts = true   # list the non-wildcard Host entries of ~/.ssh/config (default)
keybindings = "server"    # optional default --remote-keybindings for every host: local | server

[[hosts]]
name = "homelab"            # unique, [A-Za-z0-9._-]+; the generated ssh alias is seshagy-<name>
target = "milo@192.168.1.10:2222"
transport = "ssh"           # ssh (default) | iroh
proxy_command = ""          # optional ProxyCommand (%h / %p expand)
session = "work"            # optional remote herdr session to attach
keybindings = "server"      # optional: local | server; overrides [remote] keybindings

[[hosts]]
name = "laptop"
target = "milo@<64-hex iroh endpoint id>"
transport = "iroh"
keybindings = "server"
```

The two hosts above make `seshagy run` attach with
`herdr --remote seshagy-homelab --session work --remote-keybindings server` and
`herdr --remote seshagy-laptop --remote-keybindings server`; the generated ssh
config supplies the user, port, and (for iroh) `ProxyCommand iroh-ssh proxy %h`.

| Key                 | Purpose                                                                    |
| ------------------- | -------------------------------------------------------------------------- |
| `name`              | Required. Unique `[A-Za-z0-9._-]+` name; the alias handed to herdr is `seshagy-<name>`. |
| `target`            | Required. `[user@]host[:port]` or `ssh://user@host:port`.                   |
| `transport`         | `ssh` (default) or `iroh`. `iroh` writes `ProxyCommand iroh-ssh proxy %h`, so `target` is the 64-hex endpoint id. |
| `proxy_command`     | Custom transport; exclusive with `transport = "iroh"`.                       |
| `session`           | Remote herdr session to attach; empty uses the remote default.             |
| `keybindings`       | `local` or `server`, passed as `herdr --remote-keybindings`. Empty uses `[remote] keybindings`, then herdr's default (`local`). |

`[remote] keybindings` also applies to hosts discovered from `~/.ssh/config`.
An unknown `[remote] keybindings` value is ignored with a warning; an unknown
host `keybindings` value skips that host.

Entries that fail validation (bad name, target, or transport; control
characters) are skipped with a warning from `seshagy`, `seshagy hosts`, and
`seshagy run`; an iroh target that does not look like an endpoint id, or a
`proxy_command` alongside `transport = "iroh"`, only warns. Configured hosts are
rendered into `$XDG_CONFIG_HOME/seshagy/ssh_config`, which `seshagy hosts setup`
writes and Includes from `~/.ssh/config`:

```sh
seshagy hosts            # name, transport, target, and source of every host
seshagy hosts setup      # write the generated ssh config and add its Include line
seshagy hosts switch <name|local>
seshagy run [--session <name>] [<host>]
```

### Diagnostic logging

Logging is off by default. Enable it for a reproduction with TOML or a one-run
environment override:

```sh
SESHAGY_LOG_LEVEL=debug seshagy
```

Non-empty `SESHAGY_LOG_LEVEL` and `SESHAGY_LOG_FILE` values override TOML. A
file path alone does not enable logging. Default logs are private JSONL files
under `$XDG_STATE_HOME/seshagy/log/` (normally
`~/.local/state/seshagy/log/`), limited to 25 MiB each; seshagy targets the
newest 10 matching files and skips live locked files, so concurrent runs may
temporarily exceed 10. A relative explicit `file` is resolved from the process
working directory. Explicit files are locked and truncated for each run,
tightened to mode `0600`, and never pruned.
A busy explicit file causes startup to fail rather than corrupting it.

Run `seshagy diagnostics` to locate logs and see the collection workflow.
`diagnostics --json` omits expanded paths for safer issue attachment. Logs are
never uploaded automatically and are not guaranteed to be free of identifying
multiplexer IDs; inspect the file before sharing, then disable logging and
delete it when finished.

`current-agents` is a CLI-only scope (`--get-current-session-agents`) that
narrows the agent list to the multiplexer session hosting the call; the picker
shows the current session's agents inline instead.

### Theme colors

`[theme.colors]` controls TUI accents. Values can be:

- an ANSI palette index (`"8"`, `"13"`, …),
- a hex color (`"#cba6f7"`),
- or `"default"` to inherit the terminal foreground.

| Key              | Used for                             |
| ---------------- | ------------------------------------ |
| `popup_border`   | border on setup/install dialogs only       |
| `border`         | pane borders                         |
| `popup_title`    | popup / dialog headings (input prompt, setup, install menu) |
| `title`          | deprecated alias for `popup_title`   |
| `accent`         | emphasis text and the selection cursor |
| `key`            | key names in help/footer             |
| `muted`          | secondary / help / status text       |
| `success`        | success status messages              |
| `info`           | informational status messages        |
| `warning`        | warning status messages              |
| `danger`         | error/danger status messages         |
| `selection_bg`   | background bar under the selected result row (default `236`, a 256-color dark grey) |

Example:

```toml
[theme]
  [theme.colors]
    popup_border = "#cba6f7"
    border = "#313244"
    popup_title = "#b4befe"
    accent = "#cba6f7"
    key = "#f9e2af"
    muted = "#7f849c"
    success = "#a6e3a1"
    info = "#89dceb"
    warning = "#f9e2af"
    danger = "#f38ba8"
    # The solid bar behind the selected result row. Any color lipgloss
    # accepts (ANSI index or hex) works; "default" leaves the row unbarred.
    selection_bg = "236"
```

The TUI keeps the terminal's default foreground/background for list text, so
changing your terminal theme still rethemes most of seshagy without extra
config. The selected row is drawn as a solid background bar (`selection_bg`)
with the row's own foreground colors on top; set it to `"default"` to drop the
bar and mark the selection with the accent `▌` instead.

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

| State   | `icon` | `label`   | `color` |
| ------- | ------ | --------- | ------- |
| working | `●`    | `working` | `10`    |
| blocked | `∅`    | `blocked` | `9`     |
| done    | `◉`    | `done`    | `14`    |
| idle    | `○`    | `idle`    | `8`     |
| unknown | `?`    | `unknown` | `8`     |

Example `[icons.agent_state]` (defaults from `seshagy config init`):

```toml
  [icons.agent_state]
    [icons.agent_state.idle]
      icon = "○"
      label = "idle"
      color = "8"
    [icons.agent_state.working]
      icon = "●"
      label = "working"
      color = "10"
    [icons.agent_state.blocked]
      icon = "∅"
      label = "blocked"
      color = "9"
    [icons.agent_state.done]
      icon = "◉"
      label = "done"
      color = "14"
    [icons.agent_state.unknown]
      icon = "?"
      label = "unknown"
      color = "8"
```

Run `seshagy config init` to write the full default `config.toml`, then edit
colors and icons there. `seshagy config show` prints the resolved config.
