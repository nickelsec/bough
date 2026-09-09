# The transcript formats

Notes from reading real corpora of Claude Code and OpenAI Codex CLI history,
since neither format is documented by the people who write it and the details
matter if you want to read the files correctly.

Claude Code comes first and is the larger part. The Codex section is at the end
and stands on its own, though the two share a pattern worth naming up front:
both replay records they have already written, and both overcount badly if you
take the files at face value.

# Claude Code

Everything below was measured on one machine in August 2026, across 6 project
directories and 28 transcripts: 275 MB, 76,801 lines, nothing unparseable. Claude
Code version 2.1.238 and a little either side. Your corpus will differ, and the
format moves, so treat the shapes as reliable and the counts as illustrative.

## What will catch you out

Six things in these files will give you wrong numbers, and none of them fail
loudly. Each is covered in full below; this is the short version, and it is the
part worth reading before you write any code.

**The files replay.** Claude Code appends to a transcript when a session is
resumed or rewound, rewriting records it has already written, so the same
`uuid` turns up several times. Count lines instead of distinct records and the
largest session here comes out at 36,676 rather than 10,964: **more than three
times too high**. The overstatement is uneven, so there is no constant to
correct it by. Deduplicate on `uuid` first, keeping the last copy of most
fields, but see the next entry for the one that has to go the other way.

**Merging the copies rewrites `promptId`.** Later copies usually just fill in
fields the first one left empty, so taking the later value is right nearly
everywhere. `promptId` is the exception: resuming a session re-appends old
records stamped with the id of the prompt that resumed them, not with a better
version of their own. Take the later value and every replayed record files under
a handful of ids. On one project here that turned 171 prompts into 22. Keep the
first `promptId` you see and take the later value for everything else.

**Two fields arrive in more than one shape.** `toolUseResult` is an object
17,319 times and a bare string 490 times. Type it as an object only and every
string-valued line fails to decode, and if a decode failure drops the record you
lose it without a word: 85 of the 340 lines in this repository's own test
fixture went missing that way.

**Deduplicating by `uuid` is still not enough for tokens.** One reply is
written under several uuids while keeping one `message.id`, and the usage is
repeated on each. 1,533 of one project's 2,236 replies did this. Counting per
record, even deduplicated, overstates output by nearly twice.

**A quiet commit leaves no hash.** Claude Code fills in `gitOperation` by
reading what git printed, so `git commit -q` prints nothing and the field never
appears. Of 88 quiet commits measured here, **none** carried one, against 53 of
57 ordinary ones. Reading that field alone found 4 of one repository's 31
commits.

**A hash was only true when it was written.** Rebase or amend afterwards and
the transcript still names objects the repository can no longer reach. 12 of one
project's 19 recorded hashes are unreachable for that reason.

One more, which is not about the format at all. If you go looking for commits in
the shell commands instead, a command that writes a file can hold the words
`git commit` inside the text it writes. Seven of one project's apparent commits
were heredocs doing exactly that.

## Where the files live

```
~/.claude/projects/<mangled-cwd>/<sessionId>.jsonl
```

The directory name is the working directory with the separators replaced by
dashes, so `d:\my-project` becomes `d--my-project`. That is lossy and you cannot
reverse it, because a dash in the original path is indistinguishable from a
separator. Every record carries a `cwd` field, so read the real path from there.

The same directory holds things that are not transcripts:

- `memory/` and `MEMORY.md`, saved notes
- a directory named after a session id, holding that session's own working
  files, `subagents/` among them

The layout inside those has changed at least once, so do not depend on it.
Match `*.jsonl` at the top level only. A recursive walk will hand you files that
are not sessions.

## The files are append-only, with replay

Claude Code appends to a transcript when a session is resumed or rewound, and it
rewrites records it has already written. The same `uuid` shows up several times.

In the largest session measured:

| | |
|---|---|
| lines | 36,676 |
| distinct records | 10,964 |
| uuids appearing more than once | 7,919 |
| highest repeat count for one uuid | 6 |

Counting lines instead of records overstates that session by more than three to
one, and the overstatement is uneven, so you cannot correct for it with a
constant.

Repeat copies are **not identical**. Later ones tend to carry fields the earlier
ones left empty. Across the duplicates, the fields that changed were:

| field | times it differed |
|---|---|
| `cwd` | 20,413 |
| `promptId` | 5,592 |
| `toolUseResult` | 4,318 |
| `slug` | 15 |
| `parentUuid` | 10 |

Most of the `cwd` differences are a drive letter changing case, `d:\` against
`D:\`, which is worth normalising if you group by path.

So the rule is: key on `uuid`, merge later copies over earlier ones taking any
non-empty value, and keep the position of the first appearance. A field missing
from a later copy was not repeated, not cleared.

## Deduplicating by uuid is not enough for tokens

There is a second layer of repetition underneath the replay, and it bites
anything counted off `message.usage`.

One reply from the model is written under **more than one `uuid`** while keeping
a single `message.id`, and the usage figures are repeated on every copy. In one
project, 1,533 of 2,236 replies did this, three copies each carrying the same
252 output tokens.

So deduplicating records by `uuid`, which is right for the records themselves,
still counts those replies three times:

| dedup key | output tokens |
|---|---|
| by `uuid` | 12,402,105 |
| by `message.id` | 6,823,315 |

Records are one thing and replies are another. Use `uuid` for records and
`message.id` for anything charged.

Usage appears only on `assistant` records, and carries four integer fields:
`input_tokens`, `output_tokens`, `cache_read_input_tokens` and
`cache_creation_input_tokens`. The nested `cache_creation` object reconciles
exactly with the flat field, so reading both would double count. Across one
project's replies, cache reads came to around 596 times the output.

## Tool results are filed as user records

A record with `"type": "user"` is not necessarily something a person typed. Tool
output is recorded the same way. Breaking down the user records that carry a
`promptId` in one session:

| content | count |
|---|---|
| `tool_result` blocks | 2,700 |
| plain string | 114 |
| `text` blocks | 89 |
| mixed with an image or document | 2 |

Tool results outnumbered real prompts by more than thirteen to one. Check that
the content actually holds text before treating a record as a prompt.

### One submission can be several records

A `promptId` identifies a submission, not a record, and one submission is
sometimes written as several user records. Invoking a skill files its
re-invocation notice, and occasionally the skill body itself, as further records
under the same id. Read each as a new prompt and one request becomes several:
seventeen prompts across this corpus that nobody typed.

Count the first record under a `promptId` and skip the rest. Do not try to spot
these by their wording, and do not deduplicate on the text either. The same
words under a *different* id are a real second prompt, which is what somebody
typing `retry` after a failure looks like.

### Old transcripts have no promptId at all

Claude Code only began writing `promptId` partway through its life, around
2.1.8x in March 2026. A transcript from before then holds a full conversation
and none of the field. Every version in this corpus writes it, on essentially
every user record, so there was nothing here to measure the boundary against;
the dated readings come from a reader with an older archive, who saw 2.0.70 in
December 2025 and 2.1.72 in March 2026 both write none.

Do not require it to recognise a prompt. Its absence is version dependent, not
meaning dependent, and requiring it discards those files whole and without a
word: one project of 238 sessions drew as 20 prompts, because only two sessions
were recent enough to carry the field. The same records read without the
requirement hold 3,282.

Use `isMeta` instead for the part `promptId` was really doing. The harness sets
it on records it wrote itself, which is what you were trying to exclude anyway,
and it is not tied to a version the way `promptId` is. It catches things the
text screens miss, `Continue from where you left off.` among them.

Where `promptId` is still worth having is telling one submission from the next,
as above. Just treat an absent one as no information rather than as a shared
identity, or a whole old session collapses into a single prompt.

## The schema is loosely typed

Two fields arrive in more than one shape:

- `message.content` is a list of blocks 52,641 times and a bare string 400 times
- `toolUseResult` is an object 17,319 times and a string 490 times

Content blocks come in five types: `text`, `tool_use`, `tool_result`,
`document`, `image`.

Decode defensively, and ignore fields you do not recognise. The format gains and
loses keys between releases, and an unknown key should never be a parse error.

The `toolUseResult` shape is worth taking seriously rather than noting. Typing
it as an object only makes every string-valued line fail to decode, and if you
drop a record on a decode failure you lose it silently: 85 of the 340 lines in
this repository's own test fixture went missing that way.

## Commits

A commit the agent makes is recorded on the result of the tool call that made
it:

```json
"toolUseResult": {
  "gitOperation": { "commit": { "sha": "d0a65cc", "kind": "committed", "branch": "master" } }
}
```

`kind` is `committed` or `amended`. Pushes appear the same way under a
different key.

These arrive on `user`-type records, in document order, so the commit belongs to
whichever prompt was open when it landed.

**Do not trust this field to tell you a commit happened.** Claude Code fills it
in by reading what git printed, so silencing git silences the field:

| commit command | calls | carried `gitOperation` |
|---|---|---|
| without `-q` | 57 | 53 (93%) |
| with `-q` | 88 | 0 |

Relying on it alone found 4 of one repository's 31 commits and 25 of another's
47. The dependable signal is the shell command itself: a `git commit` that came
back without an error. Treat `gitOperation` as where a hash comes from when
there is one, not as whether a commit occurred.

Three more things before counting. Records replay, so this corpus holds 102
commit records covering 53 distinct ones and counting lines doubles the total.
A commit the person typed themselves in a terminal never appears at all. And a
hash is only true at the moment it was written: rebasing or amending afterwards
leaves the transcript pointing at objects the repository no longer reaches, and
12 of one project's 19 recorded hashes are unreachable for exactly that reason.

## Record types

15 types appear in the corpus. The ones worth knowing:

| type | count | what it is |
|---|---|---|
| `assistant` | 33,079 | model turns |
| `user` | 19,875 | prompts and tool results, see above |
| `attachment` | 13,424 | injected context, reminders, file contents |
| `ai-title` | 2,434 | Claude Code's own label for the session |
| `system` | 120 | includes `compact_boundary` |

`ai-title` is worth pulling out. Claude Code already names each session, and the
names are good: "Product architecture and design", "Test an MCP server
for arbitrary file read". Free labels, generated locally.

The `system` records with subtype `compact_boundary` mark where context was
compacted, 35 of them in this corpus. They are honest signals that the thread
moved on.

## Structure in the record

`parentUuid` links each record to the one before it. In this corpus there were
66,434 links, none crossing a session boundary and none dangling. That gives you
clean ordering and a way to check integrity.

It does not give you a tree. Every session was a single unbroken chain with no
branch points. Rendering it as a hierarchy draws a straight line.

Sub-agent runs are the one place real branching exists, but not where you might
look for it. Counting `isSidechain: true` over the raw lines gives 1,078 records
in this corpus. Every one of them is a replay duplicate: after collapsing on
uuid, the count is zero. Anyone reading these files line by line will find
sub-agent structure that is not there.

What does survive is the call that started the work. Sub-agents are launched
through a `Task` or `Agent` tool call whose input carries a `subagent_type` and
a `description`, 19 of them here:

    Explore   Research PDF redaction stack
    Plan      Design the architecture and milestones
    Explore   Diagnose PDF text layer accuracy

Those descriptions were written at the time, by the agent, about the work it was
about to do. They are better labels than anything reconstructed afterwards.

## Fields that carry less than you would hope

`gitBranch` is on every record, but across four months of work it held three
values: `HEAD` (48,189), `master` (14,702) and `main` (3,611). If the developer
does not use feature branches, and many do not, it tells you almost nothing.

`cwd` is effectively constant within a session, so it identifies the project and
nothing finer.

## Retention

Nothing in the corpus suggested transcripts are pruned. The oldest was four
months old and still complete. Do not rely on that.

# OpenAI Codex CLI

A second format, read the same way and worth its own notes. Be warned that the
corpus behind this section is much smaller than the Claude one: 3 rollout files,
242 lines, 7.4 MB, from Codex Desktop 0.153.4 on one machine in September 2026.
The shapes described here were each traced to code that was getting them wrong,
so they are real, but the counts prove far less than the Claude figures do.

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

