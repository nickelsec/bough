package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

const maxLine = 16 << 20

// Source reads OpenAI Codex CLI session history.
type Source struct {
	// Root is where Codex stores session rollouts.
	// Empty means the default ~/.codex/sessions.
	Root string
}

// Name identifies this agent source.
func (Source) Name() string { return "codex" }

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

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip inaccessible paths while discovering sessions
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() == 0 {
			return nil //nolint:nilerr // skip unreadable or empty files
		}

		cwd := detectCWD(p)
		if cwd == "" {
			return nil
		}
		cwd = filepath.Clean(cwd)

		g := byPath[cwd]
		if g == nil {
			g = &projectGroup{}
			byPath[cwd] = g
		}
		g.files = append(g.files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var projects []agent.Project
	for pPath, g := range byPath {
		refJSON, err := json.Marshal(g.files)
		if err != nil {
			continue
		}
		last, size := extent(g.files)

		projects = append(projects, agent.Project{
			Name:       filepath.Base(pPath),
			Path:       pPath,
			Source:     "codex",
			Ref:        string(refJSON),
			LastWorked: last,
			Bytes:      size,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
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
	var files []string
	if err := json.Unmarshal([]byte(p.Ref), &files); err != nil {
		files = []string{p.Ref}
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

func extent(files []string) (time.Time, int64) {
	var last time.Time
	var size int64
	for _, fp := range files {
		info, err := os.Stat(fp)
		if err != nil {
			continue
		}
		size += info.Size()
		if info.ModTime().After(last) {
			last = info.ModTime()
		}
	}
	return last, size
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
