package basispoints

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStripEncryptedContentReplacesMessageAndToolParts(t *testing.T) {
	const raw = `{"model":"gpt-6-astra","client_metadata":{"ticket":9007199254740993},"input":[
		{"type":"agent_message","author":"/root/worker","recipient":"/root","content":[{"type":"input_text","text":"Message Type: MESSAGE\nPayload:\n"},{"type":"encrypted_content","encrypted_content":"AGENT_CIPHERTEXT"}]},
		{"type":"agent_message","author":"/root/worker","recipient":"/root","content":[{"type":"input_text","text":"Message Type: FINAL_ANSWER\nPayload:\nplaintext result"}]},
		{"role":"user","content":[{"type":"encrypted_content","encrypted_content":"USER_CIPHERTEXT"}]},
		{"type":"message","role":"assistant","content":[{"type":"encrypted_content","encrypted_content":"ASSISTANT_CIPHERTEXT"}]},
		{"type":"reasoning","summary":[],"encrypted_content":"REASONING_CIPHERTEXT"},
		{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"type\":\"encrypted_content\",\"encrypted_content\":\"ARGUMENT_TEXT\"}"},
		{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"tool text"},{"type":"encrypted_content","encrypted_content":"TOOL_CIPHERTEXT"}]},
		{"type":"custom_tool_call_output","call_id":"call_2","output":[{"type":"encrypted_content","encrypted_content":"CUSTOM_CIPHERTEXT"}]},
		{"role":"user","content":"encrypted_content stays literal text"}
	]}`
	out, err := StripEncryptedContent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	wire := string(out)
	for _, secret := range []string{"AGENT_CIPHERTEXT", "USER_CIPHERTEXT", "ASSISTANT_CIPHERTEXT", "TOOL_CIPHERTEXT", "CUSTOM_CIPHERTEXT"} {
		if strings.Contains(wire, secret) {
			t.Fatalf("encrypted part %s was forwarded", secret)
		}
	}
	// Reasoning, tool arguments, plain text and large integers are not touched.
	for _, kept := range []string{"REASONING_CIPHERTEXT", "ARGUMENT_TEXT", "encrypted_content stays literal text", "plaintext result", "9007199254740993"} {
		if !strings.Contains(wire, kept) {
			t.Fatalf("expected %s to be preserved: %s", kept, wire)
		}
	}
	var result object
	if err := decode(out, &result); err != nil {
		t.Fatal(err)
	}
	input, _ := result["input"].([]any)
	part := func(item, index int) object {
		t.Helper()
		field := "content"
		if item >= 6 {
			field = "output"
		}
		entry, _ := input[item].(object)
		parts, _ := entry[field].([]any)
		if index >= len(parts) {
			t.Fatalf("input[%d].%s has no part %d", item, field, index)
		}
		value, _ := parts[index].(object)
		return value
	}
	for _, check := range []struct {
		item, index int
		kind        string
	}{{0, 1, "input_text"}, {2, 0, "input_text"}, {3, 0, "output_text"}, {6, 1, "input_text"}, {7, 0, "input_text"}} {
		got := part(check.item, check.index)
		if text(got["type"]) != check.kind || text(got["text"]) != encryptedContentOmitted {
			t.Fatalf("input[%d] part %d: got %v", check.item, check.index, got)
		}
	}
	if header := text(part(0, 0)["text"]); header != "Message Type: MESSAGE\nPayload:\n" {
		t.Fatalf("the sub-agent header must stay in front of the marker: %q", header)
	}
	if text(part(6, 0)["text"]) != "tool text" {
		t.Fatal("adjacent tool text must be kept")
	}
}

func TestStripEncryptedContentLetsOldCollaborationHistoryPrepare(t *testing.T) {
	source := testSource()
	source["input"] = []any{
		object{"type": "agent_message", "author": "/root/worker", "recipient": "/root",
			"content": []any{object{"type": "input_text", "text": "Payload:\n"}, object{"type": "encrypted_content", "encrypted_content": "gAAAAABopaque"}}},
		object{"role": "user", "content": "continue"},
	}
	raw, _ := json.Marshal(source)
	if _, _, err := Prepare(raw, "scope", nil); err == nil {
		t.Fatal("without the account option encrypted content must still be rejected")
	}
	stripped, err := StripEncryptedContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	body, _, err := Prepare(stripped, "scope", nil)
	if err != nil {
		t.Fatalf("stripped history must pass bridge validation: %v", err)
	}
	if strings.Contains(string(body), "gAAAAABopaque") || !strings.Contains(string(body), encryptedContentOmitted) {
		t.Fatalf("unexpected upstream body: %s", body)
	}
}

func TestStripEncryptedContentLeavesOtherRequestsUnchanged(t *testing.T) {
	raw := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]},{"type":"reasoning","encrypted_content":"x"}]}`)
	out, err := StripEncryptedContent(raw)
	if err != nil || !bytes.Equal(out, raw) {
		t.Fatalf("requests without encrypted parts must pass through byte-for-byte: %s (%v)", out, err)
	}
	if _, err := StripEncryptedContent([]byte(`{"input":`)); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}
