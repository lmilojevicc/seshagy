// installed by seshagy
// managed by seshagy; reinstalling or updating the integration overwrites this file.
// SESHAGY_INTEGRATION_ID=opencode

import { spawn } from "node:child_process";
import type { Plugin } from "@opencode-ai/plugin";

// Override the seshagy binary path via $SESHAGY_BIN; otherwise fall back to the
// one on PATH. Set SESHAGY_BIN to your worktree build during development.
const BIN = process.env.SESHAGY_BIN || "seshagy";
const SOURCE = "seshagy:opencode";

// Monotonic sequence number generated in-process (no python3 fork). Uses
// BigInt microseconds — the shared unit across all producers (shell hook uses
// time.time_ns()//1000). BigInt avoids the JS Number.MAX_SAFE_INTEGER overflow
// that the prior Date.now()*1e6 implementation had (~1.7e18 > 9e15).
// Note: opencode cannot easily read the pane's existing seq (it runs in the
// opencode server process, not the tmux pane). The shell hook bridges to this
// high-water mark; the reverse (opencode after shell) works because µs
// wall-clock naturally exceeds prior µs values.
let seqCounter = 0n;
function seq(): string {
	const ts = BigInt(Date.now()) * 1000n; // microseconds
	if (ts <= seqCounter) {
		seqCounter += 1n;
		return seqCounter.toString();
	}
	seqCounter = ts;
	return seqCounter.toString();
}

function run(args: string[]) {
	if (process.env.HERDR_ENV === "1") return; // herdr owns agent state
	const child = spawn(BIN, args, { stdio: "ignore", detached: true });
	child.on("error", () => {});
	child.unref?.();
}

// Fast path: use $TMUX_PANE when present (common single-pane case). Otherwise
// resolve by cwd — the plugin runs in the opencode server process where
// $TMUX_PANE may be absent or stale, so input.directory (project cwd) is the
// reliable pane mapping signal.
function paneArgs(cwd: string): string[] {
	return process.env.TMUX_PANE
		? ["--pane", process.env.TMUX_PANE]
		: ["--cwd", cwd];
}

function report(cwd: string, state: string) {
	run([
		"--report-agent",
		...paneArgs(cwd),
		"--agent", "opencode",
		"--state", state,
		"--source", SOURCE,
		"--seq", seq(),
	]);
}

function release(cwd: string) {
	run([
		"--release-agent",
		...paneArgs(cwd),
		"--source", SOURCE,
		"--seq", seq(),
	]);
}

// permissionPending guards against session.idle overwriting a pending blocked
// state. The opencode SDK fires session.idle while a permission prompt is still
// displayed (the session is idle because it's waiting on the user's approval,
// not because the turn finished). Without this guard, the later `done` report
// (higher seq) would overwrite the `blocked` report from permission.ask.
let permissionPending = false;

export const SeshagyPlugin: Plugin = async (input) => {
	const cwd = input.directory;
	return {
		// Turn start / tool execution → working. Either of these means the turn
		// resumed, so any prior permission prompt is no longer pending.
		"chat.message": async () => {
			permissionPending = false;
			report(cwd, "working");
		},
		"tool.execute.before": async () => {
			permissionPending = false;
			report(cwd, "working");
		},

		// Permission prompt → blocked. status "ask" means the TUI is showing an
		// approval prompt; "allow"/"deny" are the plugin's own decision and
		// indicate the turn is resuming (not blocked).
		"permission.ask": async (_input, output) => {
			if (output.status === "ask") {
				permissionPending = true;
				report(cwd, "blocked");
			} else {
				permissionPending = false;
				report(cwd, "working");
			}
		},

		// Event-bus state mapping (mirrors herdr's opencode plugin). The event
		// hook receives ALL published events, including question/elicitation
		// events that have no dedicated Hooks key. This is the ONLY path that
		// detects `question.asked` (user elicitation) and `permission.asked`
		// (event-bus version distinct from the permission.ask Hook above).
		event: async ({ event }) => {
			const type = event?.type;
			switch (type) {
				// Blocked: agent is asking for permission or asking the user a
				// question (elicitation). Set permissionPending so session.idle
				// doesn't overwrite blocked while the prompt is displayed.
				case "permission.asked":
				case "question.asked":
				case "session.error":
					permissionPending = true;
					report(cwd, "blocked");
					break;

				// Working: turn resumed after a reply/rejection (or compaction).
				case "tool.execute.before":
				case "tool.execute.after":
				case "permission.replied":
				case "question.replied":
				case "question.rejected":
				case "session.compacted":
					permissionPending = false;
					report(cwd, "working");
					break;

				// Session idle → done (only if no permission/question pending).
				case "session.idle":
					if (!permissionPending) {
						report(cwd, "done");
					}
					break;

				// session.status carries a status string for belt-and-suspenders
				// coverage alongside the named events above.
				case "session.status": {
					const status = (event?.properties?.status ?? "").toLowerCase();
					switch (status) {
						case "idle":
							if (!permissionPending) {
								report(cwd, "idle");
							}
							break;
						case "active":
						case "busy":
						case "pending":
						case "running":
						case "streaming":
						case "working":
							permissionPending = false;
							report(cwd, "working");
							break;
					}
					break;
				}

				default:
					break;
			}
		},

		// Plugin/server shutdown → release.
		dispose: async () => {
			permissionPending = false;
			release(cwd);
		},
	};
};
