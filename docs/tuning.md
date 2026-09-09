# Tuning

Every threshold here was fitted against one developer's history. That is the
central weakness of this project, and the numbers below are the part most
likely to be wrong for you.

This explains what each one does, what set it, and what happens when you move
it. Read the section on what already failed before spending time on an
approach that has been tried.

## They are constants, not flags

Worth saying plainly: none of these can be changed from the command line. They
live in `DefaultOptions()` in `internal/segment` and `internal/rollup`, and in
`Summary.Struggle()` in `internal/metrics`. Changing one means editing Go and
rebuilding.

That is deliberate for now. Guessing at a flag set before anyone has said the
defaults are wrong is how a tool ends up with fifteen options nobody uses. If
you move one of these and it works better on your history, that is worth an
issue, and it is the evidence that would justify a flag.

## What each agent gives you to work with

The thresholds below are the same whichever agent wrote the history, because
everything above `internal/agent` sees normalised turns and nothing else. What
differs is the evidence arriving underneath, and one difference is worth
knowing.

Claude Code records a context compaction, which bough passes up as
`SegmentHint`. That is the agent itself saying a boundary happened, and it is
better evidence than anything inferred from timing. Codex records no equivalent,
so a Codex session leans entirely on the gap and churn signals. Where a Claude
session would have been cut on a compaction it saw, a Codex one is cut only if
the ordinary thresholds agree.

Codex records something Claude does not: work handed to a sub-agent, with its
own prompts and token spend. bough folds that into the turn that asked for it,
so it adds to that turn's weight rather than appearing as a sitting of its own.

Everything else, prompts, tools, files, edits, lines, errors, commits and
tokens, comes through from both.

## Where a task begins

`internal/segment`, `DefaultOptions()`.

| name | default | what it does |
|---|---|---|
| `Gap` | 45 min | A pause this long ends a task on its own |
| `CompactionGap` | 20 min | The shorter pause needed when a compaction also happened |
| `FileSimilarity` | 0.10 | File overlap below this counts as different work |
| `TopicSimilarity` | 0.08 | Word overlap below this counts as a different subject |
| `MinFilesForSignal` | 3 | Files needed on each side before overlap is trusted |
| `LongRun` | 10 | Turns after which one weak signal is enough to cut |

The rule these serve matters more than the numbers. A pause or a compaction is
a strong signal and cuts on its own, because both are the user or the agent
saying the thread ended. File and topic divergence are weak, and need to agree
with something else before they cut.

That distinction was not a preference. Cutting on file divergence alone
produced **52 tasks out of 154 turns** on one UI-heavy session, most of them a
single prompt long. Weak signals acting alone shatter iterative work.

`LongRun` exists because of the opposite failure. CSS and config work touches
the same files for hours, so nothing ever diverges and a task runs forever.
After ten turns, one weak signal is allowed to cut.

**If tasks are too fragmented**, raise `Gap` and `FileSimilarity`. **If
unrelated work is being merged**, lower `Gap` first. It is the signal doing
most of the work.

## What counts as a substantive prompt

`internal/segment`, `substantiveLength`, currently 80 characters.

Content signals only apply to prompts long enough to say something. "go for
it" carries no evidence about whether the subject changed.

Eighty was chosen by measuring: across 719 real prompts the median length was
50 characters. The cutoff sits above the median on purpose, because the cost
of treating a short prompt as evidence is a false cut, and the cost of
ignoring a long one is nothing.

## Where a sitting begins

`internal/rollup`, `DefaultOptions()`.

| name | default | what it does |
|---|---|---|
| `Break` | 6 h | A pause this long separates one sitting from the next |

Six hours catches a night's sleep and leaves a lunch break alone.

Sittings are contiguous. Coming back to something a week later starts a new
sitting rather than reopening the old one, which is how the work reads back.

## Which links are drawn

`internal/rollup`, `DefaultLinkOptions()`.

| name | default | what it does |
|---|---|---|
| `MinWeight` | 2 | The least shared work a link needs |

A link joins two sittings that returned to the same files. One incidental edit
on each side is a passing visit, not resumed work.

Ambient files are excluded before links are counted. Without that, every
sitting links to every other through the agent's own plan file.

## The struggle score

`internal/metrics`, `Summary.Struggle()`.

```
0.55 * churn + 0.30 * density + 0.15 * errors
```

Churn is how often a single file was rewritten, saturating at 12. Density is
prompts per file, saturating at 3. Errors is the share of turns with a tool
failure.

**Checked once.** Against one person's memory, on three projects, and it named
the sittings they remembered as the hard ones. That is why the score is still
here. It is also one person checking a score whose weights were fitted to their
own history, which is why it stays off by default in the web view and is
labelled there as a guess. If you turn it on and the ranking matches what you
remember, an issue saying so is worth more than any amount of tuning.

**Churn counts calls, not lines, and that was tested.** Weighting it by how
much each edit changed sounds better and measured worse. One sitting wrote 451
lines across four edits, which is a generated file rather than a struggle, and
every scoring that used size put it above a sitting that returned to the same
file eleven times. Volume is not difficulty. Line counts are recorded and shown
beside the count, but they do not feed the score.

Churn leads for a measured reason: only 489 errors occurred across 18,173 tool
calls, and the segments that scored highest on every other measure had **zero**
errors. Hard work looks like the same file rewritten over and over, not like
things failing.

## Two things that already failed

Both cost real time. Neither is worth repeating.

**Grouping tasks by similarity does not work.** Bag-of-words Jaccard between
genuinely related tasks measured 0.02 to 0.09, which is noise. TF-IDF cosine
peaked at 0.25. No threshold separated signal from noise across two projects
at once: file similarity worked on one at 0.17 to 0.28 and collapsed to 0.02
on another, because CSS work has no file locality. Several tasks have no
lexical content at all, being entirely short continuations. Sittings replaced
similarity, and the grouping they produce was confirmed against memory.

**Ambient files have to be excluded from every measure.** Plan files, memory
notes and changelogs are rewritten constantly as a side effect of how an agent
works. Before they were filtered, the most rewritten file in nearly every
project was the agent's own plan: 48 rewrites in one, 29 in another. Left in,
the score reports that the user struggled with a scratchpad. The rules are in
`internal/metrics/ambient.go`.

## Checking a change

There is no ground truth for segmentation, so it was validated against
evidence the segmenter never saw: git history. Around two thirds of tasks
contain a commit. The files a task touched agree with the files in that commit
74 percent of the time on one project, dropping to 44 and 37 on two others
where Rust and CSS work changes files no prompt ever names.

If you change a threshold, that is the check worth repeating. `bough <project>
--text` next to `git log` for the same period will tell you quickly whether
the boundaries moved somewhere sensible.
