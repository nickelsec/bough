package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/graph"
	"github.com/nickelsec/bough/internal/names"
	"github.com/nickelsec/bough/internal/price"
	"github.com/nickelsec/bough/internal/repo"
	"github.com/nickelsec/bough/internal/server"
)

// library is every project bough can read, as the browser asks for them. It
// is what the server shows; the server itself knows nothing about agents,
// names or git.
type library struct {
	sources []agent.Source
	byName  map[string]agent.Source
	store   *names.Store
	here    string
	noRepo  bool
	tool    string

	// warn is where a project that would not read is reported. Opening one
	// in the browser is the moment somebody wants to know why it is short.
	warn io.Writer

	mu       sync.Mutex
	projects map[string]agent.Project
}

// projectID names a project in a URL. It is the agent and the folder, so it
// stays the same from one run to the next, and hashed, so a path full of
// spaces and backslashes never has to survive being a URL.
func projectID(p agent.Project) string {
	sum := sha256.Sum256([]byte(p.Source + "\x00" + agent.NormalisePath(p.Path)))
	return hex.EncodeToString(sum[:6])
}

// detect finds every project again, names them, and puts them in the order
// the home page shows: the one bough was run from, then the most recent.
func (h *library) detect() []agent.Project {
	var found []agent.Project
	for _, s := range h.sources {
		// Reported once, when bough started. Saying it again on every
		// refresh of the home page would bury anything else on the terminal.
		ps, _ := s.Detect()
		found = append(found, ps...)
	}
	applyNames(found, h.store)
	sort.SliceStable(found, func(i, j int) bool {
		return found[i].LastWorked.After(found[j].LastWorked)
	})
	found, _ = currentFirst(found, h.here)

	h.mu.Lock()
	h.projects = make(map[string]agent.Project, len(found))
	for _, p := range found {
		h.projects[projectID(p)] = p
	}
	h.mu.Unlock()
	return found
}

func (h *library) project(id string) (agent.Project, error) {
	h.mu.Lock()
	p, ok := h.projects[id]
	h.mu.Unlock()
	if !ok {
		return agent.Project{}, fmt.Errorf("no project %s", id)
	}
	return p, nil
}

// Projects implements server.App.
func (h *library) Projects() []server.Card {
	found := h.detect()
	here := agent.NormalisePath(h.here)
	cards := make([]server.Card, 0, len(found))
	for _, p := range found {
		cards = append(cards, server.Card{
			ID:         projectID(p),
			Name:       p.Name,
			Folder:     p.Folder,
			Path:       p.Path,
			Agent:      p.Source,
			AgentName:  agent.Display(p.Source),
			LastWorked: p.LastWorked,
			Bytes:      p.Bytes,
			Here:       here != "" && agent.NormalisePath(p.Path) == here,
		})
	}
	return cards
}

// build reads one project into a graph, asking git about it only when asked
// to.
func (h *library) build(id string, withRepo bool) (graph.Graph, error) {
	p, err := h.project(id)
	if err != nil {
		return graph.Graph{}, err
	}
	src := h.byName[p.Source]
	if src == nil {
		return graph.Graph{}, fmt.Errorf("%s came from %q, which is not one of the agents being read", p.Name, p.Source)
	}
	sessions, err := src.Sessions(p)
	if err != nil {
		fmt.Fprintf(h.warn, "bough: some of %s could not be read: %v\n", p.Name, err)
	}
	if len(sessions) == 0 {
		return graph.Graph{}, fmt.Errorf("no readable history for %s", p.Name)
	}
	opt := graph.DefaultOptions()
	opt.Tool = h.tool
	if withRepo && !h.noRepo {
		opt.Repo = repo.Read(p.Path)
	}
	return graph.Build(p, sessions, opt), nil
}

// Graph implements server.App.
func (h *library) Graph(id string) (graph.Graph, error) {
	return h.build(id, true)
}

// Summary implements server.App.
//
// Git is left out. It is the slow part of reading a big project, and nothing
// on a card needs it: the commit count there is what the agent recorded.
func (h *library) Summary(id string) (server.Summary, error) {
	g, err := h.build(id, false)
	if err != nil {
		return server.Summary{}, err
	}
	return summarise(g), nil
}

// summarise reduces a graph to the figures on its card.
func summarise(g graph.Graph) server.Summary {
	t := g.Totals
	s := server.Summary{
		ActiveMinutes: t.ActiveMinutes,
		Prompts:       t.Turns,
		Sittings:      len(g.Goals),
		Files:         t.Files,
		Commits:       len(t.Commits),
		Cost:          t.Cost,
		Start:         t.Start,
		End:           t.End,
	}
	for _, goal := range g.Goals {
		s.Tasks += len(goal.Tasks)
	}

	type share struct {
		model string
		n     int
	}
	shares := make([]share, 0, len(t.Models))
	local := len(t.Models) > 0
	for m, tk := range t.Models {
		shares = append(shares, share{m, tk.Total()})
		local = local && strings.HasSuffix(m, price.Local)
	}
	sort.Slice(shares, func(i, j int) bool {
		if shares[i].n != shares[j].n {
			return shares[i].n > shares[j].n
		}
		return shares[i].model < shares[j].model
	})
	for _, sh := range shares {
		s.Models = append(s.Models, sh.model)
	}
	s.Local = local
	return s
}

// Version implements server.App.
func (h *library) Version() string { return h.tool }

// Rename implements server.App.
func (h *library) Rename(id, name string) (string, error) {
	if h.store == nil {
		return "", errors.New("there is no settings folder on this machine to keep names in")
	}
	p, err := h.project(id)
	if err != nil {
		return "", err
	}
	if err := h.store.Set(p.Path, name); err != nil {
		return "", err
	}
	if n, ok := h.store.Name(p.Path); ok {
		return n, nil
	}
	if p.Folder != "" {
		return p.Folder, nil
	}
	return p.Name, nil
}
