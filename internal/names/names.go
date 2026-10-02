// Package names remembers what people call their projects.
//
// A project is named after its folder, and the folder is not always a good
// name. Codex makes one out of the first prompt ("so-i-have-basically-added-
// the"), and Pi can record a home directory, which then names the project after
// its owner. Nothing in the history can do better, so the person gets to.
//
// This is the one file bough writes. It lives in bough's own settings folder,
// never in an agent's history or in the project, so an upgrade, which only
// replaces the binary, keeps it.
package names

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nickelsec/bough/internal/agent"
)

// File is the name of the file inside the settings folder.
const File = "names.json"

// MaxLen is the longest name kept, in characters. A name is a label on a card,
// and one long enough to need wrapping has stopped being a name.
const MaxLen = 80

// version is written into the file so a later bough can tell what it is
// reading. Nothing reads it yet.
const version = 1

// Store is the names, keyed by the project's folder.
//
// The key is the folder rather than the folder and the agent, so a folder
// worked on with two agents is renamed once and both of its cards follow.
type Store struct {
	path  string
	names map[string]string

	// broken is why the file could not be understood, if it could not. A
	// store in that state reads as empty and refuses to write: saving would
	// replace a file bough did not understand, which could be somebody's names
	// in a shape a newer version wrote.
	broken error
}

type file struct {
	Version int               `json:"version"`
	Names   map[string]string `json:"names"`
}

// Dir is where bough keeps its settings: %AppData%\bough on Windows,
// ~/Library/Application Support/bough on a Mac, ~/.config/bough elsewhere.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "bough"), nil
}

// Open reads the names kept in dir. A missing file is no names, not an error.
//
// A file that cannot be read or parsed returns a usable, empty store along with
// the error, so the caller can say so and carry on. Names are a convenience,
// and a damaged file should not stop anyone seeing their history.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, File), names: map[string]string{}}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		s.broken = err
		return s, fmt.Errorf("reading %s: %w", s.path, err)
	}

	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		s.broken = err
		return s, fmt.Errorf("%s is not a names file bough understands: %w", s.path, err)
	}
	for k, v := range f.Names {
		if v = Clean(v); v != "" {
			s.names[key(k)] = v
		}
	}
	return s, nil
}

// Name is what the person called the project in this folder, if anything.
func (s *Store) Name(path string) (string, bool) {
	if s == nil || path == "" {
		return "", false
	}
	n, ok := s.names[key(path)]
	return n, ok
}

// Set names the project in this folder. An empty name, after cleaning, forgets
// the name and the folder's own comes back.
func (s *Store) Set(path, name string) error {
	if s == nil {
		return errors.New("no names file to write to")
	}
	if s.broken != nil {
		return fmt.Errorf("not changing %s, because it could not be read: %w", s.path, s.broken)
	}
	if path == "" {
		return errors.New("a project with no folder cannot be named")
	}

	k := key(path)
	before, had := s.names[k]
	if name = Clean(name); name == "" {
		delete(s.names, k)
	} else {
		s.names[k] = name
	}
	if err := s.save(); err != nil {
		// Put memory back the way the disk is, so a failed save does not
		// show a name that will be gone next time.
		if had {
			s.names[k] = before
		} else {
			delete(s.names, k)
		}
		return err
	}
	return nil
}

// save writes the whole file to a temporary name and then moves it into place,
// so a crash or a full disk leaves the old file rather than half of a new one.
func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file{Version: version, Names: s.names}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, File+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // a no-op once the rename has happened

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

// Clean makes text fit to be a name: one line, no control characters, no
// surrounding space, and no longer than MaxLen.
func Clean(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r) || r == utf8.RuneError:
			return -1
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > MaxLen {
		name = strings.TrimSpace(string([]rune(name)[:MaxLen]))
	}
	return name
}

// key is the folder as the rest of bough compares folders, so "D:\app" and
// "d:/app" are one project here as they are everywhere else.
func key(path string) string {
	return agent.NormalisePath(path)
}
