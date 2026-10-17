<div align="center">

# Seshagy

![cover](.github/assets/cover.gif)

_The agent-aware terminal dashboard for modern developer workflows._

</div>

Navigating the terminal shouldn't break your flow, and managing AI coding agents shouldn't require playing hide-and-seek with your panes.

`seshagy` is a lightning-fast dashboard that bridges the gap between your project directories, active multiplexer sessions, and terminal-based coding agents. It gives you a bird's-eye view of your entire workspace, letting you jump exactly where you need to be with a single keystroke.

## ✨ Why use seshagy?

- **Navigate at the speed of thought:** Stop typing `cd` and `tmux attach`. Instantly jump to existing sessions or spin up new ones from your most used directories (powered by `zoxide` and `fd`).
- **Never lose track of your AI agents:** Running Pi, Claude Code, Codex, or Opencode in the background? `seshagy` automatically tracks their states across all panes. Instantly see if an agent is _working_, _blocked_ (needs your input), _done_, or _idle_ — without hunting for windows.
- **Zero friction, zero clutter:** Designed to be invisible until you need it. Pop it open with a keybind, find your destination, and hit enter. The dashboard instantly vanishes.
- **Universal multiplexer support:** Native, auto-detecting support for both **tmux** and **herdr**. It just works.
- **Remote servers in reach:** Bring your homelab, edge box, or a Tailscale/iroh machine into the same picker and switch a herdr client to it with one key.

## 🚀 Quick Start

### 1. Install

**Homebrew:**

```sh
brew tap lmilojevicc/tap && brew install seshagy
```

The Homebrew formula installs Bash, Zsh, and Fish completions automatically.
For other installs, generate and source the script for your shell:

```sh
# Bash
source <(seshagy completion bash)
# Zsh (save as a file named _seshagy in a directory on $fpath)
seshagy completion zsh > "${fpath[1]}/_seshagy"
# Fish
seshagy completion fish | source
```

For a persistent manual install, write the generated script to your shell's
standard completions directory instead of sourcing it from every startup.

**Go:**

```sh
go install github.com/lmilojevicc/seshagy/cmd/seshagy@latest
```

**Nix:**

```sh
nix run github:lmilojevicc/seshagy
# Or install it into your profile:
nix profile install github:lmilojevicc/seshagy
```

### 2. The Recommended Setup (Day-to-Day Use)

For the best experience, `seshagy` is meant to be a one-keystroke pop-up that gets out of your way as soon as you jump to a session.

We highly recommend installing the keybind via `seshagy install keybind` for day-to-day use. This wires up an ephemeral overlay (or popup window) to your multiplexer that auto-dismisses when you make a selection:

```sh
seshagy install keybind
```

_(Note: You can also run `seshagy keybind install tmux` or `seshagy keybind install herdr` if you need to specify your multiplexer or custom keys manually)._

### 3. Basic Usage

Open the picker anytime by running `seshagy` (or pressing your new keybind!). It is type-first:
just start typing to fuzzy-search sessions, agents, directories, and hosts.

- Type a filter sigil on an empty prompt to narrow the list: `@` agents, `!` needs you,
  `$` sessions, `*` dirs, `~` zoxide, `/` find, `#` hosts. `tab` / `shift+tab` cycle them.
- Press `enter` on a session/workspace to attach, on a directory to create or switch to a
  session for it, and on an agent to focus its pane.
- Press `ctrl+a` to open the agent integration install menu.

---

## 🖥️ Remote Hosts

seshagy can switch a full-screen [herdr](https://herdr.dev) client between servers, so you can
jump from your laptop to a homelab box in one keystroke.

Because a herdr popup/plugin pane is a child of the herdr **server**, it cannot move your terminal
to another server by itself. `seshagy run` owns the terminal instead: it attaches the herdr client,
stays in the background, and switches (or returns to local) on request.

```sh
seshagy run              # local herdr server, switching enabled
seshagy run homelab      # start attached to a configured host
seshagy run --session work edge
```

Keybind a plain terminal (not a herdr pane) to `seshagy run`, then pick a host with `#` and press
`enter` to switch. `seshagy hosts switch <name|local>` does the same from a herdr pane or a script.

### Configure hosts

```toml
[remote]
ssh_config_hosts = true   # also offer the Host entries of ~/.ssh/config (default)

# Plain OpenSSH: [user@]host[:port] or ssh://user@host:port
[[hosts]]
name = "homelab"            # unique, [A-Za-z0-9._-]+; the ssh alias is seshagy-<name>
target = "milo@192.168.1.10:2222"
session = "work"            # optional: remote herdr session to attach

# Tailscale MagicDNS (or Tailscale SSH) is an ordinary ssh target
[[hosts]]
name = "tailnet"
target = "milo@homelab.tailnet.ts.net"

# iroh-ssh: target is the endpoint id; seshagy writes `ProxyCommand iroh-ssh proxy %h`
[[hosts]]
name = "edge"
target = "8f14e45fceea167a5a36dedd4bea2543a1b2c3d4e5f60718293a4b5c6d7e8f90"
transport = "iroh"

# A custom transport keeps full control of the ProxyCommand (%h / %p expand)
[[hosts]]
name = "jumpbox"
target = "10.0.0.5"
proxy_command = "ssh -W %h:%p bastion"
```

Targets are `[user@]host[:port]` or `ssh://user@host:port`. Render the generated ssh config and
let OpenSSH use it:

```sh
seshagy hosts            # list configured + discovered hosts
seshagy hosts setup      # write ~/.config/seshagy/ssh_config and Include it from ~/.ssh/config
```

`seshagy hosts setup` is the only command that edits `~/.ssh/config` (it prepends one `Include`
line and backs the file up once to `~/.ssh/config.seshagy.bak`). Hosts defined in your own
`~/.ssh/config` are used as-is, so anything OpenSSH already supports — including Tailscale SSH and
`ProxyCommand` transports — works without extra seshagy config.

---

## 🤖 Agent Tracking

`seshagy` acts as a mission control for your terminal AI tools. It knows when your agents need attention and when they are busy crunching tokens.

Supported agents include: **Pi, OpenCode, Claude Code, Codex, Cursor Agent, Antigravity, Grok Build, Cline, Devin**, and more.

### State Indicators

| State          | Meaning                                                  |
| -------------- | -------------------------------------------------------- |
| 🟢 **working** | Agent is actively generating code or running commands.   |
| 🔴 **blocked** | Agent is asking for permission or user input. Needs you! |
| 🔵 **done**    | Agent finished a turn but you haven't checked on it yet. |
| ⚪ **idle**    | Agent is sleeping or waiting for a prompt.               |

_How does it work?_ `seshagy` supports both `tmux` and `herdr` backends with different behaviors:

- Under **tmux**, it runs its own custom herdr-like detection.
- Under **herdr**, it defers completely to the native agent detection provided directly by herdr.

### Agent logos

Agent rows can show each vendor's mark from the **Herdr Agent Icons Max** font
([herdr-radar](https://github.com/hhdebb/herdr-radar), MIT; the vendor marks belong to their owners).
Install that font and seshagy uses it automatically; without it, rows fall back to the state glyph and
the agent's name. `SESHAGY_AGENT_ICONS=auto|font|none` overrides the detection, and `[icons] mode =
"text"` (or `"none"`) never prints private-use glyphs.

---

## ⌨️ TUI

The picker is always type-first: printable keys filter the list, and every action is a chord.

| Key                           | Action                                                                                 |
| ----------------------------- | -------------------------------------------------------------------------------------- |
| _type_                        | Fuzzy-filter the list (title matches rank first)                                        |
| `tab` / `shift+tab`           | Next / previous filter                                                                  |
| `↑`/`↓`, `ctrl+p`/`ctrl+n`    | Move the selection (wraps)                                                              |
| `pgup`/`pgdown`, `home`/`end` | Page through the list / jump to its ends                                                |
| `enter`                       | Attach to the session, focus the agent pane, open the directory, or switch to the host  |
| `ctrl+r`                      | Rename the selected session/workspace or agent (inline)                                 |
| `ctrl+x`, `delete`            | Kill the selected session/workspace or agent pane (`enter`/`y` confirms)                |
| `ctrl+o`                      | Toggle the preview window (the `dropdown` layout starts with it hidden)                 |
| `ctrl+y`                      | Pick a directory with `yazi`                                                            |
| `ctrl+l`                      | Reload every source                                                                     |
| `ctrl+a`                      | Open the agent integration install menu                                                 |
| `ctrl+u` / `ctrl+w`           | Clear the query / delete the last word                                                  |
| `esc`                         | Clear the query and filter first; quit when both are empty                              |
| `ctrl+c`                      | Quit                                                                                    |

### Filters

Type a sigil on an **empty** prompt to select a filter, or cycle them with `tab`. The chips sit
right of the prompt; `# hosts` only appears when you have hosts.

| Sigil | Filter      | Shows                                                      |
| ----- | ----------- | ---------------------------------------------------------- |
|       | `all`       | Everything, agents needing you first                       |
| `@`   | `agents`    | Agent panes and their state                                |
| `!`   | `needs you` | Agents that are blocked or finished a turn                 |
| `$`   | `sessions`  | Sessions (tmux) / workspaces (herdr)                       |
| `*`   | `dirs`      | `zoxide` history plus `fd` discoveries                     |
| `~`   | `zoxide`    | Frecency-ranked directories                                |
| `/`   | `find`      | `fd` (or `find` when `fd` is not installed)                |
| `#`   | `hosts`     | Remote herdr servers (see the Remote Hosts section above)  |

---

## ⚙️ Requirements & Integrations

- **Multiplexer:** [tmux](https://github.com/tmux/tmux) or [herdr](https://herdr.dev). seshagy auto-detects your environment!
- **Optional (but highly recommended):**
  - `zoxide` (for frecency-ranked directory history)
  - `fd` (for blazing-fast directory discovery)
  - `yazi` (for interactive directory picking)
  - `eza` (for richer previews)

---

## 🛠 Configuration & Customization

seshagy is highly customizable to match your workflow and aesthetic.

Configuration lives in `~/.config/seshagy/config.toml` (or your `$XDG_CONFIG_HOME`). Initialize the default config file with:

```sh
seshagy config init
```

From there, you can:

- Customize theme colors and icons to match your terminal rice.
- Tweak `fd` commands and the agent manifest catalog.
- Enable or disable the preview column.
- Define `[[hosts]]` for remote herdr servers (see the Remote Hosts section above).

See the [configuration guide](docs/configuration.md) for every key.

---

## 📖 Deep Dive & CLI

While the TUI is the main interface, seshagy provides a rich set of CLI commands for scripting, fzf-style menus, and agent integrations.

<details>
<summary>Click to view CLI commands and advanced usage</summary>

```sh
# Fetch raw data
seshagy --get-all --json
seshagy --get-sessions
seshagy --get-agents
seshagy --get-zoxide

# Agent integration internals
seshagy integration install pi
seshagy integration uninstall claude

# Manual multiplexer wiring
# herdr modes: pane (default) | popup (herdr 0.7.4+)
seshagy keybind install herdr --key f
seshagy keybind install herdr --mode popup --width 80% --height 80%
seshagy keybind install herdr --mode popup --persistent
# tmux:
seshagy keybind install tmux --mode popup
seshagy keybind install tmux --mode pane-zoomed --persistent
```

By default, installed keybinds launch `seshagy --ephemeral` and auto-dismiss on
focus loss. Pass `--persistent` when installing either a tmux or herdr keybind
to launch `seshagy` without focus-loss auto-dismiss; it stays open until you
explicitly quit with `esc` or `ctrl+c`.

Herdr popup `--width` and `--height` accept either terminal cell counts (for
example, `120`) or percentages (for example, `80%`) and both default to `80%`.
After installation, you can also edit `width`/`height` in the `[[keys.command]]`
binding in `~/.config/herdr/config.toml`.

### Multiplexer Vocabulary

seshagy speaks your multiplexer's language seamlessly:

| Concept  | tmux    | herdr     |
| -------- | ------- | --------- |
| Project  | Session | Workspace |
| Layout   | Window  | Tab       |
| Terminal | Pane    | Pane      |

</details>

## 🛠 Development

- `mise run verify` runs the fast deterministic checks (fmt, lint, vet, test, build).
- `mise run vuln` runs the vulnerability check independently.
- `mise run ci` runs `verify` and `vuln` sequentially for CI parity.
- `make build` produces a local `./seshagy` binary.

---

_Built for developers who value their time and terminal._
