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
[sources]
default = "all"
order = ["all", "sessions", "zoxide", "fd", "agents"]

[directories]
fd_command = 'fd -H -a -d 2 -t d -E .Trash . "$HOME"'

[type_first]
enabled = false
prefix = "ctrl+x"

[agents]
manifest_fallback = true   # tmux only: capture-pane screen-rule backstop (default on)
catalog_url = ""           # defaults to the herdr public catalog when empty

[tui]
input_style = "popup"        # popup | cmdline
dim_background = true         # dim the list behind the popup (popup mode only)
```

The default `order` lists tabs left→right (`agents` last). `current-agents` is
not a tab; it is a CLI-only scope (`--get-current-session-agents`), reachable in
the TUI via the `o` key (toggles the agents source between the current session
and all).

### Theme colors

`[theme.colors]` controls TUI accents. Values can be:

- an ANSI palette index (`"8"`, `"13"`, …),
- a hex color (`"#cba6f7"`),
- or `"default"` to inherit the terminal foreground (used by `active_tab` by default).

| Key              | Used for                             |
| ---------------- | ------------------------------------ |
| `popup_border`   | border on popups (input prompt, setup/install menus) |
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

Per-pane borders and border titles can be themed independently. Each defaults
to the relevant global when unset (`list_border` inherits `border`;
 `metadata_border` and `preview_border` inherit `border`; each `*_border_title`
 inherits its pane's `*_border`), so the default look is unchanged unless set.

| Key                     | Used for                                                 |
| ----------------------- | -------------------------------------------------------- |
| `list_border`           | list pane border (default: inherits `border`)             |
| `metadata_border`       | metadata/detail pane border (default: inherits `border`) |
| `preview_border`        | preview pane border (default: inherits `border`)         |
| `list_border_title`     | list pane border-title text (default: inherits `list_border`)     |
| `metadata_border_title` | metadata pane border-title text (default: inherits `metadata_border`) |
| `preview_border_title`  | preview pane border-title text (default: inherits `preview_border`)  |

When a `*_border_title` differs from its `*_border`, only the title text uses
the title color; the corners and dashes keep the border color.

Example:

```toml
[theme]
  [theme.colors]
    popup_border = "#cba6f7"
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
    # Per-pane borders and border titles. Each inherits when unset
    # (list_border -> border; metadata/preview_border -> border;
    # *_border_title -> its pane border), so these are optional.
    # list_border = "#89b4fa"
    # metadata_border = "#45475a"
    # preview_border = "#45475a"
    # list_border_title = "#cdd6f4"
    # metadata_border_title = "#cdd6f4"
    # preview_border_title = "#cdd6f4"
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

| State   | `icon` | `label`   | `color` |
| ------- | ------ | --------- | ------- |
| working | `●`    | `working` | `10`    |
| blocked | `◐`    | `blocked` | `11`    |
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
      icon = "◐"
      label = "blocked"
      color = "11"
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
