<p align="center">
  <img width="360" alt="bough" src="docs/img/logo.png">
</p>

<p align="center">
  Reads your Claude Code session history and draws what you actually built.
</p>

<p align="center">
  <a href="https://github.com/nickelsec/bough/actions/workflows/ci.yml"><img src="https://github.com/nickelsec/bough/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/nickelsec/bough/releases"><img src="https://img.shields.io/github/v/release/nickelsec/bough?color=7C8A46" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-7C8A46" alt="MIT"></a>
  <a href="https://bough.run/docs"><img src="https://img.shields.io/badge/Docs-7C8A46" alt="Documentation"></a>
</p>

<p align="center">
  <video width="100%" src="https://github.com/user-attachments/assets/af70eb60-cff7-42cc-af73-19c9cfb0c1d3" controls></video>
</p>

Not how many tokens you burned or how long your streak is. Claude Code's own
`/stats` covers that. This answers a different question: what did you actually
build?

## Run it

```
bough
```

That is the whole thing. It finds your projects, asks which one, and opens it
in your browser.

What you get is a diagram of your own work. A square on the line is a day you
sat down. Smaller squares hanging off it are the tasks inside that day. Every
circle is a single prompt you typed.

Hover anything and the path back to its day lights up, with a note saying what
it was. Click it and the full record opens beside the drawing, scrolled to the
exact prompt, in your own words and untrimmed. Work that went badly is drawn in
rust rather than green.

A task carrying a small mark is one that ended in a commit, and the note gives
you the hash and the message. Everything else in the drawing is worked out from
your history; this is the part you can go and check.

To get those right, bough also reads the git history of the project it is
describing, at the path your transcripts already name. That is a read and
nothing else: no writes, no network, no remote. It only ever looks at a
repository already on your disk, so whether it is private on a host somewhere
makes no difference. `--no-repo` turns it off, and a project that has moved or
was never a repository simply carries on without it.

The count may not match the number your host shows, and bough says so where it
is written. It counts what the agent did; `git log` holds what survived. A
commit you typed in a terminal never reaches your history, an amend is one
event more than the history keeps, and a rebase drops commits that really
happened.

The page is served from 127.0.0.1 and nothing else can reach it. Everything it
needs is inside the binary, so it keeps working with the network unplugged.

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

## In the terminal instead

Add `--text` and the same work comes back as an indented list:

```
$ bough project-one --text

project-one
===========
~/code/project-one

141 prompts across 7 sittings
274 changes to 138 files, 9 hours at the keyboard

------------------------------------------------------------------------
Sat 22 Aug     Explore text selection paths
               9 tasks, 51 prompts, 4 hours
               kept coming back to architecture.rs (8 times)

  hey i ran it locally and it just ahs my website in an exe file...
    10 prompts, 46 changes, 14 failures

  Okay, so after running the npm command and the cargo build, it...
    4 prompts, 1 failure
```

Anything piped or redirected is written as text automatically, so
`bough > notes.txt` and `bough | less` behave as you would expect rather than
opening a window.

```
bough project-one      open a project by name
bough --text           write to the terminal instead
bough --list           show every project with history
bough -v               include every prompt in the text view
bough --json           write the graph as JSON
bough --no-repo        leave the project's git history unread
```

`--json` gives you the whole structure to do something else with. It carries no
colours, sizes or positions, only what is true about the work; the page works
those out for itself.

Everything happens on your machine. Nothing is sent anywhere, no model is
called, and everything bough opens, your history and your repository alike, it
only ever reads.

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

No token counts, no cost, no streaks. No writes of any kind, to your history or
your repository. No network. It reads Claude Code only, though the seam for
other agents is already in place.

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
internal/repo    the project's own git history, read to confirm its commits
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
