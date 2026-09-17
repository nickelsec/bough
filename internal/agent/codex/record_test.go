package codex

import (
	"encoding/json"
	"testing"
)

// A blank task name is no name, not the next line of the envelope.
//
// The pattern matched "Task name:" with \s* before the capture, and \s matches
// a newline, so an empty name let (.+) reach down and take the following
// header. A sub-agent that named nothing was labelled "Sender: /root", which
// reads as a task somebody asked for.
func TestBlankTaskNameIsNoName(t *testing.T) {
	for _, body := range []string{
		"Message Type: NEW_TASK\nTask name:\nSender: /root\nPayload:\n",
		"Message Type: NEW_TASK\nTask name:   \nSender: /root\nPayload:\n",
		"Message Type: NEW_TASK\nTask name:\t\nSender: /root\nPayload:\n",
	} {
		r := agentMessage(body)
		if got := r.AgentTaskText(); got != "" {
			t.Errorf("AgentTaskText() = %q for a blank name, want empty", got)
		}
	}
}

// And a name that is there still arrives.
func TestTaskNameIsReadWhenPresent(t *testing.T) {
	r := agentMessage("Message Type: NEW_TASK\nTask name: pixel_art\nSender: /root\n")
	if got := r.AgentTaskText(); got != "pixel_art" {
		t.Errorf("AgentTaskText() = %q, want %q", got, "pixel_art")
	}
}

// agentMessage builds the record an agent_message arrives as.
func agentMessage(text string) *Record {
	payload := map[string]any{
		"type": "message", "role": "assistant",
		"content": []map[string]string{{"type": "text", "text": text}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return &Record{Type: "response_item", Payload: raw}
}
