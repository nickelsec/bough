# Changelog

Notable changes, newest first. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

Nothing yet.

## 0.6.1 - 2026-09-25

Installing needed a Go toolchain, which is a strange thing to ask of a tool
that ships as one binary and has no runtime. The builds were already there and
nobody was told about them.

### Added

- **Homebrew**: `brew install nickelsec/tap/bough`.

- **A script for everything else.** `curl -fsSL https://www.bough.run/install.sh | sh`
  on Linux and macOS, `irm https://www.bough.run/install.ps1 | iex` on Windows.
  Each works out the newest release, checks the download against the published
  checksums, and puts one binary on your path.

Every build is still on the releases page with its checksums, for anyone who
would rather not run a script they have not read.

## 0.6.0 - 2026-09-21

0.5.0 said how many tokens a piece of work took. The question people actually
ask is what it cost, and that turned out to be one short step away: the counts
were already right and already per task, they just had no price against them.

On the history this was built from, that reads as $1,132.77 for the project and
$0.58 to $85.24 across its 63 tasks. The tasks sum to the total exactly.

Rates are the published ones per model, 125 of them built into the binary. They
are a snapshot rather than a feed, and nothing is fetched at any point: reading
your own history still works with the network unplugged.

Nothing is priced on a guess. A model with no published rate, or one charged by
how large each request was, shows its token counts and no figure at all.

### Added

- **What each piece of work would have cost**, in the drawing, in the record
  panel, in the usage table and in the text output. At published API rates,
  which a flat rate subscription pays none of, so the figure is what the same
  work would have cost billed per token. Read the other way it is what the
  subscription saved.

- **A threshold filter.** One slider over whichever figure you choose: cost, or
  any of the token counts. Work under the threshold fades, so "show me
  everything over $25" is one drag.

- **Following a cost opens the table at that work**, rather than at the top of
  eighty five lines, with the line marked for a moment so it can be found.

### Changed

- **The JSON schema is now 5.** `models` holds the four token counts per model
  instead of a single output figure. The old shape could not be priced: the
  four counts are charged at rates that differ by a factor of fifty, so the
  split per model is the whole of what a bill is made from. A reader that took
  the old number now finds an object where an integer was.

- `stats` carries `cost` wherever a figure could be worked out, so the page and
  the terminal cannot disagree about what something cost.

- The usage table gained a cost column and lost two long headings, which were
  sized by their titles rather than their figures.

### Fixed

- **Cache writes were priced about 60% low.** Context stored for an hour costs
  more than context stored for five minutes, and the flat
  `cache_creation_input_tokens` field says nothing about which it was. Claude
  Code uses the hourly cache for effectively everything it writes, 96% on one
  project and 100% on another, so reading only that field understated the line
  by most of its value. It came to 4.7% of a thousand dollar total, which is
  small enough to look like rounding.

- **A sub-agent's models were dropped** when its work was folded into the turn
  that asked for it, if that turn had named no model of its own. The tokens
  arrived intact and the record of which model spent them did not, so the two
  disagreed. Claude Code only; Codex was unaffected.

- Codex never read `cache_write_input_tokens`. Every record in the corpus holds
  zero there, so no figure changes today, but a bill that silently omits its
  dearest line is worth closing before it matters.

## 0.5.0 - 2026-09-20

bough could draw what you built and not what it cost. The figures were already
being read correctly and were thrown away everywhere except a project total.

The reason to have this here rather than in one of the tools that already
count tokens is that they stop at a session id, which is an opaque hash
covering weeks of unrelated work. bough already knows where one piece of work
ends and the next begins, so it can say which piece was expensive.

Checked against ccusage on the same history: 1,574,933,652 against
1,578,503,592, a 0.2% gap that is the work done between the two runs.

### Added

- **What each task and each day was charged**, in the drawing and in the text
  output. A faint circle behind a task carries the figure; hovering breaks it
  down into output, input, cache created and cache read.

- **A token usage table**, opened from the rail. Every day and every task, in
  the five columns the agents report, with days foldable so a month of totals
  fits on one screen.

- The record panel moves around now. A day lists its tasks and each one opens,
  rather than naming work you then had to find in the drawing yourself.

### Changed

- **The JSON schema is now 4.** A turn carries `tokens`. The figure was always
  worked out, since a task's total is the sum of them, but only the sum
  survived being written out.

- The record panel sits over the drawing instead of pushing it aside. Opening
  one re-laid out every node in the diagram twice, which on a history of any
  size is what made it feel slow. Measured over six open-and-close cycles:
  19 dropped frames before, none after.

### Fixed

- A task charged for context but credited with no output no longer divides by
  zero in the text output. It takes a cancelled reply to do it, which is rare
  enough to reach a release and ordinary enough to happen to somebody.

## 0.4.5 - 2026-09-17

Twelve more findings from [@brandon-fryslie](https://github.com/brandon-fryslie),
each filed with the line it lives on and a way to reproduce it. Every one was a
real bug. Most of these are about a read that failed quietly and came back
looking like an answer.

### Changed

- **The JSON schema is now 3.** A commit carries `confirmed`, saying whether
  the repository still has the hash. Without it a hash is the transcript's own
  claim and nothing checked it, which is what you get when git is missing or
  `--no-repo` is set. The two used to look identical.

- The terminal says when git is missing or the repository would not answer.
  Those used to arrive as the same silence as work done outside a repository,
  and they mean different things for every hash in the output.

- A history that could not be read is said out loud rather than skipped. An
  unreadable directory looked exactly like one holding nothing, so a project
  disappeared and there was no way to find out why.

### Fixed

- **File paths are no longer rewritten as Windows paths.** Every path was
  lower-cased and anything shaped like `/w/app` was rewritten to `w:/app`, on
  the reading that a single letter first component meant a Windows drive. It
  does on Windows. Elsewhere `README.md` was shown as `readme.md`, and two
  directories differing only in case merged into one project, taking the
  commit hashes of one of them with it.

- Codex patch lines starting with `++` or `--` count. They were read as file
  headers, which is a unified diff's shape, not the one Codex writes. Adding
  `++i;` or removing a `-- note` counted as nothing.

- A Codex commit whose result arrived after the next prompt is no longer
  dropped. Claude Code kept these; the two readers now agree about the same
  sequence of events.

- A blank `Task name:` reads as no name instead of taking the next line, which
  used to label a sub-agent's work `Sender: /root`.

- Handed-over work says what it was. A Codex hand-off never picked up the name
  its own transcript recorded, a sub-agent turn that could not be folded showed
  as a blank prompt that search could not find, and a hand-off with nothing
  recorded printed `handed off:` and stopped.

- Building a graph no longer changes the sessions it was given. Folding a
  sub-agent wrote into the caller's own counts, so building twice gave
  different numbers the second time.

- `--agent=all` reads every agent under `--root`, and the no-history message
  names the directory it searched rather than the default location.

## 0.4.4 - 2026-09-15

The rest of [@brandon-fryslie](https://github.com/brandon-fryslie)'s review.
Mostly structure, and it should draw exactly what 0.4.3 drew: same diagram,
same numbers.

### Changed

- **The JSON schema is now 2.** A delegation's task name moved from `kind` to
  `name`. The two were one field holding different things depending on the
  agent: Claude names the sort of sub-agent, Codex names the task. A reader
  that took `kind` as the task name will find it empty on Codex. Nothing else
  in the output moved.

- A turn with no prompt says so. A sub-agent's work carried the task name in
  the prompt field, and the words "delegated task" where there was no name.
  Nobody typed either. The prompt is empty now and the name has its own field.

- The terminal says when the repository was not read. A commit hash means one
  thing when git confirmed it and another when nothing checked, and
  `--no-repo`, a directory that is not a repository, and a machine without git
  all looked the same as a clean confirmation.

### Fixed

- Commits made in a subdirectory, as `cd internal && git commit`, were kept in
  0.4.3 but only where the project path was written one particular way. Paths
  from transcripts are now compared in a single form, so the two spellings of
  a Windows drive are one place.

- Backing out of the project list no longer skips the tool's own cleanup on
  the way out.

## 0.4.3 - 2026-09-15

Codex token counts were roughly double. Everything here comes from a careful
read of the code by [@brandon-fryslie](https://github.com/brandon-fryslie),
who filed each finding with the line it lives on and a way to reproduce it.

### Fixed

- **Codex token counts were about twice what they should have been.** Codex
  reports the cached tokens as part of the input rather than beside it, and
  both numbers were being added. One real project read 3.04M tokens where the
  answer was 1.59M. Anything that reads a token count was affected: the share
  a piece of work took, the context multiple, the struggle score. Claude Code
  numbers were never wrong, and neither was the ratio of re-read to written,
  since both sides moved together.

- A Codex commit was credited to whatever tool output arrived next. Codex runs
  commands in parallel, so an unrelated command's exit code could decide
  whether a commit counted, and a commit issued before the previous result
  came back replaced it. Each commit is now settled by its own call.

- A commit made after moving into a subdirectory, as `cd internal && git
  commit`, was dropped. It is this repository's work and it is now kept. A
  path that climbs out with `..` still is not.

- Matching a commit to the repository depended on the order sessions happened
  to be read in. Every pair is now weighed together and the closest settled
  first, so the same history always reads the same way.

- `--list` printed "0 prompts" for a project whose history could not be read,
  which is what it prints for a project with nothing in it. It now says the
  history could not be read.

- A commit hash the transcript carried was cleared when the repository was
  read and could not confirm it, and kept when the repository was never read
  at all. Those cases are now told apart, so a machine without git no longer
  looks the same as a repository that has moved on.

## 0.4.2 - 2026-09-13

### Added

- Search the prompts. A fourth tab in the filter rail takes what you type and
  rings every prompt that holds all of those words, in any order, so half
  remembering the wording is enough to find it. Nobody recalls the sentence
  they typed three weeks ago, but they usually recall two words of it.

  The prompt text was already in the page, which is why this is a filter rather
  than a feature: bough keeps what you wrote in full because that is the reason
  to click into anything, and nothing was reading it.

### Changed

- The magnifying glass in the rail now belongs to the prompt search, which is
  what people mean by searching. Find a file keeps its place and its behaviour
  but is drawn as a page with a folded corner: a file is looked up rather than
  searched for.

## 0.4.1 - 2026-09-10

### Fixed

- Work was filed under the wrong day for anyone not on UTC. Agents write
  timestamps with a Z suffix and those were being read as UTC and printed as
  UTC, so east of Greenwich a good part of every evening landed on the previous
  date: a prompt typed at 01:55 in Asia/Calcutta was headed 9 September rather
  than the 10th, and the times beside prompts were out by the whole offset.
  Durations are the same in any zone, so nothing about the grouping noticed and
  every test passed. Timestamps now come back in the reader's own zone, which
  is the frame the question "which day was this" is asked in.

## 0.4.0 - 2026-09-10

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
