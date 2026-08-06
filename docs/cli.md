# CLI Helpers & Internals

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

The `--get-*` commands, `--delete-item`, `--report-agent`, `--release-agent`,
`config`, `diagnostics`, and `--version` support `--json` for machine-readable
JSON on stdout. Integration, keybind, help, and completion commands do not.
Human text output is unchanged when `--json` is omitted.

Successful responses include top-level `schema_version` and `ok` fields. With
`--json`, errors also print JSON on stdout (not stderr), for example
`{"schema_version":1,"ok":false,"error":"...","code":"usage|error"}`.
Scripts should check the exit code and the `ok` field.

`--get-* --json` returns structured fields per item (`kind`, `pane_id`, `state`,
and so on). Use `line_plain` for ANSI-free text suitable for parsing; `line`
keeps TUI styling for display.

Agent metadata helpers (used by the installed integrations to report state under `tmux`; these commands are no-ops when `$HERDR_ENV=1` is set since `herdr` handles detection natively):

```sh
seshagy --report-agent \
  --pane %1 \
  --agent pi \
  --state working \
  --source seshagy:pi \
  --seq 42

seshagy --release-agent --pane %1 --source seshagy:pi --seq 43
```

`--report-agent` accepts `--pane`, `--cwd`, `--agent`, `--state`, `--source`,
`--seq`, `--message`, `--session-id`, and `--json`. Canonical states are
`idle`, `working`, `blocked`, `done`, and `unknown`. `--release-agent` accepts
`--pane`, `--cwd`, `--source`, `--seq`, and `--json`.

`--cwd <dir>` may replace `--pane`; the pane is resolved by a unique
working-directory match across all panes (used by the OpenCode plugin, which
runs in a server process without a reliable `$TMUX_PANE`).

`--seq` is a monotonic ordering token. Reports with a sequence number `<=` the
last applied sequence are rejected (strict-greater), so stale hook reports
cannot resurrect cleared state.

Other commands:

```sh
seshagy integration install pi|codex|claude|droid|opencode
seshagy integration uninstall pi|codex|claude|droid|opencode

seshagy config path
seshagy config show
seshagy config init [--force]
seshagy diagnostics [--json]
seshagy keybind install tmux [--key <key>] [--mode popup|window|pane|pane-zoomed] [--persistent]
seshagy keybind install herdr [--key <key>] [--mode pane|popup] [--width <cells|percent>] [--height <cells|percent>] [--persistent]
seshagy keybind uninstall tmux|herdr
```

Canonical introspection forms are `seshagy --help` and `seshagy --version`.
`-h` and `help` are accepted help aliases; `version` is an accepted version
alias.

## Shell completion

Homebrew generates and installs completions for Bash, Zsh, and Fish
automatically. Other installations can generate static files without evaluating
shell output:

```sh
mkdir -p ~/.local/share/bash-completion/completions
seshagy completion bash > ~/.local/share/bash-completion/completions/seshagy

mkdir -p ~/.zfunc
seshagy completion zsh > ~/.zfunc/_seshagy

mkdir -p ~/.config/fish/completions
seshagy completion fish > ~/.config/fish/completions/seshagy.fish
```

Configure Bash to load the selected directory if it is not already on its
completion path. For Zsh, add the directory to `fpath` before initializing
completion, for example in `~/.zshrc`:

```zsh
fpath=(~/.zfunc $fpath)
autoload -Uz compinit
compinit
```

Dynamic pane, directory, agent, source, and deletable-item
values are read-only, use a short timeout, and silently return no dynamic values
when a multiplexer is slow or unavailable. Directory arguments retain native
shell directory completion as a fallback.

## Diagnostic logs and bug reports

`seshagy diagnostics` reports the effective logging level, backend/runtime
metadata, and concrete local log paths. Its `--json` form keeps expanded paths
redacted (`path_redacted: true`) so the metadata is safer to attach to an
issue. Neither form opens, truncates, locks, prunes, bundles, or uploads logs.

To capture a reproduction:

```sh
SESHAGY_LOG_LEVEL=debug seshagy
seshagy diagnostics
```

Reproduce the problem, exit cleanly, inspect the JSONL file named by
`diagnostics`, and attach it only if comfortable. Then unset the environment
variable (or set `[log].level = "off"`) and delete the file. Logs remain local
and are not automatically redacted; debug records can contain opaque
multiplexer pane/workspace identifiers. See [configuration.md](configuration.md)
for file, cap, retention, and locking details.

## Install menu integrations

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
