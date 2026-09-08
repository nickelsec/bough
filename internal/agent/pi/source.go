package pi

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

// maxLine is the scanner buffer ceiling for JSONL lines.
const maxLine = 16 << 20

// Source reads Pi session history.
type Source struct {
	// Root is where Pi keeps its session directories.
	// Empty means the default ~/.pi/agent/sessions.
	Root string
}

// Name identifies this agent source.
func (Source) Name() string { return "pi" }

func (s Source) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent", "sessions"), nil
}

// Detect reports all projects Pi has history for.
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

		path := workingDirectory(transcripts)
		name := filepath.Base(path)
		if path == "" {
			path = dir
			name = strings.Trim(e.Name(), "-")
		}

		last, size := extent(transcripts)
		projects = append(projects, agent.Project{
			Name:       name,
			Path:       path,
			Source:     "pi",
			Ref:        dir,
			LastWorked: last,
			Bytes:      size,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

// Sessions reads every transcript belonging to a Pi project.
func (s Source) Sessions(p agent.Project) ([]agent.Session, error) {
	files, err := transcriptFiles(p.Ref)
	if err != nil {
		return nil, err
	}

	var sessions []agent.Session
	var problems []error

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
		turns := ExtractTurns(recs)
		if len(turns) == 0 {
			continue
		}

		sessions = append(sessions, agent.Session{
			ID:    sessionID(recs, fp),
			Title: "",
			Turns: turns,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Turns[0].At.Before(sessions[j].Turns[0].At)
	})
	return sessions, errors.Join(problems...)
}

// ReadRecords parses a Pi transcript into lines of Record.
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

func workingDirectory(files []string) string {
	for _, fp := range files {
		if cwd := firstCWD(fp); cwd != "" {
			return filepath.Clean(cwd)
		}
	}
	return ""
}

func firstCWD(path string) string {
	f, err := os.Open(path) //#nosec G304
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for n := 0; n < 20 && sc.Scan(); n++ {
		var probe struct {
			Type string `json:"type"`
			CWD  string `json:"cwd"`
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

func sessionID(recs []*Record, path string) string {
	for _, r := range recs {
		if r.Type == "session" && r.ID != "" {
			return r.ID
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}
