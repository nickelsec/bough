package codex

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
)

var exitCodeRegex = regexp.MustCompile(`(?:Process exited with code|Command failed with exit code|Exit code:?)\s*(\d+)`)
var commitShaRegex = regexp.MustCompile(`\[[\w/-]+\s+([0-9a-f]{7,40})\]`)
var patchFileRegex = regexp.MustCompile(`(?m)^\*\*\*\s*(?:Add|Update|Delete)\s*File:\s*([^\r\n]+)`)
var execCmdRegex = regexp.MustCompile(`cmd:\s*"((?:\\.|[^"\\])*)"`)
var execWorkdirRegex = regexp.MustCompile(`workdir:\s*"((?:\\.|[^"\\])*)"`)

// leadingCD picks the directory out of a "cd somewhere && git commit" run.
//
// Codex records the command it ran but not always the directory it ran in, so
// where the command changes directory first that is the better answer than the
// session's working directory.
var leadingCD = regexp.MustCompile(`^\s*cd\s+(?:"([^"]*)"|'([^']*)'|([^\s;&|]+))`)

func commitDir(cmd string) string {
	m := leadingCD.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return shell.NormalisePath(g)
		}
	}
	return ""
}

type pendingCommit struct {
	turn  int
	amend bool
	dir   string
}

// ExtractTurns parses Codex rollout records into normalised agent.Turn instances.
// Records are deduplicated by item ID to prevent inflated counts upon session replay.
func ExtractTurns(recs []*Record) []agent.Turn {
	var turns []agent.Turn
	var cur *agent.Turn
	var currentModel string
	var pending *pendingCommit

	seenItems := make(map[string]bool)

	for _, r := range recs {
		if r.Type == "turn_context" {
			var tc TurnContext
			if err := json.Unmarshal(r.Payload, &tc); err == nil && tc.Model != "" {
				currentModel = tc.Model
			}
			continue
		}

		if r.IsHumanPrompt() {
			var item ResponseItem
			_ = json.Unmarshal(r.Payload, &item)
			if item.ID != "" && seenItems[item.ID] {
				continue
			}
			if item.ID != "" {
				seenItems[item.ID] = true
			}

			text := r.PromptText()
			turns = append(turns, agent.Turn{
				At:     r.Time(),
				Text:   text,
				Tools:  map[string]int{},
				Files:  map[string]int{},
				Edits:  map[string]int{},
				Lines:  map[string]int{},
				Models: map[string]int{},
			})
			cur = &turns[len(turns)-1]
			pending = nil
			continue
		}

		if cur == nil {
			continue
		}

		if r.Type == "event_msg" {
			handleTokenCount(cur, r, currentModel)
			continue
		}

		if r.Type == "response_item" {
			var item ResponseItem
			if err := json.Unmarshal(r.Payload, &item); err != nil {
				continue
			}
			if item.ID != "" && seenItems[item.ID] {
				continue
			}
			if item.ID != "" {
				seenItems[item.ID] = true
			}

			switch item.Type {
			case "function_call", "custom_tool_call":
				handleToolCall(cur, &item, len(turns)-1, &pending)
			case "function_call_output", "custom_tool_call_output":
				handleToolOutput(cur, &item, r.Time(), turns, &pending)
			}
		}
	}

	return turns
}

func handleTokenCount(cur *agent.Turn, r *Record, currentModel string) {
	var evt struct {
		Type string `json:"type"`
		Info struct {
			LastTokenUsage struct {
				InputTokens       int `json:"input_tokens"`
				CachedInputTokens int `json:"cached_input_tokens"`
				OutputTokens      int `json:"output_tokens"`
			} `json:"last_token_usage"`
		} `json:"info"`
	}
	if err := json.Unmarshal(r.Payload, &evt); err == nil && evt.Type == "token_count" {
		usage := evt.Info.LastTokenUsage
		cur.Tokens.Input += usage.InputTokens
		cur.Tokens.CacheRead += usage.CachedInputTokens
		cur.Tokens.Output += usage.OutputTokens
		if currentModel != "" && usage.OutputTokens > 0 {
			cur.Models[currentModel] += usage.OutputTokens
		}
	}
}

func handleToolCall(cur *agent.Turn, item *ResponseItem, turnIdx int, pending **pendingCommit) {
	cur.Tools[item.Name]++

	switch item.Name {
	case "apply_patch":
		applyPatch(cur, item.Input)
	case "exec":
		handleExec(item.Input, turnIdx, pending)
	case "exec_command", "shell_command":
		handleCommand(item.Arguments, turnIdx, pending)
	case "spawn_agent":
		handleSpawnAgent(cur, item.Arguments)
	}
}

// applyPatch reads a Codex patch and credits each file with what changed in it.
//
// A patch may carry several files, each opened by its own "*** Add File:"
// header, and the lines that follow belong to whichever header came last.
// Counting the whole patch and putting the total on the first file named it a
// rewrite and left the others looking untouched, which is exactly the wrong
// answer for a score that reads how much a file moved.
//
// Removals count as well as additions. Deleting code is work, and the Claude
// side has always counted both, so a file changed by the same amount should
// read the same whichever agent did it.
func applyPatch(cur *agent.Turn, input string) {
	at := patchFileRegex.FindAllStringSubmatchIndex(input, -1)
	if len(at) == 0 {
		return
	}

	for i, m := range at {
		file := shell.NormalisePath(strings.TrimSpace(input[m[2]:m[3]]))
		if file == "" {
			continue
		}
		cur.Files[file]++
		cur.Edits[file]++

		// This file's hunk runs to the next header, or to the end.
		to := len(input)
		if i+1 < len(at) {
			to = at[i+1][0]
		}
		if n := changed(input[m[1]:to]); n > 0 {
			cur.Lines[file] += n
		}
	}
}

// changed counts the lines a patch hunk adds or removes.
//
// The +++ and --- markers name files rather than change them, and the end of
// the patch is punctuation, so none of those count.
func changed(hunk string) int {
	n := 0
	for _, l := range strings.Split(hunk, "\n") {
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
		case strings.HasPrefix(l, "***"):
		case strings.HasPrefix(l, "+"), strings.HasPrefix(l, "-"):
			n++
		}
	}
	return n
}

func handleExec(input string, turnIdx int, pending **pendingCommit) {
	cmdMatch := execCmdRegex.FindStringSubmatch(input)
	cmd := ""
	if len(cmdMatch) > 1 {
		cmd = strings.ReplaceAll(cmdMatch[1], `\"`, `"`)
	}
	workdirMatch := execWorkdirRegex.FindStringSubmatch(input)
	workdir := ""
	if len(workdirMatch) > 1 {
		workdir = shell.NormalisePath(strings.ReplaceAll(workdirMatch[1], `\"`, `"`))
	}

	if shell.IsCommit(cmd) {
		cDir := commitDir(cmd)
		if cDir == "" {
			cDir = workdir
		}
		*pending = &pendingCommit{
			turn:  turnIdx,
			amend: shell.IsAmend(cmd),
			dir:   cDir,
		}
	}
}

func handleCommand(args json.RawMessage, turnIdx int, pending **pendingCommit) {
	var parsed struct {
		Cmd     string `json:"cmd"`
		Command string `json:"command"`
		Workdir string `json:"workdir"`
		Cwd     string `json:"cwd"`
	}
	if len(args) > 0 {
		var rawStr string
		if err := json.Unmarshal(args, &rawStr); err == nil {
			_ = json.Unmarshal([]byte(rawStr), &parsed)
		} else {
			_ = json.Unmarshal(args, &parsed)
		}
	}
	cmd := parsed.Cmd
	if cmd == "" {
		cmd = parsed.Command
	}
	cwd := shell.NormalisePath(parsed.Workdir)
	if cwd == "" {
		cwd = shell.NormalisePath(parsed.Cwd)
	}

	if shell.IsCommit(cmd) {
		cDir := commitDir(cmd)
		if cDir == "" {
			cDir = cwd
		}
		*pending = &pendingCommit{
			turn:  turnIdx,
			amend: shell.IsAmend(cmd),
			dir:   cDir,
		}
	}
}

func handleSpawnAgent(cur *agent.Turn, args json.RawMessage) {
	var parsed struct {
		AgentType string `json:"agent_type"`
		Message   string `json:"message"`
	}
	var rawStr string
	if err := json.Unmarshal(args, &rawStr); err == nil {
		_ = json.Unmarshal([]byte(rawStr), &parsed)
	} else {
		_ = json.Unmarshal(args, &parsed)
	}
	kind := parsed.AgentType
	if kind == "" {
		kind = "subagent"
	}
	cur.Delegated = append(cur.Delegated, agent.Delegation{
		Kind:        kind,
		Description: parsed.Message,
	})
}

func handleToolOutput(cur *agent.Turn, item *ResponseItem, at time.Time, turns []agent.Turn, pending **pendingCommit) {
	outputStr := ""
	var rawStr string
	if err := json.Unmarshal(item.Output, &rawStr); err == nil {
		outputStr = rawStr
	} else {
		var items []ContentItem
		if err := json.Unmarshal(item.Output, &items); err == nil {
			var sb strings.Builder
			for _, ci := range items {
				sb.WriteString(ci.Text)
			}
			outputStr = sb.String()
		}
	}

	isError := false
	if m := exitCodeRegex.FindStringSubmatch(outputStr); len(m) > 1 {
		code, _ := strconv.Atoi(m[1])
		if code != 0 {
			isError = true
		}
	}

	if isError {
		cur.Errors++
	}

	p := *pending
	if p != nil {
		if !isError && p.turn >= 0 && p.turn < len(turns) {
			c := agent.Commit{
				Kind: "committed",
				At:   at,
				Dir:  p.dir,
			}
			if p.amend {
				c.Kind = "amended"
			}
			if sm := commitShaRegex.FindStringSubmatch(outputStr); len(sm) > 1 {
				c.SHA = sm[1]
			}
			turns[p.turn].Committed = append(turns[p.turn].Committed, c)
		}
		*pending = nil
	}
}
