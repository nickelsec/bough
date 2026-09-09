package graph

import (
	"path"
	"sort"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// fold puts delegated work back inside the turn that asked for it.
//
// An agent that spawns another gets a session of its own, because the sub-agent
// has its own prompts, its own tools and its own token spend, and reading those
// into the parent would hide work that really happened. What it is not is a
// separate stretch of work. It runs inside one turn of the session that spawned
// it, usually finishing before that session's next prompt.
//
// Left alone it became a goal in its own right, which put a second date heading
// on a single afternoon and named it after an internal path like
// "/root/pixel_art". Both are wrong the same way: the diagram said the person
// started two things when they started one and it branched.
//
// So a sub-agent's turns are merged into the turn that delegated them. Its
// tools, files, edits, lines, errors and tokens are added to that turn, since
// all of it is part of what answering that prompt cost, and the hand-off the
// reader already draws under the prompt names the agent that ran.
//
// A session whose parent is not among these sessions is kept as it is. Showing
// work in the wrong place is a smaller wrong than losing it.
func fold(sessions []agent.Session) []agent.Session {
	here := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		here[s.ID] = true
	}

	// A session is folded only into a parent that is actually present. One
	// whose parent is missing stays where it is: showing work in the wrong
	// place is a smaller wrong than losing it.
	kids := map[string][]agent.Session{}
	keep := make([]agent.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.ParentID != "" && s.ParentID != s.ID && here[s.ParentID] {
			kids[s.ParentID] = append(kids[s.ParentID], s)
			continue
		}
		keep = append(keep, s)
	}
	if len(kids) == 0 {
		return sessions
	}

	// A sub-agent can spawn its own, so a child is merged only after whatever
	// it delegated has been merged into it. Walking the chain from each
	// surviving session downward reaches the deepest work first.
	for i := range keep {
		mergeInto(&keep[i], collect(kids, keep[i].ID))
	}
	return keep
}

// collect gathers a session's delegated work, deepest first, so that each child
// already carries its own sub-agents by the time it is merged upward.
func collect(kids map[string][]agent.Session, id string) []agent.Session {
	own := append([]agent.Session(nil), kids[id]...)
	for i := range own {
		if deeper := collect(kids, own[i].ID); len(deeper) > 0 {
			mergeInto(&own[i], deeper)
		}
	}
	sort.SliceStable(own, func(a, b int) bool {
		return firstAt(own[a]).Before(firstAt(own[b]))
	})
	return own
}

// mergeInto adds each child's work to the turn that delegated it.
func mergeInto(parent *agent.Session, kids []agent.Session) {
	for _, kid := range kids {
		idx := delegatingTurn(parent.Turns, firstAt(kid))
		if idx < 0 {
			continue
		}
		for _, t := range kid.Turns {
			absorb(&parent.Turns[idx], t)
		}
		describe(&parent.Turns[idx], kid)
	}
}

// delegatingTurn finds the turn a piece of delegated work belongs to.
//
// A sub-agent starts after the prompt that spawned it and finishes before the
// next one, so the last turn at or before its first record is the turn that
// asked for it. Nothing here reads the hand-off the parent recorded: the clock
// already answers the question, and the two agree.
//
// Work that begins before the session's first prompt has no turn to belong to
// and is left where it is rather than guessed at.
func delegatingTurn(turns []agent.Turn, at time.Time) int {
	idx := -1
	for i, t := range turns {
		if t.At.After(at) {
			break
		}
		idx = i
	}
	return idx
}

// absorb adds one turn's work to another.
//
// Prompts are not merged. A sub-agent's opening record is an internal task name
// rather than anything the reader typed, so it names the hand-off instead of
// joining the list of things somebody asked for.
func absorb(dst *agent.Turn, src agent.Turn) {
	addCounts(dst.Tools, src.Tools)
	addCounts(dst.Files, src.Files)
	addCounts(dst.Edits, src.Edits)
	addCounts(dst.Lines, src.Lines)
	addCounts(dst.Models, src.Models)

	dst.Errors += src.Errors
	dst.Tokens.Input += src.Tokens.Input
	dst.Tokens.Output += src.Tokens.Output
	dst.Tokens.CacheRead += src.Tokens.CacheRead
	dst.Tokens.CacheWrite += src.Tokens.CacheWrite
	dst.Committed = append(dst.Committed, src.Committed...)
	dst.Delegated = append(dst.Delegated, src.Delegated...)
}

func addCounts(dst, src map[string]int) {
	if dst == nil {
		return
	}
	for k, v := range src {
		dst[k] += v
	}
}

// describe records which sub-agent did the delegated work.
//
// What the sub-agent was asked to do is encrypted, so the only thing to hand is
// its task name, and a name is not a brief. Goal labels prefer a brief over the
// reader's own prompt, on the reasoning that the agent wrote a real description
// before handing work over. That holds when the brief is a sentence. It fails
// badly for a name like "/root/pixel_art", which named the whole afternoon
// after an internal path instead of after what was asked for.
//
// So the name goes on Kind, which says which agent ran, and Description is left
// alone. An honest blank beats a label nobody wrote.
func describe(dst *agent.Turn, kid agent.Session) {
	if len(kid.Turns) == 0 || kid.Turns[0].Text == "" {
		return
	}
	name := kid.Turns[0].Text

	// The hand-off is usually already recorded, from the spawn call in the
	// parent's own transcript. The two name the same agent differently: the
	// call says "pixel_art" where the sub-agent's transcript says the full path
	// "/root/pixel_art". Appending would show one delegation twice, so an
	// existing entry naming the same agent is left as it is.
	for i := range dst.Delegated {
		if sameAgent(dst.Delegated[i].Kind, name) {
			return
		}
	}
	for i := range dst.Delegated {
		if dst.Delegated[i].Kind == "" {
			dst.Delegated[i].Kind = name
			return
		}
	}
	dst.Delegated = append(dst.Delegated, agent.Delegation{Kind: name})
}

// sameAgent reports whether two names refer to one sub-agent. A spawn call
// names the task, its transcript names the path that task runs at, so the last
// segment is what the two have in common.
func sameAgent(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return path.Base(a) == path.Base(b)
}

func firstAt(s agent.Session) time.Time {
	if len(s.Turns) == 0 {
		return time.Time{}
	}
	return s.Turns[0].At
}
