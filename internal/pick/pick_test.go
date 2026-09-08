package pick

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

var sample = []Item{
	{Label: "project-one", Detail: "19 MB, 12 days ago"},
	{Label: "project-two", Detail: "73 MB, 20 hours ago"},
	{Label: "a-longer-project-name", Detail: "113 MB, 8 days ago"},
}

func TestDrawShowsEveryRowAndOneHighlight(t *testing.T) {
	var buf bytes.Buffer
	lines := draw(&buf, "Which project?", sample, 1, detailWidth(sample), 0)

	out := buf.String()
	// Title, a blank line, the frame, the rows with a spacer between each,
	// the closing frame, a blank line and the hint.
	if want := len(sample)*2 - 1 + 6; lines != want {
		t.Errorf("used %d lines, want %d", lines, want)
	}
	for _, it := range sample {
		if !strings.Contains(out, it.Label) {
			t.Errorf("row %q is missing", it.Label)
		}
	}
	if got := strings.Count(out, onGround); got != 1 {
		t.Errorf("%d rows filled, want exactly 1", got)
	}
	if got := strings.Count(out, bulletOn); got != 1 {
		t.Errorf("%d rows carry the chosen marker, want exactly 1", got)
	}
	if got := strings.Count(out, bulletOff); got != len(sample)-1 {
		t.Errorf("%d rows carry the empty marker, want %d", got, len(sample)-1)
	}
}

// The frame only holds together if every row is exactly as wide as the border
// above it. Anything that changes a row's width shears the near edge.
func TestEveryRowMatchesTheBorderWidth(t *testing.T) {
	var buf bytes.Buffer
	draw(&buf, "Which project?", sample, 1, detailWidth(sample), 0)

	var widths []int
	for _, line := range strings.Split(buf.String(), "\n") {
		plain := strings.TrimSpace(stripEscapes(line))
		if plain == "" || !strings.ContainsAny(plain, "\u2502\u256d\u2570") {
			continue
		}
		widths = append(widths, len([]rune(plain)))
	}
	// Every row, the spacer between each pair, and the two borders.
	if want := len(sample)*2 - 1 + 2; len(widths) != want {
		t.Fatalf("found %d framed lines, want %d", len(widths), want)
	}
	for i, w := range widths {
		if w != widths[0] {
			t.Errorf("framed line %d is %d wide, the border is %d", i, w, widths[0])
		}
	}
}

// Emoji would be the obvious thing to decorate rows with, and they are the one
// thing that cannot be used: terminals draw them two columns wide while Go
// counts them as one, so the frame shears. This guards against someone adding
// them later without knowing that.
func TestRowMarkersAreSingleWidth(t *testing.T) {
	for _, r := range []string{bulletOn, bulletOff, bar, cornerTL, cornerTR, cornerBL, cornerBR, horizontal} {
		runes := []rune(r)
		if len(runes) != 1 {
			t.Errorf("%q is %d runes, want 1", r, len(runes))
		}
		if wide(runes[0]) {
			t.Errorf("%q is drawn two columns wide, which will shear the frame", r)
		}
	}
}

// wide reports whether a rune occupies two terminal columns. The ranges are
// the emoji and CJK blocks, which is where the problem lives.
func wide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // hangul
		r >= 0x2E80 && r <= 0xA4CF, // CJK
		r >= 0xAC00 && r <= 0xD7A3, // hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,   // fullwidth forms
		r >= 0x1F300 && r <= 0x1FAFF, // emoji
		r >= 0x1F900 && r <= 0x1F9FF:
		return true
	}
	return false
}

// stripEscapes measures a line as it appears rather than as it is written.
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
	clearRows(&buf, 5)
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

// The two markers have to occupy the same space, or the filled one sits wider
// than its neighbours and is clipped at the edge of its cell. They come from
// the same block of characters for exactly that reason.
func TestBothMarkersAreTheSameWidth(t *testing.T) {
	on, off := []rune(bulletOn), []rune(bulletOff)
	if len(on) != 1 || len(off) != 1 {
		t.Fatalf("markers should be one rune each, got %d and %d", len(on), len(off))
	}
	if widthClass(on[0]) != widthClass(off[0]) {
		t.Errorf("%q and %q are drawn at different widths, so the filled one will not line up",
			bulletOn, bulletOff)
	}
}

// widthClass groups a rune by how much room a terminal gives it. Characters
// from the same block agree; characters from different blocks often do not.
func widthClass(r rune) int {
	switch {
	case r >= 0x25A0 && r <= 0x25FF: // geometric shapes
		return 1
	case r >= 0x2500 && r <= 0x257F: // box drawing
		return 2
	case r >= 0x1F300: // emoji, always double width
		return 3
	}
	return 0
}

// A name longer than the column is cut rather than pushing the frame open, and
// the cut is marked so nobody mistakes it for the whole name.
func TestLongNamesAreCutNotOverflowed(t *testing.T) {
	long := strings.Repeat("long-", 30)
	items := []Item{
		{Label: long, Detail: "2 MB, just now"},
		{Label: "short", Detail: "19 MB, 12 days ago"},
	}

	var buf bytes.Buffer
	draw(&buf, "", items, 0, detailWidth(items), 0)

	for _, line := range strings.Split(buf.String(), "\n") {
		plain := stripEscapes(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		if n := len([]rune(strings.TrimSpace(plain))); n > defaultRow {
			t.Errorf("a line ran to %d columns, past the cap of %d", n, defaultRow)
		}
	}
	if !strings.Contains(buf.String(), "\u2026") {
		t.Error("a name that did not fit should be marked as cut")
	}
}

// The box grows for a longer name rather than squeezing the detail column, so
// the two never collide.
func TestBoxWidensForLongerNames(t *testing.T) {
	narrow := []Item{{Label: "a", Detail: "2 MB"}, {Label: "b", Detail: "3 MB"}}
	wide := []Item{{Label: strings.Repeat("x", 40), Detail: "2 MB"}, {Label: "b", Detail: "3 MB"}}

	if labelColumn(wide, 4) <= labelColumn(narrow, 4) {
		t.Error("a longer name should widen the label column")
	}
}

// Rows are separated so the list does not read as a solid block.
func TestRowsAreSpacedApart(t *testing.T) {
	var buf bytes.Buffer
	draw(&buf, "", sample, 0, detailWidth(sample), 0)

	blanks := 0
	for _, line := range strings.Split(buf.String(), "\n") {
		plain := stripEscapes(line)
		trimmed := strings.TrimSpace(plain)
		// A spacer is a framed line with nothing in it.
		if len(trimmed) > 1 && strings.HasPrefix(trimmed, bar) && strings.TrimSpace(strings.Trim(trimmed, bar)) == "" {
			blanks++
		}
	}
	if want := len(sample) - 1; blanks != want {
		t.Errorf("found %d spacers between %d rows, want %d", blanks, len(sample), want)
	}
}

// stripSGR removes the colour and cursor sequences so a row can be measured as
// the user sees it.
func stripSGR(s string) string {
	var out []rune
	esc := false
	for _, r := range s {
		if r == 0x1b {
			esc = true
			continue
		}
		if esc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				esc = false
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// A frame wider than the window wraps every row, and the redraw then steps
// back over fewer lines than are actually on screen, so the list overwrites
// itself. This is the picker a reader saw scattered across their terminal.
func TestDrawFitsNarrowTerminals(t *testing.T) {
	items := []Item{
		{Label: "mark_users", Detail: "701 KB, 5 days ago"},
		{Label: "members", Detail: "1 MB, 4 days ago"},
		{Label: "a-project-with-a-very-long-name-indeed", Detail: "203 KB, 5 days ago"},
	}

	for _, width := range []int{40, 50, 60, 72, 80, 120} {
		t.Run(fmt.Sprintf("cols%d", width), func(t *testing.T) {
			restore := termWidth
			termWidth = func() int { return width }
			defer func() { termWidth = restore }()

			var sb strings.Builder
			draw(&sb, "", items, 0, detailWidth(items), 0)

			for _, line := range strings.Split(sb.String(), "\n") {
				clean := stripSGR(line)
				if clean == "" {
					continue
				}
				if n := len([]rune(clean)); n > width {
					t.Errorf("row is %d columns in a %d column window: %q", n, width, clean)
				}
			}
		})
	}
}

// Every row has to be the same width or the frame's edges bend.
func TestDrawRowsAreUniform(t *testing.T) {
	restore := termWidth
	termWidth = func() int { return 46 }
	defer func() { termWidth = restore }()

	items := []Item{
		{Label: "short", Detail: "1 KB, today"},
		{Label: "a-much-longer-project-name", Detail: "701 KB, 5 days ago"},
	}
	var sb strings.Builder
	draw(&sb, "", items, 0, detailWidth(items), 0)

	seen := map[int]bool{}
	for _, line := range strings.Split(sb.String(), "\n") {
		clean := stripSGR(line)
		if strings.Contains(clean, "│") || strings.Contains(clean, "╭") || strings.Contains(clean, "╰") {
			seen[len([]rune(clean))] = true
		}
	}
	if len(seen) != 1 {
		t.Errorf("framed lines have %d different widths, want 1: %v", len(seen), seen)
	}
}

// A window with no room for a name must not produce a negative column, which
// panics the formatter rather than merely looking wrong.
func TestDrawSurvivesAbsurdlyNarrowTerminals(t *testing.T) {
	restore := termWidth
	defer func() { termWidth = restore }()

	items := []Item{{Label: "project", Detail: "701 KB, 5 days ago"}}
	for _, width := range []int{1, 5, 12, 20, 33} {
		termWidth = func() int { return width }
		var sb strings.Builder
		draw(&sb, "", items, 0, detailWidth(items), 0) // must not panic
		if sb.Len() == 0 {
			t.Errorf("width %d drew nothing", width)
		}
	}
}
