# Architecture

## Multiplexer support

seshagy supports two multiplexer backends with a shared vocabulary:

| concept           | tmux    | herdr     |
| ----------------- | ------- | --------- |
| project container | session | workspace |
| layout group      | window  | tab       |
| terminal          | pane    | pane      |

Backend is selected automatically from the environment — there is no config
key to set. The TUI and CLI adapt their terminology to the active backend
(sessions/windows under tmux; workspaces/tabs under herdr).

**Agent state under herdr is read-only.** herdr owns agent detection and
exposes it via the `agent_status` field (`idle`/`working`/`blocked`/`done`/
`unknown`). seshagy reads this directly; it does **not** call
`herdr pane report-agent`, run the capture-pane manifest backstop, or write
`@seshagy_agent_*` options under herdr. The state-reporting hooks (shell,
Pi extension, OpenCode plugin) early-exit when `$HERDR_ENV=1` is set.

Builds from source require Go 1.26, matching `go.mod`. Shell-hook integrations
may use `bash` and `python3`; the OpenCode plugin runs on Bun/Node.

## What seshagy manages

| Area                             | What you can do                                                                   |
| -------------------------------- | --------------------------------------------------------------------------------- |
| tmux sessions / herdr workspaces | list, attach/focus, rename, kill, and preview                                     |
| project directories              | create/switch sessions or workspaces from `zoxide` or a configurable `fd` command |
| agent panes                      | list, filter, focus, or kill detected agent panes                                 |
| current session agents           | narrow the agent view to the current session/workspace (`o`)                      |
| input flow                       | use classic action keys or type-first filtering with a prefix key                 |

When a directory becomes a session, the session name is derived from the
basename: `.config` becomes `dot_config`, and unsupported characters collapse to
`_` (`foo.bar` becomes `foo_bar`).

## Agent state detection

seshagy detects four states per agent pane:

| State     | Meaning                                                           |
| --------- | ----------------------------------------------------------------- |
| `idle`    | not working, or a `done` pane that has been visited               |
| `working` | agent actively running a turn                                     |
| `blocked` | asking permission or asking the user a question                   |
| `done`    | finished a turn, pane not yet visited (clears to `idle` on focus) |

Detection uses a layered authority model mirroring
[herdr](https://herdr.dev/docs/agents):

**Tier A — lifecycle authority (hooks/plugins own state).** Pi and OpenCode
emit the full lifecycle (idle/working/blocked/done) through their
integrations. When the integration is installed and actively reporting,
seshagy uses those reports and suppresses the screen-rule backstop for that
pane. Near-instant, event-driven (~0ms).

**Tier B — partial hooks + screen rules (screen owns state).** Claude Code,
Codex, and Factory Droid have shell hooks, but their hooks miss approval-result
and ESC-interrupt transitions. For these agents seshagy **always** runs the
capture-pane screen-rule classifier in parallel with the hooks; on a positive
rule match, the screen result overwrites the hook state (so a stale `working`
after ESC is corrected to `idle` within one poll). Hooks still provide
near-instant `working`/`done` signals between screen captures.

**Tier C — screen rules only.** Cursor Agent CLI, Antigravity, Grok Build, and
the other discovered agents have no usable hooks. Their state comes entirely
from the capture-pane classifier (~1s poll cadence in the Agents source).

The screen-rule backstop captures the last 30 pane lines via
`tmux capture-pane` and matches them against per-agent TOML rules (regions,
`contains`/`regex`/`line_regex`, nested `all`/`any`/`not` gates). `blocked` is
strict: it is only set when a rule explicitly matches a known permission or
question UI. No match leaves the existing state unchanged.

### Manifest hot-update

Bundled manifests ship as an offline fallback. On launch, seshagy fetches the
latest manifests from the [herdr public
catalog](https://herdr.dev/agent-detection/) (async, non-blocking) and caches
them locally. Precedence: local override (`$XDG_CONFIG_HOME/seshagy/agent-detection/<id>.toml`)

> cached remote > bundled embed. Remote manifests are version-guarded against
> downgrade and compile-validated before caching. This keeps screen rules current
> without a seshagy release.

### Supported agents

Discovered via process-name matching plus a process-tree descendant walk (for
node-wrapped CLIs that report as `node`):

| Agent              | Process name(s)          | Detection              |
| ------------------ | ------------------------ | ---------------------- |
| Pi                 | `pi`                     | lifecycle (extension)  |
| OpenCode           | `opencode`               | lifecycle (plugin)     |
| Claude Code        | `claude`                 | partial hooks + screen |
| Codex              | `codex`                  | partial hooks + screen |
| Factory Droid      | `droid`, `factory`       | partial hooks + screen |
| Cursor Agent       | `cursor-agent`, `cursor` | screen only            |
| Antigravity        | `agy`, `antigravity`     | screen only            |
| Grok Build         | `grok`                   | screen only            |
| GitHub Copilot CLI | `copilot`                | screen only            |
| Amp                | `amp`                    | screen only            |
| Cline              | `cline`                  | screen only            |
| Devin              | `devin`                  | screen only            |
| Gemini             | `gemini`                 | screen only            |
| Hermes             | `hermes`, `hermes-agent` | screen only            |
| Kilo Code          | `kilo`, `kilocode`       | screen only            |
| Kimi Code          | `kimi`                   | screen only            |
| Kiro               | `kiro-cli`               | screen only            |
| Qoder CLI          | `qodercli`, `qoderclicn` | screen only            |

Architecture-suffixed binary names (e.g. `codex-aarch64-a`) are matched via a
prefix fallback.
