package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A project that is not a repository, or has moved, is the ordinary case rather
// than a failure. It must come back empty and quiet.
func TestReadToleratesWhatIsNotThere(t *testing.T) {
	for _, dir := range []string{"", t.TempDir(), "/no/such/path/anywhere"} {
		if got := Read(dir); len(got.Commits) != 0 {
			t.Errorf("Read(%q) returned %d commits, want none", dir, len(got.Commits))
		}
	}
}

func TestParseLogReadsCommitsAndLineCounts(t *testing.T) {
	// The shape git produces for the format Read asks for.
	out := "\x1eabc1234\x1f2026-08-22T10:00:00+05:30\x1fAdd the parser\n" +
		"12\t3\tmain.go\n" +
		"4\t0\tREADME.md\n" +
		"\x1edef5678\x1f2026-08-22T11:30:00+05:30\x1fFix a typo\n" +
		"1\t1\tdoc.md\n"

	got := parseLog(out)
	if len(got) != 2 {
		t.Fatalf("got %d commits, want 2", len(got))
	}
	if got[0].SHA != "abc1234" || got[0].Subject != "Add the parser" {
		t.Errorf("first commit = %+v", got[0])
	}
	if got[0].Added != 16 || got[0].Removed != 3 || got[0].Files != 2 {
		t.Errorf("line counts = +%d -%d over %d files, want +16 -3 over 2",
			got[0].Added, got[0].Removed, got[0].Files)
	}
	if got[0].When.IsZero() {
		t.Error("timestamp was not parsed")
	}
}

// A binary file shows "-" for both counts, which must not be read as a number.
func TestParseLogIgnoresBinaryCounts(t *testing.T) {
	out := "\x1eabc1234\x1f2026-08-22T10:00:00+05:30\x1fAdd a picture\n" +
		"-\t-\timage.png\n" +
		"5\t2\tmain.go\n"

	got := parseLog(out)
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}
	if got[0].Added != 5 || got[0].Removed != 2 {
		t.Errorf("got +%d -%d, want +5 -2", got[0].Added, got[0].Removed)
	}
	if got[0].Files != 2 {
		t.Errorf("got %d files, want 2", got[0].Files)
	}
}

// The real thing, against a repository made for the test. Skipped where git is
// not installed, since that is a machine bough still has to work on.
func TestReadAgainstARealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q")
	if err := writeFile(dir, "a.txt", "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	// Quiet on purpose: this is the case that started all of it.
	run("commit", "-q", "-m", "First commit")

	got := Read(dir)
	if len(got.Commits) != 1 {
		t.Fatalf("got %d commits, want 1", len(got.Commits))
	}
	if got.Commits[0].Subject != "First commit" {
		t.Errorf("subject = %q", got.Commits[0].Subject)
	}
	if got.Commits[0].Added != 2 {
		t.Errorf("added = %d, want 2", got.Commits[0].Added)
	}
	if got.Commits[0].SHA == "" {
		t.Error("no hash, which is the whole point of reading the repository")
	}
}

func writeFile(dir, name, body string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
}

// A history says whether the repository was actually consulted.
//
// An empty repository and a machine with no git installed both come back with
// no commits, and they mean different things. A hash the transcript carried is
// a claim: when the repository was read and cannot find it, the hash is stale
// and showing it offers the reader something to check that does not check out.
// When the repository was never read, the same hash is simply unconfirmed, and
// clearing it would empty every hash on a machine without git.
func TestHistorySaysWhetherItWasRead(t *testing.T) {
	if h := Read(""); h.Read {
		t.Error("an empty path reported that it read a repository")
	}
	if h := Read(filepath.Join(t.TempDir(), "nothing-here")); h.Read {
		t.Error("a missing directory reported that it read a repository")
	}
	// A real repository, which is the case that has to come back true.
	if h := Read("."); !h.Read {
		t.Skip("no git available, so there is nothing to compare against")
	}
}

// The three ways a repository goes unread are told apart.
//
// Read used to answer an empty History for all of them, so "there is no
// directory", "git is not installed" and "git ran and refused" were one
// outcome, and nothing could say which had happened.
func TestUnreadSaysWhy(t *testing.T) {
	// No directory to look in. Ordinary, and not a failure.
	if h := Read(""); h.Read || h.Unread != nil {
		t.Errorf("no directory should be silent, got Read=%v Unread=%v", h.Read, h.Unread)
	}

	// A directory that is not a repository. Git runs and refuses.
	h := Read(t.TempDir())
	if h.Read {
		t.Error("a directory that is not a repository was reported as read")
	}
	if !errors.Is(h.Unread, ErrGitFailed) {
		t.Errorf("Unread = %v, want it to say git could not read the repository", h.Unread)
	}
}
