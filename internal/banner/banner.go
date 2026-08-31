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

// The wordmark, drawn in block characters. The shading characters carry the
// depth, so this reads as carved rather than flat even without colour.
var wordmark = []string{
	` ██████   ██████  ██    ██  ██████  ██   ██`,
	` ██░░░██ ██░░░░██ ██    ██ ██░░░░░  ██   ██`,
	` ██████  ██    ██ ██    ██ ██  ███  ███████`,
	` ██░░░██ ██░░░░██ ██░░░░██ ██░░░██  ██░░░██`,
	` ██████   ██████   ██████   █████   ██   ██`,
}

// The bough itself, hanging under the name and centred on it, so the mark
// reads as one piece rather than two stacked things.
var bough = []string{
	`               ╭─────❧─────╮`,
	`            ❧───╯         ╰───❧`,
}

// rgb is a colour in the terminal's truecolor space.
type rgb struct{ r, g, b uint8 }

// The palette runs from parchment at the top of the letters to the green of
// new growth at the branch, so the mark reads as light falling on a tree.
var (
	top    = rgb{242, 224, 184}
	bottom = rgb{198, 190, 140}
	leaf   = rgb{134, 154, 72}
	deep   = rgb{96, 116, 54}
)

// Write draws the banner.
//
// Nothing is drawn when the output is not a terminal, so a banner never lands
// in a file or a pipe, and nothing is drawn when NO_COLOR is set beyond the
// plain text itself.
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
	for i, line := range wordmark {
		shade := blend(top, bottom, float64(i)/float64(len(wordmark)-1))
		fmt.Fprintf(w, "  %s\n", paint(line, shade, colour))
	}
	for i, line := range bough {
		// The branch darkens as it hangs, the way the underside of one does.
		shade := blend(leaf, deep, float64(i)/float64(len(bough)-1))
		fmt.Fprintf(w, "  %s\n", paint(line, shade, colour))
	}
	if tagline != "" {
		fmt.Fprintf(w, "\n  %s\n", paint(tagline, blend(bottom, leaf, 0.5), colour))
	}
	fmt.Fprintln(w)
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

// paint puts a line in colour, or leaves it alone where colour is not available.
func paint(s string, c rgb, d depth) string {
	switch d {
	case truecolor:
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", c.r, c.g, c.b, s)
	case basic:
		// Without truecolor the gradient collapses, so the whole mark takes the
		// nearest of the sixteen colours rather than banding awkwardly.
		return fmt.Sprintf("\x1b[%dm%s\x1b[0m", nearest(c), s)
	default:
		return s
	}
}

// nearest picks between the two colours the palette is built from, since a
// sixteen colour terminal has nothing closer.
func nearest(c rgb) int {
	if c.g > c.b && c.r < 180 {
		return 32 // green, for the branch
	}
	return 33 // yellow, for the name
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
