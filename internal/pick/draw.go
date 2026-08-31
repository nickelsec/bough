package pick

import (
	"fmt"
	"io"
	"strings"
)

// Terminal control sequences. These are the widely supported ones, so they work
// in Windows Terminal, the macOS terminals and everything on Linux.
const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	clearLine  = "\x1b[2K"
	lineUp     = "\x1b[1A"

	dim      = "\x1b[2m"
	bold     = "\x1b[1m"
	reversed = "\x1b[7m"
	reset    = "\x1b[0m"
)

// draw renders the list and returns how many lines it used, so the next pass
// can rewrite exactly those and nothing else.
func draw(out io.Writer, title string, items []Item, selected, width, previous int) int {
	if previous > 0 {
		// Step back over what was drawn last time and overwrite it. Redrawing
		// in place keeps the list from scrolling away as the user moves.
		fmt.Fprint(out, strings.Repeat(lineUp+clearLine, previous))
	}

	lines := 0
	if title != "" {
		fmt.Fprintf(out, "%s%s%s\n", bold, title, reset)
		lines++
	}

	for i, it := range items {
		marker := "  "
		style, end := "", ""
		if i == selected {
			marker = "> "
			style, end = reversed, reset
		}
		detail := it.Detail
		if detail != "" {
			detail = fmt.Sprintf("%s%*s%s", dim, width, detail, reset)
			if i == selected {
				// Reversed text with dim inside it renders unevenly across
				// terminals, so the highlighted row keeps one style throughout.
				detail = fmt.Sprintf("%*s", width, it.Detail)
			}
		}
		fmt.Fprintf(out, "%s%s%-24s %s%s\n", style, marker, it.Label, detail, end)
		lines++
	}

	fmt.Fprintf(out, "%sarrows to move, enter to choose, esc to cancel%s\n", dim, reset)
	return lines + 1
}

// clear removes the list once a choice is made, so the chosen output starts on
// a clean screen rather than under a menu.
func clear(out io.Writer, lines int) {
	if lines > 0 {
		fmt.Fprint(out, strings.Repeat(lineUp+clearLine, lines))
	}
}

// detailWidth is how much room the right hand column needs.
func detailWidth(items []Item) int {
	w := 0
	for _, it := range items {
		if n := len(it.Detail); n > w {
			w = n
		}
	}
	return w
}
