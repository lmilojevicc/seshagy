// installed by seshagy
// managed by seshagy; reinstalling or updating the integration overwrites this file.
// SESHAGY_INTEGRATION_ID=opencode

import { spawn } from "node:child_process";
import type { Plugin } from "@opencode-ai/plugin";

// Override the seshagy binary path via $SESHAGY_BIN; otherwise fall back to the
// one on PATH. Set SESHAGY_BIN to your worktree build during development.
const BIN = process.env.SESHAGY_BIN || "seshagy";
const SOURCE = "seshagy:opencode";

// Monotonic sequence number generated in-process (no python3 fork). Wall-clock
// nanoseconds are sufficient for sequence ordering within a single session.
let seqCounter = 0;
function seq(): string {
	const ts = Date.now() * 1_000_000;
	if (ts <= seqCounter) {
		seqCounter += 1;
		return String(seqCounter);
	}
	seqCounter = ts;
	return String(seqCounter);
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

export const SeshagyPlugin: Plugin = async (input) => {
	const cwd = input.directory;
	return {
		// Turn start / tool execution → working.
		"chat.message": async () => report(cwd, "working"),
		"tool.execute.before": async () => report(cwd, "working"),

		// Permission prompt → blocked. status "ask" means the TUI is showing an
		// approval prompt; "allow"/"deny" are the plugin's own decision and do
		// not indicate a blocked pane.
		"permission.ask": async (_input, output) => {
			if (output.status === "ask") report(cwd, "blocked");
		},

		// session.idle → done (turn finished, pane not yet visited).
		event: async ({ event }) => {
			if (event.type === "session.idle") report(cwd, "done");
		},

		// Plugin/server shutdown → release.
		dispose: async () => release(cwd),
	};
};
