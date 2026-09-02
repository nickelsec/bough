# Changelog

Notable changes, newest first. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Unreleased

Nothing yet.

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
