# seshagy

`seshagy` is a minimal Go tmux session manager. It ports the useful behavior from
`~/dotfiles/scripts/tmux-session-manager` into a small Bubble Tea TUI with a
ccmux-inspired layout: a top tab strip, a focused rounded session list, a details
pane, a live preview pane, and a compact help/status bar.

## Features

- List existing tmux sessions.
- Discover project directories from `zoxide query -l` and `fd -H -a -d 2 -t d`.
- Create/switch to a tmux session from a selected directory using the same
  basename-derived naming convention as the shell script (`foo.bar` -> `foo_bar`,
  `.config` -> `dot_config`).
- Attach to sessions when outside tmux; `switch-client` when already inside tmux.
- Detect installed hook-capable coding agents and ask before installing state
  hooks/plugins for each one. The prompt is toggle-based, so every detected
  integration can be enabled or skipped independently.
- List agent panes only after hooks/plugins report `@agent_*` metadata. seshagy
  does **not** infer agent state by inspecting foreground process names or pane
  text.
- Focus reported agent panes and preserve the `@agent_*` status tracking metadata used by
  the original script.
- Kill sessions or agent panes.
- Rename sessions in-place.
- Open `yazi`, then create/switch to a session from the directory it exits in.
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
| `1`/`a` | All sources |
| `2`/`t` | Sessions only |
| `3`/`g` | Agent panes |
| `4`/`z` | Zoxide directories |
| `5`/`f` | fd directories |
| `o` | Agents in the current tmux session |
| `/` | Filter visible rows |
| `backspace` | Clear filter when not editing |
| `r` | Refresh |
| `R` | Rename selected session |
| `x` | Kill selected session or agent pane |
| `y` | Open yazi and create/switch from its exit directory |
| `i` | Reopen the hook integration installation prompt |
| `p` | Toggle preview pane |
| `?`/`h` | Toggle compact help |
| `q`/`esc`/`ctrl-c` | Quit |

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
`opencode`, `qodercli`, and `cursor`. On startup, the TUI scans for these
agents by command or config directory and asks before installing any missing
integration. Space toggles each detected agent; Enter installs the selected
hooks/plugins; `s` or Esc skips.
