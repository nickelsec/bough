package antigravity

import (
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
)

var exitCodeRegex = regexp.MustCompile(`exited with code (\d+)`)
var commitShaRegex = regexp.MustCompile(`\[[\w/-]+\s+([0-9a-f]{7,40})\]`)

func normalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return strings.ToLower(p)
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

// ExtractTurns parses Antigravity records into normalised agent.Turn instances.
func ExtractTurns(recs []*Record) []agent.Turn {
	var turns []agent.Turn
	var cur *agent.Turn

	var pending *pendingCommit

	for _, r := range recs {
		if r.IsHumanPrompt() {
			text := r.PromptText()
			turns = append(turns, agent.Turn{
				At:    r.Time(),
				Text:  text,
				Tools: map[string]int{},
				Files: map[string]int{},
				Edits: map[string]int{},
				Lines: map[string]int{},
			})
			cur = &turns[len(turns)-1]
			pending = nil
			continue
		}

		if cur == nil {
			continue
		}

		// Handle tool invocations
		if len(r.ToolCalls) > 0 {
			for _, tc := range r.ToolCalls {
				cur.Tools[tc.Name]++

				switch tc.Name {
				case "write_to_file":
					target := normalisePath(CleanArg(tc.Args["TargetFile"]))
					if target != "" {
						cur.Files[target]++
						cur.Edits[target]++
						content := CleanArg(tc.Args["CodeContent"])
						lines := strings.Count(content, "\n") + 1
						cur.Lines[target] += lines
					}
				case "replace_file_content":
					target := normalisePath(CleanArg(tc.Args["TargetFile"]))
					if target != "" {
						cur.Files[target]++
						cur.Edits[target]++
						replacement := CleanArg(tc.Args["ReplacementContent"])
						lines := strings.Count(replacement, "\n") + 1
						cur.Lines[target] += lines
					}
				case "view_file":
					path := normalisePath(CleanArg(tc.Args["AbsolutePath"]))
					if path != "" {
						cur.Files[path]++
					}
				case "grep_search":
					path := normalisePath(CleanArg(tc.Args["SearchPath"]))
					if path != "" {
						cur.Files[path]++
					}
				case "list_dir":
					path := normalisePath(CleanArg(tc.Args["DirectoryPath"]))
					if path != "" {
						cur.Files[path]++
					}
				case "find_by_name":
					path := normalisePath(CleanArg(tc.Args["SearchDirectory"]))
					if path != "" {
						cur.Files[path]++
					}
				case "run_command":
					cmd := CleanArg(tc.Args["CommandLine"])
					cwd := normalisePath(CleanArg(tc.Args["Cwd"]))
					if isCommit(cmd) {
						cDir := commitDir(cmd)
						if cDir == "" {
							cDir = cwd
						}
						pending = &pendingCommit{
							turn:  len(turns) - 1,
							amend: amendCall.MatchString(cmd),
							dir:   cDir,
						}
					}
				case "invoke_subagent":
					if raw, ok := tc.Args["Subagents"]; ok {
						cleanRaw := CleanArg(raw)
						var subs []struct {
							Role   string `json:"Role"`
							Prompt string `json:"Prompt"`
						}
						if err := json.Unmarshal([]byte(cleanRaw), &subs); err == nil {
							for _, sub := range subs {
								cur.Delegated = append(cur.Delegated, agent.Delegation{
									Kind:        sub.Role,
									Description: sub.Prompt,
								})
							}
						} else {
							var parsed struct {
								Subagents []struct {
									Role   string `json:"Role"`
									Prompt string `json:"Prompt"`
								} `json:"Subagents"`
							}
							if err := json.Unmarshal([]byte(cleanRaw), &parsed); err == nil {
								for _, sub := range parsed.Subagents {
									cur.Delegated = append(cur.Delegated, agent.Delegation{
										Kind:        sub.Role,
										Description: sub.Prompt,
									})
								}
							}
						}
					}
				}
			}
			continue
		}

		// Handle tool execution results
		if r.Type == "GENERIC" && r.Content != "" {
			isError := false
			if strings.Contains(r.Content, "Encountered error in tool execution") {
				isError = true
			} else if m := exitCodeRegex.FindStringSubmatch(r.Content); len(m) > 1 {
				code, _ := strconv.Atoi(m[1])
				if code != 0 {
					isError = true
				}
			}

			if isError {
				cur.Errors++
			}

			// If a pending git commit succeeded, attribute commit
			if pending != nil {
				if !isError && pending.turn >= 0 && pending.turn < len(turns) {
					c := agent.Commit{
						Kind: "committed",
						At:   r.Time(),
						Dir:  pending.dir,
					}
					if pending.amend {
						c.Kind = "amended"
					}
					if sm := commitShaRegex.FindStringSubmatch(r.Content); len(sm) > 1 {
						c.SHA = sm[1]
					}
					turns[pending.turn].Committed = append(turns[pending.turn].Committed, c)
				}
				pending = nil
			}
		}
	}

	return turns
}
