package graph

import (
	"bytes"
	"strings"
	"testing"
	"time"

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

// Each sitting's second line opens with the time it began, in the column under
// its date. Two sittings on one date otherwise read the same. Tasks carry their
// time at the keyboard, and leave it out when there was none.
func TestTextSaysWhenEachSittingStartedAndForHowLong(t *testing.T) {
	// A fixed zone, since the times are printed in whatever zone they carry,
	// which for real history is the reader's own.
	zone := time.FixedZone("IST", 5*3600+1800)
	at := func(h, m int) time.Time { return time.Date(2026, 9, 25, h, m, 0, 0, zone) }
	turn := func(when time.Time, text string) agent.Turn {
		return agent.Turn{At: when, Text: text, Tools: map[string]int{}, Files: map[string]int{},
			Edits: map[string]int{}, Lines: map[string]int{}}
	}
	turns := []agent.Turn{
		turn(at(0, 5), "the first attempt, which went nowhere"),
		turn(at(17, 21), "make it even worse"),
		turn(at(17, 24), "worse again"),
		turn(at(17, 28), "and a favicon"),
	}
	opt := DefaultOptions()
	opt.Now = func() time.Time { return at(23, 0) }
	g := Build(agent.Project{Name: "p", Path: "/p"}, []agent.Session{{ID: "s", Turns: turns}}, opt)
	if len(g.Goals) != 2 {
		t.Fatalf("got %d sittings, want the two a long break makes", len(g.Goals))
	}

	var b strings.Builder
	if err := WriteText(&b, g, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"\n00:05          1 task, 1 prompt, a moment\n",
		"\n17:21          ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\n    3 prompts, 7 minutes") {
		t.Errorf("the task's keyboard time is missing from its own line:\n%s", out)
	}
	// A one-prompt task has no time between prompts to count, and says
	// nothing rather than "a moment".
	if strings.Contains(out, "    1 prompt, a moment") {
		t.Errorf("a task with no keyboard time said so:\n%s", out)
	}
}

func TestStartedAtIsBlankWithoutATime(t *testing.T) {
	if got := startedAt(Stats{}); got != "" {
		t.Errorf("startedAt of nothing = %q", got)
	}
}
