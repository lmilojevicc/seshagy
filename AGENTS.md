# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI/TUI project for `seshagy`, an agent-aware tmux dashboard. The command entry point lives in `cmd/seshagy/`. Core behavior is split under `internal/`: `config` handles TOML config loading and defaults, `sessionmgr` handles tmux sessions, directories, and agent pane metadata, `integrations` manages agent hook installs, and `tui` contains Bubble Tea UI state and rendering. Tests sit beside the code as `*_test.go` files. Root files include `go.mod`, `go.sum`, `Makefile`, and `README.md`.

## Build, Test, and Development Commands

- `mise run verify`: runs the local CI gate (`fmt:check`, `lint`, `vet`, `test`, `build`).
- `mise run vuln`: runs `govulncheck ./...`; GitHub CI runs this as a separate gate.
- `mise run release:check`: validates `.goreleaser.yml` without publishing.
- `mise run release:snapshot`: builds local release artifacts under `dist/` without publishing.
- `make build`: builds the local `./seshagy` binary from `./cmd/seshagy`.
- `make test`: runs `go test ./...` across all packages.
- `make vet`: runs `go vet ./...` for static checks.
- `make install`: installs the command with `go install ./cmd/seshagy`.
- `go run ./cmd/seshagy`: runs the TUI from the checkout.

Go 1.26 is declared in `go.mod`. Runtime behavior expects `tmux`; optional integrations use tools such as `zoxide`, `fd`, `yazi`, and `eza`.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on changed `.go` files and keep imports organized by `go fmt`/`goimports` conventions. Prefer small package-local helpers and clear domain names matching the existing vocabulary: sessions, panes, agents, integrations, sources, and launch state. Export identifiers only when they are used across packages or are part of the command-facing model.

## Testing Guidelines

Add focused table-driven tests near the package being changed. Use names like `TestParseAgentsSkipsNonAgentsAndFormatsLocation` that describe the behavior, not the implementation. `make test` is the default check before submitting. Some `sessionmgr` tests create temporary tmux sessions and skip when `tmux` is unavailable; keep external-tool-dependent tests isolated and skippable.

## Commit & Pull Request Guidelines

Recent history uses concise, imperative commit subjects, for example `Update README` and `Harden lifecycle agent integrations`. Follow that style: capitalize the subject, avoid trailing punctuation, and keep it focused on one change.

Pull requests should include a short problem/solution summary, test results such as `make test` and `make vet`, and screenshots or terminal captures for visible TUI changes. Link related issues when available and call out any config, tmux, or integration behavior changes.

## CI/CD and Release Workflow

CI mirrors the `dotty` setup: GitHub Actions runs `fmt:check`, `lint`, `vet`, `test`, `vuln`, and `build` through pinned `mise` tools. Release automation is tag-driven: pushing a `v*` tag runs GoReleaser and publishes Linux/macOS `amd64`/`arm64` archives plus checksums to GitHub Releases.

Before creating a release tag:

```sh
mise run verify
mise run vuln
mise run release:check
```

For the first release:

```sh
git tag v0.1.0
git push origin main
git push origin v0.1.0
```

Do not tag before the working tree is clean and the checks above pass.

## Agent-Specific Instructions

Do not scrape terminal pane contents when adding agent features; this project relies on explicit tmux `@agent_*` metadata reported by hooks/plugins. Preserve sequence handling for agent report/release flows so stale updates cannot resurrect cleared state.
