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
	"path"
	"regexp"
	"strings"
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

// NormalisePath puts a file path into a comparable form.
//
// The same file turns up written several ways across a session, because the
// drive letter changes case between records and separators differ by platform.
// Grouping by path only works once those are settled.
//
// This deliberately does not use path/filepath. A transcript written on
// Windows can be read on any machine, so backslashes have to be understood
// everywhere rather than only where the host happens to use them.
func NormalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return strings.ToLower(p)
}
