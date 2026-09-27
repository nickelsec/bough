package pi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
)

// maxLine is the longest line read. A tool result holding a large file, or a
// sub-agent's whole conversation, makes for long lines.
const maxLine = 64 << 20

// sourceName is what this package calls itself, in one place: the interface
// method below and every Project it produces both read it.
const sourceName = "pi"

// Source reads pi coding agent history.
type Source struct {
	// Root is where Pi keeps its sessions. Empty means wherever Pi itself
	// would put them.
	Root string
}

// Name identifies this agent, as every Project it produces spells it.
func (Source) Name() string { return sourceName }

// root resolves the directory to read from, the way Pi resolves where to
// write: PI_CODING_AGENT_SESSION_DIR, then PI_CODING_AGENT_DIR with
// "sessions" under it, then ~/.pi/agent/sessions. See getAgentDir and
// getSessionsDir in packages/coding-agent/src/config.ts.
func (s Source) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	tilde := func(p string) string {
		if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			return filepath.Join(home, p[1:])
		}
		return p
	}
	if dir := os.Getenv("PI_CODING_AGENT_SESSION_DIR"); dir != "" {
		return tilde(dir), nil
	}
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return filepath.Join(tilde(dir), "sessions"), nil
	}
	return filepath.Join(home, ".pi", "agent", "sessions"), nil
}

// modelsFile is where Pi keeps the models set up by hand, which is what says
// whether a provider's endpoint is local. It sits in the agent directory, one
// level above the sessions. A root given by hand is taken to be a sessions
// directory inside an agent directory when it is called "sessions"; otherwise
// there is no models.json to read, and the fallback names apply.
func (s Source) modelsFile() string {
	if s.Root != "" {
		if filepath.Base(filepath.Clean(s.Root)) == "sessions" {
			return filepath.Join(filepath.Dir(filepath.Clean(s.Root)), "models.json")
		}
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
			dir = filepath.Join(home, dir[1:])
		}
		return filepath.Join(dir, "models.json")
	}
	return filepath.Join(home, ".pi", "agent", "models.json")
}

// Detect reports the projects Pi has history for.
//
// Pi keeps each working directory's sessions in a folder named after it, but
// the name cannot be turned back into the path: "-" stands for "/", "\" and ":"
// alike, and is also just a dash. So the folder is only where to look, and the
// path comes from each file's header. A session directory set by hand holds
// every project's files side by side with no folders at all, so files at the
// top level are read too.
//
// A file is Pi's only if its first line is Pi's header. Another agent's files
// under the same root are left alone, which is what lets one --root hold all
// three agents' history.
func (s Source) Detect() ([]agent.Project, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	top, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var problems []error
	var files []string
	for _, e := range top {
		p := filepath.Join(root, e.Name())
		if !e.IsDir() {
			if strings.HasSuffix(e.Name(), ".jsonl") {
				files = append(files, p)
			}
			continue
		}
		inner, err := os.ReadDir(p)
		if err != nil {
			problems = append(problems, fmt.Errorf("looking in %s: %w", p, err))
			continue
		}
		for _, f := range inner {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
				files = append(files, filepath.Join(p, f.Name()))
			}
		}
	}

	type group struct {
		files []string
		// shown is the path as it was written, kept for display. Grouping
		// happens on the normalised form.
		shown string
	}
	byPath := map[string]*group{}
	for _, fp := range files {
		h, err := readHeader(fp)
		if err != nil {
			problems = append(problems, fmt.Errorf("reading %s: %w", fp, err))
			continue
		}
		if h == nil || h.Cwd == "" {
			continue
		}
		key := agent.NormalisePath(h.Cwd)
		g := byPath[key]
		if g == nil {
			g = &group{shown: filepath.Clean(h.Cwd)}
			byPath[key] = g
		}
		g.files = append(g.files, fp)
	}

	var projects []agent.Project
	for _, g := range byPath {
		sort.Strings(g.files)
		ref, err := json.Marshal(g.files)
		if err != nil {
			problems = append(problems, fmt.Errorf("recording the file list for %s: %w", g.shown, err))
			continue
		}
		last, size := shell.Extent(g.files)
		projects = append(projects, agent.Project{
			Name:       lastElem(g.shown),
			Path:       g.shown,
			Source:     sourceName,
			Ref:        string(ref),
			LastWorked: last,
			Bytes:      size,
		})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, errors.Join(problems...)
}

// lastElem is a path's final element, whichever separator it was written with.
//
// filepath.Base only splits on the separator of the machine doing the reading,
// so a session recorded on Windows and read on Linux or a Mac came back named
// "C:\work\app" rather than "app". The path in a header is whatever the
// writing machine used, so both separators are split on.
func lastElem(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/")
	if i := strings.LastIndex(p, "/"); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}

// readHeader reads a file's first line and returns it if it is Pi's header,
// or nil if the file is someone else's. An empty file is nobody's.
func readHeader(fp string) (*Entry, error) {
	f, err := os.Open(fp) //#nosec G304 -- a session file this package found for itself.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		return header(line), nil
	}
	return nil, sc.Err()
}

// header parses a line as Pi's session header, or returns nil if it is not
// one. A Claude Code record carries a cwd too, so the type is what decides.
func header(line []byte) *Entry {
	var e Entry
	if json.Unmarshal(line, &e) != nil || e.Type != "session" {
		return nil
	}
	return &e
}

// Sessions reads every session belonging to a Pi project.
//
// A fork is the one thing that needs care. /fork and /clone start a new file
// holding a copy of the conversation so far, every entry under its original id
// and timestamp, and point back at the file they came from. Reading both files
// as they stand counts everything before the fork twice, turns and tokens
// alike. So an entry a session inherited from the file it was forked from is
// left to that file, and only what happened after the fork belongs to the new
// session. A fork whose original has gone is the only record of what it
// copied, and then the copy counts.
func (s Source) Sessions(p agent.Project) ([]agent.Session, error) {
	var files []string
	if err := json.Unmarshal([]byte(p.Ref), &files); err != nil {
		return nil, fmt.Errorf("reading the file list for %s: %w", p.Name, err)
	}

	ends := readEndpoints(s.modelsFile())
	var problems []error
	var sessions []agent.Session
	for _, fp := range files {
		entries, err := readFile(fp)
		if err != nil {
			problems = append(problems, fmt.Errorf("reading %s: %w", fp, err))
			continue
		}
		if len(entries) == 0 || entries[0].Type != "session" {
			continue
		}
		h := entries[0]
		own := entries[1:]
		if h.ParentSession != "" {
			inherited := ancestry(h.ParentSession, fp)
			if len(inherited) > 0 {
				kept := own[:0:0]
				for _, e := range own {
					if !inherited[key(e)] {
						kept = append(kept, e)
					}
				}
				own = kept
			}
		}

		turns := extract(own, h.Cwd, ends)
		if len(turns) == 0 {
			continue
		}
		id := h.ID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(fp), ".jsonl")
		}
		sessions = append(sessions, agent.Session{
			ID:    id,
			Title: title(own),
			Turns: turns,
		})
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Turns[0].At.Before(sessions[j].Turns[0].At)
	})
	return sessions, errors.Join(problems...)
}

// key identifies an entry across files. Ids are only eight hex characters and
// unique within a file, so the timestamp goes with it: a copied entry keeps
// both, and two unrelated entries sharing both is not going to happen.
func key(e *Entry) string { return e.ID + "|" + e.Timestamp }

// maxForks bounds how far back a chain of forks is followed, so a loop in the
// headers cannot hang the reader.
const maxForks = 64

// ancestry gathers the keys of every entry in the files a session was forked
// from, following the chain back.
//
// The recorded path is where the original was on the machine that wrote it.
// When history has been copied somewhere else, which is what reading with
// --root usually means, that path is gone, so a file of the same name beside
// this one is tried as well.
func ancestry(parent, self string) map[string]bool {
	keys := map[string]bool{}
	seen := map[string]bool{}
	for n := 0; parent != "" && n < maxForks; n++ {
		fp := locate(parent, self)
		if fp == "" || seen[fp] {
			break
		}
		seen[fp] = true
		entries, err := readFile(fp)
		if err != nil || len(entries) == 0 {
			break
		}
		for _, e := range entries[1:] {
			keys[key(e)] = true
		}
		parent = ""
		if entries[0].Type == "session" {
			parent = entries[0].ParentSession
		}
	}
	return keys
}

func locate(recorded, self string) string {
	if _, err := os.Stat(recorded); err == nil {
		return recorded
	}
	name := filepath.Base(strings.ReplaceAll(recorded, `\`, "/"))
	beside := filepath.Join(filepath.Dir(self), name)
	if _, err := os.Stat(beside); err == nil {
		return beside
	}
	return ""
}

// title is the name the person gave the session with /name, the latest one if
// it was renamed. Pi otherwise shows the first prompt, which bough already has.
func title(entries []*Entry) string {
	t := ""
	for _, e := range entries {
		if e.Type == "session_info" && e.Name != "" {
			t = e.Name
		}
	}
	return t
}

func readFile(fp string) ([]*Entry, error) {
	f, err := os.Open(fp) //#nosec G304 -- a session file this package found for itself.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadEntries(f)
}

// ReadEntries parses a Pi session file into its entries, header first.
//
// A line that does not parse is skipped rather than failing the file, the way
// Pi itself reads them. The last line is the usual culprit: Pi appends as it
// goes, so a session still being written can end partway through a line.
func ReadEntries(r io.Reader) ([]*Entry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	var out []*Entry
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil || e.Type == "" {
			continue
		}
		out = append(out, &e)
	}
	return out, sc.Err()
}
