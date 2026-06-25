// installed by seshagy
// managed by seshagy; reinstalling or updating the integration overwrites this file.
// SESHAGY_INTEGRATION_ID=pi

import { spawn } from "node:child_process";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const BIN = "seshagy";
const SOURCE = "seshagy:pi";

function seq(): string {
	return String(Date.now());
}

function run(args: string[]) {
	if (!process.env.TMUX_PANE) return;
	const child = spawn(BIN, args, { stdio: "ignore", detached: true });
	child.on("error", () => {});
	child.unref?.();
}

function report(state: string, message?: string) {
	const args = [
		"--report-agent",
		"--pane", process.env.TMUX_PANE!,
		"--agent", "pi",
		"--state", state,
		"--source", SOURCE,
		"--seq", seq(),
	];
	if (message) args.push("--message", message);
	run(args);
}

function release() {
	run([
		"--release-agent",
		"--pane", process.env.TMUX_PANE!,
		"--source", SOURCE,
		"--seq", seq(),
	]);
}

export default function seshagyAgentState(pi: ExtensionAPI): void {
	if (!process.env.TMUX_PANE) return;

	pi.on("session_start", () => report("idle"));

	pi.on("before_agent_start", () => report("working"));

	pi.on("agent_start", () => report("working"));

	pi.on("turn_start", () => report("working"));

	pi.on("tool_call", () => report("working"));

	pi.on("turn_end", () => report("working"));

	pi.on("agent_end", () => report("done"));

	pi.on("session_shutdown", () => release());
}
