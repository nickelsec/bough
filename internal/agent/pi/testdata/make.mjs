// Writes the Pi fixtures in this directory using Pi's own code.
//
// Hand-written fixtures are how a reader ends up agreeing with itself rather
// than with the agent: the fixture holds whatever the author believed the
// format was. These are written by Pi's SessionManager, so the tree, the ids,
// the header and every entry type are exactly what Pi writes. Edit patches come
// from Pi's own diff code and each reply's cost from Pi's own pricing, so the
// tests can hold bough's figures against Pi's.
//
// Run it with the path to a Pi install, then commit what it writes:
//
//	node make.mjs ~/.pi/agent/install/releases/<version>/node_modules/@earendil-works
//
// It makes no model calls and touches nothing outside this directory. The git
// output in the bash results was captured from real git.

import { mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const pkgs = process.argv[2];
if (!pkgs) {
	console.error("usage: node make.mjs <path to node_modules/@earendil-works>");
	process.exit(2);
}
const load = (p) => import(pathToFileURL(join(pkgs, p)).href);
const { SessionManager } = await load("pi-coding-agent/dist/core/session-manager.js");
const { generateUnifiedPatch } = await load("pi-coding-agent/dist/core/tools/edit-diff.js");
const { calculateCost } = await load("pi-ai/dist/models.js");

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "sessions");
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

// The project the sessions ran in. A Windows path, since that is where the
// awkward cases live: drive letters, Git Bash paths, backslashes.
const cwd = "C:\\work\\app";

// Pi's catalog, for pricing each reply the way Pi does.
const catalog = {};
for (const f of readdirSync(join(pkgs, "pi-ai/dist/providers/data"))) {
	if (!f.endsWith(".json")) continue;
	const data = JSON.parse(readFileSync(join(pkgs, "pi-ai/dist/providers/data", f), "utf8"));
	for (const api of Object.values(data)) {
		if (typeof api !== "object") continue;
		for (const m of Object.values(api)) catalog[`${m.provider}/${m.id}`] = m;
	}
}

let clock = Date.parse("2026-09-20T10:00:00Z");
const tick = (s = 30) => (clock += s * 1000);

function usage(provider, model, input, output, cacheRead = 0, cacheWrite = 0, cacheWrite1h) {
	const u = { input, output, cacheRead, cacheWrite, totalTokens: input + output + cacheRead + cacheWrite,
		cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } };
	if (cacheWrite1h !== undefined) u.cacheWrite1h = cacheWrite1h;
	const m = catalog[`${provider}/${model}`];
	if (!m) throw new Error(`no ${provider}/${model} in Pi's catalog`);
	calculateCost(m, u);
	return u;
}

// summed is the extension's own running total for a sub-agent, which carries
// its cost as a bare number rather than the object a reply has. See
// examples/extensions/subagent/index.ts.
const summed = (u) => ({ input: u.input, output: u.output, cacheRead: u.cacheRead, cacheWrite: u.cacheWrite,
	cost: u.cost.total, contextTokens: u.totalTokens, turns: 1 });

const api = { anthropic: "anthropic-messages", "openai-codex": "openai-codex-responses", openrouter: "openai-completions" };

function reply(provider, model, content, u, stopReason = "toolUse", extra = {}) {
	return { role: "assistant", content, api: api[provider], provider, model, usage: u, stopReason, timestamp: tick(), ...extra };
}
const call = (id, name, args) => ({ type: "toolCall", id, name, arguments: args });
const result = (id, name, text, isError = false, details) => ({
	role: "toolResult", toolCallId: id, toolName: name, content: [{ type: "text", text }],
	...(details === undefined ? {} : { details }), isError, timestamp: tick(5),
});
const user = (content) => ({ role: "user", content, timestamp: tick(60) });

// ---------------------------------------------------------------------------
// main: one session covering every entry type, both branches of a /tree, a
// compaction, a skill, a sub-agent, and three providers.

const sm = SessionManager.create(cwd, out);
sm.appendModelChange("openai-codex", "gpt-5.6-luna");
sm.appendThinkingLevelChange("medium");
sm.appendMessage({ role: "system", content: "", sections: { preamble: "You are pi." }, toolsAdded: [], timestamp: tick() });

// Turn 1: write, edit, a commit that lands, a commit that fails.
const first = sm.appendMessage(user([{ type: "text", text: "add a readme and commit it" }]));
sm.appendMessage(reply("openai-codex", "gpt-5.6-luna",
	[{ type: "thinking", thinking: "plan" }, call("c1", "write", { path: "README.md", content: "# app\nhello\n" })],
	usage("openai-codex", "gpt-5.6-luna", 1000, 50, 0)));
sm.appendMessage(result("c1", "write", "Successfully wrote to README.md"));
const before = "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n";
const after = "package main\n\nfunc main() {\n\tprintln(\"hello\")\n\tprintln(\"there\")\n}\n";
sm.appendMessage(reply("openai-codex", "gpt-5.6-luna",
	[call("c2", "edit", { path: "@src/main.go", edits: [{ oldText: "\tprintln(\"hi\")\n", newText: "\tprintln(\"hello\")\n\tprintln(\"there\")\n" }] })],
	usage("openai-codex", "gpt-5.6-luna", 200, 40, 1000)));
sm.appendMessage(result("c2", "edit", "Successfully replaced 1 block(s) in src/main.go.", false,
	{ diff: "", patch: generateUnifiedPatch("src/main.go", before, after), firstChangedLine: 4 }));
sm.appendMessage(reply("openai-codex", "gpt-5.6-luna",
	[call("c3", "bash", { command: "git add -A && git commit -m 'add readme'" })],
	usage("openai-codex", "gpt-5.6-luna", 100, 30, 1200)));
sm.appendMessage(result("c3", "bash",
	"[main (root-commit) 85fd4d9] add readme\n 2 files changed, 7 insertions(+)\n create mode 100644 README.md"));
sm.appendMessage(reply("openai-codex", "gpt-5.6-luna",
	[call("c4", "bash", { command: "git commit -m 'nothing here'" })],
	usage("openai-codex", "gpt-5.6-luna", 80, 20, 1300)));
sm.appendMessage(result("c4", "bash",
	"On branch main\nnothing to commit, working tree clean\n\nCommand exited with code 1", true));
sm.appendMessage(reply("openai-codex", "gpt-5.6-luna", [{ type: "text", text: "Done." }],
	usage("openai-codex", "gpt-5.6-luna", 60, 10, 1400), "stop"));

// Turn 2: a skill, a model switch to Anthropic with an hourly cache, a failed
// request, cache warming, and a commit the person typed with "!".
sm.appendModelChange("anthropic", "claude-opus-4-8");
sm.appendMessage(user([{ type: "text", text:
	"<skill name=\"review\" location=\"C:\\\\Users\\\\u\\\\.pi\\\\agent\\\\skills\\\\review\\\\SKILL.md\">\n" +
	"References are relative to C:\\Users\\u\\.pi\\agent\\skills\\review.\n\nReview the code carefully.\n</skill>\n\nsrc/main.go please" }]));
sm.appendMessage(reply("anthropic", "claude-opus-4-8", [],
	usage("anthropic", "claude-opus-4-8", 0, 0), "error", { errorMessage: "429 rate limited" }));
sm.appendUsage("cache_warm", "anthropic", "claude-opus-4-8", usage("anthropic", "claude-opus-4-8", 0, 0, 5000));
sm.appendMessage(reply("anthropic", "claude-opus-4-8",
	[call("c5", "read", { path: "src/main.go" }), call("c6", "grep", { pattern: "println", path: "/c/work/app/src" })],
	usage("anthropic", "claude-opus-4-8", 10, 300, 2000, 4000, 4000)));
sm.appendMessage(result("c5", "read", before));
sm.appendMessage(result("c6", "grep", "src/main.go:4: println"));
sm.appendMessage(reply("anthropic", "claude-opus-4-8", [{ type: "text", text: "Looks fine." }],
	usage("anthropic", "claude-opus-4-8", 5, 100, 6000), "stop"));
sm.appendMessage({ role: "bashExecution", command: "git commit --amend -m 'add readme, amended'",
	output: "[main cd7d300] add readme, amended\n Date: Sun Sep 20 10:30:00 2026 +0000\n 2 files changed, 7 insertions(+)",
	exitCode: 0, cancelled: false, truncated: false, timestamp: tick() });
sm.appendMessage({ role: "bashExecution", command: "git commit -m 'typed but failed'",
	output: "nothing to commit", exitCode: 1, cancelled: false, truncated: false, timestamp: tick() });

// Extension entries that are not prompts.
sm.appendCustomEntry("my-extension", { count: 1 });
sm.appendCustomMessageEntry("my-extension", "Injected context, not typed by anyone.", true);
sm.appendLabelChange(first, "start");
sm.appendSessionInfo("Readme work");

// Compaction, charged to the model in use.
sm.appendCompaction("Summary of the readme work.", first, 9000, { readFiles: ["src/main.go"], modifiedFiles: ["README.md"] },
	false, usage("anthropic", "claude-opus-4-8", 9000, 400));

// Turn 3: a free OpenRouter model, and a picture with nothing typed.
sm.appendModelChange("openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free");
const third = sm.appendMessage(user([{ type: "image", data: "iVBORw0KGgo=", mimeType: "image/png" }]));
sm.appendMessage(reply("openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free",
	[call("c7", "ls", { path: "~/notes" }), call("c8", "find", { pattern: "*.go" })],
	usage("openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free", 700, 60, 100),
	"toolUse", { responseModel: "nvidia/nemotron-3-ultra-550b-a55b" }));
sm.appendMessage(result("c7", "ls", "a.txt"));
sm.appendMessage(result("c8", "find", "src/main.go"));

// Turn 4: a sub-agent, whose work lives inside the result.
sm.appendModelChange("openai-codex", "gpt-5.6-sol");
sm.appendMessage(user("delegate a test run"));
sm.appendMessage(reply("openai-codex", "gpt-5.6-sol",
	[call("c9", "subagent", { tasks: [{ agent: "tester", task: "run the tests" }, { agent: "writer", task: "write docs" }] })],
	usage("openai-codex", "gpt-5.6-sol", 400, 80, 2000)));
sm.appendMessage({ ...result("c9", "subagent", "tester: ok\nwriter: ok", false, {
	mode: "parallel", agentScope: "user", projectAgentsDir: null, results: [
		{ agent: "tester", agentSource: "user", task: "run the tests", exitCode: 0, stderr: "", model: "gpt-5.6-luna",
			usage: summed(usage("openai-codex", "gpt-5.6-luna", 300, 40, 0)),
			messages: [
				user("run the tests"),
				reply("openai-codex", "gpt-5.6-luna", [call("s1", "bash", { command: "go test ./..." })],
					usage("openai-codex", "gpt-5.6-luna", 300, 40, 0)),
				result("s1", "bash", "FAIL\n\nCommand exited with code 1", true),
			] },
		{ agent: "writer", agentSource: "user", task: "write docs", exitCode: 0, stderr: "", model: "gpt-5.6-luna",
			usage: summed(usage("openai-codex", "gpt-5.6-luna", 150, 90, 0)),
			messages: [] },
	] }), usage: usage("openai-codex", "gpt-5.6-sol", 50, 5, 0) });
sm.appendMessage(reply("openai-codex", "gpt-5.6-sol", [{ type: "text", text: "Delegated." }],
	usage("openai-codex", "gpt-5.6-sol", 100, 20, 2400), "stop"));

// Turn 5: back up the tree to the third prompt and try again from there, the
// way /tree does, with the abandoned branch summarised.
sm.branchWithSummary(third, "Tried delegating; went back.", { readFiles: [], modifiedFiles: [] }, false,
	usage("openai-codex", "gpt-5.6-sol", 500, 50));
sm.appendMessage(user("try another way"));
sm.appendMessage(reply("openai-codex", "gpt-5.6-sol",
	[call("c10", "edit", { path: "C:\\work\\app\\README.md", oldText: "hello", newText: "hello\nworld" })],
	usage("openai-codex", "gpt-5.6-sol", 300, 30, 2600)));
sm.appendMessage(result("c10", "edit", "Successfully replaced text in README.md."));
sm.appendSessionInfo("Readme and more");
const mainFile = sm.getSessionFile();

// ---------------------------------------------------------------------------
// fork: /fork copies the whole file and carries on. Only what comes after the
// copy is this session's own work.

const fork = SessionManager.forkFrom(mainFile, cwd, out);
fork.appendMessage(user("after the fork"));
fork.appendMessage(reply("openai-codex", "gpt-5.6-sol", [{ type: "text", text: "ok" }],
	usage("openai-codex", "gpt-5.6-sol", 111, 11, 0), "stop"));

// clone: /clone writes the path to the current leaf as a new file.
const clone = SessionManager.open(mainFile, out);
const cloneFile = clone.createBranchedSession(clone.getLeafId());
const cloned = SessionManager.open(cloneFile, out);
cloned.appendMessage(user("after the clone"));
cloned.appendMessage(reply("openai-codex", "gpt-5.6-sol", [{ type: "text", text: "ok" }],
	usage("openai-codex", "gpt-5.6-sol", 222, 22, 0), "stop"));

// Stable names, so the tests can find each file by what it is.
const rename = (from, to) => renameSync(from, join(dirname(from), to));
rename(mainFile, "main.jsonl");
rename(fork.getSessionFile(), "fork.jsonl");
rename(cloneFile, "clone.jsonl");
// The fork and the clone point at the main session by the path it had when
// they were made. Point them at where it would have been on the machine that
// wrote it, which is not anywhere on the machine reading it: that is the usual
// case when history is read from a copy, and the reader has to cope.
for (const f of ["fork.jsonl", "clone.jsonl"]) {
	const p = join(dirname(mainFile), f);
	const lines = readFileSync(p, "utf8").split("\n");
	const h = JSON.parse(lines[0]);
	h.parentSession = "C:\\Users\\someone\\.pi\\agent\\sessions\\--C--work-app--\\main.jsonl";
	lines[0] = JSON.stringify(h);
	writeFileSync(p, lines.join("\n"));
}
console.log("wrote", readdirSync(dirname(mainFile)).map((f) => join(dirname(mainFile), f)).join("\n      "));
