package server

import (
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/graph"
)

// The page says which agent wrote the history, whichever agent that was.
//
// It used to name Codex and say nothing for Claude Code, which read as though
// Claude were the absence of an agent rather than a choice of one. That was
// true while there was only one agent to read. It stopped being true at the
// second.
func TestThePageNamesTheAgent(t *testing.T) {
	for _, agent := range []string{"claude-code", "codex"} {
		g := graph.Graph{
			Schema:  graph.SchemaVersion,
			Project: graph.Project{Name: "a project", Path: "/p", Agent: agent},
		}
		b, err := render(g)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)

		if !strings.Contains(body, `id="agent"`) {
			t.Errorf("%s: the page has nowhere to put the agent's name", agent)
		}
		// The graph is inlined compact, so no space after the colon.
		if !strings.Contains(body, `"agent":"`+agent+`"`) {
			t.Errorf("%s: the graph reached the page without its agent", agent)
		}
	}
}

// And it spells the agent the way a person reads it, matching agent.Display so
// the page and the terminal cannot drift apart.
func TestThePageSpellsAgentsLikeTheTerminal(t *testing.T) {
	g := graph.Graph{Schema: graph.SchemaVersion, Project: graph.Project{Agent: "codex"}}
	b, err := render(g)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)

	for _, want := range []string{`"Claude Code"`, `"Codex"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page cannot spell %s", want)
		}
	}
}
