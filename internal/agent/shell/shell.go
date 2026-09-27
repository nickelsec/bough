// Package shell reads meaning out of the commands an agent ran.
//
// Every agent records shell commands, and a command means the same thing
// whoever wrote the transcript: `git commit` is a commit in Claude Code, in
// Codex, and in whatever comes next. So this lives apart from any one agent's
// package, and each source calls it rather than carrying its own copy.
//
// That is not tidiness. Everything here was arrived at by measuring a real
// corpus and finding the naive version wrong, and the comments saying so are
// the expensive part. A second copy keeps the code and loses the reasoning,
// and the next person to read it removes a guard that looks unnecessary.
package shell

import (
	"os"
	"regexp"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// optionRun matches the options that may sit between git and its subcommand.
//
// Options between git and the subcommand are ordinary, since "git -C dir
// commit" is a normal thing to write. An option or its value may also carry a
// quoted run with spaces in it. That last part is not a nicety: setting an
// identity inline, as
//
//	git -c user.name="Ada Lovelace" commit -q
//
// is common, and a pattern that stops at the space inside the quotes misses
// the commit entirely. Seven of one project's thirty one were lost that way.
var optionRun = `-\S*(?:"[^"]*"|'[^']*'|\S)*\s+` +
	`(?:(?:"[^"]*"|'[^']*'|[^-\s])(?:"[^"]*"|'[^']*'|\S)*\s+)?`

// commitCall matches a git commit in a shell command.
//
// Anchored to the start of a command or to a shell separator, so a commit
// somebody merely mentioned inside an echoed string is not counted.
// "commit-tree" is excluded by requiring a word boundary that is not a hyphen.
//
// A commit can also open the body of a conditional or a loop, where the
// keyword does the separating that a semicolon would elsewhere.
var commitCall = regexp.MustCompile(`(?:^|[|;&(]|&&|\|\||\b(?:then|else|do)\b)\s*(?:cd\s+\S+\s*&&\s*)*` +
	`git\s+(?:` + optionRun + `)*commit(?:\s|$)`)

// heredoc opens a run of text written into a file or piped to a program.
//
// Everything after it is content rather than command, and content mentioning
// git commit is not a commit. Writing a script or a test about committing does
// exactly that: it accounted for seven of one project's forty seven apparent
// commits and two of another's forty nine, which is the whole of that project's
// disagreement with its own git log.
var heredoc = regexp.MustCompile(`<<-?\s*['"]?\w`)

// dryRun marks a commit that reports what it would do and then does nothing.
// It is a rehearsal, and counting it would credit work that never landed.
var dryRun = regexp.MustCompile(`(?:^|\s)--dry-run\b`)

// separator ends one command and begins the next.
var separator = regexp.MustCompile(`[|;&]`)

// amendCall marks a commit that rewrites the one before it rather than adding
// to the history.
var amendCall = regexp.MustCompile(`\bgit\s[^|;&]*\s--amend\b`)

// IsCommit reports whether a shell command actually commits.
//
// Every match is considered rather than only the first, because one line can
// hold several git calls and the first is not always the one that lands. A
// rehearsal followed by the real thing is exactly that shape, and stopping at
// the first match would throw the commit away.
func IsCommit(cmd string) bool {
	h := heredoc.FindStringIndex(cmd)
	for _, at := range commitCall.FindAllStringIndex(cmd, -1) {
		// A heredoc before it means the match is inside written text, and so
		// is everything after it.
		if h != nil && h[0] < at[0] {
			return false
		}
		// A rehearsal reports what it would do and does nothing. The flag sits
		// after the subcommand, and only as far as the next separator: beyond
		// that it belongs to some other command on the line.
		if dryRun.MatchString(FirstCommand(cmd[at[1]:])) {
			continue
		}
		return true
	}
	return false
}

// IsAmend reports whether a command rewrites the previous commit rather than
// adding to the history. An amended commit leaves the transcript naming a hash
// the repository can no longer reach.
func IsAmend(cmd string) bool {
	return amendCall.MatchString(cmd)
}

// FirstCommand is the run of text up to the next command separator, which is
// the part an option can belong to.
func FirstCommand(s string) string {
	if at := separator.FindStringIndex(s); at != nil {
		return s[:at[0]]
	}
	return s
}

// leadingCD is a command that moves somewhere before doing anything else.
//
// A session about one project regularly commits in another: working on a tool
// and its website together, say. Those commits are real, but they are not this
// project's, and claiming them would have this project's diagram showing work
// that happened somewhere else.
//
// Agents differ in what else they record. Codex writes the command but not
// always the directory it ran in, so where the command moves first that is the
// better answer than the session's working directory.
var leadingCD = regexp.MustCompile(`^\s*cd\s+(?:"([^"]*)"|'([^']*)'|([^\s;&|]+))`)

// CommitDir is where a command committed, or "" for the project's own
// directory, which is where a command that does not move runs.
func CommitDir(cmd string) string {
	m := leadingCD.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return agent.NormalisePath(g)
		}
	}
	return ""
}

// commitLine is the "[main abc1234] subject" line git prints after a commit.
//
// The branch is not always a single word. The first commit in a repository
// prints "[main (root-commit) abc1234]" and a commit on no branch prints
// "[detached HEAD abc1234]". Codex's reader matched a bare word only, so the
// first commit of every new repository came back with no hash, and that is
// the commit a fresh project is most likely to be checked against.
var commitLine = regexp.MustCompile(`\[(detached HEAD|[\w/.-]+)(?:\s+\(root-commit\))?\s+([0-9a-f]{7,40})\]`)

// CommitRef reads the branch and abbreviated hash back out of what git printed
// for a commit, or empty strings when the output does not say.
func CommitRef(output string) (branch, sha string) {
	m := commitLine.FindStringSubmatch(output)
	if m == nil {
		return "", ""
	}
	if m[1] != "detached HEAD" {
		branch = m[1]
	}
	return branch, m[2]
}

// PendingCommit is a commit command waiting to hear whether it worked.
//
// Both sources hold these while a call is outstanding and settle them when the
// result arrives, keyed by the id of the call that issued them.
type PendingCommit struct {
	// Turn is the index of the turn the commit belongs to.
	Turn int

	// Amend says the command amended rather than created.
	Amend bool

	// Dir is where it committed, empty for the session's own directory.
	Dir string
}

// Extent reports when a set of files was last written and how much they hold.
//
// From the files rather than their contents, so listing projects stays cheap
// on a large history: it says which projects are substantial and when they
// were last touched, not how many prompts are in them.
func Extent(files []string) (time.Time, int64) {
	var last time.Time
	var size int64
	for _, fp := range files {
		info, err := os.Stat(fp)
		if err != nil {
			continue
		}
		size += info.Size()
		if info.ModTime().After(last) {
			last = info.ModTime()
		}
	}
	return last, size
}

// When reads a transcript timestamp as a time in the reader's own zone.
//
// Local time, not UTC. Agents write timestamps with a Z suffix, and time.Parse
// hands those back in UTC, which is a different calendar day from the one the
// person was sitting at for a good part of every evening. A prompt typed at
// 01:57 in Asia/Calcutta is 20:27 the previous day in UTC, and the diagram
// headed it with yesterday's date.
//
// Durations are unaffected either way, so segmenting never noticed. It is the
// day a piece of work belongs to that was wrong, which is exactly what the
// reader is looking at.
func When(stamp string) time.Time {
	if stamp == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}
	}
	//nolint:gosmopolitan // deliberate: the reader's own clock is the right
	// frame for which day a piece of work belongs to.
	return t.Local()
}
