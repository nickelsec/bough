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

	dim   = "\x1b[2m"
	bold  = "\x1b[1m"
	reset = "\x1b[0m"

	// The frame takes the olive the mark ends on, so the two belong together.
	// The selected row turns that colour into its ground.
	frame    = "\x1b[38;2;134;154;72m"
	onGround = "\x1b[48;2;134;154;72m\x1b[38;2;24;28;16m\x1b[1m"
)

// The pieces the frame is built from.
//
// Every one is a single column wide in every terminal, which emoji are not:
// most render double width, a few render single, and terminals disagree about
// which. That difference shears the right hand edge of a frame, and a picker
// that looks broken on someone else's machine is worse than a plain one that
// always lines up.
const (
	cornerTL   = "╭"
	cornerTR   = "╮"
	cornerBL   = "╰"
	cornerBR   = "╯"
	horizontal = "─"
	bar        = "│"

	// The markers standing in for a radio button.
	//
	// Both are from the same geometric block and share a width class, so the
	// filled one sits exactly where the empty one does. Mixing classes, which
	// an obvious choice like the fisheye would, leaves the filled marker wider
	// than its neighbours and clipped at the edge of its cell.
	bulletOn  = "●"
	bulletOff = "○"
)

// Layout. The label column is wide enough for a long project name, and the
// detail column is right aligned against the far edge, so the two read as
// separate columns rather than as one run of text.
const (
	labelWidth = 30
	gutter     = 4

	// maxRow is the widest the whole framed row may be, borders included, so
	// the picker still fits an eighty column terminal.
	maxRow = 78
)

// hint sits below the frame, with a blank line between them.
const hint = "↑↓ move    ↵ choose    esc cancel"

// draw renders the list and returns how many lines it used, so the next pass
// can rewrite exactly those and nothing else.
func draw(out io.Writer, title string, items []Item, selected, detail, previous int) int {
	if previous > 0 {
		// Step back over what was drawn last time and overwrite it. Redrawing
		// in place keeps the list from scrolling away as the user moves.
		fmt.Fprint(out, strings.Repeat(lineUp+clearLine, previous))
	}

	label := labelColumn(items, detail)
	inner := label + detail + gutter + 5
	lines := 0

	if title != "" {
		fmt.Fprintf(out, "  %s%s%s\n\n", bold, title, reset)
		lines += 2
	}

	fmt.Fprintf(out, "  %s%s%s%s%s\n", frame, cornerTL, strings.Repeat(horizontal, inner), cornerTR, reset)
	lines++

	// A blank framed line above and below each row, so the list breathes
	// rather than reading as a solid block of text.
	for i, it := range items {
		if i > 0 {
			fmt.Fprintf(out, "  %s\n", blankRow(inner))
			lines++
		}
		fmt.Fprintf(out, "  %s\n", row(it, i == selected, label, detail, inner))
		lines++
	}

	fmt.Fprintf(out, "  %s%s%s%s%s\n\n", frame, cornerBL, strings.Repeat(horizontal, inner), cornerBR, reset)
	lines += 2

	fmt.Fprintf(out, "  %s%s%s\n", dim, hint, reset)
	return lines + 1
}

// labelColumn is how much room the names need.
//
// The column grows for a longer name so the detail beside it is never
// squeezed, and stops once the whole row would outgrow the terminal. Past
// that point names are cut instead.
func labelColumn(items []Item, detail int) int {
	w := 0
	for _, it := range items {
		if n := len([]rune(it.Label)); n > w {
			w = n
		}
	}
	if w < labelWidth {
		w = labelWidth
	}
	// Everything the row spends besides the name: two frame edges, the marker
	// and the spaces around it, the gutter, the detail, and a trailing space.
	if room := maxRow - detail - gutter - 7; w > room {
		w = room
	}
	return w
}

// row draws one item between the frame's edges.
//
// The name sits against the near edge and the detail against the far one, so
// a long name has somewhere to go and the two columns stay apart.
func row(it Item, selected bool, label, detail, inner int) string {
	bullet := bulletOff
	if selected {
		bullet = bulletOn
	}

	name := clip(it.Label, label)
	content := fmt.Sprintf(" %s  %-*s%*s%*s ", bullet, label, name, gutter, "", detail, it.Detail)
	content = fit(content, inner)

	if selected {
		return fmt.Sprintf("%s%s%s%s%s%s%s%s%s", frame, bar, reset, onGround, content, reset, frame, bar, reset)
	}
	return fmt.Sprintf("%s%s%s%s%s%s%s%s%s", frame, bar, reset, dim, content, reset, frame, bar, reset)
}

// blankRow is the space between two entries, drawn inside the frame so the
// near edge stays unbroken down the list.
func blankRow(inner int) string {
	return fmt.Sprintf("%s%s%s%s%s%s%s", frame, bar, reset, strings.Repeat(" ", inner), frame, bar, reset)
}

// clip shortens a name that will not fit, marking that it was cut.
func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

// fit pads or trims a row so every one is the same width and the frame's near
// edge stays straight.
func fit(s string, w int) string {
	r := []rune(s)
	switch {
	case len(r) > w:
		return string(r[:w-1]) + "…"
	case len(r) < w:
		return s + strings.Repeat(" ", w-len(r))
	}
	return s
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
		if n := len([]rune(it.Detail)); n > w {
			w = n
		}
	}
	return w
}
