package pi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// FuzzReadEntries checks that no session file, however malformed, can crash
// the reader. Pi appends as it runs, so a crash or a full disk leaves a file
// cut off anywhere, and the reader has to survive whatever it finds.
func FuzzReadEntries(f *testing.F) {
	for _, name := range []string{"real.jsonl", "sessions/main.jsonl"} {
		if data, err := os.ReadFile("testdata/" + name); err == nil {
			f.Add(string(data))
		}
	}
	f.Add(`{"type":"session","version":3,"id":"s","timestamp":"t","cwd":"/w"}`)
	f.Add(`{"type":"message","id":"a","parentId":null,"message":{"role":"user","content":"hi"}}`)
	f.Add(`{"type":"message","id":"a","parentId":"z","message":{"role":"assistant","content":[{"type":"toolCall","id":"c","name":"edit","arguments":{"path":"~/x","edits":null}}],"usage":{"input":1e30}}}`)
	f.Add(`{"type":"message","message":{"role":"toolResult","toolName":"subagent","details":{"results":[{"messages":[null,{"role":"toolResult","toolName":"subagent","details":{"results":[]}}]}]}}}`)
	f.Add(`{"type":"message","message":{"role":"bashExecution","command":"git commit -m x","exitCode":0}}`)
	f.Add(`{}`)
	f.Add(``)
	f.Add(`not json`)

	f.Fuzz(func(t *testing.T, body string) {
		entries, err := ReadEntries(strings.NewReader(body))
		if err != nil {
			return
		}
		cwd := "/w"
		if len(entries) > 0 {
			cwd = entries[0].Cwd
		}
		turns := ExtractTurns(entries, cwd)
		for _, turn := range turns {
			var sum agent.Tokens
			for _, m := range turn.Models {
				sum.Add(m)
			}
			if sum != turn.Tokens {
				t.Fatalf("models sum to %+v, turn says %+v", sum, turn.Tokens)
			}
			for _, c := range turn.Committed {
				_ = c.SHA
			}
		}
	})
}

// A tool result can hold a whole file, and a sub-agent's result its whole
// conversation. A long line must not end the read early.
func TestReadEntriesHandlesVeryLongLines(t *testing.T) {
	huge, err := json.Marshal(strings.Repeat("x", 4<<20))
	if err != nil {
		t.Fatal(err)
	}
	body := `{"type":"message","id":"a","parentId":null,"timestamp":"t","message":{"role":"user","content":` + string(huge) + `}}` + "\n" +
		`{"type":"message","id":"b","parentId":"a","timestamp":"t","message":{"role":"user","content":"after the big one"}}` + "\n"
	entries, err := ReadEntries(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}
