// Command bough shows the shape of the work in a project's AI coding history.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/claude"
	"github.com/nickelsec/bough/internal/graph"
	"github.com/nickelsec/bough/internal/pick"
)

// version is set at build time. Untagged builds say so.
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "bough:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("bough", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		asJSON  = fs.Bool("json", false, "write the graph as JSON instead of text")
		list    = fs.Bool("list", false, "list the projects with history and stop")
		verbose = fs.Bool("v", false, "include every prompt in the text output")
		root    = fs.String("root", "", "read history from here instead of the usual location")
		out     = fs.String("o", "", "write to this file instead of standard output")
	)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	// Flags are accepted before or after the project name. The standard parser
	// stops at the first argument that is not a flag, which would silently
	// ignore "bough taggity --json" and print the wrong thing.
	name, flags := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}

	src := claude.Source{Root: *root}
	projects, err := src.Detect()
	if err != nil {
		return fmt.Errorf("reading history: %w", err)
	}
	if len(projects) == 0 {
		return errors.New("no Claude Code history found; looked in ~/.claude/projects")
	}

	if *list {
		return writeList(stdout, src, projects)
	}

	target, err := choose(projects, name, stderr)
	if err != nil {
		return err
	}

	sessions, err := src.Sessions(target)
	if err != nil {
		// Some transcripts may be unreadable while others are fine, so say so
		// and carry on with what did load.
		fmt.Fprintf(stderr, "bough: some history could not be read: %v\n", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no readable history for %s", target.Name)
	}

	opt := graph.DefaultOptions()
	opt.Tool = version
	g := graph.Build(target, sessions, opt)

	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	}
	return graph.WriteText(w, g, *verbose)
}

// splitArgs separates the project name from the flags, so either order works.
func splitArgs(args []string) (name string, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// A flag that takes a value and was written with a space needs its
			// value kept alongside it.
			if valueFlags[strings.TrimLeft(a, "-")] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if name == "" {
			name = a
		}
	}
	return name, flags
}

// valueFlags are the flags that take a separate value.
var valueFlags = map[string]bool{"root": true, "o": true}

// choose decides which project to read.
//
// Naming a project reads that one. Otherwise the list is always offered, even
// when the current directory has history of its own, so that opening bough
// always shows what is there rather than jumping straight into one project.
// The project you are standing in is marked and put first, so the common case
// is still a single keypress.
func choose(projects []agent.Project, arg string, out io.Writer) (agent.Project, error) {
	if arg == "" {
		return offer(projects)
	}

	if p, ok := byPath(projects, arg); ok {
		return p, nil
	}

	var matches []agent.Project
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(arg)) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return agent.Project{}, fmt.Errorf("no project matching %q; try --list", arg)
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.Name)
		}
		return agent.Project{}, fmt.Errorf("%q matches several projects: %s", arg, strings.Join(names, ", "))
	}
}

// offer asks which project to read, with the one you are standing in first.
func offer(projects []agent.Project) (agent.Project, error) {
	projects, here := currentFirst(projects)

	items := make([]pick.Item, len(projects))
	for i, p := range projects {
		label := p.Name
		if i == 0 && here {
			label += "  (here)"
		}
		items[i] = pick.Item{Label: label, Detail: describe(p)}
	}

	i, err := pick.Choose("Which project?", items)
	if errors.Is(err, pick.ErrCancelled) {
		// Backing out is a decision, not a failure.
		os.Exit(0)
	}
	if err != nil {
		return agent.Project{}, err
	}
	return projects[i], nil
}

// currentFirst moves the project matching the working directory to the front,
// and reports whether one was found. The rest keep their order.
func currentFirst(projects []agent.Project) ([]agent.Project, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return projects, false
	}
	want := strings.ToLower(filepath.Clean(cwd))

	for i, p := range projects {
		if strings.ToLower(filepath.Clean(p.Path)) != want {
			continue
		}
		ordered := make([]agent.Project, 0, len(projects))
		ordered = append(ordered, p)
		ordered = append(ordered, projects[:i]...)
		return append(ordered, projects[i+1:]...), true
	}
	return projects, false
}

// describe is the dimmer text beside a project name, enough to tell which one
// is wanted without reading any of the history.
func describe(p agent.Project) string {
	size := ""
	switch {
	case p.Bytes >= 1<<20:
		size = fmt.Sprintf("%d MB", p.Bytes>>20)
	case p.Bytes > 0:
		size = fmt.Sprintf("%d KB", p.Bytes>>10)
	}
	if p.LastWorked.IsZero() {
		return size
	}
	return fmt.Sprintf("%s, %s", size, ago(p.LastWorked))
}

// ago says how long ago something happened, the way a person would.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/24/7))
	default:
		return t.Format("Jan 2006")
	}
}

// byPath matches a project by its working directory, allowing for the drive
// letter case drifting between records on Windows.
func byPath(projects []agent.Project, path string) (agent.Project, bool) {
	want := strings.ToLower(filepath.Clean(path))
	for _, p := range projects {
		if strings.ToLower(filepath.Clean(p.Path)) == want {
			return p, true
		}
	}
	return agent.Project{}, false
}

func writeList(w io.Writer, src agent.Source, projects []agent.Project) error {
	for _, p := range projects {
		sessions, _ := src.Sessions(p)
		turns := 0
		for _, s := range sessions {
			turns += len(s.Turns)
		}
		fmt.Fprintf(w, "%-24s %-40s %d prompts\n", p.Name, p.Path, turns)
	}
	return nil
}

const usage = `bough shows the shape of the work in a project's AI coding history.

  bough              read the project in the current directory
  bough taggity      read a project by name
  bough --list       show which projects have history
  bough --json       write the graph as JSON

Options:
`
