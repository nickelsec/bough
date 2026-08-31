// Package pick asks the user to choose from a list in the terminal.
//
// The list is drawn in place and moved through with the arrow keys, which is
// what people expect from a command that opens with a question. Where that is
// not possible, because output is being piped or the terminal cannot do it, the
// same choice is offered as a numbered list instead. Both paths return the same
// answer, so nothing that uses this has to care which one ran.
package pick

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ErrCancelled is returned when the user backs out with escape or ctrl-c.
var ErrCancelled = errors.New("cancelled")

// Item is one row in the list.
type Item struct {
	// Label is the main text, for example a project name.
	Label string

	// Detail sits to the right in dimmer text, for context that helps choose
	// without competing with the label.
	Detail string
}

// Choose shows the list and returns the index the user picked.
func Choose(title string, items []Item) (int, error) {
	if len(items) == 0 {
		return 0, errors.New("nothing to choose from")
	}
	// One option is not a choice.
	if len(items) == 1 {
		return 0, nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) {
		if i, err := interactive(os.Stderr, os.Stdin, title, items); err == nil || errors.Is(err, ErrCancelled) {
			return i, err
		}
		// The terminal refused raw mode, so fall through to the plain list
		// rather than failing outright.
	}
	return numbered(os.Stderr, os.Stdin, title, items)
}

// interactive draws the list and moves through it with the arrow keys.
//
// It writes to standard error so that the chooser never lands in a pipe when
// the actual output is being redirected.
func interactive(out io.Writer, in *os.File, title string, items []Item) (int, error) {
	fd := int(in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}
	defer term.Restore(fd, state)

	// A terminal left in raw mode after a crash is unusable, so the cursor and
	// the screen are put back whatever happens.
	defer fmt.Fprint(out, showCursor)
	fmt.Fprint(out, hideCursor)

	width := detailWidth(items)
	selected := 0
	drawn := 0

	for {
		drawn = draw(out, title, items, selected, width, drawn)

		key, err := readKey(in)
		if err != nil {
			return 0, err
		}
		switch key {
		case keyUp:
			if selected > 0 {
				selected--
			}
		case keyDown:
			if selected < len(items)-1 {
				selected++
			}
		case keyHome:
			selected = 0
		case keyEnd:
			selected = len(items) - 1
		case keyEnter:
			clear(out, drawn)
			return selected, nil
		case keyCancel:
			clear(out, drawn)
			return 0, ErrCancelled
		}
	}
}

// numbered offers the same choice without needing a capable terminal.
func numbered(out io.Writer, in io.Reader, title string, items []Item) (int, error) {
	fmt.Fprintf(out, "%s\n\n", title)
	for i, it := range items {
		fmt.Fprintf(out, "  %2d  %-24s %s\n", i+1, it.Label, it.Detail)
	}
	fmt.Fprintf(out, "\nChoose 1 to %d: ", len(items))

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return 0, ErrCancelled
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(items) {
		return 0, fmt.Errorf("%q is not one of the choices", strings.TrimSpace(line))
	}
	return n - 1, nil
}
