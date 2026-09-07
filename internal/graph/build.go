package graph

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/metrics"
	"github.com/nickelsec/bough/internal/repo"
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

	// SkipRepo leaves the project's git history alone. The commits bough shows
	// then come from the transcript only, which means the ones made quietly
	// arrive without a hash.
	SkipRepo bool

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

	// A commit made in another repository is not this project's work, whether
	// or not the repository can be read, so it goes first either way.
	onlyHere(p.Path, sessions)
	if !opt.SkipRepo {
		fromRepo(p.Path, sessions)
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
		LineChurn:     s.LineChurn,
		Models:        s.Models,
		ChurnFile:     s.ChurnFile,
		// Two places is plenty for a hint, and it keeps the same history from
		// producing byte-different output across platforms.
		Struggle: math.Round(s.Struggle()*100) / 100,
	}
	// Left out when the work was charged nothing, which is how a history read
	// before this was recorded arrives.
	if s.Tokens.Total() > 0 {
		out.Tokens = &Tokens{
			Input:      s.Tokens.Input,
			Output:     s.Tokens.Output,
			CacheRead:  s.Tokens.CacheRead,
			CacheWrite: s.Tokens.CacheWrite,
		}
	}
	for i, f := range s.TopFiles {
		if i >= topFileLimit {
			break
		}
		out.TopFiles = append(out.TopFiles, FileCount{Path: f.Path, Edits: f.Edits})
	}
	for _, c := range s.Commits {
		out.Commits = append(out.Commits, Commit{
			SHA:     c.SHA,
			Kind:    c.Kind,
			Branch:  c.Branch,
			Subject: c.Subject,
			Added:   c.Added,
			Removed: c.Removed,
		})
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
		for _, c := range t.Committed {
			row.Committed = append(row.Committed, Commit{
				SHA:     c.SHA,
				Kind:    c.Kind,
				Branch:  c.Branch,
				Subject: c.Subject,
				Added:   c.Added,
				Removed: c.Removed,
			})
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

// matchWindow is how far a commit in the repository may sit from the tool call
// that made it and still be the same commit.
//
// The two happen within a second or two of each other, so this is generous.
// It is not tight enough to worry about: on the history this was fitted to, 47
// of 49 commits landed within five seconds and only one had another commit
// close enough to be mistaken for it.
const matchWindow = 90 * time.Second

// fromRepo fills in what the transcript could not say.
//
// A commit made with git's quiet flag reaches the transcript with no hash,
// because Claude Code recovers the hash by reading what git printed. The
// repository has it, sitting where the transcript already says the project is,
// so the two are matched by time.
//
// The repository is also the better authority when the two disagree. A hash in
// a transcript was true when it was written; rebasing or amending afterwards
// leaves it pointing at something the repository can no longer reach, and a
// hash nobody can look up is worse than none.
//
// Nothing here is required. A project that has moved, was never a repository,
// or is on a machine without git leaves the transcript's own account standing.
func fromRepo(dir string, sessions []agent.Session) {
	if dir == "" {
		return
	}
	h := repo.Read(dir)
	if len(h.Commits) == 0 {
		return
	}
	for _, sess := range sessions {
		for i := range sess.Turns {
			for j := range sess.Turns[i].Committed {
				c := &sess.Turns[i].Committed[j]
				found := h.Near(c.At, matchWindow)
				if found == nil {
					// The repository was read and has no commit here, so any
					// hash the transcript carried is one the repository can no
					// longer reach: rebased, amended, or dropped. Showing it
					// would offer the reader something to check that does not
					// check out, which is worse than showing nothing.
					c.SHA = ""
					c.Branch = ""
					continue
				}
				c.SHA = found.SHA
				c.Subject = found.Subject
				c.Added = found.Added
				c.Removed = found.Removed
			}
		}
	}
}

// onlyHere drops commits the agent made in some other repository.
//
// A session about one project regularly commits in another: a tool and its
// website worked on together, a fix made in a dependency. Those commits happen,
// but they are not this project's, and counting them puts work on the diagram
// that was done somewhere else. Ten of one project's forty seven commit calls
// were made in a sibling repository.
//
// A command that does not move is committing where the session is, which is
// this project.
func onlyHere(dir string, sessions []agent.Session) {
	for _, sess := range sessions {
		for i := range sess.Turns {
			kept := sess.Turns[i].Committed[:0]
			for _, c := range sess.Turns[i].Committed {
				if here(c.Dir, dir) {
					kept = append(kept, c)
				}
			}
			sess.Turns[i].Committed = kept
		}
	}
}

// here reports whether a commit was made in this project's own directory.
//
// A command that does not move is running where the session is, which is here.
func here(in, project string) bool {
	return in == "" || sameDir(in, project)
}

// sameDir compares two paths for being the same place.
//
// The same directory is written several ways in one session. On this corpus a
// single project's commits arrived as "d:/thing", "/d/thing" and with no
// path at all, which are one directory and have to compare equal or real work
// is thrown away. The drive is folded into a leading letter so the two spellings
// meet, and the result is compared whole rather than by suffix, since a suffix
// test would make "site" and "my-site" the same place.
func sameDir(a, b string) bool {
	return driveForm(a) == driveForm(b)
}

// driveForm puts a path into one shape: lower case, forward slashes, no
// trailing separator, and a Windows drive written as "d:/" whether it arrived
// that way or as the "/d/" a shell uses.
func driveForm(p string) string {
	p = strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	if m := shellDrive.FindStringSubmatch(p); m != nil {
		p = m[1] + ":/" + m[2]
	}
	p = path.Clean(p)
	return strings.TrimSuffix(p, "/")
}

// shellDrive matches the "/d/some/path" a unix style shell uses for a Windows
// drive, so it can be written the way the transcript records it.
var shellDrive = regexp.MustCompile(`^/([a-z])/(.*)$`)
