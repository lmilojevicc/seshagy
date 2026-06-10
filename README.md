# seshagy

`seshagy` is a minimal Go tmux session manager. It ports the useful behavior from
`~/dotfiles/scripts/tmux-session-manager` into a small Bubble Tea TUI with a
ccmux-inspired layout: a top tab strip, a focused rounded session list, a details
pane, a live preview pane, and a compact help/status bar.

## Features

- List existing tmux sessions.
- Discover project directories from `zoxide query -l` and a configurable fd command.
- Create/switch to a tmux session from a selected directory using the same
  basename-derived naming convention as the shell script (`foo.bar` -> `foo_bar`,
  `.config` -> `dot_config`).
- Attach to sessions when outside tmux; `switch-client` when already inside tmux.
- Detect installed hook-capable coding agents and ask before installing state
  hooks/plugins for each one on the first TUI launch only. The prompt is
  toggle-based, so every detected integration can be enabled or skipped
  independently.
- Use terminal-default foreground/background colors and ANSI palette accents so
  the TUI follows the active terminal theme instead of forcing a fixed surface.
- Configure source icons, icon colors, and whether to use Nerd Font icons,
  plain ASCII labels (`S`, `Z`, `F`, `A`), or no source icons through the user
  config file.
- Optional type-first mode starts filtering as soon as you type; app actions are
  then run by pressing a configurable prefix first (`ctrl+x` by default).
- List agent panes only after hooks/plugins report `@agent_*` metadata. seshagy
  does **not** infer agent state by inspecting foreground process names or pane
  text.
- Focus reported agent panes and preserve the `@agent_*` status tracking metadata used by
  the original script.
- Kill sessions or agent panes.
- Rename sessions in-place.
- Open `yazi`, then create/switch to a session from the directory it exits in;
  yazi is blocked with an error when seshagy is running inside a tmux popup.
- CLI compatibility helpers for scripting/fzf-style integrations:
  `--get-*`, `--delete-item`, `--report-agent`, and `--release-agent`.
- Herdr-style integration commands: `seshagy integration status`,
  `seshagy integration install <target>`, and
  `seshagy integration uninstall <target>`.

## Install

```sh
go install ./cmd/seshagy
```

Or build a local binary:

```sh
make build
./seshagy
```

Runtime tools:

- Required for session operations: `tmux`
- Optional directory sources: `zoxide`, `fd`
- Optional directory picker: `yazi`
- Optional richer directory previews: `eza`

## TUI keys

| Key | Action |
| --- | --- |
| `enter` | Attach/switch to a session, create/switch from a directory, or focus an agent pane |
| `j/k`, arrows | Move selection |
| `1`..`6` | Select the source at that position in `sources.order` |
| `a` | All sources |
| `t` | Sessions only |
| `g` | Agent panes |
| `o` | Agents in the current tmux session |
| `z` | Zoxide directories |
| `f` | fd directories |
| `s` | Cycle agent state filter in agent panes |
| `S` | Clear agent state filter in agent panes |
| `/` | Filter visible rows |
| `backspace` | Clear filter when not editing |
| `r` | Refresh |
| `R` | Rename selected session |
| `x` | Kill selected session or agent pane |
| `y` | Open yazi and create/switch from its exit directory |
| `i` | Open the hook integration installation prompt manually |
| `m` | Change classic/type-first input mode |
| `p` | Toggle preview pane |
| `?`/`h` | Toggle compact help |
| `q`/`esc`/`ctrl-c` | Quit |

When type-first mode is enabled, normal typing edits the filter immediately.
Press the configured action prefix first (`ctrl+x` by default) before most
action keys, for example `ctrl+x` then `g` to open the Agents pane, or
`ctrl+x` then `m` to change input mode. `enter` and arrow/page/home/end
navigation keys never require the prefix.
`backspace` edits the filter and `esc` clears it.

## CLI helpers

```sh
seshagy --get-all
seshagy --get-sessions
seshagy --get-agents
seshagy --get-current-session-agents
seshagy --get-zoxide
seshagy --get-fd
seshagy --delete-item '<rendered line from --get-all>'
seshagy integration status
seshagy integration install pi
seshagy integration uninstall pi
seshagy config path
seshagy config show
seshagy config init
```

Agent metadata helpers are what installed hooks/plugins call. They mirror the
shell script's `@agent_*` tmux metadata behavior:

```sh
seshagy --report-agent --pane %1 --agent pi --state working --message 'running tests' --source hook
seshagy --release-agent --pane %1 --source hook
```

Recognized states are normalized to `working`, `blocked`, `aborted`, `done`,
`idle`, or `unknown`.

Supported hook/plugin targets are `pi`, `claude`, `codex`, `copilot`, `droid`,
`opencode`, `qodercli`, and `cursor`. On the first TUI launch, seshagy scans for
these agents by command or config directory and asks before installing any
missing integration. That first-launch prompt is recorded under the user's XDG
state directory and is not shown again automatically. After that, use `i` in the
TUI or `seshagy integration install <target>` to install integrations manually.
Space toggles each detected agent; Enter installs the selected hooks/plugins;
`s` or Esc skips.

## Configuration

The config file is TOML at `$XDG_CONFIG_HOME/seshagy/config.toml`, falling back
to `~/.config/seshagy/config.toml` when `XDG_CONFIG_HOME` is unset. Use
`seshagy config path` to print the exact path, `seshagy config show` to print the
effective config, and `seshagy config init` to write the default file.

Default config:

```toml
[sources]
default = "all"
order = ["all", "sessions", "agents", "current-agents", "zoxide", "fd"]

[directories]
fd_command = 'fd -H -a -d 2 -t d -E .Trash . "$HOME"'

[icons]
mode = "icons"

[icons.session]
icon = ""
label = "S"
color = "10"

[icons.zoxide]
icon = "󰉖"
label = "Z"
color = "14"

[icons.fd]
icon = "󰥩"
label = "F"
color = "11"

[icons.agent]
icon = "󰚩"
label = "A"
color = "13"

[type_first]
enabled = false
prefix = "ctrl+x"

[setup]
type_first_prompt_seen = false
```

Set `sources.order` to change the tab order. Number keys follow that order, so
the first source is `1`, the second is `2`, and so on. Source names are `all`,
`sessions`, `agents`, `current-agents`, `zoxide`, and `fd`. Set
`sources.default` to choose the source seshagy loads on startup.

Set `directories.fd_command` to change which directories populate the fd source.
The command should print one directory path per line.

Set `icons.mode` to `"text"` to render the configured plain labels instead of
Nerd Font icons, or `"none"` for true no-icons mode. In no-icons mode, no source
icons, session state glyphs, or text labels are shown; agent rows still use state
labels such as `[working] pi`. Each `color` is a terminal color value understood
by Lip Gloss/ANSI rendering: ANSI palette indexes such as `10`, bright SGR
values such as `92`, or truecolor hex values such as `#a6e3a1`.

On first TUI startup, seshagy asks whether to enable type-first mode and records
the answer in this config file. After that, edit `type_first.enabled` or
`type_first.prefix` manually to change the behavior.

## Theme behavior

The dashboard intentionally avoids hard-coded app foreground/background colors.
It renders on top of the terminal's default colors, uses reverse video for the
selected row, and uses the terminal's ANSI accent palette for source icons,
borders, status labels, and state markers. Changing the terminal color scheme is
therefore enough to retheme seshagy.
