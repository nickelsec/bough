package graph

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/nickelsec/bough/internal/price"
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
	// Only when there were any. A project that commits nothing does not need
	// telling so every time it is read.
	//
	// A hash means two different things depending on whether the repository was
	// consulted, so a reading that could not consult it says so. Otherwise "not
	// a git repository", "git is not installed" and --no-repo all look the same
	// as a clean confirmation, and every hash shown is a claim nothing checked.
	if n := len(t.Commits); n > 0 {
		if p.RepoRead {
			fmt.Fprintf(w, "%s\n", plural(n, "commit"))
		} else {
			fmt.Fprintf(w, "%s, as the transcript recorded them: the repository was not read\n",
				plural(n, "commit"))
		}
	}
	// Most of what a project costs is the model re-reading the conversation
	// rather than writing anything, which is worth saying once.
	//
	// As a ratio rather than a share. Every project measured came out between
	// 99.4 and 99.9 per cent context, so the percentage says the same thing
	// about all of them and rounds to a hundred, which reads as though nothing
	// was written at all. How many times over the conversation was re-read for
	// each token written is the part that actually varies: 176 times on one
	// project here and 732 on another.
	if t.Tokens != nil && t.Tokens.Output > 0 {
		over := t.Tokens.CacheRead / t.Tokens.Output
		fmt.Fprintf(w, "%s written, %s re-read: %dx more context than output\n",
			big(t.Tokens.Output), big(t.Tokens.CacheRead), over)
		if names := models(t.Models); names != "" {
			fmt.Fprintf(w, "%s\n", names)
		}
		// What it would have cost at published rates, which is not what anyone
		// paid: a subscription is flat rate and the transcript says nothing
		// about which was in use. Saying so once, here, is what keeps the
		// figure from reading as a receipt.
		if t.Cost != nil {
			fmt.Fprintf(w, "%s at API rates, priced %s\n",
				dollars(*t.Cost), price.Taken().Format("Jan 2006"))
		}
	}

	for _, goal := range g.Goals {
		fmt.Fprintf(w, "\n%s\n", strings.Repeat("-", 72))
		fmt.Fprintf(w, "%-14s %s\n", goal.Period, goal.Label)

		s := goal.Stats
		fmt.Fprintf(w, "%-14s %s, %s, %s\n", "",
			plural(len(goal.Tasks), "task"), plural(s.Turns, "prompt"), hours(s.ActiveMinutes))

		if s.Churn > 1 {
			// The count says the file was returned to; the lines say whether
			// that meant a typo or a rewrite.
			if s.LineChurn > 0 {
				fmt.Fprintf(w, "%-14s kept coming back to %s (%d times, %d lines)\n", "",
					baseName(s.ChurnFile), s.Churn, s.LineChurn)
			} else {
				fmt.Fprintf(w, "%-14s kept coming back to %s (%d times)\n", "",
					baseName(s.ChurnFile), s.Churn)
			}
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
			// What the work cost, written the way the project total above is:
			// what was produced, then how much context it took to produce it.
			// The total of the four would be the bigger number and the less
			// useful one, since cache read is around 99 per cent of it and
			// says mostly how long the conversation had grown.
			if tk := task.Stats.Tokens; tk != nil && tk.Output > 0 {
				fmt.Fprintf(w, ", %s written", big(tk.Output))
				if tk.CacheRead > 0 {
					fmt.Fprintf(w, ", %dx context", tk.CacheRead/tk.Output)
				}
				// Without the "at API rates" note the project line carries.
				// Repeating it on every task would say it sixty times.
				if task.Stats.Cost != nil {
					fmt.Fprintf(w, ", %s", dollars(*task.Stats.Cost))
				}
			}
			fmt.Fprintln(w)

			// What the work committed, so it can be checked against the
			// repository. A commit whose hash could not be recovered still
			// counts, since the fact of it is what says the work landed.
			if n := len(task.Stats.Commits); n > 0 {
				named := 0
				for _, c := range task.Stats.Commits {
					if c.SHA == "" {
						continue
					}
					named++
					if c.Subject != "" {
						fmt.Fprintf(w, "    %s  %s\n", c.SHA, oneLine(c.Subject, subjectWidth))
					} else {
						fmt.Fprintf(w, "    %s\n", c.SHA)
					}
				}
				if named == 0 {
					fmt.Fprintf(w, "    %s\n", plural(n, "commit"))
				}
			}

			if verbose {
				for _, turn := range task.Turns {
					fmt.Fprintf(w, "      %s  %s\n",
						turn.At.Format("02 Jan 15:04"), oneLine(turn.Text, 60))
					for _, d := range turn.Delegated {
						fmt.Fprintf(w, "                      handed off: %s\n", handoff(d))
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

// subjectWidth keeps a commit message on one line. Most are well short of it,
// but nothing stops one running to a paragraph.
const subjectWidth = 64

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

// big shortens a token count. Nobody needs the last six digits of a billion.
// dollars writes a cost the way money is written, keeping a small one legible.
//
// A cheap task is worth a fraction of a cent, and "$0.00" reads as free rather
// than as nearly nothing, so anything under a cent keeps enough places to show
// it was not nought. Above that, two places, because that is what money has.
func dollars(d float64) string {
	switch {
	case d >= 0.01:
		return fmt.Sprintf("$%.2f", d)
	case d > 0:
		return fmt.Sprintf("$%.4f", d)
	}
	return "$0"
}

func big(n int) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%dk", n/1000)
	}
	return strconv.Itoa(n)
}

// models names what did the work, most first. One model needs no share; more
// than one is the case worth breaking down.
// The share is of output tokens rather than of everything. Cache reads are
// around 99% of every figure and they say how long the conversation was, not
// how much of the work a model did.
func models(spend map[string]Tokens) string {
	if len(spend) == 0 {
		return ""
	}
	m := make(map[string]int, len(spend))
	for k, v := range spend {
		m[k] = v.Output
	}
	names := make([]string, 0, len(m))
	all := 0
	for k, v := range m {
		names = append(names, k)
		all += v
	}
	sort.Slice(names, func(i, j int) bool {
		if m[names[i]] != m[names[j]] {
			return m[names[i]] > m[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) == 1 {
		return names[0]
	}
	// Every model wrote nothing, so there are no shares to work out. Reachable
	// on a history where only cache reads were recorded, and dividing by the
	// total would panic rather than saying so.
	if all == 0 {
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	// Rounded rather than truncated, and the largest share takes whatever the
	// rounding left over, so the parts add up to a hundred rather than to
	// ninety nine.
	parts := make([]string, len(names))
	rest := 100
	for i := len(names) - 1; i > 0; i-- {
		pct := int(float64(100*m[names[i]])/float64(all) + 0.5)
		parts[i] = fmt.Sprintf("%s %d%%", names[i], pct)
		rest -= pct
	}
	parts[0] = fmt.Sprintf("%s %d%%", names[0], rest)
	return strings.Join(parts, ", ")
}

// handoff names a piece of delegated work.
//
// Agents differ in what they record. Claude writes a brief before handing work
// over, and that describes it. Codex encrypts the brief and leaves only the
// name of the task, so the name is what there is to show.
//
// When a spawn recorded none of the three, the line said so rather than
// trailing off after the colon. That happens on Codex where the brief is
// encrypted and the call names neither the sort of sub-agent nor the task, and
// it was the case this function was written for: printing the description
// alone left a bare "handed off:" with nothing after it, and so did returning
// an empty string here.
func handoff(d Delegation) string {
	switch {
	case d.Description != "":
		return d.Description
	case d.Name != "":
		return d.Name
	case d.Kind != "":
		return d.Kind
	default:
		return UnnamedHandoff
	}
}

// UnnamedHandoff is what a hand-off is called when the transcript recorded
// nothing about it. Said plainly, because the alternative is a line that looks
// truncated, and a reader cannot tell a bug from a silence.
const UnnamedHandoff = "an unnamed task"
