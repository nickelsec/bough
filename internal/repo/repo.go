// Package repo reads the git history of a project bough is describing.
//
// The transcript records that a commit happened, and usually not much else.
// Claude Code recovers the hash by reading what git printed, so a commit made
// with -q leaves no hash at all: across the corpus this was built against, none
// of 88 quiet commits carried one, against 53 of 57 ordinary ones. Working from
// the transcript alone therefore found 4 of one project's 31 commits.
//
// The repository has all of it, sitting on disk at the path the transcript
// already names. This reads it, so a commit the agent made can be matched to
// the commit that is actually in the history.
//
// Two rules hold here. Nothing is written, ever: every command below is a read.
// And a project with no repository, or one that has moved, is not an error. It
// simply means there is nothing to add, and the transcript stands on its own.
package repo

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Commit is one commit as the repository has it now.
type Commit struct {
	SHA     string
	Subject string
	When    time.Time

	// Added and Removed are lines changed, which say how big the commit was
	// where a count of files does not.
	Added   int
	Removed int
	Files   int
}

// History is a project's commits, newest first.
type History struct {
	Commits []Commit

	// Read says the repository was actually consulted. A history with no
	// commits in it is ambiguous otherwise: an empty repository and a machine
	// without git look identical, and they mean different things for a hash
	// the transcript carried.
	Read bool

	// Unread says why, when Read is false. Three different things used to
	// arrive as the same empty history: there was no directory to look in, git
	// is not installed, or git ran and failed. The first is ordinary and the
	// other two are worth telling somebody about, and nothing could tell them
	// apart.
	Unread error
}

// Read returns the commit history of the repository at dir.
//
// A missing directory, a directory that is not a repository, and a git that is
// not installed all return an empty history and no error. None of those are
// problems the user needs telling about: they only mean the drawing keeps to
// what the transcript knew.
//
// They do change what a hash means, though, which is why History says whether
// it managed to read anything. A hash the transcript carried is a claim until
// the repository confirms it; when the repository was never read, that claim
// stands unchecked, and clearing the ones it cannot find would throw away
// every hash on a machine with no git installed.
func Read(dir string) History {
	if dir == "" {
		// Nothing to read, which is not a failure: plenty of work happens
		// outside a repository.
		return History{}
	}
	dir = filepath.Clean(dir)

	// Git missing is worth saying plainly. It is the one cause a reader can do
	// something about, and it makes every hash in the output unverified.
	if _, err := exec.LookPath("git"); err != nil {
		return History{Unread: ErrNoGit}
	}

	// --numstat gives lines added and removed per file. The record separator is
	// a character that cannot appear in a subject line.
	//
	// The limit is there so a long lived repository cannot make the tool sit
	// and think. Nobody's agent history reaches back far enough for the older
	// commits to be matchable anyway.
	out, err := run(dir, "log", "--no-merges", "--numstat",
		"--max-count=5000", "--format=%x1e%h%x1f%aI%x1f%s")
	if err != nil {
		// Ran and refused. Usually because the directory is not a repository,
		// which is ordinary, but it also covers a repository too broken to
		// read, and the message says which.
		return History{Unread: fmt.Errorf("%w: %w", ErrGitFailed, err)}
	}
	return History{Commits: parseLog(out), Read: true}
}

// ErrNoGit is returned when git is not installed, and ErrGitFailed when it ran
// and would not answer. Neither is returned when there was simply no directory
// to look in, which is not a failure at all.
var (
	ErrNoGit     = errors.New("git is not installed")
	ErrGitFailed = errors.New("git could not read the repository")
)

// parseLog turns git's output into commits.
func parseLog(out string) []Commit {
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		lines := strings.Split(rec, "\n")
		head := strings.Split(lines[0], "\x1f")
		if len(head) < 3 {
			continue
		}
		c := Commit{SHA: head[0], Subject: head[2]}
		if t, err := time.Parse(time.RFC3339, head[1]); err == nil {
			c.When = t
		}
		for _, l := range lines[1:] {
			if l == "" {
				continue
			}
			// added \t removed \t path, where a binary file gives "-" for both.
			parts := strings.SplitN(l, "\t", 3)
			if len(parts) < 3 {
				continue
			}
			c.Files++
			c.Added += atoi(parts[0])
			c.Removed += atoi(parts[1])
		}
		commits = append(commits, c)
	}
	return commits
}

// Sorted returns the commits oldest first, which is the order work happened in.
func (h *History) Sorted() []Commit {
	out := append([]Commit(nil), h.Commits...)
	sort.Slice(out, func(i, j int) bool { return out[i].When.Before(out[j].When) })
	return out
}

func run(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	//#nosec G204 // the arguments are fixed above; only the directory varies,
	// and it comes from the transcript rather than from anything a caller typed.
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
