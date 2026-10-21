# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI/TUI project for `seshagy`, an agent-aware terminal dashboard that supports both tmux and [herdr](https://herdr.dev) as multiplexer backends. The command entry point is `cmd/seshagy/`. Core packages live under `internal/`: `config` for TOML config, `sessionmgr` for the `Multiplexer` backend abstraction (tmux/herdr/noop), tmux sessions and agent pane metadata + detection engine, `integrations` for hook/plugin installs, `remote` for remote hosts and the `seshagy run` launcher, and `tui` for the Bubble Tea picker. Tests sit beside code as `*_test.go` files. The active backend is auto-detected from the environment (`$HERDR_ENV=1` → herdr wins; else `$TMUX` → tmux; else noop).

### TUI picker

The TUI (`internal/tui/`) is a single type-first picker. Printable runes always fuzzy-filter the merged item list; a ctrl chord selects an inline scope instead (`ctrl+a` agents, `ctrl+s` agent state, `ctrl+t` sessions, `ctrl+z` zoxide, `ctrl+f` find, `ctrl+e` remotes, `ctrl+g` integrations), pressing the active scope's chord returns to all, and tab/shift+tab cycle scopes. Under herdr only, `alt+m` (never `ctrl+m` — Bubble Tea delivers `ctrl+m` as enter) steps between the local server and each enabled saved `herdr machine` profile; the selected machine is named in the prompt title (`filterTitle()`), and every item-scoped action resolves its backend from `Item.Machine` (`muxFor`/`itemMux`), never from the selection. The machine rows (footer, help) and the title suffix are hidden when the backend has no `MachineSelector`, so tmux/noop are byte-identical to before machines. `F1` opens a centered help overlay (`help.go`) that holds what the footer does not — the filter keys with the active one highlighted and the current `ctrl+s` state, the query tokens, and the remaining keys — closing on F1/esc/q and swallowing every other key; the footer (`view.go`) keeps only the core actions (`↑↓ move`, `⏎ open`, `^r rename`, `^x kill`, `^o preview`, `^y yazi`, `F1 help`, `esc quit`) and drops its hints from the right. `keymap.go` is the one binding table: `keys.go` dispatch, the footer hints, and the help rows all read it, and every handled key is either a footer hint or a help row. The query itself reads herdr-navigator prefix tokens, ANDed with the scope and state filter: `!x` agent name or kind, `@` every agent, `@x` agent workspace or state, `#x` agent state, `/x` item path; a bare prefix is ignored and the remaining tokens are plain fuzzy text. `scope.go` owns the scope enum, `filterRows`, `itemRank`, and `dedupeDirs`; `query.go` the token grammar (`parseQuery`, `queryTokens.matches`); `model.go` the picker state; `commands.go` the tea.Cmds; `refresh_cache.go` the per-source cache/TTL/generation; `view.go` and `theme.go` the rendering; `icons.go` the agent logos (Herdr Agent Icons Max font, MIT, herdr-radar). Sessions, agents, zoxide, and fd load as independent sources with their own cache entry; hosts load once at startup and on reload. ModeAll remains for the CLI's `--get-*` output only.

### Agent detection subsystem

The agent-state-detection subsystem lives in `internal/sessionmgr/`:

- `agents.go` — pane scan format (`@seshagy_agent_*` fields), `ParseAgents`, `detectAgent`/`detectAgentName` (process-name table + arch-suffix + descendant walk for node-based agents), `NormalizeAgentState`, `isStateFresh`.
- `agents_report.go` — `ReportAgent`/`ReleaseAgent` (seq strict-`>` + per-pane flock + tombstone release), `MarkAgentVisited`/`MarkActiveDoneAgentsIdle` (done→idle-on-visit), `ResolvePaneByCwd` (cwd→pane unique-match for the OpenCode plugin).
- `agents_capture.go` — `CaptureAgentPane`, `ApplyManifestFallback` (the capture-pane screen-rule backstop; Tier A/B authority gate via `integrations.LifecycleAuthorityFor`).
- `manifest.go` — manifest TOML schema, compiler, classifier (`detectManifest`), regex normalization, gate matcher. The manifest classifier itself emits only idle/working/blocked/done; the broader `AgentState` enum adds `unknown` for herdr's `agent_status` wire value (undetected).
- `mux.go` — `Multiplexer` interface, `Terms` (terminology), `BackendKind`, `Detect`/`DetectFromEnv` (env-based backend selection: herdr wins over tmux), and the optional `MachineSelector` capability interface (herdr only).
- `machines.go` — saved herdr SSH "machines" (`herdr machine list/status --json`): `Machine`/`MachineStatus`, `MachineSelector` implementation, and the `--machine <id>` argv scoping every herdr API call goes through (`herdrBackend.output`/`run`). `Item.Machine` names the server an item lives on so actions route correctly.
- `tmux_backend.go` / `herdr_backend.go` / `noop_backend.go` — backend implementations. Under herdr, agent-state writes (`ReportAgent`/`ReleaseAgent`/`MarkAgentVisited`/`MarkActiveDoneAgentsIdle`) and the capture-pane manifest backstop are no-ops — herdr owns detection.
- `herdr_parse.go` — JSON parsers for herdr CLI output (workspace/pane/agent payloads); ids are treated as opaque strings.
- `completion.go` — read-only one-call pane/session snapshots used only by shell completion; never route it through full source/agent loading.
- `manifest_regions.go` — region slice helpers (whole_recent, osc_title, bottom_lines(N), bottom_non_empty_lines(N), after_last_prompt_marker, after_last_horizontal_rule, prompt_box_body, osc_progress).
- `manifest_update.go` — launch-time async fetch of manifests from the herdr public catalog; local-override > cached-remote > bundled precedence; version-guarded; HTTPS-only.
- `proctree.go` — process-tree descendant walk (node-agent discovery).
- `flock_{unix,windows}.go` — per-pane flock via `x/sys/unix.Flock` (cgo-free).

Bundled manifests: `internal/sessionmgr/manifests/*.toml` (offline fallback; hot-updated from herdr.dev at runtime).

Integrations live in `internal/integrations/`:

- `install.go` — registry (`Available`: pi, codex, claude, droid, opencode), `Install`/`Uninstall`, shell-hook JSON merge (idempotent, preserves user + herdr entries), `LifecycleAuthority` per integration.
- `assets/seshagy-agent-state.sh` — shared TMUX-gated best-effort hook script (codex/claude/droid).
- `assets/seshagy-agent-state.ts` — Pi TypeScript extension.
- `assets/seshagy-opencode-plugin.ts` — OpenCode TS plugin (session.idle/permission.ask/tool.execute).

### Remote hosts subsystem

Remote herdr servers live in `internal/remote/`:

- `host.go` — the host registry (`Host`, `Alias`, `Find`, `Hosts`), the generated ssh config (`RenderSSHConfig`/`WriteSSHConfig`/`IncludeLine`/`EnsureUserSSHConfigInclude`, 0600 atomic write), and `~/.ssh/config` discovery (`DiscoverSSHConfigHosts`, one level of `Include`, no wildcards, `seshagy-*` aliases skipped). Configured hosts use the generated alias `seshagy-<name>`.
- `launcher.go` / `launcher_windows.go` — the `seshagy run` launcher. Only a launcher owns the terminal, so only it can switch the herdr client between the local server and a remote host. It runs one attached `herdr` client at a time, advertises itself as `<pid>.json` + `<pid>.sock` under `$XDG_STATE_HOME/seshagy/launchers/`, accepts `SwitchRequest`s on that unix socket (`RequestSwitch`, `ErrNoLauncher` when none is attached), replies before SIGTERMing the client (a clean detach), then starts the pending target and falls back remote→local when a remote client exits. It refuses to run under `HERDR_ENV=1`.
- `internal/config` composes the list (`RemoteHosts`/`ValidateRemoteHosts`); `remote` never imports `config`. The `seshagy run` / `seshagy hosts [setup|switch]` commands live in `cmd/seshagy/run.go`.
- The TUI (`internal/tui/commands.go`) feeds `# hosts` rows from `remote.Hosts`, and `enter` on a host either asks the launcher to switch (herdr) or opens `tmux new-window ... ssh <alias>` (tmux).

## Build, Test, and Development Commands

- `mise run verify`: runs the fast deterministic checks (`fmt:check`, `lint`, `vet`, `test`, `build`).
- `mise run ci`: runs `verify` and `vuln` sequentially for local/agent CI parity.
- `mise run fmt`: formats Go and YAML files using the configured formatters.
- `mise run vuln`: runs `govulncheck ./...` independently.
- `mise run release:check`: validates `.goreleaser.yml` without publishing.
- `make build`: builds the local `./seshagy` binary from `./cmd/seshagy`.
- `go run ./cmd/seshagy`: runs the TUI from the checkout.

Go 1.26.9 or newer is required by `go.mod`. Runtime behavior expects a multiplexer (`tmux` or `herdr`); optional tools include `zoxide`, `fd`, `yazi`, and `eza`.

## Coding Style & Naming Conventions

Use `mise run fmt` before submitting changes. Formatting uses `golangci-lint fmt`, `gofumpt`, `goimports`, `gci`, `golines`, and `yamlfmt`. Prefer small package-local helpers and existing domain terms: sessions, panes, agents, integrations, sources, and launch state. Export identifiers only when used across packages or by command-facing code.

## CLI output

All user-facing stdout/stderr output from `cmd/seshagy` and `internal/` MUST go through the `internal/cli` package (`cli.Error`/`Warn`/`Note` → stderr, `cli.Success`/`Info` → stdout, `cli.Print`/`Println`/`Printf` for verbatim machine-readable output like `--json`/`--version`/config paths/TOML/`--get-*` list lines). It applies cargo/rustc-style severity coloring (only the prefix word is colored) and auto-disables color for non-TTY streams or `NO_COLOR` (`CLICOLOR_FORCE` overrides). Never call `fmt.Print`/`Printf`/`Println` (implicit stdout) or `fmt.Fprint*` to `os.Stdout`/`os.Stderr` directly — the golangci-lint `forbidigo` rule in `.golangci.yml` fails CI on both, and the pi-lens ast-grep rule `no-raw-cli-output` flags them in the editor too. Formatting into a non-CLI buffer (`strings.Builder`) with `fmt.Fprintf` is allowed but needs a `//nolint:forbidigo` comment (it writes to a buffer, not a stream); `fmt.Sprintf` needs no exemption. The TUI (`internal/tui`) is out of scope — it renders via its own Bubble Tea/lipgloss theme.

## Structured diagnostics

Diagnostic logging is owned by `internal/logging` and is file-only JSONL; never attach `slog` to stdout/stderr or call `slog.SetDefault` (it mutates the standard `log` package). New events must be registered in the static safety handler and emitted through `logging.LogAttrs` with one operation owner to avoid duplicates. Allowed fields are stable enums/categories, counts, booleans, durations, generations, and raw multiplexer IDs at debug level only. Never log raw errors, user-controlled names or messages, paths/cwd, commands/argv, environment/config values, pane captures/OSC content, prompts/search/key input, subprocess output, URLs, or agent-supplied session IDs. Use `logging.ClassifyError` and add privacy-sentinel tests for new call sites. Logging `off` must perform no log filesystem, random-ID, lock, or retention work. Human `diagnostics` output may show local paths; `diagnostics --json` must keep expanded paths redacted.

## Testing Guidelines

Add focused table-driven tests near the package being changed. Use names like `TestParseAgentsSkipsNonAgentsAndFormatsLocation` that describe behavior. `mise run verify` is the default fast check; run `mise run ci` before handoff, and use `mise run test:focused ./internal/sessionmgr ParseAgents` for narrow loops. Some `sessionmgr` tests create temporary tmux sessions and skip when `tmux` is unavailable.

## Agent-state invariants

- **Namespace:** only `@seshagy_agent_*` under tmux. Never `@agent_*` — that namespace belongs to the separate `gentle-agent-state` / `tmux-agent-state` project, NOT to herdr. (herdr exposes agent state via its own CLI/socket API and the `agent_status` field, not tmux user options.) Under herdr (`HERDR_ENV=1`), seshagy writes NO agent state at all — herdr owns detection; the `@seshagy_agent_*` writes, capture-pane manifest backstop, and state-reporting hooks are all tmux-only and suppress under herdr.
- **Stale-can't-resurrect:** every state write goes through `@seshagy_agent_seq` strict-`>` + flock + tombstone release. `MarkAgentVisited` does NOT advance the seq.
- **No pane scraping except the capture-pane manifest backstop** (the `manifest_fallback` sanctioned exception, default-on, hot-updated from herdr).
- **Authority model:** lifecycle agents (pi/opencode) suppress manifest when hooks are fresh; partial-hook agents (codex/claude/droid) and hook-less agents always run the manifest classifier. Manifest overwrites state only on a positive rule match; no-match preserves the existing state.
- **Done is hook-only:** capture-only agents (cursor/agy/grok + hot-update-only agents like amp/cline/devin/gemini/hermes/kilo/kimi/kiro/qodercli) can show working/blocked/idle but NOT done — `done` requires a hook/plugin report because the screen cannot distinguish "turn finished" from "idle and waiting". This is inherent to screen-based detection.
- **Release suppression:** after `ReleaseAgent` clears state, capture-pane manifest is suppressed for 10s (`@seshagy_agent_released_at`) to prevent visual resurrection of a just-released pane.
- **Zero new go.mod deps if avoidable.** cgo-free. `BurntSushi/toml` is the only non-stdlib dep for manifests.

## Shell completion invariants

The operational parser remains authoritative; Cobra is a completion-only shadow
tree pinned to v1.10.2. Any PR that changes a command, alias, flag, positional,
fixed enum, or registry must update the shadow tree and focused completion/parity
tests in the same PR. Generated endpoint/grouping patches are pinned and
fail-closed; never add an executable `eval` path for command-line input.

## Commit & Pull Request Guidelines

All repository commits must follow Conventional Commits: `<type>[optional scope]: <description>`. Keep descriptions focused, imperative, and without trailing punctuation. Examples include `feat(tui): add ranked fuzzy search`, `fix(sessionmgr): prevent stale agent resurrection`, and `refactor(config): simplify validation`. Mark breaking changes with `!` before the colon.

Changes are accepted through pull requests only. Pull requests should include a short problem/solution summary, `mise run ci` results, and screenshots or terminal captures for visible TUI changes. Call out any config, tmux, integration, or completion-parity behavior changes. Use only generic `Closes #NN` issue links.

## CI/CD and Release Workflow

GitHub Actions runs formatting, linting, vet, tests, vulnerability checks, and build through pinned `mise` tools. Releases are tag-driven: after `mise run ci` and `mise run release:check` pass on a clean tree, push a `v*` tag to run GoReleaser.
