package graph

import (
	"fmt"
	"math"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/metrics"
	"github.com/nickelsec/bough/internal/rollup"
	"github.com/nickelsec/bough/internal/segment"
)

// Options carry the tuning from each stage, so a caller can change how the work
// is divided without this package knowing what the knobs mean.
type Options struct {
	Segment segment.Options
	Rollup  rollup.Options
	Links   rollup.LinkOptions

	// Tool is the version string recorded in the output.
	Tool string

	// Now supplies the timestamp, so tests can pin it.
	Now func() time.Time
}

// DefaultOptions are the fitted defaults for every stage.
func DefaultOptions() Options {
	return Options{
		Segment: segment.DefaultOptions(),
		Rollup:  rollup.DefaultOptions(),
		Links:   rollup.DefaultLinkOptions(),
		Now:     time.Now,
	}
}

// Build turns a project's sessions into a graph.
//
// Goals from every session are gathered and ordered by when they happened, so
// a project worked on across several sessions reads as one run of work rather
// than as separate piles. Sessions are an artefact of how the agent stores
// things and mean little to the person who did the work.
func Build(p agent.Project, sessions []agent.Session, opt Options) Graph {
	if opt.Now == nil {
		opt.Now = time.Now
	}

	var goals []rollup.Goal
	titles := map[int]string{}

	for _, sess := range sessions {
		tasks := segment.Split(sess.Turns, opt.Segment)
		for _, g := range rollup.Group(tasks, opt.Rollup) {
			titles[len(goals)] = sess.Title
			goals = append(goals, g)
		}
	}
	goals, titles = inTimeOrder(goals, titles)

	g := Graph{
		Schema:    SchemaVersion,
		Generated: opt.Now().UTC(),
		Tool:      opt.Tool,
		Project: Project{
			Name:     p.Name,
			Path:     p.Path,
			Agent:    p.Source,
			Sessions: len(sessions),
		},
	}

	var everyTurn []agent.Turn
	for i, goal := range goals {
		turns := goal.Turns()
		everyTurn = append(everyTurn, turns...)

		out := Goal{
			ID:     fmt.Sprintf("g%d", i+1),
			Label:  rollup.Label(turns),
			Title:  titles[i],
			Period: rollup.Period(first(turns), last(turns)),
			Stats:  statsOf(turns),
		}
		for j, t := range goal.Tasks {
			out.Tasks = append(out.Tasks, Task{
				ID:      fmt.Sprintf("g%d.t%d", i+1, j+1),
				Label:   rollup.Label(t.Turns),
				Reasons: reasonsOf(t),
				Stats:   statsOf(t.Turns),
				Turns:   turnsOf(t.Turns),
			})
		}
		g.Goals = append(g.Goals, out)
	}

	for _, l := range rollup.Links(goals, opt.Links) {
		g.Links = append(g.Links, Link{
			From:   fmt.Sprintf("g%d", l.From+1),
			To:     fmt.Sprintf("g%d", l.To+1),
			Files:  l.Files,
			Weight: l.Weight,
		})
	}

	g.Totals = statsOf(everyTurn)
	return g
}

// inTimeOrder sorts goals by when they started, keeping each one's session
// title with it.
func inTimeOrder(goals []rollup.Goal, titles map[int]string) ([]rollup.Goal, map[int]string) {
	type pair struct {
		goal  rollup.Goal
		title string
	}
	pairs := make([]pair, len(goals))
	for i, g := range goals {
		pairs[i] = pair{g, titles[i]}
	}
	// Insertion sort keeps this stable, so goals starting at the same moment
	// stay in the order their sessions were read.
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0 && first(pairs[j].goal.Turns()).Before(first(pairs[j-1].goal.Turns())); j-- {
			pairs[j], pairs[j-1] = pairs[j-1], pairs[j]
		}
	}
	out := make([]rollup.Goal, len(pairs))
	newTitles := make(map[int]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.goal
		newTitles[i] = p.title
	}
	return out, newTitles
}

func statsOf(turns []agent.Turn) Stats {
	s := metrics.Summarise(turns)
	out := Stats{
		Start:         s.Start,
		End:           s.End,
		SpanMinutes:   int(s.Span.Minutes()),
		ActiveMinutes: int(s.Active.Minutes()),
		Turns:         s.Turns,
		Edits:         s.Edits,
		Files:         s.Files,
		Errors:        s.Errors,
		Churn:         s.Churn,
		ChurnFile:     s.ChurnFile,
		// Two places is plenty for a hint, and it keeps the same history from
		// producing byte-different output across platforms.
		Struggle: math.Round(s.Struggle()*100) / 100,
	}
	for i, f := range s.TopFiles {
		if i >= topFileLimit {
			break
		}
		out.TopFiles = append(out.TopFiles, FileCount{Path: f.Path, Edits: f.Edits})
	}
	return out
}

// topFileLimit keeps the output readable. Beyond a handful the tail is noise,
// and the full list would dwarf everything else in the file.
const topFileLimit = 8

func turnsOf(turns []agent.Turn) []Turn {
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		row := Turn{
			At:     t.At,
			Text:   t.Text,
			Files:  len(t.Files),
			Errors: t.Errors,
		}
		for _, n := range t.Edits {
			row.Edits += n
		}
		for _, d := range t.Delegated {
			row.Delegated = append(row.Delegated, Delegation{Kind: d.Kind, Description: d.Description})
		}
		out = append(out, row)
	}
	return out
}

func reasonsOf(t segment.Task) []string {
	if len(t.Reasons) == 0 {
		return nil
	}
	out := make([]string, len(t.Reasons))
	for i, r := range t.Reasons {
		out[i] = string(r)
	}
	return out
}

func first(turns []agent.Turn) time.Time {
	if len(turns) == 0 {
		return time.Time{}
	}
	return turns[0].At
}

func last(turns []agent.Turn) time.Time {
	if len(turns) == 0 {
		return time.Time{}
	}
	return turns[len(turns)-1].At
}
