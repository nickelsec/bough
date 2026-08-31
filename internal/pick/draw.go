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
	onDetail = "\x1b[48;2;134;154;72m\x1b[38;2;48;56;28m"
)

// The pieces the frame is built from.
//
// Every one is a single column wide in every terminal, which emoji are not:
// most render double width, a few render single, and terminals disagree about
// which. That difference shears the right hand edge of a frame, and a picker
// that looks broken on someone else's machine is worse than a plain one that
// always lines up. These carry the same meaning without the risk.
const (
	cornerTL   = "╭"
	cornerTR   = "╮"
	cornerBL   = "╰"
	cornerBR   = "╯"
	horizontal = "─"
	bar        = "│"

	// bulletOn and bulletOff stand in for a radio button.
	bulletOn  = "◉"
	bulletOff = "○"
)

// labelWidth is the column the details line up against.
const labelWidth = 22

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

	inner := labelWidth + detail + 7
	lines := 0

	if title != "" {
		fmt.Fprintf(out, "  %s%s%s\n\n", bold, title, reset)
		lines += 2
	}

	fmt.Fprintf(out, "  %s%s%s%s%s\n", frame, cornerTL, strings.Repeat(horizontal, inner), cornerTR, reset)
	lines++

	for i, it := range items {
		fmt.Fprintf(out, "  %s\n", row(it, i == selected, detail, inner))
		lines++
	}

	fmt.Fprintf(out, "  %s%s%s%s%s\n\n", frame, cornerBL, strings.Repeat(horizontal, inner), cornerBR, reset)
	lines += 2

	fmt.Fprintf(out, "  %s%s%s\n", dim, hint, reset)
	return lines + 1
}

// row draws one item between the frame's edges.
//
// The selected row is filled rather than merely marked, so it is obvious at a
// glance which one enter would take.
func row(it Item, selected bool, detail, inner int) string {
	bullet, body := bulletOff, fmt.Sprintf("%-*s  %*s", labelWidth, it.Label, detail, it.Detail)
	if selected {
		bullet = bulletOn
	}

	// One space of breathing room inside each edge.
	content := fmt.Sprintf(" %s  %s ", bullet, body)
	content = fit(content, inner)

	if selected {
		return fmt.Sprintf("%s%s%s%s%s%s%s%s", frame, bar, reset, onGround, content, reset, frame, bar+reset)
	}
	return fmt.Sprintf("%s%s%s%s%s%s%s%s", frame, bar, reset, dim, content, reset, frame, bar+reset)
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
