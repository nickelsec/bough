# The Pi session file format

Notes from reading Pi's session files, the same way as [the Claude Code
transcripts](/docs/format/claude-code) and [the Codex rollouts](/docs/format/codex).

Pi is open source, so unlike the other two this page was written against the
code that writes the files: `packages/coding-agent/src/core/session-manager.ts`
and the message types in `packages/ai/src/types.ts`, as of Pi 0.87.1 in
September 2026. The real corpus behind it is small, a handful of sessions on one
machine across three providers, so where the code and a real file could
disagree, trust your own files. Pi documents the format itself in
`docs/session-format.md` inside its package, and that is worth reading first.

## What will catch you out

**A session is a tree, not a list.** Every entry has an `id` and a `parentId`.
Going back to an earlier point with `/tree` and carrying on adds a second child
to that point, in the same file. Pi builds what the model sees by walking back
from the newest entry, so the abandoned branch drops out of its context. It does
not drop out of the history: those prompts were typed, answered and paid for.
Read every entry in file order, and treat an entry whose parent is not the one
before it as a jump in the tree. A file can also have more than one root.

**Forks copy the whole conversation into a new file.** `/fork` and `/clone`
start a new session file holding the conversation so far, every entry under its
original `id` and `timestamp`, and the new header's `parentSession` names the
file it came from. `/fork` copies everything, abandoned branches included;
`/clone` copies only the path to where you are. Add up both files as they stand
and everything before the fork is counted twice. Match copied entries by `id`
and `timestamp` together, since ids are only eight hex characters and unique
within one file. A fork can also land in a different project's folder. If the
original has been deleted, the copy is the only record left, and then it counts.

**A skill prompt is stored with the skill's text pasted in.** `/skill:review
this file` is saved as a user message that opens
`<skill name="review" location="...">`, holds the skill's whole body, and ends
with the words that followed the command. Show that as the prompt and a page of
instructions nobody typed becomes the label on the work.

**Sub-agents are an extension, and Pi leaves their cost out of its totals.** Pi
has no sub-agents of its own. The example `subagent` extension runs each one as
a separate process that saves no session. What it did survives only inside the
tool result: `details.results[]` carries every message the sub-agent exchanged,
with each reply's usage. Pi does not add that to the session's usage. Count it
yourself or a delegated task looks free.

**The answering model is not always the billed one.** A reply records the
`model` that was asked for and, when the provider answered as something else,
a `responseModel`. OpenRouter reports its models without the `:free` suffix, so
pricing by `responseModel` bills free work at the paid rate. Pi prices by the
requested model, except when Anthropic falls back to another model, where the
fallback is what did the work.

**A failed request is still a reply.** An error from the provider is saved as an
assistant message with `stopReason: "error"`, an `errorMessage`, empty content
and all-zero usage. It opens nothing and costs nothing, but it is there.

## Where the files live

```
~/.pi/agent/sessions/--<cwd>--/<timestamp>_<session id>.jsonl
```

The folder name is the working directory with its leading separator removed and
every `/`, `\` and `:` turned into `-`, so `D:\pi-test` becomes
`--D--pi-test--`. That cannot be turned back into the path, since a `-` in the
name could have been any of the four. Read `cwd` from the header instead.

`PI_CODING_AGENT_DIR` moves the whole agent directory, and
`PI_CODING_AGENT_SESSION_DIR` moves just the sessions. A session directory set
by hand holds every project's files side by side, with no per-project folders.

## The header

The first line of every file:

```json
{"type":"session","version":3,"id":"<uuid>","timestamp":"2026-09-24T18:34:41.984Z","cwd":"C:\\Users\\you"}
```

Version 1 files have no `version` and no `id` or `parentId` on their entries,
which then simply run in file order. Pi migrates old files when it opens them.
Claude Code records carry a `cwd` too, so `type` is what says a file is Pi's.

## Entry types

```
message                a conversation message, with a role (below)
model_change           the model switched: provider, modelId
thinking_level_change  the reasoning level switched
usage                  model work that is not a reply, cache warming for one
compaction             earlier context summarised, with the summary's usage
branch_summary         an abandoned branch summarised, after a /tree jump
context_edit           a later change to what the model sees, not to history
custom                 an extension's own state
custom_message         an extension's message to the model, not a prompt
label                  a bookmark on an entry
session_info           the name given with /name, latest wins
```

Messages have a role: `user`, `assistant`, `toolResult`, `system`,
`bashExecution` for a command you ran yourself with `!`, and `custom`,
`branchSummary` and `compactionSummary` for things Pi or an extension put there.

Entry timestamps are ISO strings. The timestamp inside a message is Unix
milliseconds.

## Tokens

Every assistant message carries usage:

```json
{"input":6108,"output":68,"cacheRead":0,"cacheWrite":0,"reasoning":21,"totalTokens":6176,"cost":{"input":0.0012216,"output":0.0000816,"cacheRead":0,"cacheWrite":0,"total":0.0013032}}
```

Unlike Codex, `input` does not include `cacheRead`, so the four counts can be
added as they are. `reasoning` is already inside `output`; do not add it again.
`cacheWrite1h`, when present, is the part of `cacheWrite` held for an hour.

Usage also turns up on `usage` entries, on `compaction` and `branch_summary`
entries for the summary they paid for, and on a tool result that did model work
of its own. Those last three name no model. The one in use is the honest guess.

Pi works out a cost for every piece of usage from its own price catalog, which
is models.dev plus its own corrections. bough prices from LiteLLM's table
instead and uses Pi's figure only in its tests, where the two agree to the cent
on a real session. Subscription and custom providers record a cost of zero.

## Tools

The built-in tools and the arguments that matter:

```
read    path
write   path, content
edit    path, edits: [{oldText, newText}]  (older files: oldText, newText)
bash    command
grep    pattern, path
find    pattern, path
ls      path
```

On Windows a `powershell` tool can stand in for `bash`, with the same
`command`. A command that exits non-zero comes back as a result with
`isError: true`, which is how a commit that failed is told from one that
landed. The git output itself, `[main (root-commit) 85fd4d9] ...`, is in the
result's text.

A path can be relative to `cwd`, start with `@`, start with `~`, be a
`file://` URL, or on Windows be a Git Bash path such as `/d/work`. Pi resolves
all of those before touching the disk, so a reader has to as well.

An `edit` result carries `details.patch`, a unified diff with `---` and `+++`
headers, which is the reliable way to count the lines that changed.

## Local models

A model run on your own machine appears under whatever provider name was given
to it in Pi's `models.json`, for example `ollama`, with a `baseUrl` such as
`http://localhost:11434/v1`. The session records only the provider and model,
not where the server was, so `models.json` is the one place that says a model
was local. Pi's built-in `llama.cpp` provider is always local. A local endpoint
is not proof on its own: a proxy on localhost that forwards to a hosted model
looks exactly the same.
