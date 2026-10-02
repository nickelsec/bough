package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/graph"
)

func TestMain(m *testing.M) {
	// A test server has nobody to show it to.
	openBrowser = func(string) {}
	os.Exit(m.Run())
}

// page is where the fake's one project opens.
const page = "/p/p1"

// fake is an App with one project in it, so the server can be tested without
// any history on disk.
type fake struct {
	g graph.Graph

	mu      sync.Mutex
	name    string
	renamed []string
	slow    chan struct{} // when set, Summary waits for it
}

func (f *fake) Projects() []Card {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := f.name
	if name == "" {
		name = f.g.Project.Name
	}
	return []Card{{
		ID: "p1", Name: name, Path: f.g.Project.Path,
		Agent: "claude-code", AgentName: "Claude Code",
		LastWorked: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC), Bytes: 100,
	}}
}

func (f *fake) Summary(id string) (Summary, error) {
	if f.slow != nil {
		<-f.slow
	}
	if id != "p1" {
		return Summary{}, errors.New("no such project")
	}
	return Summary{ActiveMinutes: 42, Prompts: 2, Sittings: 1, Tasks: 1}, nil
}

func (f *fake) Graph(id string) (graph.Graph, error) {
	if id != "p1" {
		return graph.Graph{}, errors.New("no such project")
	}
	return f.g, nil
}

func (f *fake) Version() string { return "v9.9.9" }

func (f *fake) Rename(_ string, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renamed = append(f.renamed, name)
	f.name = name
	return name, nil
}

func TestTheHomePageListsTheProjects(t *testing.T) {
	url := start(t, sample())
	resp, body := get(t, url+"/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(body, `"name":"example"`) || !strings.Contains(body, `"id":"p1"`) {
		t.Errorf("the home page does not carry the project:\n%.400s", body)
	}
}

// The foot of the home page names the bough serving it.
func TestTheHomePageSaysWhichBoughThisIs(t *testing.T) {
	url := start(t, sample())
	_, body := get(t, url+"/")
	if !strings.Contains(body, `"version":"v9.9.9"`) {
		t.Error("the home page does not carry the version")
	}
}

func TestAProjectThatIsNotThereIsNotFound(t *testing.T) {
	url := start(t, sample())
	resp, _ := get(t, url+"/p/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

// The project page knows its way home, and what to send to rename it. A page
// saved to disk is rendered without either, and has to say so.
func TestTheProjectPageKnowsTheWayHome(t *testing.T) {
	url := start(t, sample())
	_, body := get(t, url+page)
	if !strings.Contains(body, `window.BOUGH_APP = {"home":"/","id":"p1"`) {
		t.Errorf("the page was not told where home is")
	}

	alone, err := render(sample(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(alone, []byte("window.BOUGH_APP = null")) {
		t.Errorf("a page with no server behind it should say so")
	}
}

func TestSummariesArriveWhenTheyAreReady(t *testing.T) {
	g := sample()
	f := &fake{g: g, slow: make(chan struct{})}
	url := startApp(t, f)

	var got struct {
		Summaries map[string]Summary `json:"summaries"`
		Done      bool               `json:"done"`
	}
	_, body := get(t, url+"/api/summaries")
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.Done || len(got.Summaries) != 0 {
		t.Fatalf("figures reported before they were worked out: %s", body)
	}

	close(f.slow)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, body = get(t, url+"/api/summaries")
		_ = json.Unmarshal([]byte(body), &got)
		if got.Done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !got.Done || got.Summaries["p1"].ActiveMinutes != 42 {
		t.Fatalf("the figures never arrived: %s", body)
	}
}

func TestRenamingNeedsThePagesToken(t *testing.T) {
	f := &fake{g: sample()}
	url := startApp(t, f)

	resp := post(t, url+"/api/name", "", `{"id":"p1","name":"Shiny"}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a rename with no token got %d, want 403", resp.StatusCode)
	}
	if len(f.renamed) != 0 {
		t.Fatal("the rename went through anyway")
	}

	// The token is the one in the page.
	_, home := get(t, url+"/")
	i := strings.Index(home, `"token":"`)
	if i < 0 {
		t.Fatal("the home page carries no token")
	}
	token := home[i+9 : i+9+32]

	resp = post(t, url+"/api/name", token, `{"id":"p1","name":"Shiny"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a rename with the token got %d", resp.StatusCode)
	}
	_, home = get(t, url+"/")
	if !strings.Contains(home, `"name":"Shiny"`) {
		t.Error("the home page does not show the new name")
	}
	_, proj := get(t, url+page)
	if !strings.Contains(proj, "<title>Shiny") {
		t.Error("the project page does not show the new name")
	}
}

// A web page elsewhere can point a name it owns at 127.0.0.1 and have the
// browser talk to bough as if it were that site. The browser still says which
// name it used, and anything but bough's own address is turned away.
func TestRequestsForAnotherNameAreRefused(t *testing.T) {
	url := start(t, sample())
	for _, path := range []string{"/", page, "/api/summaries"} {
		req, _ := http.NewRequest(http.MethodGet, url+path, nil)
		req.Host = "evil.example:80"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s under another name got %d, want 403", path, resp.StatusCode)
		}
	}
}

func startApp(t *testing.T, app App) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(chan string, 1)
	go Serve(ctx, app, "/", func(u string) { urls <- u })
	t.Cleanup(cancel)
	select {
	case u := <-urls:
		return strings.TrimSuffix(u, "/")
	case <-time.After(5 * time.Second):
		t.Fatal("the server never reported an address")
		return ""
	}
}

func post(t *testing.T, url, token, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Bough-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}
