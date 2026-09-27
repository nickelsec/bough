package server

import (
	"strings"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/graph"
)

// The page says which agent wrote the history, whichever agent that was.
//
// It used to name Codex and say nothing for Claude Code, which read as though
// Claude were the absence of an agent rather than a choice of one. That was
// true while there was only one agent to read. It stopped being true at the
// second.
func TestThePageNamesTheAgent(t *testing.T) {
	for _, a := range agent.Agents {
		source := a.Source
		g := graph.Graph{
			Schema:  graph.SchemaVersion,
			Project: graph.Project{Name: "a project", Path: "/p", Agent: source},
		}
		b, err := render(g)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)

		if !strings.Contains(body, `id="agent"`) {
			t.Errorf("%s: the page has nowhere to put the agent's name", source)
		}
		// The graph is inlined compact, so no space after the colon.
		if !strings.Contains(body, `"agent":"`+source+`"`) {
			t.Errorf("%s: the graph reached the page without its agent", source)
		}
	}
}

// And it spells the agent the way a person reads it, without holding its own
// copy of how.
//
// The page used to carry a switch mirroring agent.Display, with a comment
// saying so, which is exactly the arrangement that lets two things that must
// agree stop agreeing. The name is carried in the graph now, so there is one
// place that decides it.
func TestThePageSpellsAgentsLikeTheTerminal(t *testing.T) {
	for _, a := range agent.Agents {
		g := graph.Build(
			agent.Project{Name: "p", Path: "/p", Source: a.Source},
			nil,
			graph.Options{Now: func() time.Time { return time.Time{} }},
		)
		b, err := render(g)
		if err != nil {
			t.Fatal(err)
		}
		// The graph is inlined compact, so no space after the colon.
		if want := `"agentName":"` + a.Display + `"`; !strings.Contains(string(b), want) {
			t.Errorf("%s: the page was not handed the name to show", a.Source)
		}
	}

	// And the page does not work it out for itself.
	js, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	src := withoutComments(string(js))
	for _, a := range agent.Agents {
		if strings.Contains(src, `"`+a.Display+`"`) {
			t.Errorf("the page spells %q itself instead of reading it from the graph", a.Display)
		}
	}
}

// The dotted lines between sittings can be turned off from "What to show",
// and they start on: the drawing is complete until somebody asks otherwise.
func TestTheLinksSwitchStartsOn(t *testing.T) {
	page, err := assets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `id="f-links" role="switch" aria-checked="true"`) {
		t.Error("the links switch is missing or does not start on")
	}
	css, err := assets.ReadFile("bough.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "#tree.no-links .links { display: none; }") {
		t.Error("nothing hides the links when the switch is off")
	}
}
