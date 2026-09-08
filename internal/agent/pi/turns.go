package pi

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
)

// syntheticPrefixes matches prompts that are terminal controls or harness artifacts
// rather than human development instructions.
var syntheticPrefixes = []string{
	"/exit",
	"/quit",
	"/clear",
	"/bye",
}

// isSynthetic reports whether prompt text is a control or harness command.
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

// normalisePath puts a file path into a comparable cross-platform form.
func normalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return strings.ToLower(p)
}

// toolInput represents the arguments for Pi tools (read, bash, write, edit).
type toolInput struct {
	Path     string `json:"path"`
	FilePath string `json:"file_path"`
	Command  string `json:"command"`
	Content  string `json:"content"`
	NewText  string `json:"new_text"`
}

func (t toolInput) path() string {
	if t.Path != "" {
		return t.Path
	}
	return t.FilePath
}

var optionRun = `-\S*(?:"[^"]*"|'[^']*'|\S)*\s+` +
	`(?:(?:"[^"]*"|'[^']*'|[^-\s])(?:"[^"]*"|'[^']*'|\S)*\s+)?`

var commitCall = regexp.MustCompile(`(?:^|[|;&(]|&&|\|\||\b(?:then|else|do)\b)\s*(?:cd\s+\S+\s*&&\s*)*` +
	`git\s+(?:` + optionRun + `)*commit(?:\s|$)`)

var heredoc = regexp.MustCompile(`<<-?\s*['"]?\w`)
var dryRun = regexp.MustCompile(`(?:^|\s)--dry-run\b`)
var amendCall = regexp.MustCompile(`\bgit\s[^|;&]*\s--amend\b`)
var separator = regexp.MustCompile(`[|;&]`)
var leadingCD = regexp.MustCompile(`^\s*cd\s+(?:"([^"]*)"|'([^']*)'|([^\s;&|]+))`)

func isCommit(cmd string) bool {
	h := heredoc.FindStringIndex(cmd)
	for _, at := range commitCall.FindAllStringIndex(cmd, -1) {
		if h != nil && h[0] < at[0] {
			return false
		}
		if dryRun.MatchString(firstCommand(cmd[at[1]:])) {
			continue
		}
		return true
	}
	return false
}

func firstCommand(s string) string {
	if at := separator.FindStringIndex(s); at != nil {
		return s[:at[0]]
	}
	return s
}

func commitDir(cmd string) string {
	m := leadingCD.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return normalisePath(g)
		}
	}
	return ""
}

type pendingCommit struct {
	turn  int
	amend bool
	dir   string
}

type pendingEdit struct {
	turn  int
	path  string
	lines int
}

// ExtractTurns parses Pi records into normalised agent.Turn instances.
func ExtractTurns(recs []*Record) []agent.Turn {
	var turns []agent.Turn
	var cur *agent.Turn

	pendingCommits := map[string]pendingCommit{}
	pendingEdits := map[string]pendingEdit{}

	for _, r := range recs {
		if r.IsHumanPrompt() {
			text := r.PromptText()
			if isSynthetic(text) {
				continue
			}

			turns = append(turns, agent.Turn{
				At:    r.Time(),
				Text:  text,
				Tools: map[string]int{},
				Files: map[string]int{},
				Edits: map[string]int{},
				Lines: map[string]int{},
			})
			cur = &turns[len(turns)-1]
			continue
		}

		if cur == nil || r.Message == nil {
			continue
		}

		msg := r.Message

		// Token usage and model tracking
		if msg.Role == "assistant" {
			if msg.Usage != nil {
				cur.Tokens.Add(agent.Tokens{
					Input:      msg.Usage.Input,
					Output:     msg.Usage.Output,
					CacheRead:  msg.Usage.CacheRead,
					CacheWrite: msg.Usage.CacheWrite,
				})
			}
			if msg.Model != "" && msg.Usage != nil && msg.Usage.Output > 0 {
				if cur.Models == nil {
					cur.Models = map[string]int{}
				}
				cur.Models[msg.Model] += msg.Usage.Output
			}

			// Parse tool calls
			for _, b := range msg.Content.Blocks {
				if b.Type != "toolCall" {
					continue
				}
				cur.Tools[b.Name]++

				var in toolInput
				if len(b.Arguments) > 0 {
					_ = json.Unmarshal(b.Arguments, &in)
				}

				filePath := normalisePath(in.path())
				if filePath != "" {
					cur.Files[filePath]++
				}

				switch b.Name {
				case "write", "edit":
					if filePath != "" {
						cur.Edits[filePath]++
						lines := 0
						if in.Content != "" {
							lines = strings.Count(in.Content, "\n") + 1
						} else if in.NewText != "" {
							lines = strings.Count(in.NewText, "\n") + 1
						}
						if b.ID != "" {
							pendingEdits[b.ID] = pendingEdit{
								turn:  len(turns) - 1,
								path:  filePath,
								lines: lines,
							}
						}
					}
				case "bash":
					if in.Command != "" && b.ID != "" && isCommit(in.Command) {
						pendingCommits[b.ID] = pendingCommit{
							turn:  len(turns) - 1,
							amend: amendCall.MatchString(in.Command),
							dir:   commitDir(in.Command),
						}
					}
				}
			}
			continue
		}

		// ToolResult processing
		if msg.Role == "toolResult" {
			if msg.IsError {
				cur.Errors++
			}

			// Handle edits confirmation
			if edit, ok := pendingEdits[msg.ToolCallID]; ok {
				delete(pendingEdits, msg.ToolCallID)
				if !msg.IsError && edit.turn >= 0 && edit.turn < len(turns) && edit.lines > 0 {
					turns[edit.turn].Lines[edit.path] += edit.lines
				}
			}

			// Handle commit confirmation
			if comm, ok := pendingCommits[msg.ToolCallID]; ok {
				delete(pendingCommits, msg.ToolCallID)
				if !msg.IsError && comm.turn >= 0 && comm.turn < len(turns) {
					c := agent.Commit{
						Kind: "committed",
						At:   r.Time(),
						Dir:  comm.dir,
					}
					if comm.amend {
						c.Kind = "amended"
					}
					turns[comm.turn].Committed = append(turns[comm.turn].Committed, c)
				}
			}
		}
	}

	return turns
}
