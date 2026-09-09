# Changelog

Notable changes, newest first. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

Nothing yet.

## 0.4.0 - 2026-09-09

bough reads OpenAI Codex CLI as well as Claude Code. Thanks to
[@valdecircarvalho](https://github.com/valdecircarvalho), who wrote the
multi-agent plumbing and the first Codex parser, and who went and measured
their own corpus to answer a question about deduplication rather than guessing
at it. That measurement is what the cross-file replay handling is built on.

Codex support is new and has had far less exposure than Claude Code. The
numbers here were checked by hand against a real corpus, but a small one.
Please report any that look wrong.

### Added

- Reading of OpenAI Codex CLI history from `~/.codex/sessions`, grouped by
  project the same way Claude Code history is. Prompts, tools, files, edits,
  lines, errors, commits and token counts all come through.
- `--agent=claude`, `--agent=codex` and `--agent=all`, which is the default. A
  project worked on with either agent appears in the same list.
- Work handed to a Codex sub-agent is read and counted. A sub-agent has its own
  rollout with its own prompts and token spend, and all of it is part of what
  answering the original prompt cost.
- A Codex section in [docs/format.md](docs/format.md), covering the rollout
  layout, the replay that spans files, the sub-agent records and the two token
  streams that report the same figures.

### Fixed

- Delegated work no longer appears as a sitting of its own. A Codex sub-agent
  runs inside one turn of the session that spawned it, usually finishing before
  the next prompt, so drawing it alongside said the person started two things
  when they started one and it branched. It now folds into the turn that asked
  for it. This also put a second date heading on a single afternoon.
- A resumed Codex session came back as several sessions with its early prompts
  counted once per resume. Codex replays earlier items into a new rollout under
  the ids they already had, so deduplicating within a file never sees the first
  copy. Rollouts are now grouped by session before anything is deduplicated.
- Every Codex session gained a prompt nobody typed. The desktop app packs a
  plugin catalogue, the environment and more into one user record as separate
  chunks, and testing only how the joined text began let a wholly machine
  generated record read as a prompt. Each known block is now stripped by its own
  closing tag.
- Every Codex token figure was doubled. Newer rollouts carry a
  `token_usage_record` per response and an `event_msg` saying the same thing;
  both were being added. The record now wins where it exists, with the event
  kept as a fallback for older rollouts.
- A Codex sub-agent's work went missing entirely. Its rollout carries the
  parent's session id, so grouping on that folded it away, and it has no user
  message at all since its task arrives as an `agent_message`. Both are now
  handled, and a reply from a sub-agent no longer opens a turn on the parent
  that nobody asked for.
- A wall of ciphertext could appear as the label on a piece of work. What one
  Codex agent asks another is encrypted, and the brief was being read straight
  into the diagram. Encrypted payloads are recognised and left out; the
  delegation is still recorded.
- Multi-file Codex patches credited every changed line to the first file named,
  which corrupts the churn the struggle score reads. Each hunk now counts
  against the file its own header names.
- Codex counted added lines only, where Claude Code has always counted added and
  removed both. The same change measured differently depending on which agent
  made it.

### Changed

- Every project says which agent wrote its history, in the picker, in `--list`
  and on the page. Only Codex was named before, which read as though Claude
  Code were the absence of an agent rather than a choice of one. That was fair
  while there was one agent to read and stopped being fair at the second.

- The project columns are measured rather than fixed. A Codex project is named
  after a directory and can run well past the old 24 characters, and one long
  row then pushed its own path and count out of line with every other row.

- The diagram opens centred, at a size worth looking at, whatever the shape of
  the history.

  Two things were wrong. A long history was fitted to the window whole, which on
  a project that grows sideways and never grows taller drew the day squares at
  under three pixels: a line rather than a diagram. It now opens at a size the
  nodes can be read at, anchored to the most recent work, with a new button to
  see every day at once when that is what you want.

  A short history had the opposite problem. Node sizes come from how much work
  they hold, so a one day project draws smaller shapes, and a fixed cap on the
  opening scale left the whole diagram forty pixels across in the middle of a
  fourteen hundred pixel window: centred, and still reading as lost. The ceiling
  is now the size a node may reach rather than a raw scale.

- The drawing is moved by one transform rather than two. The page carried a
  viewBox that fitted and centred the canvas inside the window before the pan
  and zoom transform ran at all, and the canvas is not the drawing: the layout
  leaves a wide, lopsided margin to pan into. On a one day history that put the
  middle of the tree eleven hundred pixels right of the middle of the window,
  off the screen. Every layout test in the repository worked in canvas units and
  so agreed the view was centred while the page showed it against the right hand
  edge.

## 0.3.6 - 2026-09-09

### Changed

- What a shell command means now lives in one place rather than inside the
  Claude package. A commit is a commit whoever wrote the transcript, so commit
  detection, the heredoc and dry-run guards and path normalisation moved to
  `internal/agent/shell` for every agent to share. Nothing about the output
  changed: the graph is byte for byte what it was on six projects here.

### Added

- A test for the heredoc guard that actually needs it. The guard is why a
  script about committing is not counted as a commit, and it was carrying seven
  false positives on one project before it existed, but nothing pinned it: the
  cases around it are turned away earlier and passed with the guard removed. It
  would have survived any refactor by luck rather than by test.

## 0.3.5 - 2026-09-08

### Fixed

- `bough --version` said `dev` for every install the readme documents. The
  version was only ever passed in by the release build, so both
  `go install ...@v0.3.4` and a plain `go build` reported nothing useful, and
  a bug report could not say which build it came from. Go records the version
  it resolved, so bough asks for it: an install by version now says that
  version, and a build from a checkout says the revision, with `+dirty` when
  the tree had uncommitted changes. The `tool` field in the JSON output was
  wrong in the same way.

## 0.3.4 - 2026-09-08

### Fixed

- The project picker drew itself in pieces across the terminal. It never wrote
  a carriage return, and the terminal is in raw mode while it runs, so a bare
  newline dropped a row and left the cursor in the column it was already in.
  Every line started further right than the one before, and the cursor-up that
  begins each redraw climbed the same crooked path and cleared the wrong part
  of each row. Reported against three terminals; the width work in 0.3.3 made
  it less likely but was not the cause.

- The list is no longer drawn taller than the window. Past about a dozen
  projects the top scrolled away, and the redraw cannot reach what is no longer
  on screen. It now shows what fits, scrolls with the selection, and says how
  many entries lie either side.

### Note

The diagram opens centred on the work rather than on the canvas it is drawn on.
The layout keeps a wide margin to pan into, and fitting to that margin made a
short history open at about half the size it could, sitting off centre.

## 0.3.3 - 2026-09-08

### Fixed

- The project picker scattered itself across narrow terminals. It drew to a
  fixed width instead of asking how wide the window was, so every row wrapped
  and the redraw then stepped back over fewer lines than were really on screen,
  overwriting the wrong ones. Six projects in a 60 column window put 28 rows on
  screen while the picker counted 15. It now measures the terminal, gives the
  name the room the size does not need, and falls back to the numbered list
  when there is no room for a frame at all.

- Clipping a column to nothing crashed rather than drawing nothing, which a
  narrow enough window could reach.

## 0.3.2 - 2026-09-08

### Fixed

- Transcripts written before Claude Code started recording `promptId` now
  read normally. bough required that field to recognise a prompt, and it only
  appeared around 2.1.8x in March 2026, so anything older drew as an empty
  project. One reader's project of 238 sessions showed as 20 prompts; the same
  history holds 3,282. Reported by @tenequm, with the measurements that made it
  findable.

### Changed

- Harness records are recognised by `isMeta` rather than only by how their text
  begins. That is what `promptId` was really screening out, and it does not
  depend on the version that wrote the file. It also catches a few the text
  screens missed, `Continue from where you left off.` among them.

- The warning added in 0.3.1 for old transcripts is gone, because there is no
  longer anything to warn about. They are read instead of explained away.

## 0.3.1 - 2026-09-08

### Fixed

- Some prompts were counted twice. Invoking a skill writes its re-invocation
  notice to the transcript as a second user record under the same promptId, and
  reading that as a new prompt split one request into two. Seventeen prompts
  across the histories here were never typed by anyone.

- Merging the replayed copies of a record could rewrite its promptId. Resuming
  a session re-appends old records stamped with the id of the prompt that
  resumed them, so taking the later value filed unrelated work under a handful
  of prompts. On one project that collapsed 171 prompts onto 22. The first
  promptId now wins; every other field still takes the later value.

- A transcript too old to hold a promptId now says so. bough finds prompts by
  that field, and Claude Code only started writing it partway through its life,
  so an older transcript drew as an empty project with no explanation. Thanks
  to the reader who spotted this against their own archive.

## 0.3.0 - 2026-09-08

### Added

- What the work cost, along the bottom of the diagram and in the text output.
  Most of it turns out to be the model re-reading the conversation rather than
  writing anything: between 176 and 732 times more context than output across
  the histories this was measured on. A long session is expensive because it is
  long, not because the model said much.

- Which model did the work. One name when a project used one, and a split with
  shares when it changed partway through.

- How much a file changed, not just how often. "Kept coming back to build.go
  (9 times)" now says how many lines that meant, because nine typo fixes and
  nine rewrites are not the same work.

### Changed

- The figures along the bottom fold away. Prompts, days, changes, files and
  time stay out; commits, tokens and model open when you click the row. Ten
  figures on one line wrapped on a laptop.

### Fixed

- Tokens were counted several times over. One reply is written to the
  transcript under more than one uuid while keeping a single message id, and
  1,533 of one project's 2,236 replies did exactly that. Totals now agree with
  ccusage to within a fifth of a per cent on cache reads.

- Anything hidden stayed visible if it had a display of its own. The stylesheet
  had no rule for the hidden attribute, so the browser's own was easy to
  override by accident.

### Note

The struggle score is unchanged. Weighting it by lines was built and rejected:
one sitting wrote 451 lines across four edits, a generated file rather than a
struggle, and scoring on size put it above a sitting that returned to the same
file eleven times. Volume is not difficulty.

It has now been checked against one person's memory on three projects and it
picked out the sittings they remembered as hard. That is one person checking a
score fitted to their own history, so it stays off by default. If you turn it
on and the ranking matches what you remember, please say so in an issue.

## 0.2.0 - 2026-09-04

### Added

- Commits the agent made are read from the history and shown on the work they
  belong to: a small mark on the task in the diagram, the hashes in the note
  and in the text output, and a `commits` list in the JSON.

  This is the closest thing in the graph to a fact. Where a task begins and
  ends, and how hard it looked, are all guesses. A commit happened or it did
  not.

  Only commits the agent made are recorded, since a commit typed in a terminal
  never reaches the history.

- bough now reads the git history of the project it is describing, at the path
  the transcripts already name, to confirm those commits and pick up their
  messages and line counts.

  It has to. Claude Code recovers a commit hash by reading what git printed,
  and `git commit -q` prints nothing, so the transcript alone knew 4 of one
  project's 31 commits and 25 of another's 47. Reading the repository as well
  takes those to 24 and 47, and every hash shown now resolves in the repository
  it came from.

  This is a read and nothing else: no writes, no network, no remote. It only
  ever opens a repository already on the disk, so a repository being private
  somewhere makes no difference to it. `--no-repo` turns it off, and a project
  that has moved or was never a repository carries on without it.

  Where the two disagree the repository wins, and a hash it cannot find is
  dropped rather than shown. A recorded hash was true when it was written, but
  rebasing or amending afterwards leaves it pointing at nothing, and offering a
  reader something to check that does not check out is worse than saying
  nothing.

- The commit count carries a mark explaining why it may not match the number a
  forge shows. bough counts what the agent did and `git log` holds what
  survived, which are usually the same number and sometimes are not: commits
  typed by hand never reach the history, an amend is one event more than the
  history keeps, and a rebase drops commits that really happened.

### Fixed

- Text that talks about committing was read as a commit. A command that writes
  a script or a changelog holding the words `git commit` opens a heredoc first,
  and everything after that is content rather than command. This accounted for
  seven of one project's apparent commits and for the whole of another's
  disagreement with its own git log.

- Commits the agent made in a different repository were counted as this
  project's. A session about one project regularly commits in another, a tool
  and its website worked on together being the ordinary case, and ten of one
  project's forty seven commit calls were made next door. Where a command moved
  before committing is now read, and only commits made here are kept.

- A commit was missed when the command set an identity inline, as
  `git -c user.name="Ada Lovelace" commit`. The pattern matching git's options
  stopped at the space inside the quotes. Seven of one project's thirty one
  went missing exactly there.

- `git commit --dry-run` was counted. It reports what it would do and does
  nothing, so it is a rehearsal rather than a commit.

- A commit in the body of a shell conditional or loop was missed, since only
  `;`, `&&` and `|` were understood as separating one command from the next
  and `then`, `else` and `do` do the same job. Related: only the first git call
  on a line was considered, so a rehearsal followed by the real commit lost
  both.

- `toolUseResult` is a string on some records and an object on others. Reading
  it as an object only made those lines fail to decode, and a line that fails to
  decode is skipped, so 85 of the 340 records in the test fixture disappeared
  without a word. Both shapes are now accepted.

## 0.1.0

First working version.

- Reads Claude Code history and recovers the shape of the work: sittings, the
  tasks inside them, and the prompts underneath
- Handles the append-only replay in the transcript files, where 72 percent of
  records repeat and a naive parser over-reports by more than three times
- Groups tasks into sittings by overnight gaps. Similarity grouping was tried
  first, measured at noise, and dropped
- Draws the result as a node diagram in the browser, served on loopback with
  nothing fetched from outside
- Prints the same thing as text for a terminal, a pipe or a file
- Reports a struggle score, off by default, and says plainly that it has not
  been checked against anyone's memory
- Reads a history written on one platform from any other. Paths recorded in a
  transcript are split on both separators rather than the host's own, so a
  Windows history counts correctly on Linux and macOS
