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

		// session.idle → done ONLY when no permission prompt is pending. If a
		// permission is pending, the idle is because the session is waiting on
		// the user's approval — leave the state on blocked.
		//
		// permission.replied → the user answered; the turn resumes → working.
		event: async ({ event }) => {
			if (event.type === "permission.replied") {
				permissionPending = false;
				report(cwd, "working");
				return;
			}
			if (event.type === "session.idle") {
				if (permissionPending) {
					return;
				}
				report(cwd, "done");
			}
		},

		// Plugin/server shutdown → release.
		dispose: async () => {
			permissionPending = false;
			release(cwd);
		},
	};
};
