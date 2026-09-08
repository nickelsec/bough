package pick

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
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

	// indent is the margin every line is drawn with, and it counts against the
	// window just as the frame does.
	indent = 2

	// defaultRow is the widest a framed row may be when the terminal will not
	// say how wide it is, borders included, so the picker still fits an eighty
	// column window.
	defaultRow = 78

	// minRow is the narrowest frame worth drawing. Below this there is no room
	// for a name and a size beside it, and the numbered list reads better than
	// a frame squeezed to nothing.
	minRow = 34
)

// termWidth is how wide the picker may draw.
//
// It asks the terminal rather than assuming, because a frame wider than the
// window wraps every row onto a second line. The redraw then steps back by the
// number of rows it believes it wrote, which is fewer than the rows actually
// on screen, so each pass overwrites the wrong lines and the list smears into
// itself. A picker that works at eighty columns and shreds at sixty is worse
// than one that is always plain.
//
// Standard error is the one it measures, since that is where the list goes.
var termWidth = realTermWidth

func realTermWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return defaultRow
	}
	// One column short of the edge. A row drawn right up to the last cell
	// wraps on some terminals and not others, and the ones that wrap put a
	// stray blank line under every entry.
	if w > defaultRow {
		return defaultRow
	}
	return w - 1
}

// termHeight is how many rows the picker may use.
//
// The list is redrawn by stepping the cursor back up over what it wrote, which
// only reaches what is still on screen. Once the list is taller than the
// window the top scrolls away, the cursor cannot climb past the first visible
// row, and every pass lands lower than the last. About a dozen projects is
// enough to reach that on an ordinary window.
var termHeight = realTermHeight

func realTermHeight() int {
	_, h, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || h <= 0 {
		return 24
	}
	return h
}

// window is how many entries fit at once, and which of them to show.
//
// What the frame spends besides the entries: the title and its blank line, the
// two frame edges, the blank line under the frame, the hint, and a row left
// clear for the shell prompt.
func window(count, selected int, titled bool) (first, shown int) {
	spare := termHeight() - 6
	if titled {
		spare -= 2
	}
	// Every entry after the first also carries a blank line above it.
	fits := (spare + 1) / 2
	if fits < 1 {
		fits = 1
	}
	if fits >= count {
		return 0, count
	}
	// Hold the selection near the middle, so a long list scrolls rather than
	// jumping a page at a time.
	first = selected - fits/2
	if first < 0 {
		first = 0
	}
	if first+fits > count {
		first = count - fits
	}
	return first, fits
}

// raw writes a carriage return before every newline.
//
// The picker puts the terminal in raw mode, which switches off the newline
// translation a terminal normally does, so a bare linefeed drops a row and
// leaves the cursor in the column it was already in. Each line then starts
// further right than the one before, and the cursor-up beginning the next
// redraw climbs that same crooked path and clears the wrong part of each row.
// Names and pieces of the frame end up at scattered columns with no left edge
// in sight, which is what three different terminals showed.
//
// Harmless when translation is on, since a terminal collapses the pair.
type raw struct{ to io.Writer }

func (w raw) Write(p []byte) (int, error) {
	var out []byte
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, b)
	}
	if _, err := w.to.Write(out); err != nil {
		return 0, err
	}
	// Report the caller's length, not the translated one, or fmt takes output
	// that went out whole for a short write.
	return len(p), nil
}

// marker stands in for an entry at the edge of the view, saying the list
// carries on past it. The entry it replaces stays reachable, since the view
// scrolls with the selection.
func marker(inner int, text string) string {
	body := fit("  "+text, inner)
	return fmt.Sprintf("%s%s%s%s%s%s%s%s%s", frame, bar, reset, dim, body, reset, frame, bar, reset)
}

// hint sits below the frame, with a blank line between them.
const hint = "↑↓ move    ↵ choose    esc cancel"

// draw renders the list and returns how many lines it used, so the next pass
// can rewrite exactly those and nothing else.
func draw(dst io.Writer, title string, items []Item, selected, detail, previous int) int {
	out := raw{to: dst}
	if previous > 0 {
		// Step back over what was drawn last time and overwrite it. Redrawing
		// in place keeps the list from scrolling away as the user moves.
		//
		// Back to column zero before climbing: cursor-up keeps whatever column
		// it is in, so stepping up from where the last line ended would clear
		// the wrong part of every row on the way.
		fmt.Fprint(out, "\r"+strings.Repeat(lineUp+clearLine, previous))
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

	// Only what fits is drawn, for the reason on termHeight above.
	first, shown := window(len(items), selected, title != "")

	// A blank framed line above and below each row, so the list breathes
	// rather than reading as a solid block of text.
	for i := first; i < first+shown; i++ {
		if i > first {
			fmt.Fprintf(out, "  %s\n", blankRow(inner))
			lines++
		}
		text := row(items[i], i == selected, label, detail, inner)
		// Say when the list runs on, so a cut one does not look whole.
		if i == first && first > 0 {
			text = marker(inner, "↑ "+strconv.Itoa(first)+" more")
		}
		if i == first+shown-1 && first+shown < len(items) {
			text = marker(inner, "↓ "+strconv.Itoa(len(items)-first-shown)+" more")
		}
		fmt.Fprintf(out, "  %s\n", text)
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
	// Everything the row spends besides the name: the two space indent every
	// line carries, two frame edges, the marker and the spaces around it, the
	// gutter, the detail, and a trailing space.
	if room := termWidth() - detail - gutter - indent - 7; w > room {
		w = room
	}
	// A narrow window can take that below nothing, and a negative column
	// panics the formatter rather than merely looking wrong. Keep enough for a
	// name worth reading; the caller drops to the numbered list when even this
	// will not fit.
	if w < 8 {
		w = 8
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
	size := clip(it.Detail, detail)
	content := fmt.Sprintf(" %s  %-*s%*s%*s ", bullet, label, name, gutter, "", detail, size)
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
//
// A column with no room at all yields nothing rather than a lone ellipsis, and
// never a negative slice: a window can be narrow enough to leave the detail no
// space whatever, and that must look empty rather than panic.
func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return strings.Repeat("…", w)
	}
	return string(r[:w-1]) + "…"
}

// fit pads or trims a row so every one is the same width and the frame's near
// edge stays straight.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	switch {
	case len(r) > w:
		if w == 1 {
			return "…"
		}
		return string(r[:w-1]) + "…"
	case len(r) < w:
		return s + strings.Repeat(" ", w-len(r))
	}
	return s
}

// clear removes the list once a choice is made, so the chosen output starts on
// a clean screen rather than under a menu.
func clearRows(out io.Writer, lines int) {
	if lines > 0 {
		fmt.Fprint(out, strings.Repeat(lineUp+clearLine, lines))
	}
}

// detailWidth is how much room the right hand column needs.
//
// It is capped at a share of the window, because the name is what a person
// picks by. Given a narrow terminal the size should lose its room first.
func detailWidth(items []Item) int {
	w := 0
	for _, it := range items {
		if n := len([]rune(it.Detail)); n > w {
			w = n
		}
	}
	if room := (termWidth() - indent) / 3; w > room {
		w = room
	}
	return w
}
