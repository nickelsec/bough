package metrics

import (
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

func turn(min int, edits ...string) agent.Turn {
	t := agent.Turn{
		At:    time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute),
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
	}
	for _, f := range edits {
		t.Files[f]++
		t.Edits[f]++
	}
	return t
}

func TestSummariseCountsWork(t *testing.T) {
	turns := []agent.Turn{
		turn(0, "a.go", "b.go"),
		turn(10, "a.go"),
		turn(20, "a.go"),
	}
	turns[1].Errors = 2
	turns[0].Tools["Edit"] = 3

	s := Summarise(turns)

	if s.Turns != 3 {
		t.Errorf("turns = %d, want 3", s.Turns)
	}
	if s.Edits != 4 {
		t.Errorf("edits = %d, want 4", s.Edits)
	}
	if s.Files != 2 {
		t.Errorf("files = %d, want 2", s.Files)
	}
	if s.Errors != 2 {
		t.Errorf("errors = %d, want 2", s.Errors)
	}
	if s.Churn != 3 || s.ChurnFile != "a.go" {
		t.Errorf("churn = %d on %q, want 3 on a.go", s.Churn, s.ChurnFile)
	}
	if s.Span != 20*time.Minute {
		t.Errorf("span = %v, want 20m", s.Span)
	}
	if s.Tools["Edit"] != 3 {
		t.Errorf("tools = %v", s.Tools)
	}
}

// Span counts the pauses, active time does not. A task left open overnight is
// not a task that took all night.
func TestActiveTimeExcludesBreaks(t *testing.T) {
	turns := []agent.Turn{
		turn(0),
		turn(5),
		turn(600), // came back the next morning
		turn(605),
	}
	s := Summarise(turns)

	if s.Span != 605*time.Minute {
		t.Errorf("span = %v, want the whole window", s.Span)
	}
	if s.Active != 10*time.Minute {
		t.Errorf("active = %v, want 10m of real work", s.Active)
	}
}

// Ordering has to be stable, or the same history renders differently each run.
func TestTopFilesIsDeterministic(t *testing.T) {
	turns := []agent.Turn{turn(0, "b.go", "a.go", "c.go")}
	first := Summarise(turns).TopFiles
	for i := 0; i < 20; i++ {
		got := Summarise(turns).TopFiles
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("ordering changed between runs: %v then %v", first, got)
			}
		}
	}
	if first[0].Path != "a.go" {
		t.Errorf("ties should break on path, got %v", first)
	}
}

func TestSummariseHandlesNothing(t *testing.T) {
	s := Summarise(nil)
	if s.Turns != 0 || s.Struggle() != 0 {
		t.Errorf("empty input should give an empty summary, got %+v", s)
	}
}

// Rewriting one file over and over should read as harder than moving steadily
// through many files, even when the second run is larger.
func TestStruggleRanksChurnAboveVolume(t *testing.T) {
	var stuck []agent.Turn
	for i := 0; i < 12; i++ {
		stuck = append(stuck, turn(i*3, "stubborn.go"))
	}

	var smooth []agent.Turn
	for i := 0; i < 12; i++ {
		smooth = append(smooth, turn(i*3, string(rune('a'+i))+".go"))
	}

	hard := Summarise(stuck).Struggle()
	easy := Summarise(smooth).Struggle()

	if hard <= easy {
		t.Errorf("churn %.2f should rate harder than steady progress %.2f", hard, easy)
	}
	if hard < 0 || hard > 1 || easy < 0 || easy > 1 {
		t.Errorf("scores out of range: %.2f and %.2f", hard, easy)
	}
}

// Errors are a weak signal on their own, so they must not dominate the score.
func TestStruggleDoesNotHingeOnErrors(t *testing.T) {
	clean := []agent.Turn{turn(0, "a.go"), turn(5, "a.go")}
	noisy := []agent.Turn{turn(0, "a.go"), turn(5, "a.go")}
	noisy[0].Errors = 5
	noisy[1].Errors = 5

	if d := Summarise(noisy).Struggle() - Summarise(clean).Struggle(); d > 0.2 {
		t.Errorf("errors moved the score by %.2f, more than they should", d)
	}
}

// The agent rewrites its own plan and memory constantly. Counting that as
// effort put a scratchpad at the top of the hardest work on every project.
func TestAmbientFilesDoNotCountAsChurn(t *testing.T) {
	var turns []agent.Turn
	for i := 0; i < 20; i++ {
		turns = append(turns, turn(i*3, "/home/x/.claude/plans/some-session.md"))
	}
	turns = append(turns, turn(70, "real.go"), turn(73, "real.go"))

	s := Summarise(turns)

	if s.ChurnFile != "real.go" {
		t.Errorf("churn file = %q, want the user's own file", s.ChurnFile)
	}
	if s.Churn != 2 {
		t.Errorf("churn = %d, want 2", s.Churn)
	}
	// The edits still happened, so they are still reported, just separately.
	if s.Edits != 22 || s.AmbientEdits != 20 {
		t.Errorf("edits = %d with %d ambient, want 22 and 20", s.Edits, s.AmbientEdits)
	}
}

func TestAmbient(t *testing.T) {
	ambient := []string{
		"/home/x/.claude/plans/session.md",
		`C:\Users\x\.claude\projects\p\memory\notes.md`,
		"/proj/CHANGELOG.md",
		"/proj/memory.md",
		"/proj/Cargo.lock",
		"/proj/CLAUDE.md",
	}
	for _, p := range ambient {
		if !Ambient(p) {
			t.Errorf("Ambient(%q) = false, want true", p)
		}
	}

	real := []string{
		"/proj/src/main.go",
		"/proj/ui/hero.css",
		"/proj/Cargo.toml",
		"/proj/docs/format.md",
	}
	for _, p := range real {
		if Ambient(p) {
			t.Errorf("Ambient(%q) = true, want false", p)
		}
	}
}
