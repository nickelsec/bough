package segment

import (
	"testing"
	"time"

	"github.com/nickelsec/boughs/internal/agent"
)

// at builds a turn at a given minute offset, with optional files.
func at(min int, text string, files ...string) agent.Turn {
	t := agent.Turn{
		At:    time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute),
		Text:  text,
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
	}
	for _, f := range files {
		t.Files[f]++
		t.Edits[f]++
	}
	return t
}

const longA = "rewrite the checkout validation so the discount code is applied before tax is calculated"
const longB = "the pdf export renders page one blank whenever the document has an embedded font in it"

func TestSplitCutsOnLongPause(t *testing.T) {
	turns := []agent.Turn{
		at(0, longA),
		at(5, "keep going"),
		at(300, longB),
	}
	tasks := Split(turns, DefaultOptions())

	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if len(tasks[0].Turns) != 2 {
		t.Errorf("first task has %d turns, want 2", len(tasks[0].Turns))
	}
	if len(tasks[1].Reasons) == 0 || tasks[1].Reasons[0] != ReasonGap {
		t.Errorf("reasons = %v, want the pause recorded", tasks[1].Reasons)
	}
}

// A short pause is how normal work feels. Cutting there would shred everything.
func TestSplitIgnoresShortPause(t *testing.T) {
	turns := []agent.Turn{at(0, longA), at(10, "keep going"), at(20, "now the tax line")}
	if got := len(Split(turns, DefaultOptions())); got != 1 {
		t.Errorf("got %d tasks, want 1", got)
	}
}

// Compaction during continuous work means the context filled up, not that the
// user changed subject, so it needs a pause beside it before it counts.
func TestSplitTreatsCompactionWithPauseAsBoundary(t *testing.T) {
	withPause := []agent.Turn{at(0, longA), at(40, longB)}
	withPause[0].SegmentHint = true
	if got := len(Split(withPause, DefaultOptions())); got != 2 {
		t.Errorf("compaction after a pause: got %d tasks, want 2", got)
	}

	continuous := []agent.Turn{at(0, longA), at(2, longB)}
	continuous[0].SegmentHint = true
	if got := len(Split(continuous, DefaultOptions())); got != 1 {
		t.Errorf("compaction mid-flow: got %d tasks, want 1", got)
	}
}

// This is the failure that shaped the design. Cutting whenever the file set
// moved turned one afternoon of styling into dozens of separate tasks, so a
// content signal on its own is not allowed to cut.
func TestSplitDoesNotShatterOnFilesAlone(t *testing.T) {
	turns := []agent.Turn{
		at(0, longA, "a.css", "b.css", "c.css"),
		at(5, "make the padding smaller on the nav", "d.css", "e.css", "f.css"),
		at(10, "now the logo is too big next to it", "g.css", "h.css", "i.css"),
	}
	if got := len(Split(turns, DefaultOptions())); got != 1 {
		t.Errorf("got %d tasks, want 1; a single weak signal must not cut", got)
	}
}

// Two weak signals agreeing is enough, since the prompt is both about something
// else and working somewhere else.
func TestSplitCutsWhenWeakSignalsAgree(t *testing.T) {
	turns := []agent.Turn{
		at(0, longA, "checkout.go", "tax.go", "discount.go"),
		at(5, "and round the total to two places", "checkout.go", "tax.go", "discount.go"),
		at(40, longB, "pdf.go", "font.go", "render.go"),
	}
	tasks := Split(turns, DefaultOptions())
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if len(tasks[1].Reasons) < 2 {
		t.Errorf("reasons = %v, want both signals recorded", tasks[1].Reasons)
	}
}

// Short follow-ups carry no evidence about subject, so they never trigger a cut
// no matter what they touch.
func TestSplitIgnoresContinuationPrompts(t *testing.T) {
	turns := []agent.Turn{
		at(0, longA, "checkout.go", "tax.go", "discount.go"),
		at(5, "go for it", "totally.go", "unrelated.go", "elsewhere.go"),
		at(10, "commit", "another.go", "different.go", "file.go"),
	}
	if got := len(Split(turns, DefaultOptions())); got != 1 {
		t.Errorf("got %d tasks, want 1", got)
	}
}

func TestSplitHandlesEmptyAndSingle(t *testing.T) {
	if Split(nil, DefaultOptions()) != nil {
		t.Error("no turns should give no tasks")
	}
	if got := len(Split([]agent.Turn{at(0, "hello")}, DefaultOptions())); got != 1 {
		t.Errorf("got %d tasks, want 1", got)
	}
}

// History with no timestamps still has to segment rather than crash or run on.
func TestSplitToleratesMissingTimestamps(t *testing.T) {
	turns := []agent.Turn{
		{Text: longA, Files: map[string]int{}, Edits: map[string]int{}},
		{Text: longB, Files: map[string]int{}, Edits: map[string]int{}},
	}
	if got := len(Split(turns, DefaultOptions())); got < 1 {
		t.Errorf("got %d tasks, want at least 1", got)
	}
}

func TestJaccard(t *testing.T) {
	a := map[string]int{"x": 1, "y": 1}
	b := map[string]int{"y": 1, "z": 1}
	if got := jaccard(a, b); got != 1.0/3.0 {
		t.Errorf("jaccard = %v, want 1/3", got)
	}
	if got := jaccard(a, nil); got != 0 {
		t.Errorf("jaccard with empty = %v, want 0", got)
	}
}

func TestWordsOfDropsNoise(t *testing.T) {
	got := wordsOf("Please can you go and fix the checkout validation for me")
	want := map[string]bool{"checkout": true, "validation": true}
	for _, w := range got {
		if !want[w] {
			t.Errorf("kept %q, expected only %v", w, keys(want))
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, keys(want))
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
