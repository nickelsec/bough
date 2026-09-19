# The Codex CLI rollout file format

Notes from reading real Codex CLI rollout files, the same way as [the Claude
Code transcripts](/docs/format/claude-code) and worth their own page.

Be warned that the corpus behind this page is much smaller than the Claude one:
3 rollout files, 242 lines, 7.4 MB, from Codex Desktop 0.153.4 on one machine in
September 2026. The shapes described here were each traced to code that was
getting them wrong, so they are real, but the counts prove far less than the
Claude figures do.

The format also moved recently and visibly. A parser written against an earlier
Codex CLI read this corpus with three separate faults, described below. Treat
anything here as true of 0.153.x and check it against your own files.

## What will catch you out

**Replay spans files, not lines.** Claude Code replays inside one transcript;
Codex writes a whole new rollout and replays the earlier items into it under the
ids they already had. Deduplicate within a file and you catch none of it, since
the first copy is in a different file. Group rollouts by session before you
deduplicate, or a resumed session comes back as several sessions with its early
prompts counted once per resume.

**A sub-agent's rollout carries its parent's session id.** Spawning an agent
writes a rollout whose `session_meta.session_id` is the parent's, while its own
`id` differs and `parent_thread_id` names the parent. Group on `session_id`
alone and the sub-agent's prompts and tokens vanish into the agent that spawned
it. Group on `id` alone and a resumed session splits. Both fields are needed:
`parent_thread_id` is what tells the two cases apart.

**Usage is reported twice.** Newer rollouts carry a `token_usage_record` line
per response and an `event_msg` of type `token_count` saying the same thing.
Add both and every token figure in your output doubles. Prefer the record: it
carries a `response_id`, which is what lets a replayed response be recognised as
one already counted, and the event carries no id at all. Older rollouts have
only the event, so it still has to be read as a fallback.

**Pair a tool call with its result by `call_id`.** Codex issues calls in
parallel and the results come back interleaved, so the next output is often not
the answer to the last call. Both the call and its output carry the same
`call_id`, and that is the only thing joining them. Settle a `git commit` on
whichever output arrives next and an unrelated command's exit code decides
whether the commit counted.

**The cached tokens are inside `input_tokens`, not beside them.** A usage
record reads like four separate figures and is not:

```json
{"input_tokens":28739,"cached_input_tokens":28032,"output_tokens":11,"total_tokens":28750}
```

`total_tokens` is `input_tokens` plus `output_tokens`, which is what says the
28,032 cached are part of the 28,739 rather than additional to them. Add
`input_tokens` and `cached_input_tokens` together and you have counted the
cache twice: on one real project that turned 1.59M tokens into 3.04M. The
fresh input is `input_tokens - cached_input_tokens`, which here is 707.

Claude Code reports these already separated, so a parser that reads both
agents cannot use one rule for the pair.

**A single user record holds several injected blocks.** The desktop app sends a
plugin catalogue, the environment, the permissions and more as separate
`input_text` chunks of one `user` message. Test only how the joined text begins
and whichever block happens to arrive first decides the answer: a record that is
entirely machine-generated reads as a prompt, and every session gains one that
nobody typed. Strip each known block by its own closing tag and treat the record
as a prompt only if something is left.

**A sub-agent's rollout has no user message at all.** The work it was asked to
do arrives as an `agent_message` from the agent that spawned it. Count only
`user` messages as prompts and the whole session comes back empty, and an empty
session is usually dropped, so the work disappears rather than being merely
mislabelled.

**Agents message each other in both directions.** A reply from a sub-agent is
also an `agent_message`. Treat every one as new work and the parent gains a turn
each time a sub-agent reports back. Only `NEW_TASK` opens work; the envelope
says which is which.

**What one agent asks another is encrypted.** The brief in a `spawn_agent` call
and the payload of an `agent_message` are Fernet tokens: base64url, beginning
`gAAAAA`, a few hundred characters with no spaces. There is nothing to read.
What matters is not printing one as though it were something a person wrote,
which is how a wall of ciphertext ended up as a label on a diagram here. The
envelope around the payload is plain text and carries a `Task name:` line, which
is the honest thing to show instead.

## Where the files live

```
~/.codex/sessions/YYYY/MM/DD/rollout-<timestamp>-<session id>.jsonl
```

One file per session, in date directories. Unlike Claude Code there is no
per-project directory: the project is `session_meta.cwd`, so grouping by project
means reading the head of every file. A rollout whose `cwd` sits inside
`~/.codex` is internal rather than a project of yours.

## Record types

Every line is `{timestamp, ordinal, type, payload}`, and `type` is the outer
discriminator with a second `type` inside most payloads.

```
session_meta          once, first: session_id, id, parent_thread_id, cwd
turn_context          the active model, and the cwd for that turn
response_item         the conversation: message, reasoning, function_call,
                      custom_tool_call, their outputs, and agent_message
event_msg             progress: task_started, item_completed, token_count
token_usage_record    per response usage, with a response_id
world_state           the harness's own bookkeeping
```

`response_item` is where the work is. The rest is mostly noise for these
purposes, with the exception of `token_usage_record`.

## Tokens

`token_usage_record` carries both `usage`, for the response that just finished,
and `turn_token_usage`, a running total. Sum `usage`. Summing the running total
counts the first response once per response that follows it.

The running total resets per turn rather than accumulating across a file, so it
cannot stand in for a session total either.

## Sub-agents

Codex records work handed to another agent, which Claude Code does not do in the
same shape. It is worth reading: a `spawn_agent` call names the task, the
sub-agent gets its own rollout with its own prompts, tools and token spend, and
that spend is part of what answering the original prompt cost.

It is not a separate stretch of work. The sub-agent runs inside one turn of the
session that spawned it, usually finishing before that session's next prompt, so
its natural home is that turn rather than a place beside it.

`session_meta.source` describes the spawn, including `depth`, so nesting beyond
one level exists. Nothing here has measured it past depth 1.
