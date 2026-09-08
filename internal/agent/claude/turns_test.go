package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSyntheticRecognisesHarnessText(t *testing.T) {
	synthetic := []string{
		"<system-reminder>do a thing</system-reminder>",
		"[Request interrupted by user]",
		"[Image: original 2880x166, displayed at 2000x115]",
		"This session is being continued from a previous conversation",
		"Base directory for this skill: /skills/humanizer",
		"",
		"   ",
	}
	for _, s := range synthetic {
		if !isSynthetic(s) {
			t.Errorf("isSynthetic(%q) = false, want true", s)
		}
	}

	real := []string{
		"add a --json flag to the cli",
		"now make it 50px",
		"why is the hero section crowded?",
	}
	for _, s := range real {
		if isSynthetic(s) {
			t.Errorf("isSynthetic(%q) = true, want false", s)
		}
	}
}

// The same file appears with different drive letter casing and separators
// across a session. Grouping by file only works if those collapse together.
func TestNormalisePathCollapsesCasingAndSeparators(t *testing.T) {
	same := []string{
		`D:\proj\src\main.go`,
		`d:\proj\src\main.go`,
		`d:/proj/src/main.go`,
		`d:/proj//src/main.go`,
	}
	want := normalisePath(same[0])
	for _, p := range same[1:] {
		if got := normalisePath(p); got != want {
			t.Errorf("normalisePath(%q) = %q, want %q", p, got, want)
		}
	}
	if normalisePath("") != "" {
		t.Error("empty path should stay empty")
	}
}

// The earlier version of this used path/filepath, which splits on whatever
// separator the host machine happens to use. That passed on Windows and failed
// everywhere else, because a transcript written on Windows is still full of
// backslashes when it is read on Linux. Pinning the exact result catches that,
// where comparing two paths to each other did not.
func TestNormalisePathIsTheSameOnEveryPlatform(t *testing.T) {
	cases := map[string]string{
		`D:\proj\src\main.go`:  "d:/proj/src/main.go",
		`d:/proj//src/main.go`: "d:/proj/src/main.go",
		`/home/x/proj/main.go`: "/home/x/proj/main.go",
		`C:\Users\x\notes.md`:  "c:/users/x/notes.md",
	}
	for in, want := range cases {
		if got := normalisePath(in); got != want {
			t.Errorf("normalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractTurnsAttributesWorkToThePrompt(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"add a flag"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Edit","input":{"file_path":"D:/proj/main.go"}},` +
			`{"type":"tool_use","name":"Read","input":{"file_path":"d:/proj/main.go"}}]}}`,
		`{"uuid":"3","type":"user","message":{"role":"user","content":[{"type":"tool_result","is_error":true}]}}`,
		`{"uuid":"4","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"now write the docs"}]}}`,
		`{"uuid":"5","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Write","input":{"file_path":"/proj/README.md"}}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Text != "add a flag" {
		t.Errorf("first prompt = %q", turns[0].Text)
	}
	if turns[0].Tools["Edit"] != 1 || turns[0].Tools["Read"] != 1 {
		t.Errorf("tools = %v, want one Edit and one Read", turns[0].Tools)
	}
	// Edit and Read touched the same file written two ways, so it counts twice
	// against one path and only the edit is recorded as a change.
	if len(turns[0].Files) != 1 {
		t.Errorf("files = %v, want a single normalised path", turns[0].Files)
	}
	if len(turns[0].Edits) != 1 {
		t.Errorf("edits = %v, want one", turns[0].Edits)
	}
	if turns[0].Errors != 1 {
		t.Errorf("errors = %d, want 1", turns[0].Errors)
	}
	if turns[1].Tools["Write"] != 1 {
		t.Errorf("second turn tools = %v", turns[1].Tools)
	}
}

func TestExtractTurnsSkipsSyntheticPrompts(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"real request"}]}}`,
		`{"uuid":"2","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"uuid":"3","type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{}}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	// Work that followed the interruption belongs to the request that prompted it.
	if turns[0].Tools["Bash"] != 1 {
		t.Errorf("tools = %v, want the Bash call credited to the real prompt", turns[0].Tools)
	}
}

func TestExtractTurnsRecordsCompactionHint(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"first"}]}}`,
		`{"uuid":"2","type":"system","subtype":"compact_boundary"}`,
		`{"uuid":"3","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"second"}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if !turns[0].SegmentHint {
		t.Error("compaction after the first turn was not recorded")
	}
	if turns[1].SegmentHint {
		t.Error("second turn should carry no hint")
	}
}

// The counts here were established by hand against this machine's history
// during design. They are the check that noise filtering did not drift.
func TestRealCorpusTurnCounts(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	// One session on the machine this was fitted against. Every other machine
	// skips, which is the point: the numbers below are a record of what the
	// filtering did on a history that was read by hand, not a claim about
	// anyone else's.
	ref := os.Getenv("BOUGH_REFERENCE_SESSION")
	if ref == "" {
		t.Skip("no reference session set")
	}
	fp := filepath.Join(home, ".claude", "projects", ref)
	f, err := os.Open(fp)
	if err != nil {
		t.Skip("this machine does not have the reference session")
	}
	defer f.Close()

	recs, err := ReadRecords(f)
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	// Reading this session line by line reports 375 prompts. Collapsing the
	// replay and dropping harness text leaves what the user actually typed.
	if len(turns) < 120 || len(turns) > 175 {
		t.Errorf("got %d turns, expected roughly 146; noise filtering has drifted", len(turns))
	}
	t.Logf("reference session: %d records, %d human turns", len(recs), len(turns))
}

// Sub-agent work carries a description written at the time, which is a better
// label than anything that could be inferred from the prompts around it.
func TestExtractTurnsCapturesDelegations(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"research the options"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Task","input":{"subagent_type":"Explore","description":"Research PDF redaction stack"}},` +
			`{"type":"tool_use","name":"Task","input":{"subagent_type":"Plan","description":"Design the architecture"}}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if len(turns[0].Delegated) != 2 {
		t.Fatalf("got %d delegations, want 2", len(turns[0].Delegated))
	}
	if turns[0].Delegated[0].Kind != "Explore" {
		t.Errorf("kind = %q, want Explore", turns[0].Delegated[0].Kind)
	}
	if turns[0].Delegated[0].Description != "Research PDF redaction stack" {
		t.Errorf("description = %q", turns[0].Delegated[0].Description)
	}
}

// A commit belongs to the prompt that was running when it happened, which is
// how a task comes to know whether the work it holds actually landed.
func TestExtractTurnsCreditsCommitsToTheirPrompt(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"fix the parser"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git commit -F -"}}]}}`,
		`{"uuid":"3","type":"user","toolUseResult":{"stdout":"ok","gitOperation":{"commit":` +
			`{"sha":"d0a65cc","kind":"committed","branch":"main"}}},` +
			`"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}}`,
		`{"uuid":"4","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"now the docs"}]}}`,
		`{"uuid":"5","type":"user","toolUseResult":"plain string result",` +
			`"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t9"}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if len(turns[0].Committed) != 1 {
		t.Fatalf("first turn holds %d commits, want 1", len(turns[0].Committed))
	}
	if got := turns[0].Committed[0]; got.SHA != "d0a65cc" || got.Kind != "committed" {
		t.Errorf("got %+v, want sha d0a65cc kind committed", got)
	}
	// The second prompt committed nothing, and a string result must not be
	// mistaken for one.
	if len(turns[1].Committed) != 0 {
		t.Errorf("second turn holds %d commits, want 0", len(turns[1].Committed))
	}
}

// The bug this guards against: Claude Code fills in gitOperation by reading
// what git printed, so "git commit -q" leaves it empty. Trusting that field
// alone found 4 of one project's 31 commits and 25 of another's 47. The command
// is the dependable signal; the hash is a bonus when git was not silenced.
func TestQuietCommitsAreStillCommits(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"ship it"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git add -A && git commit -q -F -"}}]}}`,
		`{"uuid":"3","type":"user","toolUseResult":{"stdout":"","stderr":""},` +
			`"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns[0].Committed) != 1 {
		t.Fatalf("a quiet commit was not counted: %d commits", len(turns[0].Committed))
	}
	if got := turns[0].Committed[0]; got.Kind != "committed" || got.SHA != "" {
		t.Errorf("got %+v, want kind committed and no sha", got)
	}
}

// A commit that git refused is not a commit. Nothing staged is the usual
// reason, and it happens often enough to matter.
func TestRefusedCommitsDoNotCount(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"commit"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git commit -m nope"}}]}}`,
		`{"uuid":"3","type":"user","message":{"role":"user","content":[` +
			`{"type":"tool_result","tool_use_id":"t1","is_error":true}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)
	if len(turns[0].Committed) != 0 {
		t.Errorf("a failed commit was counted: %+v", turns[0].Committed)
	}
}

// Commands that merely mention committing must not be mistaken for one.
func TestCommitDetectionIsNotFooledByLookalikes(t *testing.T) {
	for _, cmd := range []string{
		"git log --oneline",
		"git commit-tree abc",
		`echo "remember to git commit later"`,
		"git status",
		"git push origin main",
		"git -c core.editor=vim log",
	} {
		if commitCall.MatchString(cmd) {
			t.Errorf("%q was read as a commit", cmd)
		}
	}
	for _, cmd := range []string{
		"git commit -q -F -",
		"cd d:/x && git add -A && git commit -m x",
		"git -C /some/dir commit -m x",
		// An identity set inline. The value is quoted and holds a space, which
		// an option pattern built on \S stops at: seven of one project's
		// thirty one commits went missing exactly here.
		`git add -A && git -c user.name="Ada Lovelace" -c user.email="a@b.com" commit -q -F -`,
		`git -c user.name='Ada Lovelace' commit -q`,
		"git --no-pager commit -m x",
		"git commit",
	} {
		if !commitCall.MatchString(cmd) {
			t.Errorf("%q was not read as a commit", cmd)
		}
	}
}

// A command that writes text is not a command that commits, even when the text
// it writes says "git commit". Writing a script or a changelog about committing
// does exactly that, and it accounted for every one of one project's two
// apparent commits above what its git log holds.
func TestTextThatMentionsCommittingIsNotACommit(t *testing.T) {
	writing := []string{
		"cat > note.sh <<'SH'\ngit commit -q -m hello\nSH",
		"cd /d/x; python - <<'PY'\ns = 'git commit -m x'\nPY",
		"cat >> CHANGELOG.md <<'MD'\nRun git commit when done.\nMD",
	}
	for _, cmd := range writing {
		if isCommit(cmd) {
			t.Errorf("text was read as a commit: %.48q", cmd)
		}
	}

	// A real commit routinely takes its message on a heredoc, which opens
	// after the commit rather than before it.
	committing := []string{
		"git commit -q -F - <<'MSG'\nA message\nMSG",
		"cd /d/x && git add -A && git commit -F - <<'EOF'\nAnother\nEOF",
		"git commit -m short",
	}
	for _, cmd := range committing {
		if !isCommit(cmd) {
			t.Errorf("a commit was not read as one: %.48q", cmd)
		}
	}
}

// Shell is not only pipes and semicolons. A commit can sit in the body of a
// conditional or a loop, where a keyword does the separating, and it can be a
// rehearsal that reports what it would do and then does nothing.
func TestCommitDetectionHandlesShellAndRehearsals(t *testing.T) {
	for _, cmd := range []string{
		// --dry-run prints what would happen. Nothing lands, so nothing counts.
		"git commit --dry-run -m x",
		"git add -A && git commit --dry-run",
		"git revert --no-commit HEAD",
		"git merge --no-commit topic",
		"git help commit",
	} {
		if isCommit(cmd) {
			t.Errorf("%q was read as a commit", cmd)
		}
	}
	for _, cmd := range []string{
		"if git diff --cached --quiet; then echo none; else git commit -m x; fi",
		"for f in a b; do git commit -m $f; done",
		"(cd /d/x && git commit -m y)",
		"git commit -am x",
		// The rehearsal belongs to the command before it, not to this one.
		"git commit --dry-run && git commit -m x",
	} {
		if !isCommit(cmd) {
			t.Errorf("%q was not read as a commit", cmd)
		}
	}
}

// One submission written as several user records is one prompt. Invoking a
// skill files its re-invocation notice under the promptId of the request that
// triggered it, and reading that as a second prompt splits one request in two.
func TestExtractTurnsOnePromptPerPromptID(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"humanize the docs"}]}}`,
		`{"uuid":"2","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"(Re-invocation of /humanizer, instructions unchanged.)"}]}}`,
		`{"uuid":"3","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"now ship it"}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Text != "humanize the docs" {
		t.Errorf("first turn = %q, want what the user typed rather than the notice", turns[0].Text)
	}
	if turns[1].Text != "now ship it" {
		t.Errorf("second turn = %q", turns[1].Text)
	}
}

// The same words twice is two prompts when the ids differ. Somebody typing
// "retry" after a failure is asking again, not being echoed by the harness,
// and the corpus this was built against holds exactly that case.
func TestExtractTurnsRepeatedTextWithNewIDCountsTwice(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"retry"}]}}`,
		`{"uuid":"2","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"retry"}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if turns := ExtractTurns(recs); len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
}

// A synthetic record must not claim the id on its way out. If it did, a real
// prompt filed under the same one would be dropped rather than deduplicated,
// turning a fix for double counting into a loss of work.
func TestExtractTurnsSyntheticDoesNotClaimPromptID(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"[Image: original 800x600]"}]}}`,
		`{"uuid":"2","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"what is wrong with this screenshot"}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if turns[0].Text != "what is wrong with this screenshot" {
		t.Errorf("turn = %q, want the real prompt kept", turns[0].Text)
	}
}

// Claude Code only began writing promptId partway through its life, so a
// transcript from before then holds real prompts and none of the field.
// Requiring it discarded those files in full: one reader measured a project of
// 238 sessions drawing as 20 prompts, against 3,282 in the same records read
// without the requirement.
func TestExtractTurnsReadsTranscriptsWithoutPromptID(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "legacy.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	recs, err := ReadRecords(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if r.PromptID != "" {
			t.Fatalf("fixture is meant to hold no promptId, found %q", r.PromptID)
		}
	}

	turns := ExtractTurns(recs)
	if len(turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(turns))
	}

	// Each prompt stands alone. An empty id is no information, so grouping on
	// it would fold the whole session into one turn.
	want := []string{"add a retry flag to the fetcher", "now write a test for it", "ship it"}
	for i, w := range want {
		if turns[i].Text != w {
			t.Errorf("turn %d = %q, want %q", i, turns[i].Text, w)
		}
	}

	// The work still attaches to the prompt that asked for it.
	if turns[0].Edits["/work/legacy/fetch.go"] != 1 {
		t.Errorf("edits = %v, want the edit credited to the first prompt", turns[0].Edits)
	}
	if turns[0].Tokens.Output != 200 {
		t.Errorf("output tokens = %d, want 200", turns[0].Tokens.Output)
	}
}

// The harness marks its own records with isMeta. Those read as ordinary typed
// prompts otherwise, and on an old transcript there is no promptId to tell
// them apart by.
func TestExtractTurnsSkipsMetaRecords(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","message":{"role":"user","content":[{"type":"text","text":"real request"}]}}`,
		`{"uuid":"2","type":"user","isMeta":true,"message":{"role":"user","content":[{"type":"text","text":"Caveat: generated while running local commands."}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if turns[0].Text != "real request" {
		t.Errorf("turn = %q", turns[0].Text)
	}
}
