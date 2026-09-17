package graph

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
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

	// Repo is the project's git history, when the caller read it.
	//
	// Passed in rather than read here. Building a graph is arithmetic over
	// sessions, and a function that shells out to git cannot be tested without
	// a filesystem and a git to shell out to. Reading the repository is the
	// caller's business; deciding what the commits mean is this package's.
	//
	// A zero History means it was not read, which is not the same as a
	// repository with no commits in it: a hash the transcript carried stays as
	// it is rather than being cleared as unreachable.
	Repo repo.History

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

	// Everything below writes into the turns it is given: onlyHere drops
	// commits made elsewhere, and matching against the repository rewrites the
	// hashes. Those edits used to land in the caller's own slices, so calling
	// Build twice on one set of sessions gave two different answers and
	// nothing else could reuse them afterwards.
	sessions = clone(sessions)

	// Delegated work belongs inside the turn that asked for it, so it is put
	// back before anything is measured or divided. Doing it here rather than in
	// each agent keeps the sub-agent's own session intact up to this point,
	// which is what makes its prompts and tokens countable at all.
	sessions = fold(sessions)

	// A commit made in another repository is not this project's work, whether
	// or not the repository can be read, so it goes first either way.
	onlyHere(p.Path, sessions)
	repoRead := fromRepo(opt.Repo, sessions)

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
			Name:      p.Name,
			Path:      p.Path,
			Agent:     p.Source,
			AgentName: agent.Display(p.Source),
			Sessions:  len(sessions),
			RepoRead:  repoRead,
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
			Stats:  statsOf(turns, repoRead),
		}
		for j, t := range goal.Tasks {
			out.Tasks = append(out.Tasks, Task{
				ID:      fmt.Sprintf("g%d.t%d", i+1, j+1),
				Label:   rollup.Label(t.Turns),
				Reasons: reasonsOf(t),
				Stats:   statsOf(t.Turns, repoRead),
				Turns:   turnsOf(t.Turns, repoRead),
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

	g.Totals = statsOf(everyTurn, repoRead)
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

func statsOf(turns []agent.Turn, repoRead bool) Stats {
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
		out.Commits = append(out.Commits, commitOf(c, repoRead))
	}
	return out
}

// topFileLimit keeps the output readable. Beyond a handful the tail is noise,
// and the full list would dwarf everything else in the file.
const topFileLimit = 8

func turnsOf(turns []agent.Turn, repoRead bool) []Turn {
	out := make([]Turn, 0, len(turns))
	for _, t := range turns {
		row := Turn{
			At: t.At,
			// Says rather than Text, so a turn that was handed over rather
			// than typed shows what it was called instead of nothing.
			Text:   t.Says(),
			Files:  len(t.Files),
			Errors: t.Errors,
		}
		for _, n := range t.Edits {
			row.Edits += n
		}
		for _, d := range t.Delegated {
			row.Delegated = append(row.Delegated, Delegation{Kind: d.Kind, Name: d.Name, Description: d.Description})
		}
		for _, c := range t.Committed {
			row.Committed = append(row.Committed, commitOf(c, repoRead))
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
// fromRepo fills in what the transcript could not say, and reports whether the
// repository was actually consulted.
func fromRepo(h repo.History, sessions []agent.Session) bool {
	// Nothing was consulted, so nothing can be confirmed or contradicted. The
	// hashes the transcript carried stay as they are: unverified, but the best
	// that is known. Clearing them here would empty every hash on a machine
	// with no git installed.
	if !h.Read {
		return false
	}
	// Every commit the agent made, gathered before any of them is matched.
	//
	// Matching one at a time as they were walked let the order sessions
	// happened to be in decide the answer. A repository commit is claimed by
	// the first agent commit to reach it, so when two fell inside the same
	// window the one visited first took it, whether or not it was the closer.
	// Sessions are grouped by file rather than by time, so that order is not
	// even the order the work happened in.
	var made []*agent.Commit
	for _, sess := range sessions {
		for i := range sess.Turns {
			for j := range sess.Turns[i].Committed {
				made = append(made, &sess.Turns[i].Committed[j])
			}
		}
	}

	for _, c := range pair(made, h.Commits, matchWindow) {
		// The repository was read and has no commit here, so any hash the
		// transcript carried is one the repository can no longer reach:
		// rebased, amended, or dropped. Showing it would offer the reader
		// something to check that does not check out, which is worse than
		// showing nothing.
		c.SHA = ""
		c.Branch = ""
	}
	return true
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
//
// So is one that moves somewhere relative. `cd internal && git commit` runs in
// a subdirectory of the session, which is still this repository, and comparing
// the bare "internal" against an absolute project path never matched: a real
// commit in this project was dropped from the diagram without a word. Only an
// absolute path can name somewhere else, because only an absolute path says
// where it starts from.
//
// A path that climbs out with .. is the exception. It is relative but it can
// leave the project, so it is resolved against the project and compared.
func here(in, project string) bool {
	if in == "" {
		return true
	}
	if !rooted(in) {
		if !strings.Contains(in, "..") {
			return true
		}
		return agent.SamePath(path.Join(agent.NormalisePath(project), in), project)
	}
	return agent.SamePath(in, project)
}

// rooted reports whether a path says for itself where it starts.
//
// Both spellings of a Windows drive count, since a transcript carries "d:/x"
// and "/d/x" for the same place, and both are absolute.
func rooted(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.HasPrefix(p, "/") || driveLetter.MatchString(p)
}

// driveLetter matches a path that opens with a Windows drive, as "d:/work".
var driveLetter = regexp.MustCompile(`^[a-zA-Z]:/`)

// pair matches the commits an agent made to the ones in the repository,
// closest pair first, and returns the ones nothing matched.
//
// Matching them one at a time let the order sessions were walked in decide the
// answer: a repository commit went to the first agent commit that reached it,
// so when two fell inside the same window the one visited first took it,
// closer or not. Sessions are grouped by file rather than by time, so that
// order is not even the order the work happened in.
//
// Deciding all of them together removes the question. Every pair inside the
// window is measured, the closest is settled first, and each side drops out
// once it is spoken for. Ties fall to the earlier commit so the same input
// always gives the same answer.
func pair(made []*agent.Commit, have []repo.Commit, window time.Duration) []*agent.Commit {
	type link struct {
		made, have int
		gap        time.Duration
	}

	var links []link
	for m, c := range made {
		if c == nil || c.At.IsZero() {
			continue
		}
		for h := range have {
			if have[h].When.IsZero() {
				continue
			}
			gap := have[h].When.Sub(c.At)
			if gap < 0 {
				gap = -gap
			}
			if gap > window {
				continue
			}
			links = append(links, link{made: m, have: h, gap: gap})
		}
	}

	sort.SliceStable(links, func(i, j int) bool {
		if links[i].gap != links[j].gap {
			return links[i].gap < links[j].gap
		}
		if links[i].have != links[j].have {
			return links[i].have < links[j].have
		}
		return links[i].made < links[j].made
	})

	tookMade := make([]bool, len(made))
	tookHave := make([]bool, len(have))
	for _, l := range links {
		if tookMade[l.made] || tookHave[l.have] {
			continue
		}
		tookMade[l.made] = true
		tookHave[l.have] = true

		found := have[l.have]
		c := made[l.made]
		c.SHA = found.SHA
		c.Subject = found.Subject
		c.Added = found.Added
		c.Removed = found.Removed
	}

	var missed []*agent.Commit
	for i, c := range made {
		if c != nil && !tookMade[i] {
			missed = append(missed, c)
		}
	}
	return missed
}

// commitOf copies a commit across the boundary into the shape the output uses.
//
// Written out twice before, once for a task's list and once for a prompt's, so
// a field added to one arrived in the graph from one place and not the other.
// confirmed is true when the repository was read. A hash that survived
// fromRepo with the repository read is one the repository still has, since
// every hash it could not reach was cleared there.
func commitOf(c agent.Commit, confirmed bool) Commit {
	return Commit{
		SHA:       c.SHA,
		Kind:      c.Kind,
		Branch:    c.Branch,
		Confirmed: confirmed && c.SHA != "",
		Subject:   c.Subject,
		Added:     c.Added,
		Removed:   c.Removed,
	}
}

// clone copies the sessions deeply enough that nothing below can be seen by
// the caller.
//
// Deep enough that nothing here writes to the caller's data.
//
// This used to copy only the turns and their commit lists, on the reasoning
// that the maps counting tools, files and lines were read and never written.
// Folding a sub-agent writes them: absorb adds the sub-agent's counts into the
// parent turn's own maps, and describe names the parent's delegation. So one
// build left the caller holding different numbers than it passed in, and a
// second build added the sub-agent's work on top again.
func clone(sessions []agent.Session) []agent.Session {
	out := make([]agent.Session, len(sessions))
	for i, s := range sessions {
		s.Turns = append([]agent.Turn(nil), s.Turns...)
		for j := range s.Turns {
			t := &s.Turns[j]
			t.Tools = copyCount(t.Tools)
			t.Files = copyCount(t.Files)
			t.Edits = copyCount(t.Edits)
			t.Lines = copyCount(t.Lines)
			t.Models = copyCount(t.Models)
			if t.Committed != nil {
				t.Committed = append([]agent.Commit(nil), t.Committed...)
			}
			if t.Delegated != nil {
				t.Delegated = append([]agent.Delegation(nil), t.Delegated...)
			}
		}
		out[i] = s
	}
	return out
}

// copyCount copies one counting map, keeping nil as nil so a turn that never
// had one does not gain an empty map and stop comparing equal to itself.
func copyCount(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
