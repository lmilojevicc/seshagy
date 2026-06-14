# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI/TUI project for `seshagy`, an agent-aware tmux dashboard. The command entry point is `cmd/seshagy/`. Core packages live under `internal/`: `config` for TOML config, `sessionmgr` for tmux sessions and agent pane metadata, `integrations` for hook installs, and `tui` for Bubble Tea UI state/rendering. Tests sit beside code as `*_test.go` files.

## Build, Test, and Development Commands

- `mise run verify`: runs CI checks (`fmt:check`, `lint`, `vet`, `test`, `build`).
- `mise run fmt`: formats Go and YAML files using the configured formatters.
- `mise run vuln`: runs `govulncheck ./...`; GitHub CI runs this as a separate gate.
- `mise run release:check`: validates `.goreleaser.yml` without publishing.
- `make build`: builds the local `./seshagy` binary from `./cmd/seshagy`.
- `go run ./cmd/seshagy`: runs the TUI from the checkout.

Go 1.26 is in `go.mod`. Runtime behavior expects `tmux`; optional tools include `zoxide`, `fd`, `yazi`, and `eza`.

## Coding Style & Naming Conventions

Use `mise run fmt` before submitting changes. Formatting uses `golangci-lint fmt`, `gofumpt`, `goimports`, `gci`, `golines`, and `yamlfmt`. Prefer small package-local helpers and existing domain terms: sessions, panes, agents, integrations, sources, and launch state. Export identifiers only when used across packages or by command-facing code.

## Testing Guidelines

Add focused table-driven tests near the package being changed. Use names like `TestParseAgentsSkipsNonAgentsAndFormatsLocation` that describe behavior. `mise run verify` is the default check; use `mise run test:focused ./internal/sessionmgr ParseAgents` for narrow loops. Some `sessionmgr` tests create temporary tmux sessions and skip when `tmux` is unavailable.

## Commit & Pull Request Guidelines

Recent history uses concise, imperative commit subjects, for example `Update README` and `Harden lifecycle agent integrations`. Capitalize the subject, avoid trailing punctuation, and keep it focused on one change.

Pull requests should include a short problem/solution summary, `mise run verify` results, and screenshots or terminal captures for visible TUI changes. Call out any config, tmux, or integration behavior changes.

## CI/CD and Release Workflow

GitHub Actions runs formatting, linting, vet, tests, vulnerability checks, and build through pinned `mise` tools. Releases are tag-driven: after `mise run verify`, `mise run vuln`, and `mise run release:check` pass on a clean tree, push a `v*` tag to run GoReleaser.

## Agent-Specific Instructions

Do not scrape terminal pane contents when adding agent features; this project relies on explicit tmux `@agent_*` metadata reported by hooks/plugins. The opt-in `manifest_fallback` setting is an exception: it captures the last 30 pane lines for Herdr screen-rule matching when hooks are silent. Preserve sequence handling for agent report/release flows so stale updates cannot resurrect cleared state.
