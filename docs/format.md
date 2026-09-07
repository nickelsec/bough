# The Claude Code transcript format

Notes from reading a real corpus, since Anthropic does not document this and the
details matter if you want to read the files correctly.

Everything below was measured on one machine in August 2026, across 6 project
directories and 28 transcripts: 275 MB, 76,801 lines, nothing unparseable. Claude
Code version 2.1.238 and a little either side. Your corpus will differ, and the
format moves, so treat the shapes as reliable and the counts as illustrative.

## What will catch you out

Five things in these files will give you wrong numbers, and none of them fail
loudly. Each is covered in full below; this is the short version, and it is the
part worth reading before you write any code.

**The files replay.** Claude Code appends to a transcript when a session is
resumed or rewound, rewriting records it has already written, so the same
`uuid` turns up several times. Count lines instead of distinct records and the
largest session here comes out at 36,676 rather than 10,964: **more than three
times too high**. The overstatement is uneven, so there is no constant to
correct it by. Deduplicate on `uuid` first, keeping the last copy.

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
