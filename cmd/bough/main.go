// Command bough shows the shape of the work in a project's AI coding history.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/claude"
	"github.com/nickelsec/bough/internal/agent/codex"
	"github.com/nickelsec/bough/internal/banner"
	"github.com/nickelsec/bough/internal/graph"
	"github.com/nickelsec/bough/internal/pick"
	"github.com/nickelsec/bough/internal/repo"
	"github.com/nickelsec/bough/internal/server"
)

// version is stamped by the release build. Anything else asks the toolchain.
var version = ""

// released reports what to print for --version.
//
// The release workflow passes the tag in, but that is not how most people get
// this. Both installs the readme documents go through the toolchain instead,
// and a plain build has nothing passed in at all, so every one of them used to
// answer "dev" including `go install ...@v0.3.4`. Go records the version it
// resolved, so ask for it rather than claiming not to know.
//
// A build from a working copy has no module version and reports "(devel)".
// That case keeps the revision, which is the part that identifies it, and says
// when the tree had uncommitted changes so a report naming it can be trusted.
func released() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 12 {
				s.Value = s.Value[:12]
			}
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = ", modified"
			}
		}
	}
	if revision == "" {
		return "dev"
	}
	return "dev (" + revision + dirty + ")"
}

func main() {
	env := Env{In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Dir: workingDir()}
	if err := run(os.Args[1:], env); err != nil {
		fmt.Fprintln(os.Stderr, "bough:", err)
		os.Exit(1)
	}
}

// Env is everything run reads from the world besides its arguments.
//
// These used to be reached for where they were needed: run passed os.Stdin to
// the chooser and the chooser asked the operating system for the working
// directory itself. That left choosing and cancelling impossible to drive
// through run, so the test that claimed to cover cancelling tested a copy of
// the rule kept in the test file, and passed with the real rule deleted.
type Env struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	// Dir is where bough was run, which decides the project offered first. It
	// may be empty: the question does not always have an answer.
	Dir string
}

func run(args []string, env Env) error {
	stdout, stderr := env.Out, env.Err
	fs := flag.NewFlagSet("bough", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		asJSON    = fs.Bool("json", false, "write the graph as JSON instead of text")
		asText    = fs.Bool("text", false, "write to the terminal instead of opening a browser")
		list      = fs.Bool("list", false, "list the projects with history and stop")
		verbose   = fs.Bool("v", false, "include every prompt in the text output")
		root      = fs.String("root", "", "read history from here instead of the usual location")
		out       = fs.String("o", "", "write to this file instead of standard output")
		showVer   = fs.Bool("version", false, "print the version and stop")
		noRepo    = fs.Bool("no-repo", false, "do not read the project's git history")
		agentFlag = fs.String("agent", "all", "which agent history to read: claude, codex, or all")
	)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	// Flags are accepted before or after the project name. The standard parser
	// stops at the first argument that is not a flag, which would silently
	// ignore "bough my-project --json" and print the wrong thing.
	name, flags := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		return err
	}

	// Before anything reads the disk, so it answers on a machine with no
	// history on it at all.
	if *showVer {
		fmt.Fprintln(stdout, released())
		return nil
	}

	build := buildable()

	var sources []agent.Source
	var wanted []agent.Known
	switch word := strings.ToLower(*agentFlag); word {
	case "all", "":
		wanted = agent.Agents
	default:
		k, ok := agent.ByFlag(word)
		if !ok {
			return fmt.Errorf("unknown agent %q; supported: %s", *agentFlag, agentWords())
		}
		wanted = []agent.Known{k}
	}
	for _, k := range wanted {
		if open := build[k.Source]; open != nil {
			sources = append(sources, open(*root))
		}
	}

	sourcesMap := make(map[string]agent.Source, len(sources))
	var projects []agent.Project
	for _, s := range sources {
		sourcesMap[s.Name()] = s
		found, err := s.Detect()
		// Some of a history may be unreadable while the rest is fine, so say
		// what went wrong and carry on with what was found. Silence here used
		// to make a project simply disappear.
		if err != nil {
			fmt.Fprintf(stderr, "bough: some %s history could not be read: %v\n", agent.Display(s.Name()), err)
		}
		projects = append(projects, found...)
	}
	if len(projects) == 0 {
		// Named from the registry, so the wording follows whichever agents
		// were actually looked at rather than naming one of them by hand.
		// The directory named is the one actually searched. It used to come
		// from the registry's default location even when --root sent the search
		// somewhere else, so the message pointed at a directory nothing had
		// looked in.
		//
		// Under a custom root every agent is read from the one place, so it is
		// said once rather than repeated after each agent's name.
		var names []string
		for _, k := range wanted {
			names = append(names, k.Display)
		}
		if *root != "" {
			return fmt.Errorf("no history found; looked for %s in %s",
				strings.Join(names, " and "), *root)
		}
		var said []string
		for _, k := range wanted {
			said = append(said, k.Display+" in "+k.Where)
		}
		return fmt.Errorf("no history found; looked for %s", strings.Join(said, " and "))
	}

	if *list {
		return writeList(stdout, sourcesMap, projects)
	}

	target, err := choose(projects, name, env)
	if errors.Is(err, pick.ErrCancelled) {
		// Backing out is a decision, not a failure. It reaches main as a value
		// so everything deferred on the way here still runs.
		return nil
	}
	if err != nil {
		return err
	}

	src := sourcesMap[target.Source]
	if src == nil {
		// Falling back to the first reader meant a project was read by the
		// wrong agent and the answer looked as valid as any other. There is
		// nothing sensible to do here: a project whose source is not among the
		// ones being read cannot be read.
		return fmt.Errorf("%s came from %q, which is not one of the agents being read", target.Name, target.Source)
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
	opt.Tool = released()
	// Reading git happens here, at the edge, rather than inside the graph.
	// Building a graph is arithmetic over sessions; shelling out is not, and a
	// package that does both cannot be tested without a filesystem.
	if !*noRepo {
		opt.Repo = repo.Read(target.Path)
		// Git missing, or a repository that would not answer, used to look
		// exactly like work done outside a repository. It changes what every
		// hash in the output means, so it is worth one line.
		if opt.Repo.Unread != nil {
			fmt.Fprintf(stderr, "bough: %v, so no commit below is confirmed\n", opt.Repo.Unread)
		}
	}
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

	if useBrowser(*asText, *out, stdout) {
		return browse(g, stderr)
	}
	return graph.WriteText(w, g, *verbose)
}

// useBrowser decides between the page and the terminal.
//
// The page is the default because it is what the tool exists to show, but only
// when there is somebody watching. Anything redirected or piped gets text, so
// that reading bough into a file or through less behaves as it always has
// rather than opening a window and hanging on a port.
func useBrowser(textWanted bool, outFile string, stdout io.Writer) bool {
	if textWanted || outFile != "" {
		return false
	}
	f, ok := stdout.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// browse serves the graph and waits for the reader to finish with it.
func browse(g graph.Graph, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := server.Serve(ctx, g, func(url string) {
		fmt.Fprintf(stderr, "bough is showing %s at %s\n", g.Project.Name, url)
		fmt.Fprintf(stderr, "press ctrl-c when you are done\n")
	})
	if err != nil {
		return err
	}
	return nil
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
var valueFlags = map[string]bool{"root": true, "o": true, "agent": true}

// choose decides which project to read.
//
// Naming a project reads that one. Otherwise the list is always offered, even
// when the current directory has history of its own, so that opening bough
// always shows what is there rather than jumping straight into one project.
// The project you are standing in is marked and put first, so the common case
// is still a single keypress.
func choose(projects []agent.Project, arg string, env Env) (agent.Project, error) {
	if arg == "" {
		return offer(projects, env)
	}

	if p, ok := byPath(projects, arg); ok {
		return p, nil
	}

	var matches []agent.Project
	for _, p := range projects {
		name := strings.ToLower(p.Name)
		tagged := fmt.Sprintf("%s [%s]", name, strings.ToLower(p.Source))
		argLower := strings.ToLower(arg)
		if strings.Contains(name, argLower) || strings.Contains(tagged, argLower) {
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
			if m.Source != "" {
				names = append(names, fmt.Sprintf("%s [%s]", m.Name, m.Source))
			} else {
				names = append(names, m.Name)
			}
		}
		return agent.Project{}, fmt.Errorf("%q matches several projects: %s", arg, strings.Join(names, ", "))
	}
}

// offer asks which project to read, with the one you are standing in first.
func offer(projects []agent.Project, env Env) (agent.Project, error) {
	// The mark only appears when there is a question to ask. Naming a project
	// means you know what you want, and a banner would be in the way.
	banner.Write(env.Out, "what did you actually build?")

	projects, here := currentFirst(projects, env.Dir)

	items := make([]pick.Item, len(projects))
	for i, p := range projects {
		label := p.Name
		if i == 0 && here {
			label += "  (here)"
		}
		items[i] = pick.Item{Label: label, Detail: describe(p)}
	}

	i, err := pick.From(env.In, env.Out, "Which project?", items)
	if err != nil {
		// Cancelling comes back as a value rather than as an exit. Calling
		// os.Exit here skipped every deferred close on the way out and made
		// this path impossible to drive from a test.
		return agent.Project{}, err
	}
	return projects[i], nil
}

// currentFirst moves the project matching the working directory to the front,
// and reports whether one was found. The rest keep their order.
//
// The directory is passed in rather than read here. It was an ambient fact
// reached for three levels below run, which is the same reason the writers are
// passed: a caller cannot ask what this does from anywhere else.
func currentFirst(projects []agent.Project, cwd string) ([]agent.Project, bool) {
	if cwd == "" {
		return projects, false
	}
	want := agent.NormalisePath(cwd)

	for i, p := range projects {
		if agent.NormalisePath(p.Path) != want {
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
	detail := size
	if !p.LastWorked.IsZero() {
		if detail != "" {
			detail = fmt.Sprintf("%s, %s", detail, ago(p.LastWorked))
		} else {
			detail = ago(p.LastWorked)
		}
	}
	// Named for every agent, and spelled the way the listing and the page spell
	// it. Naming only the unfamiliar one made the others look like the absence
	// of an agent rather than a choice of one.
	if p.Source != "" {
		badge := "[" + agent.Display(p.Source) + "]"
		if detail != "" {
			detail = badge + " " + detail
		} else {
			detail = badge
		}
	}
	return detail
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
	// The same normaliser the rest of the tool compares paths with. This used
	// filepath.Clean, which only understands the separator the host happens to
	// use, so a transcript written on Windows and read anywhere else compared
	// as a different place.
	want := agent.NormalisePath(path)
	for _, p := range projects {
		if agent.NormalisePath(p.Path) == want {
			return p, true
		}
	}
	return agent.Project{}, false
}

// writeList prints one line per project, in columns wide enough for what is
// actually in them.
//
// The widths used to be fixed at 24 and 40, which held while every project was
// a short name in a short path. A Codex project is named after a directory that
// can run well past both, and one long row then pushed its own path and count
// out of line with every other row. Measuring first costs a pass over a list
// that is already in memory.
//
// The agent gets a column of its own rather than being stuck onto the name. It
// is the same width on every row so the eye can run down it, and it is filled
// in for every agent: naming only the unfamiliar one implies the others are
// somehow the default, which stopped being true when the second one arrived.
func writeList(w io.Writer, sources map[string]agent.Source, projects []agent.Project) error {
	type row struct {
		name    string
		agent   string
		path    string
		prompts int

		// unread means the history could not be read, which is a different
		// thing from a project with no prompts in it. Both used to print as
		// "0 prompts", so a permission problem or a corrupt file read as an
		// empty project and there was nothing to say otherwise.
		unread bool
	}

	rows := make([]row, 0, len(projects))
	var nameW, agentW, pathW int
	for _, p := range projects {
		src := sources[p.Source]
		turns := 0
		unread := false
		if src != nil {
			sessions, err := src.Sessions(p)
			if err != nil {
				unread = true
			}
			for _, s := range sessions {
				turns += len(s.Turns)
			}
		}
		r := row{name: p.Name, path: p.Path, prompts: turns, unread: unread}
		if p.Source != "" {
			r.agent = "[" + agent.Display(p.Source) + "]"
		}
		rows = append(rows, r)
		nameW = wider(nameW, r.name)
		agentW = wider(agentW, r.agent)
		pathW = wider(pathW, r.path)
	}

	// The path column is padded to what the paths need, up to a limit. Paths
	// vary by far more than names do, and padding to the longest let one deep
	// path push the counts on every other row most of a screen to the right to
	// line up with nothing. Capping it means the common case still lines up and
	// an unusually long path overflows its own row rather than everyone else's.
	if pathW > pathLimit {
		pathW = pathLimit
	}

	for _, r := range rows {
		// A history that failed to read says so rather than reporting a
		// number. The count came from a discarded error, so a project whose
		// transcripts could not be opened printed "0 prompts" exactly as an
		// empty one does, and nothing said which of the two it was.
		count := fmt.Sprintf("%d prompts", r.prompts)
		switch {
		case r.unread && r.prompts > 0:
			count = fmt.Sprintf("%d prompts, some could not be read", r.prompts)
		case r.unread:
			count = "could not be read"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n",
			nameW, r.name, agentW, r.agent, pathW, r.path, count)
	}
	return nil
}

const usage = `bough shows the shape of the work in a project's AI coding history.

  bough              choose a project and open it in a browser
  bough my-project   open a project by name
  bough --text       write to the terminal instead
  bough --list       show which projects have history
  bough --json       write the graph as JSON
  bough --version    print the version
  bough --no-repo    leave the project's git history unread
  bough --agent=codex read only a specific agent (claude, codex, all)

Anything piped or redirected is written as text, so bough > notes.txt and
bough | less behave as you would expect.

Options:
`

// pathLimit caps the path column. Chosen so an ordinary project path lines up
// and a deep one overflows its own row instead of everyone else's.
const pathLimit = 44

// wider reports the greater of a width and the width of a string, counting
// runes rather than bytes so a name outside ASCII still lines up.
func wider(at int, s string) int {
	if n := utf8.RuneCountInString(s); n > at {
		return n
	}
	return at
}

// agentWords lists what --agent accepts, for when somebody gets it wrong.
func agentWords() string {
	var all []string
	for _, k := range agent.Agents {
		all = append(all, k.Flag...)
	}
	return strings.Join(append(all, "all"), ", ")
}

// buildable is every source bough can construct, keyed by what it calls
// itself.
//
// Separate from agent.Agents by necessity: the registry sits below the agent
// packages so the core can read it, and only this package may import them to
// make one. A test pins the two together, since half-registering an agent is
// quiet in both directions.
func buildable() map[string]func(root string) agent.Source {
	return map[string]func(root string) agent.Source{
		"claude-code": func(root string) agent.Source { return claude.Source{Root: root} },
		"codex":       func(root string) agent.Source { return codex.Source{Root: root} },
	}
}

// workingDir is where bough was run, or "" if the question cannot be answered.
func workingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
