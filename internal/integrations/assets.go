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
session_id=""

read_session_id() {
  [ ! -t 0 ] || return 0
  payload="$(cat 2>/dev/null || true)"
  [ -n "$payload" ] || return 0
  command -v python3 >/dev/null 2>&1 || return 0
  session_id="$(printf '%%s' "$payload" | python3 -c 'import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(0)
for key in ("session_id", "sessionId", "conversation_id", "conversationId"):
    value = data.get(key)
    if isinstance(value, str) and value:
        print(value)
        break
' 2>/dev/null || true)"
}

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
  session|start)
    state="unknown"
    read_session_id
    ;;
  idle|working|blocked|done|aborted|unknown) ;;
  release|end|shutdown)
    "$bin" --release-agent --source "$source" >/dev/null 2>&1 || true
    exit 0
    ;;
  *) exit 0 ;;
esac

if [ -n "$message" ] && [ -n "$session_id" ]; then
  "$bin" --report-agent --agent "$agent" --state "$state" --source "$source" --message "$message" --session-id "$session_id" >/dev/null 2>&1 || true
elif [ -n "$message" ]; then
  "$bin" --report-agent --agent "$agent" --state "$state" --source "$source" --message "$message" >/dev/null 2>&1 || true
elif [ -n "$session_id" ]; then
  "$bin" --report-agent --agent "$agent" --state "$state" --source "$source" --session-id "$session_id" >/dev/null 2>&1 || true
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
const idleDebounceMs = parseDurationEnv("SESHAGY_PI_IDLE_DEBOUNCE_MS", 250);
const retryGraceMs = parseDurationEnv("SESHAGY_PI_RETRY_GRACE_MS", 2500);
const retryableErrorPattern = /overloaded|provider.?returned.?error|rate.?limit|too many requests|429|500|502|503|504|service.?unavailable|server.?error|internal.?error|network.?error|connection.?error|connection.?refused|connection.?lost|websocket.?closed|websocket.?error|other side closed|fetch failed|upstream.?connect|reset before headers|socket hang up|ended without|http2 request did not get a response|timed? out|timeout|terminated|retry delay/i;
let reportSeq = Date.now() * 1000;

function nextReportSeq(): string {
  reportSeq += 1;
  return String(reportSeq);
}

function parseDurationEnv(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed < 0) return fallback;
  return parsed;
}

function run(args: string[]) {
  if (!process.env.TMUX_PANE) return;
  const child = spawn(BIN, args, { stdio: "ignore", detached: true });
  child.on("error", () => {});
  child.unref?.();
}

function report(state: string, message?: string) {
  const args = ["--report-agent", "--agent", "pi", "--state", state, "--source", SOURCE, "--seq", nextReportSeq()];
  if (message) args.push("--message", message);
  run(args);
}

function release() {
  run(["--release-agent", "--source", SOURCE, "--seq", nextReportSeq()]);
}

function lastAssistantMessage(messages: unknown[]): any | undefined {
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const message = messages[i] as any;
    if (message?.role === "assistant") return message;
  }
  return undefined;
}

function retryableErrorMessage(event: any): string | undefined {
  const messages = Array.isArray(event?.messages) ? event.messages : [];
  const assistant = lastAssistantMessage(messages);
  if (assistant?.stopReason !== "error") return undefined;
  const errorMessage = String(assistant.errorMessage ?? "");
  if (!retryableErrorPattern.test(errorMessage)) return undefined;
  return errorMessage || "retryable provider error";
}

export default function (pi) {
  if (!process.env.TMUX_PANE) return;
  let agentActive = false;
  let retryHoldActive = false;
  let failureBlocked = false;
  let failureMessage: string | undefined;
  let blockedCount = 0;
  let blockedMessage: string | undefined;
  let lastState: string | undefined;
  let lastMessage: string | undefined;
  let idleTimer: ReturnType<typeof setTimeout> | undefined;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;

  function clearTimer(timer: ReturnType<typeof setTimeout> | undefined) {
    if (timer) clearTimeout(timer);
  }

  function clearPendingTimers() {
    clearTimer(idleTimer);
    clearTimer(retryTimer);
    idleTimer = undefined;
    retryTimer = undefined;
  }

  function clearFailureState() {
    retryHoldActive = false;
    failureBlocked = false;
    failureMessage = undefined;
  }

  function desiredState() {
    if (blockedCount > 0) return { state: "blocked", message: blockedMessage };
    if (failureBlocked) return { state: "blocked", message: failureMessage };
    if (agentActive || retryHoldActive) return { state: "working", message: undefined };
    return { state: "idle", message: undefined };
  }

  function publishState(force = false) {
    const next = desiredState();
    if (!force && next.state === lastState && next.message === lastMessage) return;
    lastState = next.state;
    lastMessage = next.message;
    report(next.state, next.message);
  }

  function scheduleIdle() {
    clearPendingTimers();
    clearFailureState();
    idleTimer = setTimeout(() => {
      idleTimer = undefined;
      publishState();
    }, idleDebounceMs);
    idleTimer.unref?.();
  }

  function holdForRetry(message: string) {
    clearPendingTimers();
    retryHoldActive = true;
    failureBlocked = false;
    failureMessage = message;
    publishState();
    retryTimer = setTimeout(() => {
      retryTimer = undefined;
      retryHoldActive = false;
      failureBlocked = true;
      publishState();
    }, retryGraceMs);
    retryTimer.unref?.();
  }

  pi.on?.("session_start", () => publishState(true));
  pi.on?.("agent_start", () => {
    clearPendingTimers();
    clearFailureState();
    agentActive = true;
    publishState();
  });
  pi.on?.("agent_end", (event) => {
    if (!agentActive) return;
    agentActive = false;
    const retryableMessage = retryableErrorMessage(event);
    if (retryableMessage) {
      holdForRetry(retryableMessage);
      return;
    }
    scheduleIdle();
  });
  pi.events?.on?.("herdr:blocked", (data) => {
    if (data?.active) {
      clearPendingTimers();
      blockedCount += 1;
      blockedMessage = data?.label;
      publishState();
      return;
    }
    blockedCount = Math.max(0, blockedCount - 1);
    if (blockedCount === 0) blockedMessage = undefined;
    publishState();
  });
  pi.on?.("session_shutdown", () => {
    clearPendingTimers();
    release();
  });
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
let reportSeq = Date.now() * 1000;

function nextReportSeq() {
  reportSeq += 1;
  return String(reportSeq);
}

function sessionIDFromProperties(properties) {
  return typeof properties?.sessionID === "string" && properties.sessionID
    ? properties.sessionID
    : undefined;
}

function run(args) {
  if (!process.env.TMUX_PANE) return Promise.resolve();
  return new Promise((resolve) => {
    const child = spawn(BIN, args, { stdio: "ignore", detached: true });
    child.on("error", resolve);
    child.on("close", resolve);
    child.unref?.();
  });
}

function report(state, sessionID) {
  const args = ["--report-agent", "--agent", "opencode", "--state", state, "--source", SOURCE, "--seq", nextReportSeq()];
  if (sessionID) args.push("--session-id", sessionID);
  return run(args);
}

function release() {
  return run(["--release-agent", "--source", SOURCE, "--seq", nextReportSeq()]);
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
    "chat.message": async ({ event } = {}) => report("working", sessionIDFromProperties(event?.properties)),
    event: async ({ event }) => {
      const type = event?.type;
      const sessionID = sessionIDFromProperties(event?.properties);
      const status = stateFromStatus(event?.properties?.status);
      if (status) return report(status, sessionID);
      switch (type) {
        case "session.created":
        case "session.updated":
          return sessionID ? report("idle", sessionID) : undefined;
        case "tool.execute.before":
        case "tool.execute.after":
        case "permission.replied":
        case "question.replied":
        case "question.rejected":
        case "session.compacted":
          return report("working", sessionID);
        case "permission.asked":
        case "question.asked":
        case "session.error":
          return report("blocked", sessionID);
        case "session.idle":
          return report("idle", sessionID);
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
