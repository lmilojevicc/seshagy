package integrations

import (
	"fmt"
	"strings"
)

const shellHookName = "seshagy-agent-state.sh"

func shellHookAsset(target Target, binaryPath string) string {
	return fmt.Sprintf(`#!/bin/sh
# installed by seshagy
# managed by seshagy; reinstalling or updating the integration overwrites this file.
# SESHAGY_INTEGRATION_ID=%s
# SESHAGY_INTEGRATION_VERSION=%d

set -eu
agent="${1:-%s}"
state="${2:-idle}"
message="${3:-}"

[ -n "${TMUX_PANE:-}" ] || exit 0
bin="${SESHAGY_BIN:-}"
if [ -z "$bin" ]; then
  bin=%s
fi
if [ ! -x "$bin" ]; then
  bin="$(command -v seshagy 2>/dev/null || true)"
fi
[ -n "$bin" ] || exit 0

source="seshagy:$agent"
case "$state" in
  session|start) state="idle" ;;
  idle|working|blocked|done|aborted|unknown) ;;
  release|end|shutdown)
    "$bin" --release-agent --source "$source" >/dev/null 2>&1 || true
    exit 0
    ;;
  *) exit 0 ;;
esac

if [ -n "$message" ]; then
  "$bin" --report-agent --agent "$agent" --state "$state" --source "$source" --message "$message" >/dev/null 2>&1 || true
else
  "$bin" --report-agent --agent "$agent" --state "$state" --source "$source" >/dev/null 2>&1 || true
fi
`, target, installVersion, target, shellQuoteLiteral(binaryPath))
}

func piExtensionAsset(binaryPath string) string {
	return fmt.Sprintf(`// installed by seshagy
// managed by seshagy; reinstalling or updating the integration overwrites this file.
// SESHAGY_INTEGRATION_ID=pi
// SESHAGY_INTEGRATION_VERSION=%d
// @ts-nocheck

import { spawn } from "node:child_process";

const BIN = process.env.SESHAGY_BIN || %q;
const SOURCE = "seshagy:pi";

function run(args: string[]) {
  if (!process.env.TMUX_PANE) return;
  const child = spawn(BIN, args, { stdio: "ignore", detached: true });
  child.on("error", () => {});
  child.unref?.();
}

function report(state: string, message?: string) {
  const args = ["--report-agent", "--agent", "pi", "--state", state, "--source", SOURCE];
  if (message) args.push("--message", message);
  run(args);
}

function release() {
  run(["--release-agent", "--source", SOURCE]);
}

export default function (pi) {
  if (!process.env.TMUX_PANE) return;
  let blockedCount = 0;
  let idleTimer: ReturnType<typeof setTimeout> | undefined;

  function clearIdle() {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = undefined;
  }

  pi.on?.("session_start", () => report("idle"));
  pi.on?.("agent_start", () => {
    clearIdle();
    report("working");
  });
  pi.on?.("agent_end", () => {
    clearIdle();
    idleTimer = setTimeout(() => report("done"), 250);
    idleTimer.unref?.();
  });
  pi.events?.on?.("herdr:blocked", (data) => {
    if (data?.active) {
      blockedCount += 1;
      report("blocked", data?.label);
    } else {
      blockedCount = Math.max(0, blockedCount - 1);
      report(blockedCount > 0 ? "blocked" : "working");
    }
  });
  pi.on?.("session_shutdown", release);
}
`, installVersion, binaryPath)
}

func opencodePluginAsset(binaryPath string) string {
	return fmt.Sprintf(`// installed by seshagy
// managed by seshagy; reinstalling or updating the integration overwrites this file.
// SESHAGY_INTEGRATION_ID=opencode
// SESHAGY_INTEGRATION_VERSION=%d

import { spawn } from "node:child_process";

const BIN = process.env.SESHAGY_BIN || %q;
const SOURCE = "seshagy:opencode";

function run(args) {
  if (!process.env.TMUX_PANE) return Promise.resolve();
  return new Promise((resolve) => {
    const child = spawn(BIN, args, { stdio: "ignore", detached: true });
    child.on("error", resolve);
    child.on("close", resolve);
    child.unref?.();
  });
}

function report(state) {
  return run(["--report-agent", "--agent", "opencode", "--state", state, "--source", SOURCE]);
}

function release() {
  return run(["--release-agent", "--source", SOURCE]);
}

function stateFromStatus(status) {
  switch (String(status || "").toLowerCase()) {
    case "idle": return "idle";
    case "active":
    case "busy":
    case "pending":
    case "running":
    case "streaming":
    case "working": return "working";
    default: return undefined;
  }
}

export const SeshagyAgentStatePlugin = async () => {
  if (!process.env.TMUX_PANE) return {};
  return {
    "chat.message": async () => report("working"),
    event: async ({ event }) => {
      const type = event?.type;
      const status = stateFromStatus(event?.properties?.status);
      if (status) return report(status);
      switch (type) {
        case "tool.execute.before":
        case "tool.execute.after":
        case "permission.replied":
        case "question.replied":
        case "question.rejected":
        case "session.compacted":
          return report("working");
        case "permission.asked":
        case "question.asked":
        case "session.error":
          return report("blocked");
        case "session.idle":
          return report("done");
        case "session.deleted":
          return release();
        default:
          return undefined;
      }
    },
  };
};
`, installVersion, binaryPath)
}

func shellQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
