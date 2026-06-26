#!/bin/sh
# installed by seshagy
# managed by seshagy; reinstalling or updating the integration overwrites this file.
# SESHAGY_INTEGRATION_ID=shell-hook

# A state-reporting hook must NEVER fail the host agent. No set -e/-u: every
# command is explicitly guarded with || true or checked. The script always
# exits 0 (except when invoked via the codex PermissionRequest command string
# which appends '; exit 1' OUTSIDE this script).
agent="${1:-}"
state="${2:-idle}"
message="${3:-}"
session_id=""

next_seq() {
  counter_file="${TMPDIR:-/tmp}/seshagy-seq-${TMUX_PANE:-$$}"
  # Persistent per-pane counter is the PRIMARY source: it survives process
  # and agent restarts in the same pane so seq stays strictly increasing above
  # any tombstone left by a prior run. Pane ids are never reused within a tmux
  # server, so keying by TMUX_PANE alone is collision-free across agents.
  # NOTE: the read-modify-write below is not atomic; two concurrent hook fires
  # in the same pane may both read the same prev and collide. Benign: the
  # resulting duplicate seq is rejected by seshagy's strict-`>` guard.
  if [ -f "$counter_file" ]; then
    prev="$(cat "$counter_file" 2>/dev/null || true)"
    if [ -n "$prev" ] && [ "$prev" -ge 0 ] 2>/dev/null; then
      n=$((prev + 1))
      if printf '%s\n' "$n" > "$counter_file" 2>/dev/null; then
        echo "$n"
        return 0
      fi
    fi
  fi
  # Seed the counter from microsecond wall-clock (best available) so the first
  # value clears any tombstone; subsequent calls increment from the file.
  # Microseconds are the shared unit across all producers (shell, python, JS/TS):
  # they stay within JS Number.MAX_SAFE_INTEGER so a JS/TS producer taking over a
  # pane previously driven by this shell hook keeps incrementing correctly.
  if command -v python3 >/dev/null 2>&1; then
    base="$(python3 -c 'import time; print(time.time_ns() // 1000)' 2>/dev/null)" || base=""
  elif command -v perl >/dev/null 2>&1; then
    base="$(perl -MTime::HiRes=time -e 'print int(time()*1e6)' 2>/dev/null)" || base=""
  elif command -v date >/dev/null 2>&1; then
    # BSD date lacks sub-second precision; seconds * 1e6 seeds the counter only.
    base="$(date +%s 2>/dev/null)000000" || base=""
  else
    base=""
  fi
  # No wall-clock tool available: cannot maintain a counter, fall back to 0.
  if [ -z "$base" ]; then
    echo 0
    return 0
  fi
  # Bridge: if the pane already has a higher seq (e.g. left by a different
  # producer like the opencode plugin, or a prior agent with clock skew), seed
  # the counter above it so new reports aren't rejected as stale by seshagy's
  # strict-> guard. Best-effort: on error, fall back to wall-clock base.
  if [ -n "${TMUX_PANE:-}" ]; then
    existing_seq="$(tmux show-option -qvpt "$TMUX_PANE" @seshagy_agent_seq 2>/dev/null || true)"
    if [ -n "$existing_seq" ] && [ "$existing_seq" -ge 0 ] 2>/dev/null; then
      if [ "$((existing_seq + 1))" -gt "$base" ]; then
        base="$((existing_seq + 1))"
      fi
    fi
  fi
  printf '%s\n' "$((base + 1))" > "$counter_file" 2>/dev/null || true
  echo "$((base + 1))"
}

read_session_id() {
  [ ! -t 0 ] || return 0
  payload="$(cat 2>/dev/null || true)"
  [ -n "$payload" ] || return 0
  command -v python3 >/dev/null 2>&1 || return 0
  session_id="$(printf '%s' "$payload" | python3 -c 'import json, sys
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

reject_unsafe_value() {
  case "$1" in
    *[\"\'\\$]*) return 1 ;;
    *'
'*) return 1 ;;
  esac
  return 0
}

[ -n "${TMUX_PANE:-}" ] || exit 0
# Resolve the seshagy binary: $SESHAGY_BIN (if set + executable), else PATH.
bin=''
if [ -n "${SESHAGY_BIN:-}" ] && [ -x "${SESHAGY_BIN:-}" ]; then
  bin="$SESHAGY_BIN"
fi
if [ -z "$bin" ]; then
  bin="$(command -v seshagy 2>/dev/null || true)"
fi
[ -n "$bin" ] || exit 0

source="seshagy:$agent"
seq="$(next_seq)"
case "$state" in
  session|start)
    state="unknown"
    read_session_id
    ;;
  idle|working|blocked|done|aborted|interrupted|unknown) ;;
  release|end|shutdown)
    "$bin" --release-agent --pane "$TMUX_PANE" --source "$source" --seq "$seq" >/dev/null 2>&1 || true
    exit 0
    ;;
  *) exit 0 ;;
esac

# Sanitize message/session_id: if they contain shell-injection characters,
# drop the unsafe field rather than failing the hook. State reporting is
# best-effort and must never break the host agent.
if [ -n "$message" ] && ! reject_unsafe_value "$message"; then
  message=""
fi
if [ -n "$session_id" ] && ! reject_unsafe_value "$session_id"; then
  session_id=""
fi

if [ -n "$message" ] && [ -n "$session_id" ]; then
  "$bin" --report-agent --pane "$TMUX_PANE" --agent "$agent" --state "$state" --source "$source" --message "$message" --session-id "$session_id" --seq "$seq" >/dev/null 2>&1 || true
elif [ -n "$message" ]; then
  "$bin" --report-agent --pane "$TMUX_PANE" --agent "$agent" --state "$state" --source "$source" --message "$message" --seq "$seq" >/dev/null 2>&1 || true
elif [ -n "$session_id" ]; then
  "$bin" --report-agent --pane "$TMUX_PANE" --agent "$agent" --state "$state" --source "$source" --session-id "$session_id" --seq "$seq" >/dev/null 2>&1 || true
else
  "$bin" --report-agent --pane "$TMUX_PANE" --agent "$agent" --state "$state" --source "$source" --seq "$seq" >/dev/null 2>&1 || true
fi

exit 0
