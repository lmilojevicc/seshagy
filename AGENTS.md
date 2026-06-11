# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI/TUI project for `seshagy`, an agent-aware tmux dashboard. The command entry point lives in `cmd/seshagy/`. Core behavior is split under `internal/`: `config` handles TOML config loading and defaults, `sessionmgr` handles tmux sessions, directories, and agent pane metadata, `integrations` manages agent hook installs, and `tui` contains Bubble Tea UI state and rendering. Tests sit beside the code as `*_test.go` files. Root files include `go.mod`, `go.sum`, `Makefile`, and `README.md`.

## Build, Test, and Development Commands

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

## Agent-Specific Instructions

Do not scrape terminal pane contents when adding agent features; this project relies on explicit tmux `@agent_*` metadata reported by hooks/plugins. Preserve sequence handling for agent report/release flows so stale updates cannot resurrect cleared state.
