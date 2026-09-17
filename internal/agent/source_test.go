package agent

import "testing"

// The two spellings of a Windows drive are one place.
//
// A unix style shell writes "/d/work/site" where the transcript elsewhere says
// "d:/work/site", and one project's commits arrived as both inside a single
// session. There were two normalisers in the tree doing this differently, and
// the one the agents called did not fold the drive at all, so the same
// directory compared as two and real work was thrown away as belonging
// somewhere else.
//
// This is asked of SamePath rather than of NormalisePath, because "/d/work" is
// only a drive on Windows and is an ordinary directory anywhere else. Deciding
// they match is safe; rewriting one into the other is not.
func TestSamePathFoldsBothDriveSpellings(t *testing.T) {
	for _, in := range []string{
		"d:/work/site",
		"/d/work/site",
		`D:\work\site`,
		"D:/Work/Site/",
		"d:/work/./site",
	} {
		if !SamePath(in, "d:/work/site") {
			t.Errorf("SamePath(%q, \"d:/work/site\") = false, want true", in)
		}
	}
}

// Folding the two drive spellings must not make every unix path match every
// other one that happens to share a first letter.
func TestSamePathDoesNotOverreach(t *testing.T) {
	for _, pair := range [][2]string{
		{"/w/app", "/x/app"},
		{"/home/u/app", "/home/u/other"},
		{"/d/work/site", "/d/work/other"},
		{"/home/u/App", "/home/u/app"},
	} {
		if SamePath(pair[0], pair[1]) {
			t.Errorf("SamePath(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

// A relative path stays relative. Only an absolute one says where it starts.
func TestNormalisePathLeavesRelativePathsAlone(t *testing.T) {
	for in, want := range map[string]string{
		"internal":     "internal",
		"./internal":   "internal",
		"internal/foo": "internal/foo",
		"":             "",
	} {
		if got := NormalisePath(in); got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

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
		// Only the drive letter is folded. The rest of a Windows path keeps the
		// case it was written in, because this is the spelling shown in the top
		// files list, and Windows users do not name their directories in lower
		// case. Deciding that two spellings are one file is SamePath's job, and
		// it folds case when a drive says case does not matter.
		`C:\Users\x\notes.md`: "c:/Users/x/notes.md",
	}
	for in, want := range cases {
		if got := NormalisePath(in); got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A unix path is not a Windows one, and must not be folded as though it were.
//
// The rewrite that turns "/d/work" into "d:/work" was being applied to every
// path with a single letter first component, so "/w/app/one.go" was shown to
// the reader as "w:/app/one.go". Upstream's own patch test used "/w/app"
// paths, and passed only because it normalised what it expected as well as
// what it got, so both sides moved together.
func TestNormalisePathLeavesUnixPathsAlone(t *testing.T) {
	for _, in := range []string{
		"/w/app/one.go",
		"/d/work/site",
		"/home/u/app",
		"/usr/bin",
		"/x",
	} {
		if got := NormalisePath(in); got != in {
			t.Errorf("NormalisePath(%q) = %q, want it unchanged", in, got)
		}
	}
}

// Two directories differing only in case are two directories where the
// filesystem says they are.
//
// Codex groups projects by the normalised working directory. Folding case into
// every path merged "/home/u/App" and "/home/u/app" into one project, read only
// whichever spelling was seen first from git, and then cleared the commit
// hashes of everything from the other one as unreachable.
func TestNormalisePathKeepsUnixCaseApart(t *testing.T) {
	if NormalisePath("/home/u/App") == NormalisePath("/home/u/app") {
		t.Error("two unix directories differing only in case were merged into one")
	}
}

// Normalising is idempotent, because normalised paths get joined and passed on
// and normalised again. "//d/x" cleaned to "/d/x", which then folded to "d:/x",
// so one path compared unequal to itself depending on how many times it had
// been through.
func TestNormalisePathIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"//d/x", "/d/x", `D:\x`, "d:/x", "/home/u/app", "/w/app", "", "internal/foo",
	} {
		once := NormalisePath(in)
		if twice := NormalisePath(once); twice != once {
			t.Errorf("NormalisePath(%q) = %q, but again = %q", in, once, twice)
		}
	}
}

// Windows does not care about case, so two spellings of one Windows path are
// one file even though NormalisePath leaves the case it was given.
func TestSamePathFoldsCaseOnWindowsPaths(t *testing.T) {
	if !SamePath(`C:\Users\x\notes.md`, "c:/users/x/notes.md") {
		t.Error("two spellings of one Windows path did not match")
	}
}
