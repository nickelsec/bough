package banner

import (
	"bytes"
	"strings"
	"testing"
)

// Every row of the mark has to be the same width, and its two colour keys have
// to line up with its glyphs, or the letters shear and the colours slide off
// the shapes they belong to.
func TestArtRowsLineUp(t *testing.T) {
	want := len([]rune(art[0].glyphs))
	for i, row := range art {
		if got := len([]rune(row.glyphs)); got != want {
			t.Errorf("row %d is %d glyphs wide, want %d", i, got, want)
		}
		if got := len(row.fg); got != want {
			t.Errorf("row %d has %d foreground keys against %d glyphs", i, got, want)
		}
		if got := len(row.bg); got != want {
			t.Errorf("row %d has %d background keys against %d glyphs", i, got, want)
		}
	}
}

// Every key used in the art has to name a real colour, or a cell silently
// loses its paint.
func TestEveryColourKeyIsKnown(t *testing.T) {
	for i, row := range art {
		for _, keys := range []string{row.fg, row.bg} {
			for j := 0; j < len(keys); j++ {
				k := keys[j]
				if k == '_' {
					continue
				}
				if _, ok := palette[k]; !ok {
					t.Errorf("row %d column %d uses key %q, which is not in the palette", i, j, string(k))
				}
			}
		}
	}
}

// The mark has to line up under the name, or it reads as two drawings stacked
// on each other rather than one thing.
func TestBoughIsCentredUnderTheName(t *testing.T) {
	width := len([]rune(art[0].glyphs))

	for i, line := range bough {
		runes := []rune(line)
		left := len(runes) - len([]rune(strings.TrimLeft(line, " ")))
		right := width - len(runes)
		if diff := left - right; diff < -2 || diff > 2 {
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

// Colour is written in runs rather than one escape per character, or the
// output balloons and slower terminals tear as it draws.
func TestColourIsWrittenInRuns(t *testing.T) {
	row := art[0]
	painted := paintRow(row, truecolor)

	escapes := strings.Count(painted, "\x1b[38")
	if escapes >= len([]rune(row.glyphs)) {
		t.Errorf("%d colour escapes for %d cells; runs are not being merged",
			escapes, len([]rune(row.glyphs)))
	}
}

// Whatever the colouring does, the glyphs themselves must come through intact.
func TestPaintingKeepsTheGlyphs(t *testing.T) {
	for i, row := range art {
		if got := stripEscapes(paintRow(row, truecolor)); got != row.glyphs {
			t.Errorf("row %d came out as %q, want %q", i, got, row.glyphs)
		}
	}
}

func TestGradientRunsDownTheMark(t *testing.T) {
	var buf bytes.Buffer
	writeWith(&buf, "", truecolor)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < len(art) {
		t.Fatalf("only %d lines came out", len(lines))
	}
	// The top of the mark is lit and the bottom is in shadow, so the two rows
	// cannot be identical.
	if lines[0] == lines[len(art)-1] {
		t.Error("the top and bottom of the mark are the same, so there is no depth")
	}
}

// A terminal without truecolor should still get something readable rather than
// a mess of unsupported escapes.
func TestSixteenColourFallback(t *testing.T) {
	var buf bytes.Buffer
	writeWith(&buf, "", basic)

	out := buf.String()
	if strings.Contains(out, "38;2;") || strings.Contains(out, "48;2;") {
		t.Error("truecolor escapes leaked into the sixteen colour output")
	}
	if !strings.Contains(out, "\x1b[33m") && !strings.Contains(out, "\x1b[32m") {
		t.Error("expected the mark in the two colours the palette falls back to")
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

// stripEscapes reads a line as it appears rather than as it is written.
func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
