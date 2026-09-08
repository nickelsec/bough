package antigravity

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

// Source reads Antigravity session history.
type Source struct {
	// Root is where Antigravity keeps conversation folders.
	// Empty means the default ~/.gemini/antigravity-cli/brain.
	Root string
}

// Name identifies this agent source.
func (Source) Name() string { return "antigravity" }

func (s Source) root() (string, error) {
	if s.Root != "" {
		return s.Root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", "brain"), nil
}

type projectGroup struct {
	files []string
}

// Detect reports all projects Antigravity has history for.
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

	byPath := map[string]*projectGroup{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		transcript := filepath.Join(root, e.Name(), ".system_generated", "logs", "transcript.jsonl")
		info, err := os.Stat(transcript)
		if err != nil || info.Size() == 0 {
			continue
		}

		cwd := detectCWD(transcript)
		if cwd == "" {
			continue
		}
		cwd = filepath.Clean(cwd)

		g := byPath[cwd]
		if g == nil {
			g = &projectGroup{}
			byPath[cwd] = g
		}
		g.files = append(g.files, transcript)
	}

	var projects []agent.Project
	for pPath, g := range byPath {
		refJSON, _ := json.Marshal(g.files)
		last, size := extent(g.files)

		projects = append(projects, agent.Project{
			Name:       filepath.Base(pPath),
			Path:       pPath,
			Source:     "antigravity",
			Ref:        string(refJSON),
			LastWorked: last,
			Bytes:      size,
		})
	}

	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	return projects, nil
}

// Sessions reads every transcript belonging to an Antigravity project.
func (s Source) Sessions(p agent.Project) ([]agent.Session, error) {
	var files []string
	if err := json.Unmarshal([]byte(p.Ref), &files); err != nil {
		// Fallback: if Ref is a single directory path
		files = []string{p.Ref}
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

		convID := conversationID(fp)
		sessions = append(sessions, agent.Session{
			ID:    convID,
			Title: "",
			Turns: turns,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Turns[0].At.Before(sessions[j].Turns[0].At)
	})
	return sessions, errors.Join(problems...)
}

// ReadRecords parses an Antigravity transcript into lines of Record.
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

	var fallbackDir string
	for n := 0; n < 200 && sc.Scan(); n++ {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		for _, tc := range r.ToolCalls {
			if cwd := CleanArg(tc.Args["Cwd"]); cwd != "" && !isInternalPath(cwd) {
				return cwd
			}
			if fallbackDir == "" {
				if dir := CleanArg(tc.Args["DirectoryPath"]); dir != "" && !isInternalPath(dir) {
					fallbackDir = dir
				} else if dir := CleanArg(tc.Args["SearchDirectory"]); dir != "" && !isInternalPath(dir) {
					fallbackDir = dir
				} else {
					for _, k := range []string{"TargetFile", "AbsolutePath", "SearchPath"} {
						if p := CleanArg(tc.Args[k]); p != "" && filepath.IsAbs(p) && !isInternalPath(p) {
							fallbackDir = filepath.Dir(p)
							break
						}
					}
				}
			}
		}
	}
	return fallbackDir
}

func isInternalPath(p string) bool {
	norm := strings.ReplaceAll(p, `\`, "/")
	return strings.Contains(norm, "/.gemini/") || strings.HasPrefix(norm, "/tmp/")
}

func conversationID(transcriptPath string) string {
	parts := strings.Split(filepath.Clean(transcriptPath), string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == ".system_generated" && i > 0 {
			return parts[i-1]
		}
	}
	return filepath.Base(transcriptPath)
}
