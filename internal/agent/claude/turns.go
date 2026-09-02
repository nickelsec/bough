package claude

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
)

// synthetic matches prompts the harness injected rather than the user typing.
//
// These arrive as ordinary user records with real text in them, so they have to
// be recognised by their opening. Counting them as prompts inflates the record
// of what a person actually asked for.
var syntheticPrefixes = []string{
	"<",                               // reminders and wrappers
	"[Request interrupted",            // the user stopping a turn
	"[Image:",                         // pasted screenshots
	"This session is being continued", // resume preamble
	"Caveat: The messages below",      // harness preamble
	"Base directory for this skill",   // skill loading
	"<local-command-stdout>",          // slash command output
	"API Error",                       // transport failures
}

// isSynthetic reports whether prompt text came from the harness.
func isSynthetic(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	for _, p := range syntheticPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// normalisePath puts a file path into a comparable form.
//
// The same file turns up written several ways across a session, because the
// drive letter changes case between records and separators differ by platform.
// Grouping by path only works once those are settled.
//
// This deliberately does not use path/filepath. A transcript written on
// Windows can be read on any machine, so backslashes have to be understood
// everywhere rather than only where the host happens to use them.
func normalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return strings.ToLower(p)
}

// toolInput is the subset of tool arguments worth reading. Tools name their
// file argument differently, so several spellings are accepted.
type toolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`

	// Sub-agent calls describe the work they were given, which is a label
	// written at the time rather than one inferred afterwards.
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`
}

// path returns whichever file argument the tool supplied.
func (t toolInput) path() string {
	switch {
	case t.FilePath != "":
		return t.FilePath
	case t.NotebookPath != "":
		return t.NotebookPath
	default:
		return t.Path
	}
}

// editingTools are the tools that change a file rather than just reading it.
// Repeated edits to one file are the clearest sign of a struggle, so they are
// tracked apart from reads.
// delegatingTools hand work to a sub-agent.
var delegatingTools = map[string]bool{
	"Task":  true,
	"Agent": true,
}

var editingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"NotebookEdit": true,
}

// ExtractTurns walks a session's records and returns one Turn per human prompt,
// with everything the agent did in response attributed to it.
//
// Records arrive in order, so the work belonging to a prompt is simply
// everything between it and the next one. Synthetic prompts are skipped, and
// any activity they would have owned is credited to the prompt before them,
// which is where it belongs since the harness was acting on that request.
func ExtractTurns(recs []*Record) []agent.Turn {
	var turns []agent.Turn
	var cur *agent.Turn

	for _, r := range recs {
		if r.IsCompactBoundary() {
			if cur != nil {
				cur.SegmentHint = true
			}
			continue
		}

		if r.IsHumanPrompt() {
			text := r.PromptText()
			if isSynthetic(text) {
				continue
			}
			turns = append(turns, agent.Turn{
				At:    r.Time(),
				Text:  strings.TrimSpace(text),
				Tools: map[string]int{},
				Files: map[string]int{},
				Edits: map[string]int{},
			})
			cur = &turns[len(turns)-1]
			continue
		}

		if cur == nil || r.Message == nil {
			continue
		}
		for _, b := range r.Message.Content.Blocks {
			switch b.Type {
			case "tool_use":
				cur.Tools[b.Name]++
				if len(b.Input) == 0 {
					continue
				}
				var in toolInput
				if err := json.Unmarshal(b.Input, &in); err != nil {
					continue
				}
				if delegatingTools[b.Name] && in.Description != "" {
					cur.Delegated = append(cur.Delegated, agent.Delegation{
						Kind:        in.SubagentType,
						Description: in.Description,
					})
				}
				p := normalisePath(in.path())
				if p == "" {
					continue
				}
				cur.Files[p]++
				if editingTools[b.Name] {
					cur.Edits[p]++
				}
			case "tool_result":
				if b.IsError {
					cur.Errors++
				}
			}
		}
	}
	return turns
}
