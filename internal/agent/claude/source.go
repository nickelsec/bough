package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// Source reads Claude Code history.
type Source struct {
	// Root is where Claude Code keeps its projects. Empty means the usual
	// location under the user's home directory.
	Root string
}

// Name identifies this agent.
func (Source) Name() string { return "claude-code" }

// root resolves the directory to read from.
func (s Source) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// Detect reports the projects Claude Code has history for.
//
// Not being installed is not a failure. A machine without Claude Code simply
// has no projects, which lets a caller ask every agent it knows about without
// special casing any of them.
func (s Source) Detect() ([]agent.Project, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []agent.Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		transcripts, err := transcriptFiles(dir)
		if err != nil || len(transcripts) == 0 {
			continue
		}

		// The directory name is the working directory with its separators
		// replaced by dashes, which cannot be reversed. The real path is on the
		// records, so read the first one that carries it.
		path := workingDirectory(transcripts)
		name := filepath.Base(path)
		if path == "" {
			path = dir
			name = e.Name()
		}

		last, size := extent(transcripts)
		projects = append(projects, agent.Project{
			Name:       name,
			Path:       path,
			Source:     "claude-code",
			Ref:        dir,
			LastWorked: last,
			Bytes:      size,
		})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

// Sessions reads every transcript belonging to a project.
//
// A transcript that cannot be read is reported without abandoning the others,
// since partial history is still worth showing.
func (s Source) Sessions(p agent.Project) ([]agent.Session, error) {
	files, err := transcriptFiles(p.Ref)
	if err != nil {
		return nil, err
	}

	var sessions []agent.Session
	var problems []error
	for _, fp := range files {
		// Reading transcripts is what this package is for. The path came from
		// walking the history directory, not from anything a caller supplied.
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
		turns := ExtractTurns(recs)
		if len(turns) == 0 {
			if err := unreadable(recs); err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", filepath.Base(fp), err))
			}
			continue
		}
		sessions = append(sessions, agent.Session{
			ID:    sessionID(recs, fp),
			Title: sessionTitle(recs),
			Turns: turns,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Turns[0].At.Before(sessions[j].Turns[0].At)
	})
	return sessions, errors.Join(problems...)
}

// unreadable explains why a transcript yielded nothing, when the reason is
// worth telling somebody about.
//
// A prompt is recognised by its promptId, and Claude Code only began writing
// that field partway through its life. A transcript from before then still
// holds a full conversation but draws as an empty project, which looks like
// bough losing the work rather than declining to guess at it. An empty file,
// or one holding only tool traffic, is ordinary and says nothing.
func unreadable(recs []*Record) error {
	said := 0
	for _, r := range recs {
		if r.Type != "user" || r.IsSidechain || r.Message == nil {
			continue
		}
		if r.PromptID != "" {
			return nil
		}
		if r.PromptText() != "" {
			said++
		}
	}
	if said == 0 {
		return nil
	}
	return fmt.Errorf("%d prompts carry no promptId, so this transcript is too "+
		"old for bough to read; it predates the version of Claude Code that "+
		"started recording the field", said)
}

// transcriptFiles lists the session files in a project directory.
//
// Only the top level counts. The same directory also holds subagents,
// tool-results and memory, none of which are sessions, so walking it would
// pick up files that are not transcripts at all.
func transcriptFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// extent reports when a project was last worked on and how much history it
// holds, taken from the files themselves so that listing projects does not
// mean parsing them.
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

// workingDirectory recovers the real project path from the records.
//
// The cwd is stamped on almost every record, so this reads the opening lines of
// a transcript rather than the whole thing. Parsing entire sessions to learn one
// string made listing projects as slow as reading all of the history.
func workingDirectory(files []string) string {
	for _, fp := range files {
		if cwd := firstCWD(fp); cwd != "" {
			return filepath.Clean(cwd)
		}
	}
	return ""
}

// cwdProbeLines is how far into a transcript to look for a working directory.
// The field appears within the first few records in practice, and stopping
// early keeps project listing fast on large histories.
const cwdProbeLines = 200

// firstCWD reads the start of a transcript looking for a working directory.
func firstCWD(path string) string {
	// As above: a transcript this package found for itself.
	f, err := os.Open(path) //#nosec G304
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for n := 0; n < cwdProbeLines && sc.Scan(); n++ {
		var probe struct {
			CWD string `json:"cwd"`
		}
		if err := json.Unmarshal(sc.Bytes(), &probe); err != nil {
			continue
		}
		if probe.CWD != "" {
			return probe.CWD
		}
	}
	return ""
}

// sessionTitle returns the label Claude Code gave the session.
//
// Claude Code names sessions itself and the names are specific enough to be
// worth reusing, so there is no need to infer one. Falling back to the slug
// keeps something readable when a session was never titled.
func sessionTitle(recs []*Record) string {
	slug := ""
	for _, r := range recs {
		if r.AITitle != "" {
			return r.AITitle
		}
		if slug == "" && r.Slug != "" {
			slug = r.Slug
		}
	}
	return slug
}

// sessionID returns the session identifier, falling back to the file name,
// which is what Claude Code names the file after anyway.
func sessionID(recs []*Record, path string) string {
	for _, r := range recs {
		if r.SessionID != "" {
			return r.SessionID
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}
