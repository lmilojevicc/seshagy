# Contributing to seshagy

Thanks for your interest in contributing to `seshagy`! `seshagy` is an
agent-aware terminal dashboard written in Go that supports both
[tmux](https://github.com/tmux/tmux) and [herdr](https://herdr.dev) as
multiplexer backends. This guide covers the essentials; for the full picture
(structure, conventions, agent-state invariants), read
[`AGENTS.md`](./AGENTS.md) — it is the source of truth.

## Quick start

```sh
git clone https://github.com/lmilojevicc/seshagy.git
cd seshagy
mise install        # installs the pinned dev tools (Go, linters, …)
mise run verify     # fmt:check + lint + vet + test + build
go run ./cmd/seshagy # run the TUI from the checkout
```

You will need Go 1.26 (matching `go.mod`). Runtime needs a multiplexer
(`tmux` or `herdr`); optional helpers are `zoxide`, `fd`, `yazi`, and `eza`.

## Development commands

All tasks are defined in `mise.toml`.

| Command | What it does |
| --- | --- |
| `mise run fmt` | Format Go and YAML files (`golangci-lint fmt` + `yamlfmt`). |
| `mise run verify` | The CI gate: `fmt:check`, `lint`, `vet`, `test`, `build`. |
| `mise run lint` | `golangci-lint run ./...`. |
| `mise run vet` | `go vet ./...`. |
| `mise run test` | `go test ./...`. |
| `mise run test:focused ./internal/sessionmgr ParseAgents` | Run a focused subset (`<package> <run-pattern>`). |
| `mise run vuln` | `govulncheck ./...` (CI runs this as a separate gate). |
| `mise run release:check` | Validate `.goreleaser.yml` without publishing. |
| `make build` | Build the local `./seshagy` binary from `./cmd/seshagy`. |

See `AGENTS.md` for details on each.

## Project structure

The command entry point is `cmd/seshagy/`. Core packages live under `internal/`:

- `config/` — TOML config parsing.
- `sessionmgr/` — the `Multiplexer` backend abstraction (tmux/herdr/noop),
  tmux sessions, and the agent pane metadata + detection engine.
- `integrations/` — hook/plugin installs (pi, codex, claude, droid, opencode).
- `tui/` — Bubble Tea UI state and rendering.

Tests sit beside code as `*_test.go`. The active backend is auto-detected from
the environment (`$HERDR_ENV=1` → herdr wins; else `$TMUX` → tmux; else noop).
`AGENTS.md` has the authoritative, deeper breakdown, including the
agent-state-detection subsystem and the agent-state invariants you must
respect.

## Coding style

Run `mise run fmt` before submitting. Formatting uses `golangci-lint fmt`,
`gofumpt`, `goimports`, `gci`, `golines`, and `yamlfmt`. Prefer small
package-local helpers and the project's existing domain terms: sessions,
panes, agents, integrations, sources, and launch state. Export identifiers
only when they are used across packages or by command-facing code.

Keep dependencies minimal: `BurntSushi/toml` is the only non-stdlib
dependency. Add no new `go.mod` dependencies if avoidable, and stay cgo-free.
See `AGENTS.md` for the full conventions and the agent-state invariants
(namespacing, stale-can't-resurrect, authority model, etc.).

## Testing

Add focused, table-driven tests next to the package you change, with names
that describe behavior (for example
`TestParseAgentsSkipsNonAgentsAndFormatsLocation`). `mise run verify` is the
default check; iterate on a narrow slice with
`mise run test:focused ./internal/sessionmgr ParseAgents`.

A few notes:

- Some `sessionmgr` tests create temporary tmux sessions and **skip** when
  `tmux` is unavailable, so a missing `tmux` does not fail the suite.
- Tests must be **deterministic** when run inside herdr: clear `HERDR_ENV`
  (for example `env -u HERDR_ENV env -u TMUX mise run verify`) so backend
  auto-detection does not change behavior between environments.

## Pull requests

Open PRs against `main`; the project uses squash merge.

- Keep each PR focused on a single change.
- Run `mise run verify` before pushing.
- Include a short problem/solution summary in the PR description and reference
  `mise run verify` results.
- Add screenshots or terminal captures for any visible TUI change.
- Call out any config, tmux, herdr, or integration behavior changes.

Link issues with `Closes #NN` so they auto-close on merge. The PR template in
`.github/PULL_REQUEST_TEMPLATE.md` guides you through this.

## Commit messages

Use concise, imperative subjects. Capitalize the first word and avoid
trailing punctuation; keep each commit focused on one change.

Examples:

```text
Add herdr keybind installer
Update README
Harden lifecycle agent integrations
```

## Releases

Releases are **tag-driven**. Once `mise run verify`, `mise run vuln`, and
`mise run release:check` pass on a clean tree, push a `v*` tag and GoReleaser
takes it from there. Do not cut a release from a dirty tree.

## Reporting bugs and ideas

Use the GitHub issue templates in `.github/ISSUE_TEMPLATE/`. When filing a
bug, please include:

- which **multiplexer** you are running (`tmux` or `herdr`),
- your `seshagy --version` output, and
- your OS (and architecture).

That context determines which backend code path applies.
