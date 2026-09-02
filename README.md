# bough

[![CI](https://github.com/nickelsec/bough/actions/workflows/ci.yml/badge.svg)](https://github.com/nickelsec/bough/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25-00ADD8)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

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

Run `bough` on its own and it asks which project, then opens it in your
browser:

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
bough taggity          open a project by name
bough --text           write to the terminal instead
bough --list           show every project with history
bough -v               include every prompt in the text view
bough --json           write the graph as JSON
```

Anything piped or redirected is written as text, so `bough > notes.txt` and
`bough | less` behave as you would expect rather than opening a window.

`--json` gives you the whole structure to do something else with. It carries no
colours, sizes or positions, only what is true about the work; the page works
those out for itself.

The page is served from 127.0.0.1 and nothing else can reach it. Everything it
needs is inside it, so it keeps working with the network unplugged.

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
two of the obvious approaches turned out not to work. Grouping tasks by what
they have in common measured at noise, and cutting on any single weak signal
turned one afternoon of styling into fifty two tasks out of a hundred and
fifty four prompts. Sittings and corroborated signals replaced both.

The thresholds that remain are fitted to one developer's history and will suit
somebody else's differently. What each one does, what set it, and what happens
when you move it is in [docs/tuning.md](docs/tuning.md). They are constants
rather than flags for now, so changing one means editing Go.

## The transcript format

Reading these files correctly is most of the work, and Anthropic does not
document them. What was learned is written down in
[docs/format.md](docs/format.md): where they live, the append-only replay that
makes a naive parser overcount by more than three to one, the tool results
filed as though the user typed them, and the fields that carry less than they
look like they do.

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
internal/server  the local page, served on loopback only
internal/pick    the list you choose a project from
internal/banner  the mark it opens with
assets           the artwork, and the script that sizes it for the page
```

Nothing above `internal/agent` knows which agent the history came from, and a
test fails if that ever stops being true. That is what makes a second agent
cheap to add.

## Status

Early. It works on the history it was built against, and the parts that are
guesses are marked as guesses.

Two things worth knowing before you rely on it. The thresholds are fitted to
one person's history, so your boundaries may fall in places you disagree with.
The struggle score has never been checked against anyone's memory of their own
work, so it is off by default and labelled as unproven where it appears.

Only Claude Code is read so far. The seam for a second agent exists and is
tested, but nothing else is implemented yet.

## Contributing

Issues and pull requests are welcome, and questions are as useful as code at
this stage. See [CONTRIBUTING.md](CONTRIBUTING.md) for how to get set up and
the four rules that hold the design together.

## Licence

MIT. See [LICENSE](LICENSE).
