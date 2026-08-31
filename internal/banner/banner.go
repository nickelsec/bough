// Package banner draws the mark bough opens with.
//
// It is the first thing anyone sees, so it gets the same care as the rest: it
// degrades rather than breaking, and it stays out of the way of anything being
// piped somewhere.
package banner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// The bough hanging under the name, centred on it and spreading as it falls,
// so the two read as one drawing.
var bough = []string{
	`               ╭─────❧─────╮`,
	`            ❧───╯         ╰───❧`,
}

// rgb is a colour in the terminal's truecolor space.
type rgb struct{ r, g, b uint8 }

// The palette. Light falls from the top left, so the lit faces of the letters
// are parchment and the bodies are the olive and green the picker frame and
// the bough are drawn in.
var palette = map[byte]rgb{
	'L': {242, 224, 184}, // lit face
	'S': {170, 162, 118}, // the dry olive it falls away to
	'B': {150, 172, 84},  // leaf green
	'D': {74, 92, 42},    // the deeper green beneath
}

// leaf and deep colour the bough, which is drawn plainly rather than per cell.
var (
	leaf = rgb{134, 154, 72}
	deep = rgb{96, 116, 54}
)

// Write draws the banner.
//
// Nothing is drawn when the output is not a terminal, so a banner never lands
// in a file or a pipe.
func Write(w io.Writer, tagline string) {
	if !isTerminal(w) {
		return
	}
	WriteTo(w, tagline)
}

// WriteTo draws the banner regardless of where it is going. Write is the one
// to reach for; this exists so the mark can be captured and tested.
func WriteTo(w io.Writer, tagline string) {
	writeWith(w, tagline, colourDepth())
}

// writeWith draws the mark at a stated colour depth.
func writeWith(w io.Writer, tagline string, colour depth) {
	fmt.Fprintln(w)
	for _, row := range art {
		fmt.Fprintf(w, "  %s\n", paintRow(row, colour))
	}
	for i, line := range bough {
		// The branch darkens as it hangs, the way the underside of one does.
		shade := blend(leaf, deep, float64(i)/float64(len(bough)-1))
		fmt.Fprintf(w, "  %s\n", paint(line, shade, colour))
	}
	if tagline != "" {
		fmt.Fprintf(w, "\n  %s\n", paint(tagline, palette['S'], colour))
	}
	fmt.Fprintln(w)
}

// paintRow colours a line of the wordmark cell by cell.
//
// Runs of cells sharing a colour are emitted together rather than one escape
// sequence per character, which keeps the output to a fraction of the size and
// stops slower terminals from tearing as it draws.
func paintRow(row artRow, colour depth) string {
	if colour == plain {
		return row.glyphs
	}

	glyphs := []rune(row.glyphs)
	var b strings.Builder
	var openFG, openBG byte

	for i, g := range glyphs {
		fg, bg := key(row.fg, i), key(row.bg, i)
		if fg != openFG || bg != openBG {
			if openFG != 0 || openBG != 0 {
				b.WriteString(reset)
			}
			b.WriteString(style(fg, bg, colour))
			openFG, openBG = fg, bg
		}
		b.WriteRune(g)
	}
	if openFG != 0 || openBG != 0 {
		b.WriteString(reset)
	}
	return b.String()
}

// key reads the colour for one cell, treating a short line as empty rather
// than reaching past its end.
func key(s string, i int) byte {
	if i >= len(s) {
		return '_'
	}
	return s[i]
}

// style is the escape sequence for one pair of colours.
func style(fg, bg byte, colour depth) string {
	var b strings.Builder
	if c, ok := palette[fg]; ok {
		b.WriteString(ink(c, false, colour))
	}
	if c, ok := palette[bg]; ok {
		b.WriteString(ink(c, true, colour))
	}
	return b.String()
}

// ink writes a colour, as foreground or as background.
func ink(c rgb, background bool, colour depth) string {
	layer := 38
	if background {
		layer = 48
	}
	if colour == truecolor {
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", layer, c.r, c.g, c.b)
	}
	// Sixteen colours cannot hold the palette, so only the foreground is set
	// and the backgrounds are dropped. Filling cells with an approximate
	// colour looks worse than leaving them alone.
	if background {
		return ""
	}
	return fmt.Sprintf("\x1b[%dm", nearest(c))
}

const reset = "\x1b[0m"

// paint puts a whole line in one colour.
func paint(s string, c rgb, d depth) string {
	if d == plain {
		return s
	}
	return ink(c, false, d) + s + reset
}

// blend mixes two colours, with t running from 0 at the first to 1 at the second.
func blend(a, b rgb, t float64) rgb {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return rgb{mix(a.r, b.r), mix(a.g, b.g), mix(a.b, b.b)}
}

// depth is how much colour a terminal can show.
type depth int

const (
	plain depth = iota
	basic
	truecolor
)

// nearest picks between the two colours the palette is built from, since a
// sixteen colour terminal has nothing closer.
func nearest(c rgb) int {
	if c.g > c.b && c.r < 200 {
		return 32 // green, for the body of the letters
	}
	return 33 // yellow, for the lit faces
}

// colourDepth works out what the terminal can show.
func colourDepth() depth {
	if os.Getenv("NO_COLOR") != "" {
		return plain
	}
	switch os.Getenv("COLORTERM") {
	case "truecolor", "24bit":
		return truecolor
	}
	t := os.Getenv("TERM")
	switch {
	case strings.Contains(t, "256color"), strings.Contains(t, "truecolor"):
		return truecolor
	case t == "dumb":
		return plain
	case t == "":
		// Windows Terminal and the modern console host do truecolor without
		// setting TERM at all, and this is the common case there.
		if os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" {
			return truecolor
		}
		return plain
	default:
		return basic
	}
}

// isTerminal reports whether this is a screen rather than a file or a pipe.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
