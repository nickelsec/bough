# The Claude Code transcript format

Notes from reading a real corpus, since Anthropic does not document this and the
details matter if you want to read the files correctly.

Everything below was measured on one machine in August 2026, across 6 project
directories and 28 transcripts: 275 MB, 76,801 lines, nothing unparseable. Claude
Code version 2.1.238 and a little either side. Your corpus will differ, and the
format moves, so treat the shapes as reliable and the counts as illustrative.

## Where the files live

```
~/.claude/projects/<mangled-cwd>/<sessionId>.jsonl
```

The directory name is the working directory with the separators replaced by
dashes, so `d:\chaff-app` becomes `d--chaff-app`. That is lossy and you cannot
reverse it, because a dash in the original path is indistinguishable from a
separator. Every record carries a `cwd` field, so read the real path from there.

The same directory holds things that are not transcripts:

- `subagents/*.meta.json`, one per sub-agent run
- `tool-results/`, large tool outputs written to their own files
- `memory/`, saved notes

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
names are good: "Chaff product architecture and design", "Test elevenlabs-mcp
for arbitrary file read vulnerability". Free labels, generated locally.

The `system` records with subtype `compact_boundary` mark where context was
compacted, 35 of them in this corpus. They are honest signals that the thread
moved on.

## Structure in the record

`parentUuid` links each record to the one before it. In this corpus there were
66,434 links, none crossing a session boundary and none dangling. That gives you
clean ordering and a way to check integrity.

It does not give you a tree. Every session was a single unbroken chain with no
branch points. Rendering it as a hierarchy draws a straight line.

The one place real branching exists is sub-agent runs, marked by
`isSidechain: true`, with 1,078 such records here and 907 of them in a single
session.

## Fields that carry less than you would hope

`gitBranch` is on every record, but across four months of work it held three
values: `HEAD` (48,189), `master` (14,702) and `main` (3,611). If the developer
does not use feature branches, and many do not, it tells you almost nothing.

`cwd` is effectively constant within a session, so it identifies the project and
nothing finer.

## Retention

Nothing in the corpus suggested transcripts are pruned. The oldest was four
months old and still complete. Do not rely on that.
