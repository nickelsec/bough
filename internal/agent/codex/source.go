package codex

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

const maxLine = 16 << 20

// Source reads OpenAI Codex CLI session history.
type Source struct {
	// Root is where Codex stores session rollouts.
	// Empty means the default ~/.codex/sessions.
	Root string
}

// Name identifies this agent source.
// sourceName is what this package calls itself, in one place: the interface
// method below and every Project it produces both read it.
const sourceName = "codex"

// Name identifies this agent, as every Project it produces spells it.
func (Source) Name() string { return sourceName }

func (s Source) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "sessions"), nil
}

type projectGroup struct {
	files []string

	// shown is the path as it was written, kept for display. Grouping happens
	// on the normalised form, which lower cases and folds a Windows drive, and
	// that is not what anybody wants to read back.
	shown string
}

// Detect reports all projects Codex CLI has history for.
func (s Source) Detect() ([]agent.Project, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	byPath := map[string]*projectGroup{}

	// What could not be read is collected rather than dropped. A history that
	// is there but unreadable used to look exactly like a history that is not
	// there, so a project disappeared and nothing said why.
	var problems []error

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Recorded and stepped over: one unreadable path does not end
			// the walk.
			problems = append(problems, fmt.Errorf("looking in %s: %w", p, err))
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			problems = append(problems, fmt.Errorf("reading %s: %w", p, err))
			return nil
		}
		// An empty file is not a failure. A session that recorded nothing is
		// ordinary and there is nothing to say about it.
		if info.Size() == 0 {
			return nil
		}

		cwd := detectCWD(p)
		if cwd == "" {
			return nil
		}
		// Grouped on the normalised form, the same one every other path
		// comparison in the tool uses. filepath.Clean only understands the
		// separator the host happens to use, so the same project written two
		// ways in one session became two projects.
		key := agent.NormalisePath(cwd)

		g := byPath[key]
		if g == nil {
			g = &projectGroup{shown: filepath.Clean(cwd)}
			byPath[key] = g
		}
		g.files = append(g.files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var projects []agent.Project
	for _, g := range byPath {
		pPath := g.shown
		refJSON, err := json.Marshal(g.files)
		if err != nil {
			problems = append(problems, fmt.Errorf("recording the file list for %s: %w", pPath, err))
			continue
		}
		last, size := shell.Extent(g.files)

		projects = append(projects, agent.Project{
			Name:       filepath.Base(pPath),
			Path:       pPath,
			Source:     sourceName,
			Ref:        string(refJSON),
			LastWorked: last,
			Bytes:      size,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, errors.Join(problems...)
}

// Sessions reads every rollout transcript belonging to a Codex project.
//
// Rollouts are gathered by session id rather than read one file at a time.
// Resuming a session writes a fresh rollout that replays the earlier items
// under the ids they already had, so the repeats sit across files rather than
// within one. Reading each file on its own means the deduplication never sees
// the first copy, and a resumed session comes back as several sessions with
// its early prompts counted once per resume.
func (s Source) Sessions(p agent.Project) ([]agent.Session, error) {
	// A ref is the list of rollout files this project was found in, written by
	// Detect. Reading a malformed one as a single file path invented a path
	// nobody recorded and then reported the project as empty, which reads as
	// history that is not there rather than as a ref that could not be read.
	var files []string
	if err := json.Unmarshal([]byte(p.Ref), &files); err != nil {
		return nil, fmt.Errorf("reading the file list for %s: %w", p.Name, err)
	}

	var problems []error

	// Records by session, in the order the sessions were first seen.
	type group struct {
		id     string
		parent string
		recs   []*Record
	}
	var order []string
	byID := map[string]*group{}

	for _, fp := range files {
		f, err := os.Open(fp) //#nosec G304
		if err != nil {
			problems = append(problems, err)
			continue
		}
		recs, err := ReadRecords(f)
		_ = f.Close()
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if len(recs) == 0 {
			continue
		}

		id, parent := identify(recs, fp)
		g := byID[id]
		if g == nil {
			g = &group{id: id, parent: parent}
			byID[id] = g
			order = append(order, id)
		}
		g.recs = append(g.recs, recs...)
	}

	var sessions []agent.Session
	for _, id := range order {
		g := byID[id]
		// Rollouts are named by date and read in order, but a replay carries
		// the timestamps it had the first time, so sort by record rather than
		// trusting the order the files arrived in.
		sort.SliceStable(g.recs, func(i, j int) bool {
			return g.recs[i].Time().Before(g.recs[j].Time())
		})
		turns := ExtractTurns(g.recs)
		if len(turns) == 0 {
			continue
		}
		sessions = append(sessions, agent.Session{
			ID:       id,
			Title:    "",
			Turns:    turns,
			ParentID: g.parent,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Turns[0].At.Before(sessions[j].Turns[0].At)
	})
	return sessions, errors.Join(problems...)
}

// ReadRecords parses a Codex rollout transcript into lines of Record.
func ReadRecords(r io.Reader) ([]*Record, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)

	var recs []*Record
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		recs = append(recs, &rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

func detectCWD(transcriptPath string) string {
	f, err := os.Open(transcriptPath) //#nosec G304
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)

	for n := 0; n < 20 && sc.Scan(); n++ {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		if r.Type == "session_meta" {
			var sm SessionMeta
			if err := json.Unmarshal(r.Payload, &sm); err == nil && sm.Cwd != "" {
				if !isInternalPath(sm.Cwd) {
					return sm.Cwd
				}
			}
		}
		if r.Type == "turn_context" {
			var tc TurnContext
			if err := json.Unmarshal(r.Payload, &tc); err == nil && tc.Cwd != "" {
				if !isInternalPath(tc.Cwd) {
					return tc.Cwd
				}
			}
		}
	}
	return ""
}

// identify names the session a rollout belongs to, and the session that
// delegated it where there is one.
//
// A resumed rollout shares its predecessor's SessionID and joins it. A
// sub-agent's rollout also carries the parent's SessionID, but it is its own
// piece of work and keeps its own id, so it is grouped on ID instead. Without
// that split a spawned agent's prompts and tokens disappear into the agent that
// spawned it.
//
// The parent is returned alongside so the graph can put the work back where it
// happened: inside the turn that asked for it, rather than beside it.
func identify(recs []*Record, path string) (id, parent string) {
	for _, r := range recs {
		if r.Type != "session_meta" {
			continue
		}
		var sm SessionMeta
		if err := json.Unmarshal(r.Payload, &sm); err != nil {
			continue
		}
		if sm.ParentThreadID != "" && sm.ID != "" {
			return sm.ID, sm.ParentThreadID
		}
		if sm.SessionID != "" {
			return sm.SessionID, ""
		}
		if sm.ID != "" {
			return sm.ID, ""
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl"), ""
}

func isInternalPath(p string) bool {
	norm := strings.ReplaceAll(p, `\`, "/")
	return strings.Contains(norm, "/.codex/") || strings.HasPrefix(norm, "/tmp/")
}
