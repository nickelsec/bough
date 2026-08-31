# bough

Reads your Claude Code history and shows you the work you did.

Not how many tokens you burned or how long your streak is. Claude Code's own
`/stats` covers that. This answers a different question: what did I actually
build?

```
$ bough taggity

taggity
=======
d:\taggity

181 prompts across 4 sittings
556 changes to 122 files, 11 hours at the keyboard

------------------------------------------------------------------------
Mon 10 Aug     Audit repo for release readiness
               7 tasks, 79 prompts, 4 hours
               kept coming back to spec.go (19 times)

  i feel like there should be two workflows one with ai and...
    27 prompts, 75 changes, 2 failures

  im asking why not the user itslef specify the pattern and what...
    13 prompts, 45 changes, 1 failure
```

Everything happens on your machine. Nothing is sent anywhere, no model is
called, and your history is only ever read.

## Install

```
go install github.com/nickelsec/bough/cmd/bough@latest
```

Or build it yourself:

```
git clone https://github.com/nickelsec/bough
cd bough
go build ./cmd/bough
```

One dependency, `golang.org/x/term`, for reading arrow keys.

## Use

Run `bough` on its own and it asks which project:

```
  ╭──────────────────────────────────────────────────────────╮
  │ ○  taggity  (here)                    19 MB, 12 days ago │
  │                                                          │
  │ ●  chaff-app                         73 MB, 21 hours ago │
  │                                                          │
  │ ○  mcp-security-toolkit               52 MB, 3 weeks ago │
  ╰──────────────────────────────────────────────────────────╯

  ↑↓ move    ↵ choose    esc cancel
```

The project you are standing in comes first. Or name one directly:

```
bough taggity          read a project by name
bough --list           show every project with history
bough -v               include every prompt
bough --json           write the graph as JSON
```

`--json` gives you the whole structure to do something else with. It carries no
colours, sizes or positions, only what is true about the work.

## What it does

Claude Code keeps a transcript of every session. Those transcripts hold the
shape of what you built, and nothing surfaces it. bough reads them and
rebuilds three levels:

**Prompts** are what you typed, with the files and failures that followed.

**Tasks** are runs of prompts working towards one thing. Where one ends and the
next begins is worked out from how long you paused, where the agent compacted
its context, and whether you changed both subject and files at once.

**Sittings** are the days. People stop for the night and come back to something
else, and that turns out to be a better guide to what belongs together than
anything cleverer.

It also notices when a sitting picked up work from an earlier one, and which
file you kept going back to.

## What it does not do

No token counts, no cost, no streaks. No writes of any kind to your history.
No network. It reads Claude Code only, though the seam for other agents is
already in place.

## How the grouping was arrived at

Every part of this was measured against real history rather than guessed, and
two of the obvious approaches turned out not to work.

Grouping tasks by what they have in common does not work. With the agent's own
plan and memory files set aside, the median file overlap between neighbouring
tasks is zero on three of five sessions tried, and even a task's best match
anywhere in its session sits at noise. The two candidate signals disagree about
which projects they work on, so no threshold holds across all of them. Sittings
are used instead because they need no tuning and cannot drift.

Splitting on any single weak signal does not work either. Cutting whenever the
file set changed turned one afternoon of styling into fifty two tasks out of a
hundred and fifty four prompts, nearly all of them one tweak to the same page.
A pause or a compaction now cuts on its own; anything else needs corroboration.

The thresholds that remain are fitted to one developer's history. They will
suit somebody else's differently, and they are exposed rather than baked in for
that reason.

## The transcript format

Reading these files correctly is most of the work, and Anthropic does not
document them. What was learned is written down in [docs/format.md](docs/format.md):
where they live, the append-only replay that makes a naive parser overcount by
more than three to one, the tool results filed as though the user typed them,
and the fields that carry less than they look like they do.

That document is probably useful to anyone else reading this format, whatever
they are building.

## Layout

```
cmd/bough        the command
internal/agent   the boundary between bough and the agents it reads
  .../claude     reading Claude Code, the only one so far
internal/segment prompts into tasks
internal/rollup  tasks into sittings, and the links between them
internal/metrics how long, how much, how hard
internal/graph   the finished structure, ready to serialise
internal/pick    the list you choose a project from
internal/banner  the mark it opens with
```

Nothing above `internal/agent` knows which agent the history came from, and a
test fails if that ever stops being true. That is what makes a second agent
cheap to add.

## Contributing

Issues and pull requests are welcome. `docs/format.md` is the place to start if
you want to understand the data, and the package comments explain why each
piece works the way it does rather than restating what the code says.

Run the tests with `go test ./...`. The parser has a fuzz target:

```
go test ./internal/agent/claude -fuzz FuzzReadRecords
```

## Licence

MIT. See [LICENSE](LICENSE).
