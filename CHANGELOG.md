# Changelog

Notable changes, newest first. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

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
