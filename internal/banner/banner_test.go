package banner

import (
	"bytes"
	"strings"
	"testing"
)

// The mark has to line up, or it reads as two separate drawings stacked on
// each other rather than one thing.
func TestBoughIsCentredUnderTheName(t *testing.T) {
	width := len([]rune(wordmark[0]))

	for i, line := range bough {
		runes := []rune(line)
		left := len(runes) - len([]rune(strings.TrimLeft(line, " ")))
		right := width - len(runes)
		if diff := left - right; diff < -1 || diff > 1 {
			t.Errorf("bough line %d sits %d off centre (%d left, %d right)", i, diff, left, right)
		}
	}
}

// The branch spreads as it hangs. If the second line is not the wider one the
// shape stops reading as a bough.
func TestBoughSpreadsDownward(t *testing.T) {
	first := len([]rune(strings.TrimSpace(bough[0])))
	second := len([]rune(strings.TrimSpace(bough[1])))
	if second <= first {
		t.Errorf("the hanging line is %d wide against %d above it; it should be wider", second, first)
	}
}

// Every row of the name has to be the same width or the block letters shear.
func TestWordmarkRowsAreEqual(t *testing.T) {
	want := len([]rune(wordmark[0]))
	for i, line := range wordmark {
		if got := len([]rune(line)); got != want {
			t.Errorf("row %d is %d wide, want %d", i, got, want)
		}
	}
}

func TestGradientRunsTopToBottom(t *testing.T) {
	var buf bytes.Buffer
	writeWith(&buf, "", truecolor)

	// The first row is the lightest and the last row of the name the darkest,
	// so the two must differ.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < len(wordmark) {
		t.Fatalf("only %d lines came out", len(lines))
	}
	if lines[0] == lines[len(wordmark)-1] {
		t.Error("the top and bottom of the name are the same colour, so there is no gradient")
	}
}

// A terminal without truecolor should still get something readable rather than
// a mess of unsupported escapes.
func TestSixteenColourFallback(t *testing.T) {
	var buf bytes.Buffer
	writeWith(&buf, "", basic)

	out := buf.String()
	if strings.Contains(out, "38;2;") {
		t.Error("truecolor escapes leaked into the sixteen colour output")
	}
	if !strings.Contains(out, "\x1b[33m") || !strings.Contains(out, "\x1b[32m") {
		t.Error("expected the name in yellow and the branch in green")
	}
}

// Some people turn colour off, and some terminals cannot do it at all.
func TestPlainOutputCarriesNoEscapes(t *testing.T) {
	var buf bytes.Buffer
	writeWith(&buf, "what did you actually build?", plain)

	if strings.Contains(buf.String(), "\x1b") {
		t.Error("plain output should have no escape sequences in it")
	}
	if !strings.Contains(buf.String(), "what did you actually build?") {
		t.Error("the tagline went missing")
	}
}

// A banner in a pipe or a file is noise, so it only goes to a screen.
func TestNothingIsDrawnAwayFromATerminal(t *testing.T) {
	var buf bytes.Buffer
	Write(&buf, "tagline")

	if buf.Len() != 0 {
		t.Errorf("expected nothing when the output is not a terminal, got:\n%s", buf.String())
	}
}
