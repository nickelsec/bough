package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/graph"
)

// The layout is arithmetic over the graph, so it is checked like arithmetic
// rather than by looking at it. A tree that tangles on somebody else's history
// is the main risk in the drawing, and it is catchable here.
//
// Node runs the check because the layout has to be the same code the browser
// uses. Reimplementing it in Go would test a copy rather than the thing.
func TestLayoutHoldsUp(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available to run the layout")
	}

	dir := t.TempDir()
	graphs := writeGraphs(t, dir)
	script := filepath.Join(dir, "check.js")
	if err := os.WriteFile(script, []byte(layoutCheck), 0o644); err != nil {
		t.Fatal(err)
	}

	args := append([]string{script, "layout.js"}, graphs...)
	out, err := exec.Command("node", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("layout checks failed:\n%s", out)
	}
	t.Logf("\n%s", out)
}

// writeGraphs produces the shapes worth checking: a busy project, a quiet one,
// one with a single sitting, and one with no links at all.
func writeGraphs(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string

	cases := map[string]graph.Graph{
		"busy":   synthetic(10, 11),
		"quiet":  synthetic(2, 1),
		"single": synthetic(1, 4),
		// Around three times wider than tall. Long enough not to fit at a
		// legible scale on a 1440 screen, short enough to fit whole on a 1920
		// one, which is the shape that used to shrink as the window grew. The
		// busy fixture is eight times wider and never comes close.
		"middling": synthetic(12, 3),
	}
	for name, g := range cases {
		body, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name+".json")
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

// synthetic builds a graph shaped like real history: sittings a day or two
// apart, each holding a few tasks of differing weight.
func synthetic(sittings, tasksEach int) graph.Graph {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	g := graph.Graph{Schema: graph.SchemaVersion, Project: graph.Project{Name: "synthetic"}}

	for s := 0; s < sittings; s++ {
		day := start.AddDate(0, 0, s*2)
		goal := graph.Goal{
			ID:     "g" + itoa(s+1),
			Label:  "a sitting",
			Period: day.Format("Mon 2 Jan"),
			Stats: graph.Stats{
				Start: day, End: day.Add(4 * time.Hour),
				Turns: 10 * (s + 1), Edits: 12 * (s + 1),
			},
		}
		for i := 0; i < tasksEach; i++ {
			goal.Tasks = append(goal.Tasks, graph.Task{
				ID:    goal.ID + ".t" + itoa(i+1),
				Label: "a piece of work",
				Stats: graph.Stats{
					Turns: i + 1, Edits: i * 4,
					// Every third task is hard, so the crooked path is exercised.
					Struggle: map[bool]float64{true: 0.7, false: 0.2}[i%3 == 0],
				},
				Turns: make([]graph.Turn, i+1),
			})
		}
		g.Goals = append(g.Goals, goal)
	}
	return g
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
