package shell

import (
	"testing"
)

// The same file appears with different drive letter casing and separators
// across a session. Grouping by file only works if those collapse together.
func TestNormalisePathCollapsesCasingAndSeparators(t *testing.T) {
	same := []string{
		`D:\proj\src\main.go`,
		`d:\proj\src\main.go`,
		`d:/proj/src/main.go`,
		`d:/proj//src/main.go`,
	}
	want := NormalisePath(same[0])
	for _, p := range same[1:] {
		if got := NormalisePath(p); got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", p, got, want)
		}
	}
	if NormalisePath("") != "" {
		t.Error("empty path should stay empty")
	}
}

// The earlier version of this used path/filepath, which splits on whatever
// separator the host machine happens to use. That passed on Windows and failed
// everywhere else, because a transcript written on Windows is still full of
// backslashes when it is read on Linux. Pinning the exact result catches that,
// where comparing two paths to each other did not.
func TestNormalisePathIsTheSameOnEveryPlatform(t *testing.T) {
	cases := map[string]string{
		`D:\proj\src\main.go`:  "d:/proj/src/main.go",
		`d:/proj//src/main.go`: "d:/proj/src/main.go",
		`/home/x/proj/main.go`: "/home/x/proj/main.go",
		`C:\Users\x\notes.md`:  "c:/users/x/notes.md",
	}
	for in, want := range cases {
		if got := NormalisePath(in); got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// Commands that merely mention committing must not be mistaken for one.
func TestCommitDetectionIsNotFooledByLookalikes(t *testing.T) {
	for _, cmd := range []string{
		"git log --oneline",
		"git commit-tree abc",
		`echo "remember to git commit later"`,
		"git status",
		"git push origin main",
		"git -c core.editor=vim log",
	} {
		if commitCall.MatchString(cmd) {
			t.Errorf("%q was read as a commit", cmd)
		}
	}
	for _, cmd := range []string{
		"git commit -q -F -",
		"cd d:/x && git add -A && git commit -m x",
		"git -C /some/dir commit -m x",
		// An identity set inline. The value is quoted and holds a space, which
		// an option pattern built on \S stops at: seven of one project's
		// thirty one commits went missing exactly here.
		`git add -A && git -c user.name="Ada Lovelace" -c user.email="a@b.com" commit -q -F -`,
		`git -c user.name='Ada Lovelace' commit -q`,
		"git --no-pager commit -m x",
		"git commit",
	} {
		if !commitCall.MatchString(cmd) {
			t.Errorf("%q was not read as a commit", cmd)
		}
	}
}

// A command that writes text is not a command that commits, even when the text
// it writes says "git commit". Writing a script or a changelog about committing
// does exactly that, and it accounted for every one of one project's two
// apparent commits above what its git log holds.
func TestTextThatMentionsCommittingIsNotACommit(t *testing.T) {
	writing := []string{
		"cat > note.sh <<'SH'\ngit commit -q -m hello\nSH",
		"cd /d/x; python - <<'PY'\ns = 'git commit -m x'\nPY",
		"cat >> CHANGELOG.md <<'MD'\nRun git commit when done.\nMD",
	}
	for _, cmd := range writing {
		if IsCommit(cmd) {
			t.Errorf("text was read as a commit: %.48q", cmd)
		}
	}

	// A real commit routinely takes its message on a heredoc, which opens
	// after the commit rather than before it.
	committing := []string{
		"git commit -q -F - <<'MSG'\nA message\nMSG",
		"cd /d/x && git add -A && git commit -F - <<'EOF'\nAnother\nEOF",
		"git commit -m short",
	}
	for _, cmd := range committing {
		if !IsCommit(cmd) {
			t.Errorf("a commit was not read as one: %.48q", cmd)
		}
	}
}

// Shell is not only pipes and semicolons. A commit can sit in the body of a
// conditional or a loop, where a keyword does the separating, and it can be a
// rehearsal that reports what it would do and then does nothing.
func TestCommitDetectionHandlesShellAndRehearsals(t *testing.T) {
	for _, cmd := range []string{
		// --dry-run prints what would happen. Nothing lands, so nothing counts.
		"git commit --dry-run -m x",
		"git add -A && git commit --dry-run",
		"git revert --no-commit HEAD",
		"git merge --no-commit topic",
		"git help commit",
	} {
		if IsCommit(cmd) {
			t.Errorf("%q was read as a commit", cmd)
		}
	}
	for _, cmd := range []string{
		"if git diff --cached --quiet; then echo none; else git commit -m x; fi",
		"for f in a b; do git commit -m $f; done",
		"(cd /d/x && git commit -m y)",
		"git commit -am x",
		// The rehearsal belongs to the command before it, not to this one.
		"git commit --dry-run && git commit -m x",
	} {
		if !IsCommit(cmd) {
			t.Errorf("%q was not read as a commit", cmd)
		}
	}
}

// The heredoc guard is the reason a script about committing is not a commit,
// and the cases above happen not to exercise it: a bare newline inside written
// text is not a separator the pattern anchors on, so they are rejected before
// the guard is reached. This one puts a real separator inside the heredoc, so
// removing the guard makes it pass as a commit.
func TestHeredocGuardIsLoadBearing(t *testing.T) {
	cmd := "cat > s.sh <<'SH'\nmake all; git commit -m x\nSH"
	if IsCommit(cmd) {
		t.Error("a commit inside written text was counted")
	}
}
