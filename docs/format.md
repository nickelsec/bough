# The transcript formats

Neither Claude Code nor OpenAI Codex CLI documents the files it writes, and the
details matter if you want to read them correctly. These are notes from reading
real corpora of both, written while building bough and each traced to code that
was getting something wrong.

- [The Claude Code JSONL transcript format](/docs/format/claude-code), the
  larger of the two: where the files live, what a record holds, and the four
  things that will catch you out.
- [The Codex CLI rollout file format](/docs/format/codex), read the same way,
  from a smaller corpus.

One pattern is worth naming before either page. Both agents replay records they
have already written, so both overcount badly if you take the files at face
value. On Claude that means a naive line count runs about three times too high.
On Codex it means the input token figure already contains the cached figure, so
adding the two doubles every total.
