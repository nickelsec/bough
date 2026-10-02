package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nickelsec/bough/internal/graph"
)

// App is what the server shows: every project, and each one opened.
//
// It is an interface so the server knows nothing about where history lives or
// how it is read. The command line implements it; the tests fake it.
type App interface {
	// Projects is every project with history, in the order to show them. It
	// is asked again each time the home page loads, so a project started
	// while bough is open turns up on a refresh. It reads no history, only
	// where it is, so it is cheap.
	Projects() []Card

	// Summary works out the figures on a project's card. It reads the whole
	// history, so it is slow on a big one, and the server calls it in the
	// background rather than while anyone waits.
	Summary(id string) (Summary, error)

	// Graph reads a project in full, git included, to open it.
	Graph(id string) (graph.Graph, error)

	// Rename gives a project a name of its own, or with "" takes it away. It
	// returns the name the project now has.
	Rename(id, name string) (string, error)

	// Version is the bough doing the serving, for the foot of the home page.
	Version() string
}

// Card is a project as the home page lists it, before any of its history has
// been read.
type Card struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Folder string `json:"folder,omitempty"`
	Path   string `json:"path"`

	Agent     string `json:"agent"`
	AgentName string `json:"agentName"`

	LastWorked time.Time `json:"lastWorked"`
	Bytes      int64     `json:"bytes"`

	// Here marks the project in the folder bough was run from.
	Here bool `json:"here,omitempty"`
}

// version is what changes when a project's history does. A summary or a graph
// worked out for one version is good until the next.
func (c Card) version() string {
	return fmt.Sprintf("%d/%d", c.Bytes, c.LastWorked.UnixNano())
}

// Summary is the figures on a card.
type Summary struct {
	ActiveMinutes int `json:"activeMinutes"`
	Prompts       int `json:"prompts"`
	Sittings      int `json:"sittings"`
	Tasks         int `json:"tasks"`
	Files         int `json:"files"`
	Commits       int `json:"commits"`

	// Cost is absent when a model had no published rate, which is not the
	// same as free. Local is set when every model ran on the person's own
	// machine, so a $0 can say why.
	Cost  *float64 `json:"cost,omitempty"`
	Local bool     `json:"local,omitempty"`

	// Models did the work, most first.
	Models []string `json:"models,omitempty"`

	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// summary is one card's figures, or why there are none.
type summary struct {
	Summary

	version string
	err     string
}

// opened is a project read in full, kept until its history changes. Opening
// a big project reads every file and asks git about every commit, which takes
// seconds, and going back and forth between projects should not pay that
// each time.
type opened struct {
	version string
	g       graph.Graph
}

// site is the running server's state.
type site struct {
	app   App
	token string

	mu        sync.Mutex
	cards     []Card
	summaries map[string]summary
	graphs    map[string]opened
	wake      chan struct{}
}

func newSite(app App) (*site, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("making a token for this run: %w", err)
	}
	return &site{
		app:       app,
		token:     hex.EncodeToString(b),
		summaries: map[string]summary{},
		graphs:    map[string]opened{},
		wake:      make(chan struct{}, 1),
	}, nil
}

// refresh asks for the project list again and sets the summaries going for
// anything new or changed.
func (s *site) refresh() []Card {
	cards := s.app.Projects()
	s.mu.Lock()
	s.cards = cards
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return cards
}

// card finds a project by its id, asking for the list again if it is not in
// the last one, since it may have been started since.
func (s *site) card(id string) (Card, bool) {
	s.mu.Lock()
	cards := s.cards
	s.mu.Unlock()
	for _, c := range cards {
		if c.ID == id {
			return c, true
		}
	}
	for _, c := range s.refresh() {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}

// work fills in the summaries one project at a time, in the order the page
// shows them, so the cards at the top fill first.
//
// One at a time on purpose. Reading history is disk bound, several at once
// would not be faster, and the project somebody opens meanwhile should not have
// to queue behind all of them.
func (s *site) work(ctx context.Context) {
	for {
		s.mu.Lock()
		cards := s.cards
		s.mu.Unlock()

		for _, c := range cards {
			if ctx.Err() != nil {
				return
			}
			s.mu.Lock()
			have, ok := s.summaries[c.ID]
			s.mu.Unlock()
			if ok && have.version == c.version() {
				continue
			}
			sum, err := s.app.Summary(c.ID)
			got := summary{version: c.version(), Summary: sum}
			if err != nil {
				got.err = err.Error()
			}
			s.mu.Lock()
			s.summaries[c.ID] = got
			s.mu.Unlock()
		}

		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		}
	}
}

// graph reads a project in full, or returns the copy read earlier if its
// history has not changed since.
func (s *site) graph(c Card) (graph.Graph, error) {
	s.mu.Lock()
	have, ok := s.graphs[c.ID]
	s.mu.Unlock()
	if ok && have.version == c.version() {
		return have.g, nil
	}
	g, err := s.app.Graph(c.ID)
	if err != nil {
		return graph.Graph{}, err
	}
	s.mu.Lock()
	s.graphs[c.ID] = opened{version: c.version(), g: g}
	s.mu.Unlock()
	return g, nil
}

// routes wires up the pages, what they ask for, and the artwork.
//
// Everything goes through guard first.
func (s *site) routes(host string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		page, err := renderHome(s.refresh(), s.token, s.app.Version())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writePage(w, page)
	})

	mux.HandleFunc("GET /p/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := s.card(r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		g, err := s.graph(c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The graph is kept from when it was read, but a rename since then
		// should show, so the name is always today's.
		g.Project.Name = c.Name
		page, err := render(g, &pageApp{Home: "/", ID: c.ID, Token: s.token, Folder: c.Folder})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writePage(w, page)
	})

	mux.HandleFunc("GET /api/summaries", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		out := struct {
			Summaries map[string]Summary `json:"summaries"`
			Failed    map[string]string  `json:"failed,omitempty"`
			Done      bool               `json:"done"`
		}{Summaries: map[string]Summary{}, Failed: map[string]string{}, Done: true}
		for _, c := range s.cards {
			have, ok := s.summaries[c.ID]
			switch {
			case !ok || have.version != c.version():
				out.Done = false
			case have.err != "":
				out.Failed[c.ID] = have.err
			default:
				out.Summaries[c.ID] = have.Summary
			}
		}
		s.mu.Unlock()
		writeJSON(w, out)
	})

	mux.HandleFunc("POST /api/name", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Bough-Token") != s.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "not a rename", http.StatusBadRequest)
			return
		}
		if _, ok := s.card(req.ID); !ok {
			http.NotFound(w, r)
			return
		}
		name, err := s.app.Rename(req.ID, req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.refresh()
		writeJSON(w, map[string]string{"name": name})
	})

	mux.Handle("GET /img/", http.FileServer(http.FS(assets)))
	return guard(host, mux)
}

// guard turns away any request not addressed to this server by its own name.
//
// Listening on loopback keeps other machines out, but not other web pages. A
// page anywhere can point a name it controls at 127.0.0.1 and have the browser
// talk to this port as if it were that site, which would let it read a
// person's history. The browser still sends the name it thinks it is talking
// to, so anything that does not say exactly this server's address is refused.
func guard(host string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writePage(w http.ResponseWriter, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// History grows while bough is open, so nothing is kept by the browser.
	w.Header().Set("Cache-Control", "no-store")
	// A failed write means the browser went away mid-response, which is
	// normal and leaves nothing to do.
	_, _ = w.Write(page)
}

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
