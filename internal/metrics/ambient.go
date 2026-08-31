package metrics

import (
	"path/filepath"
	"strings"
)

// Ambient reports whether a file is one the agent keeps rather than one the
// user was working on.
//
// Plan files, memory notes and changelogs get rewritten constantly as a side
// effect of how the agent works, so they float to the top of any measure based
// on how often a file changed. On the history this was built against, the most
// rewritten file in nearly every project was the agent's own plan: 48 rewrites
// in one, 29 in another. Counting those as effort says the user struggled with
// a scratchpad.
//
// The user's own README or changelog does get filtered out along with them.
// That is the right trade: someone reading their history wants to see the work,
// and documentation churn is rarely the part they remember.
func Ambient(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))

	if ambientNames[base] {
		return true
	}
	// Plan files are named after the session that made them and live in the
	// agent's own directory, so the name is unpredictable but the location is not.
	dir := strings.ToLower(filepath.ToSlash(path))
	for _, marker := range ambientDirs {
		if strings.Contains(dir, marker) {
			return true
		}
	}
	return false
}

var ambientNames = map[string]bool{
	"memory.md":         true,
	"changelog.md":      true,
	"claude.md":         true,
	"agents.md":         true,
	"todo.md":           true,
	"notes.md":          true,
	"cargo.lock":        true,
	"package-lock.json": true,
	"pnpm-lock.yaml":    true,
	"yarn.lock":         true,
	"go.sum":            true,
	"poetry.lock":       true,
}

// ambientDirs are locations that only ever hold an agent's own bookkeeping.
var ambientDirs = []string{
	"/.claude/",
	"/.cursor/",
	"/.codex/",
	"/memory/",
}
