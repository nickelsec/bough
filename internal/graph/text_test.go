package graph

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// A reading that did not consult the repository says so.
//
// A commit hash means two things. Read the repository and a hash is one it
// confirmed, with anything stale cleared. Do not read it and every hash is
// whatever the transcript claimed, unchecked. "Not a git repository", "git is
// not installed" and --no-repo all arrive at the same place, and without a
// word about it they look exactly like a clean confirmation.
func TestTextSaysWhenTheRepositoryWasNotRead(t *testing.T) {
	g := Graph{
		Project: Project{Name: "x", RepoRead: false},
		Totals:  Stats{Commits: []Commit{{SHA: "abc1234"}}},
	}
	var b bytes.Buffer
	if err := WriteText(&b, g, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "the repository was not read") {
		t.Errorf("nothing said the repository went unread:\n%s", b.String())
	}

	g.Project.RepoRead = true
	b.Reset()
	if err := WriteText(&b, g, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "the repository was not read") {
		t.Errorf("a confirmed reading claimed it was not read:\n%s", b.String())
	}
}

// A task charged for context but credited with no output would divide by
// zero. It sounds impossible and is not: a prompt cancelled before the reply
// finished is charged for what it read and produces nothing.
func TestTextSurvivesCacheWithoutOutput(t *testing.T) {
	p, s := sample()
	s[0].Turns[0].Tokens = agent.Tokens{CacheRead: 5000}

	var b bytes.Buffer
	if err := WriteText(&b, Build(p, s, Options{Now: fixedNow}), false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "x context") {
		t.Error("printed a ratio against no output")
	}
}
