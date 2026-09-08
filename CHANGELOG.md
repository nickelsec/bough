# Changelog

Notable changes, newest first. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

Nothing yet.

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
