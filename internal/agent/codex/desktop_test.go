package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The desktop app writes a shape the original parser was not built for, and
// each test here pins one thing it got wrong on a real session. The figures
// come from a rollout pair on disk rather than being invented.

// A record can hold several injected blocks and nothing else. Every session
// opens with one, so counting it added a prompt to every session in the graph.
func TestInjectionOnlyRecordIsNotAPrompt(t *testing.T) {
	// The environment block arrives first here on purpose. A filter that only
	// looks at how the joined text begins happens to reject that one, so it
	// looks correct. The desktop app sends the plugin catalogue first, and then
	// the same filter sees a tag it does not know and calls the record a
	// prompt, which is what put an extra prompt in every session.
	both := []string{
		"<recommended_plugins>\n- Airtable\n</recommended_plugins>",
		"<environment_context>\n  <cwd>C:\\work</cwd>\n</environment_context>",
	}

	if rec := userMessage("m1", both[1], both[0]); rec.IsHumanPrompt() {
		t.Error("harness injections were counted as a prompt")
	}
	if rec := userMessage("m1", both[0], both[1]); rec.IsHumanPrompt() {
		t.Error("harness injections were counted as a prompt, in the order the app sends them")
	}
}

// The blocks are stripped rather than the record rejected outright, so a prompt
// that happens to arrive alongside one still counts.
func TestPromptSurvivesAnInjectionInTheSameRecord(t *testing.T) {
	rec := userMessage("m2",
		"<recommended_plugins>\n- Airtable\n</recommended_plugins>",
		"make me a pixel themed site")

	if !rec.IsHumanPrompt() {
		t.Fatal("a real prompt was dropped because an injection shared its record")
	}
	if got := rec.PromptText(); got != "make me a pixel themed site" {
		t.Errorf("prompt = %q, want the typed text alone", got)
	}
}

// Both usage streams appear in one session and report the same figures, so
// adding both doubles every total in the graph.
func TestUsageIsNotCountedTwice(t *testing.T) {
	recs := []*Record{
		userMessage("m1", "do the thing"),
		raw("token_usage_record", `{"response_id":"r1","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":7}}`),
		raw("event_msg", `{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":7}}}`),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if got := turns[0].Tokens.Input; got != 100 {
		t.Errorf("input = %d, want 100: the two streams report the same response", got)
	}
	if got := turns[0].Tokens.Output; got != 7 {
		t.Errorf("output = %d, want 7", got)
	}
}

// Rollouts predating the usage record still have to be read.
func TestLegacyUsageStillCounts(t *testing.T) {
	recs := []*Record{
		userMessage("m1", "do the thing"),
		raw("event_msg", `{"type":"token_count","info":{"last_token_usage":{"input_tokens":55,"cached_input_tokens":5,"output_tokens":3}}}`),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if got := turns[0].Tokens.Input; got != 55 {
		t.Errorf("input = %d, want 55: a session with no usage records must fall back", got)
	}
}

// A sub-agent's rollout carries no user message at all. Its work arrives as a
// delegated task, and without that its session came back empty and was dropped.
func TestDelegatedTaskOpensATurn(t *testing.T) {
	recs := []*Record{
		raw("response_item", `{"type":"agent_message","id":"a1","content":[{"type":"input_text","text":"Message Type: NEW_TASK\nTask name: /root/pixel_art\nSender: /root\nPayload:\n"}]}`),
		raw("token_usage_record", `{"response_id":"r1","usage":{"input_tokens":12,"cached_input_tokens":0,"output_tokens":4}}`),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1: a sub-agent's work went missing", len(turns))
	}
	if got := turns[0].Text; got != "/root/pixel_art" {
		t.Errorf("label = %q, want the task name", got)
	}
}

// Agents message each other both ways. A reply is not new work, and counting
// one opened a turn on the parent that nobody asked for.
func TestReplyFromSubAgentIsNotATurn(t *testing.T) {
	recs := []*Record{
		userMessage("m1", "build it"),
		raw("response_item", `{"type":"agent_message","id":"a2","content":[{"type":"input_text","text":"Message Type: TASK_COMPLETE\nTask name: /root/pixel_art\nSender: /root/pixel_art\n"}]}`),
	}

	if got := len(ExtractTurns(recs)); got != 1 {
		t.Errorf("got %d turns, want 1: a sub-agent reporting back is not a request", got)
	}
}

// The brief one agent hands another is stored encrypted. It is unreadable, and
// printing it put a wall of base64 where the reader expects a prompt.
func TestEncryptedBriefIsNotShown(t *testing.T) {
	cipher := "gAAAAABqoVGCbKXgAMRcpFtdMQ_z-aiA-mueExipbREEEXsLqrlxT7LvTrKNNvcx6bWRm1eCocaxCUE7CrzP9Sjivyi7D6glGbJF"

	if !Encrypted(cipher) {
		t.Fatal("a Fernet token was not recognised as ciphertext")
	}
	if Encrypted("make me a website") {
		t.Error("ordinary prose was mistaken for ciphertext")
	}

	spawn := `{"type":"function_call","id":"f1","name":"spawn_agent","arguments":"{\"task_name\":\"pixel_art\",\"message\":\"` + cipher + `\"}"}`
	recs := []*Record{
		userMessage("m1", "build it"),
		raw("response_item", spawn),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 || len(turns[0].Delegated) != 1 {
		t.Fatal("the delegation was not recorded")
	}
	d := turns[0].Delegated[0]
	if d.Description != "" {
		t.Errorf("description = %q, want empty: ciphertext must not be shown", d.Description)
	}
	if d.Kind != "pixel_art" {
		t.Errorf("kind = %q, want the task name", d.Kind)
	}
}

// A sub-agent's rollout carries the parent's session_id. Grouping on that alone
// folded the two together and hid the sub-agent's prompts and tokens inside the
// agent that spawned it.
func TestSubAgentIsItsOwnSession(t *testing.T) {
	dir := t.TempDir()
	day := filepath.Join(dir, "2026", "09", "09")
	if err := os.MkdirAll(day, 0o750); err != nil {
		t.Fatal(err)
	}

	parent := strings.Join([]string{
		`{"timestamp":"2026-09-09T12:30:09Z","type":"session_meta","payload":{"session_id":"S","id":"S","cwd":"C:\\work\\site"}}`,
		`{"timestamp":"2026-09-09T12:30:11Z","type":"response_item","payload":{"type":"message","role":"user","id":"m1","content":[{"type":"input_text","text":"build me a site"}]}}`,
	}, "\n")

	child := strings.Join([]string{
		`{"timestamp":"2026-09-09T12:30:59Z","type":"session_meta","payload":{"session_id":"S","id":"C","parent_thread_id":"S","cwd":"C:\\work\\site"}}`,
		`{"timestamp":"2026-09-09T12:31:03Z","type":"response_item","payload":{"type":"agent_message","id":"a1","content":[{"type":"input_text","text":"Message Type: NEW_TASK\nTask name: /root/pixel_art\nSender: /root\n"}]}}`,
	}, "\n")

	writeRollout(t, filepath.Join(day, "rollout-parent.jsonl"), parent)
	writeRollout(t, filepath.Join(day, "rollout-child.jsonl"), child)

	s := Source{Root: dir}
	projects, err := s.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	sessions, err := s.Sessions(projects[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2: a spawned agent is its own work", len(sessions))
	}

	var total int
	for _, sess := range sessions {
		total += len(sess.Turns)
	}
	if total != 2 {
		t.Errorf("got %d turns across the sessions, want 2", total)
	}
}

func writeRollout(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func raw(kind, payload string) *Record {
	return &Record{
		Timestamp: "2026-09-09T12:30:11Z",
		Type:      kind,
		Payload:   []byte(payload),
	}
}

func userMessage(id string, chunks ...string) *Record {
	var b strings.Builder
	b.WriteString(`{"type":"message","role":"user","id":"` + id + `","content":[`)
	for i, c := range chunks {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"type":"input_text","text":`)
		b.WriteString(quoteJSON(c))
		b.WriteString("}")
	}
	b.WriteString("]}")
	return raw("response_item", b.String())
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
