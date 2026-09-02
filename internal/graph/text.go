package graph

import (
	"fmt"
	"io"
	"path"
	"strings"
)

// WriteText prints a graph as something a person can read in a terminal.
//
// This exists so the core can be checked without a browser. Someone reading
// their own history here should be able to say whether the shape matches what
// they remember, which is the only test that really matters and the one no
// amount of unit testing replaces.
func WriteText(w io.Writer, g Graph, verbose bool) error {
	p := g.Project
	fmt.Fprintf(w, "%s\n%s\n", p.Name, strings.Repeat("=", len(p.Name)))
	if p.Path != "" {
		fmt.Fprintf(w, "%s\n", p.Path)
	}

	t := g.Totals
	fmt.Fprintf(w, "\n%s across %s\n", plural(t.Turns, "prompt"), plural(len(g.Goals), "sitting"))
	if t.Edits > 0 {
		fmt.Fprintf(w, "%s to %s, %s at the keyboard\n",
			plural(t.Edits, "change"), plural(t.Files, "file"), hours(t.ActiveMinutes))
	}

	for _, goal := range g.Goals {
		fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 72))
		fmt.Fprintf(w, "%-14s %s\n", goal.Period, goal.Label)

		s := goal.Stats
		fmt.Fprintf(w, "%-14s %s, %s, %s\n", "",
			plural(len(goal.Tasks), "task"), plural(s.Turns, "prompt"), hours(s.ActiveMinutes))

		if s.Churn > 1 {
			fmt.Fprintf(w, "%-14s kept coming back to %s (%d times)\n", "",
				baseName(s.ChurnFile), s.Churn)
		}

		for _, task := range goal.Tasks {
			fmt.Fprintf(w, "\n  %s\n", task.Label)
			fmt.Fprintf(w, "    %s", plural(task.Stats.Turns, "prompt"))
			if task.Stats.Edits > 0 {
				fmt.Fprintf(w, ", %s", plural(task.Stats.Edits, "change"))
			}
			if task.Stats.Errors > 0 {
				fmt.Fprintf(w, ", %s", plural(task.Stats.Errors, "failure"))
			}
			fmt.Fprintln(w)

			if verbose {
				for _, turn := range task.Turns {
					fmt.Fprintf(w, "      %s  %s\n",
						turn.At.Format("02 Jan 15:04"), oneLine(turn.Text, 60))
					for _, d := range turn.Delegated {
						fmt.Fprintf(w, "                      handed off: %s\n", d.Description)
					}
				}
			}
		}
	}

	if len(g.Links) > 0 {
		fmt.Fprintf(w, "\n%s\ncame back to earlier work\n", strings.Repeat("-", 72))
		for _, l := range g.Links {
			fmt.Fprintf(w, "  %s and %s both worked on %s\n",
				period(g, l.From), period(g, l.To), fileList(l.Files, 3))
		}
	}
	return nil
}

// period finds how a goal is described, for referring to it in prose.
func period(g Graph, id string) string {
	for _, goal := range g.Goals {
		if goal.ID == id {
			return goal.Period
		}
	}
	return id
}

func fileList(files []string, limit int) string {
	var names []string
	for i, f := range files {
		if i >= limit {
			names = append(names, fmt.Sprintf("and %d more", len(files)-limit))
			break
		}
		names = append(names, baseName(f))
	}
	return strings.Join(names, ", ")
}

// oneLine flattens a prompt onto a single line, since dictated and pasted text
// arrives full of line breaks.
func oneLine(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= width {
		return s
	}
	cut := strings.LastIndex(s[:width], " ")
	if cut < width/2 {
		cut = width
	}
	return s[:cut] + "..."
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	if strings.HasSuffix(word, "e") {
		return fmt.Sprintf("%d %ss", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// hours says a duration the way a person would.
func hours(minutes int) string {
	switch {
	case minutes < 1:
		return "a moment"
	case minutes < 60:
		return fmt.Sprintf("%d minutes", minutes)
	case minutes < 120:
		return fmt.Sprintf("an hour and %d minutes", minutes-60)
	default:
		return fmt.Sprintf("%d hours", minutes/60)
	}
}

// baseName is the file name out of a path recorded in a transcript.
//
// Not filepath.Base, which only splits on the separator the host machine uses.
// A transcript written on Windows and read on Linux would come back whole.
func baseName(p string) string {
	return path.Base(strings.ReplaceAll(p, `\`, "/"))
}
