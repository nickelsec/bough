package pick

import (
	"bytes"
	"strings"
	"testing"
)

var sample = []Item{
	{Label: "taggity", Detail: "19 MB, 12 days ago"},
	{Label: "chaff-app", Detail: "73 MB, 20 hours ago"},
	{Label: "chaff-ui", Detail: "113 MB, 8 days ago"},
}

func TestDrawShowsEveryRowAndOneHighlight(t *testing.T) {
	var buf bytes.Buffer
	lines := draw(&buf, "Which project?", sample, 1, detailWidth(sample), 0)

	out := buf.String()
	if lines != len(sample)+2 {
		t.Errorf("used %d lines, want title, %d rows and a hint", lines, len(sample))
	}
	for _, it := range sample {
		if !strings.Contains(out, it.Label) {
			t.Errorf("row %q is missing", it.Label)
		}
	}
	if got := strings.Count(out, reversed); got != 1 {
		t.Errorf("%d rows highlighted, want exactly 1", got)
	}
	if !strings.Contains(out, "> chaff-app") {
		t.Error("the marker is not on the selected row")
	}
}

// Redrawing has to step back over exactly what it wrote, or the list walks down
// the screen as the user moves through it.
func TestDrawRewritesInPlace(t *testing.T) {
	var buf bytes.Buffer
	lines := draw(&buf, "Which project?", sample, 0, detailWidth(sample), 0)

	buf.Reset()
	draw(&buf, "Which project?", sample, 1, detailWidth(sample), lines)

	if got := strings.Count(buf.String(), lineUp); got != lines {
		t.Errorf("stepped back %d lines, want %d", got, lines)
	}
}

func TestClearRemovesTheList(t *testing.T) {
	var buf bytes.Buffer
	clear(&buf, 5)
	if got := strings.Count(buf.String(), lineUp); got != 5 {
		t.Errorf("cleared %d lines, want 5", got)
	}
}

func TestReadKeyUnderstandsArrowsAndPlainKeys(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  key
	}{
		{"up arrow", []byte{27, '[', 'A'}, keyUp},
		{"down arrow", []byte{27, '[', 'B'}, keyDown},
		{"home", []byte{27, '[', 'H'}, keyHome},
		{"end", []byte{27, '[', 'F'}, keyEnd},
		{"application up", []byte{27, 'O', 'A'}, keyUp},
		{"enter", []byte{13}, keyEnter},
		{"newline", []byte{10}, keyEnter},
		{"escape alone", []byte{27}, keyCancel},
		{"ctrl-c", []byte{3}, keyCancel},
		{"q", []byte{'q'}, keyCancel},
		{"vim down", []byte{'j'}, keyDown},
		{"vim up", []byte{'k'}, keyUp},
		{"anything else", []byte{'z'}, keyNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.bytes); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// Output being piped somewhere should not mean losing the choice, so the same
// question is asked as a plain numbered list.
func TestNumberedFallback(t *testing.T) {
	var out bytes.Buffer
	i, err := numbered(&out, strings.NewReader("2\n"), "Which project?", sample)
	if err != nil {
		t.Fatal(err)
	}
	if i != 1 {
		t.Errorf("chose %d, want the second item", i)
	}
	for _, it := range sample {
		if !strings.Contains(out.String(), it.Label) {
			t.Errorf("row %q is missing from the fallback", it.Label)
		}
	}
}

func TestNumberedRejectsNonsense(t *testing.T) {
	for _, in := range []string{"0\n", "99\n", "banana\n"} {
		var out bytes.Buffer
		if _, err := numbered(&out, strings.NewReader(in), "pick", sample); err == nil {
			t.Errorf("input %q should not be accepted", strings.TrimSpace(in))
		}
	}
}

// One option is not a choice, so it should not be presented as one.
func TestChooseSkipsASingleOption(t *testing.T) {
	i, err := Choose("Which project?", sample[:1])
	if err != nil {
		t.Fatal(err)
	}
	if i != 0 {
		t.Errorf("got %d, want 0", i)
	}
}

func TestChooseOnNothing(t *testing.T) {
	if _, err := Choose("pick", nil); err == nil {
		t.Error("expected an error when there is nothing to choose from")
	}
}
