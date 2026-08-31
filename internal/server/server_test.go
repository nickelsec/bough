package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/graph"
)

func sample() graph.Graph {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	return graph.Graph{
		Schema:  graph.SchemaVersion,
		Project: graph.Project{Name: "example", Path: "/work/example", Agent: "claude-code"},
		Goals: []graph.Goal{{
			ID:     "g1",
			Label:  "rework the export path",
			Period: "Sat 1 Aug",
			Stats:  graph.Stats{Start: start, End: start.Add(time.Hour), Turns: 2},
			Tasks: []graph.Task{{
				ID:    "g1.t1",
				Label: "rework the export path",
				Stats: graph.Stats{Turns: 2},
				Turns: []graph.Turn{
					{At: start, Text: "rework the export path"},
					{At: start.Add(time.Minute), Text: "keep going"},
				},
			}},
		}},
		Totals: graph.Stats{Turns: 2},
	}
}

// start runs a server and hands back its address, so a test can talk to it.
func start(t *testing.T, g graph.Graph) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	urls := make(chan string, 1)
	go Serve(ctx, g, func(u string) { urls <- u })

	select {
	case url := <-urls:
		return url
	case <-time.After(5 * time.Second):
		t.Fatal("the server never reported an address")
		return ""
	}
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(body)
}

// A person's coding history is theirs. Binding anything but loopback would put
// it on the network, so this is the test that matters most in this package.
func TestServesOnLoopbackOnly(t *testing.T) {
	url := start(t, sample())

	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("listening on %s, which is not loopback", url)
	}
}

func TestServesThePage(t *testing.T) {
	url := start(t, sample())
	resp, body := get(t, url+"/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("content type %q, want html", got)
	}
	for _, want := range []string{"<style>", "window.BOUGH", "id=\"tree\"", "example"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
}

// The page has to work with no network at all, and keep working when saved to
// disk, so everything it needs is already inside it.
func TestPageFetchesNothingExternal(t *testing.T) {
	url := start(t, sample())
	_, body := get(t, url+"/")

	// The server's own address is allowed to appear, and so is the SVG
	// namespace, which is an identifier rather than somewhere to fetch from.
	cleaned := strings.ReplaceAll(body, url, "")
	cleaned = strings.ReplaceAll(cleaned, "http://www.w3.org/2000/svg", "")

	for _, bad := range []string{"http://", "https://", "//cdn", "googleapis", "fonts.g"} {
		if strings.Contains(cleaned, bad) {
			t.Errorf("the page reaches out to %q", bad)
		}
	}
}

// Inlining the graph means the page is whole the moment it loads, with nothing
// to wait on and nothing to break if it is saved and opened later.
func TestGraphIsInlinedAndParses(t *testing.T) {
	g := sample()
	url := start(t, g)
	_, body := get(t, url+"/")

	const marker = "window.BOUGH = "
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("the graph was not inlined")
	}
	rest := body[i+len(marker):]
	end := strings.Index(rest, ";</script>")
	if end < 0 {
		t.Fatal("could not find the end of the inlined graph")
	}

	var parsed graph.Graph
	if err := json.Unmarshal([]byte(rest[:end]), &parsed); err != nil {
		t.Fatalf("the inlined graph is not valid JSON: %v", err)
	}
	if parsed.Project.Name != g.Project.Name {
		t.Errorf("inlined project is %q, want %q", parsed.Project.Name, g.Project.Name)
	}
	if len(parsed.Goals) != len(g.Goals) {
		t.Errorf("inlined %d goals, want %d", len(parsed.Goals), len(g.Goals))
	}
}

func TestServesTheArtwork(t *testing.T) {
	url := start(t, sample())

	for _, name := range []string{"logo.png", "icon-32.png", "icon-180.png"} {
		resp, body := get(t, url+"/img/"+name)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d, want 200", name, resp.StatusCode)
			continue
		}
		if len(body) == 0 {
			t.Errorf("%s came back empty", name)
		}
		if got := resp.Header.Get("Content-Type"); got != "image/png" {
			t.Errorf("%s: content type %q", name, got)
		}
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	url := start(t, sample())
	resp, _ := get(t, url+"/nothing-here")

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

// A project with nothing in it still has to produce a page rather than an
// error, since somebody will point this at a directory they barely used.
func TestEmptyGraphStillRenders(t *testing.T) {
	url := start(t, graph.Graph{
		Schema:  graph.SchemaVersion,
		Project: graph.Project{Name: "empty", Agent: "claude-code"},
	})
	resp, body := get(t, url+"/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(body, "empty") {
		t.Error("the page does not name the project")
	}
}

// Stopping should not leave the port held.
func TestShutdownIsClean(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(chan string, 1)
	done := make(chan error, 1)

	go func() { done <- Serve(ctx, sample(), func(u string) { urls <- u }) }()
	<-urls
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stopping gave an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("the server did not stop")
	}
}

// People paste markup into these conversations, so a prompt can contain the
// text that ends a script element. Left alone it would cut the page in half.
func TestPromptsCannotBreakOutOfTheScript(t *testing.T) {
	g := sample()
	g.Goals[0].Tasks[0].Turns[0].Text = `look at </script><script>alert(1)</script> this`

	url := start(t, g)
	_, body := get(t, url+"/")

	// The dangerous sequence must not survive into the page as written.
	if strings.Contains(body, "</script><script>alert") {
		t.Error("a prompt closed the script element and opened another")
	}
	// It still has to be there, since the prompt is what the reader came to
	// see. Go's encoder escapes the angle brackets on its way into JSON, which
	// already neutralises this; the replacer is a second line in case that
	// behaviour is ever turned off.
	if !strings.Contains(body, `u003c/script`) && !strings.Contains(body, `<\/script`) {
		t.Error("the prompt was lost rather than escaped")
	}
}

// A project name showing up in the title should be text, not markup.
func TestProjectNameIsEscaped(t *testing.T) {
	g := sample()
	g.Project.Name = `a<b"c&d`

	url := start(t, g)
	_, body := get(t, url+"/")

	if strings.Contains(body, `<title>a<b`) {
		t.Error("the project name went into the title unescaped")
	}
	if !strings.Contains(body, "a&lt;b&quot;c&amp;d") {
		t.Error("the project name was not escaped as expected")
	}
}
