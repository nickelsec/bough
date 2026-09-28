# The transcript formats

> Read this on the web at [www.bough.run/docs/format](https://www.bough.run/docs/format).

Neither Claude Code nor OpenAI Codex CLI documents the files it writes, and the
details matter if you want to read them correctly. Pi documents its own and is
open source, which helps, but it has traps of its own. These are notes from
reading real corpora, written while building bough and each traced to code that
was getting something wrong.

- [The Claude Code JSONL transcript format](/docs/format/claude-code), the
  largest of the three: where the files live, what a record holds, and the four
  things that will catch you out.
- [The Codex CLI rollout file format](/docs/format/codex), read the same way,
  from a smaller corpus.
- [The Pi session file format](/docs/format/pi), written against Pi's own
  source: a tree rather than a list, and forks that copy everything.

One pattern is worth naming before any of them. All three agents write the same
work more than once, so all three overcount badly if you take the files at face
value. On Claude a naive line count runs about three times too high. On Codex
the input token figure already contains the cached figure, so adding the two
doubles every total. On Pi a fork copies the whole conversation into a new
file, so everything before the fork is counted twice.
