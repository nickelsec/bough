# Contributing

Thanks for looking. bough is early and the rough edges are real, so bug reports
and questions are as useful as code right now.

## Getting set up

```
go build ./cmd/bough
go test ./...
```

Two optional extras. The layout test runs the drawing code under Node and
skips itself if Node is missing, so install it if you are touching the web
view. Regenerating the artwork and the embedded fonts needs Python and
fontTools, but building never does.

Run `make lint test` before opening a pull request. CI runs the same commands
on Linux, macOS and Windows.

## Four rules that hold the design together

Break any of these and the project stops being what it is.

1. **The core never imports an agent package.** `segment`, `rollup` and
   `metrics` see normalised turns and nothing else. That is what makes a
   second agent cheap. Enforced by `TestCoreDoesNotDependOnAnyAgent`.
2. **The graph carries no presentation.** No coordinates, no colours, no
   sizes. The renderer works out its own layout every time. Enforced by
   `TestGraphCarriesNoPresentation`.
3. **The page fetches nothing.** Fonts, artwork and the graph are all embedded.
   A page that phones home would contradict the one promise this makes.
   Enforced by `TestPageFetchesNothingExternal`.
4. **Agent data directories are read only.** bough opens files under
   `~/.claude` and never writes there. No test enforces this yet, so it is on
   reviewers to notice.

## Never commit real history

Transcripts contain whatever you typed, which includes pasted keys and
customer names. The one fixture in the repo,
`internal/agent/claude/testdata/replay.jsonl`, keeps the record structure with
every prompt replaced by a `text-<hash>` placeholder. Any new fixture goes
through the same treatment. When in doubt, generate one rather than cutting it
from your own history.

## Commits and pull requests

Say what changed and why, in the present tense. If a number appears in the
message it should be one you measured. No attribution trailers.

Small pull requests get read faster than large ones. If you are planning
something big, open an issue first so you do not build the wrong thing.
