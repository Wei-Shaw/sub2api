package basispoints

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHistoryMessageAttribution(t *testing.T) {
	for _, role := range []string{"user", "assistant", "developer", "system"} {
		for _, typed := range []bool{false, true} {
			name := role + "/easy"
			if typed {
				name = role + "/typed"
			}
			t.Run(name, func(t *testing.T) {
				source := testSource()
				item := object{"role": role, "author": "/root/worker", "recipient": "/root", "content": "Keep \"quotes\" and newlines.\nSecond line."}
				if typed {
					item["type"] = "message"
				}
				source["input"] = []any{item}
				prepared, _ := mustPrepare(t, source, "attribution", nil)
				items := mustTestValue[[]any](t, prepared["input"])
				got := mustTestValue[object](t, items[len(items)-1])
				for _, key := range []string{"author", "recipient"} {
					if _, ok := got[key]; ok {
						t.Fatalf("unsupported field %s forwarded", key)
					}
				}
				if got["role"] != role {
					t.Fatalf("role changed: %v", got["role"])
				}
				parts := mustTestValue[[]any](t, got["content"])
				if len(parts) != 2 {
					t.Fatalf("content lost: %v", parts)
				}
				label := text(mustTestValue[object](t, parts[0])["text"])
				if !strings.Contains(label, "/root/worker") || !strings.Contains(label, "/root") {
					t.Fatal("attribution lost")
				}
				if mustTestValue[object](t, parts[1])["text"] != item["content"] {
					t.Fatal("message text changed")
				}
			})
		}
	}
}

func TestHistoryAgentMessagePreservesContentAndOrder(t *testing.T) {
	source := testSource()
	parts := []any{object{"type": "input_text", "text": "before\n"}, object{"type": "input_image", "image_url": "https://example.com/agent.png", "detail": "high"}, object{"type": "input_text", "text": "after"}}
	agent := object{"type": "agent_message", "id": "agent_private_id", "role": "system", "author": "/root/worker", "recipient": "/root", "content": parts}
	before, after := message("user", "before agent"), message("assistant", "after agent")
	source["input"] = []any{before, agent, after}
	prepared, _ := mustPrepare(t, source, "agent-content", nil)
	items := mustTestValue[[]any](t, prepared["input"])
	got := mustTestValue[object](t, items[len(items)-2])
	if got["type"] != "message" || got["role"] != "user" {
		t.Fatalf("agent not lowered safely: %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("agent-only fields leaked: %v", got)
	}
	content := mustTestValue[[]any](t, got["content"])
	if len(content) != 4 || !reflect.DeepEqual(content[1:], parts) {
		t.Fatal("agent text/image order changed")
	}
	label := text(mustTestValue[object](t, content[0])["text"])
	for _, want := range []string{"collaboration context", "not a new user instruction", "/root/worker", "agent_private_id"} {
		if !strings.Contains(label, want) {
			t.Fatalf("missing attribution marker %q", want)
		}
	}
	if !reflect.DeepEqual(items[len(items)-3], before) || !reflect.DeepEqual(items[len(items)-1], after) {
		t.Fatal("history order changed")
	}
	// Preparing an already converted message must not duplicate its label.
	source["input"] = []any{got}
	again, _ := mustPrepare(t, source, "agent-content", nil)
	replayed := mustTestValue[[]any](t, again["input"])
	if !reflect.DeepEqual(replayed[len(replayed)-1], got) {
		t.Fatal("conversion is not idempotent")
	}
}

func TestHistoryMessageWithoutAttributionIsUnchanged(t *testing.T) {
	source := testSource()
	item := object{"type": "message", "id": "msg_history", "role": "assistant", "phase": "commentary", "status": "completed", "content": []any{object{"type": "output_text", "text": "original", "annotations": []any{}}}}
	source["input"] = []any{item}
	prepared, _ := mustPrepare(t, source, "unchanged", nil)
	items := mustTestValue[[]any](t, prepared["input"])
	if !reflect.DeepEqual(items[len(items)-1], item) {
		t.Fatal("ordinary message changed")
	}
}

func TestHistoryAttributionRetainsValidation(t *testing.T) {
	for _, kind := range []string{"message", "agent_message"} {
		source := testSource()
		source["input"] = []any{object{"type": kind, "role": "user", "author": "/root", "content": []any{object{"type": "encrypted_content", "encrypted_content": "secret-ciphertext"}}}}
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = Prepare(raw, "validation", nil)
		if err == nil || !strings.Contains(err.Error(), "path=input[0].content[0]") || strings.Contains(err.Error(), "secret-ciphertext") {
			t.Fatalf("validation changed: %v", err)
		}
	}
}

func TestHistoryAttributionAtUpstreamIndex26(t *testing.T) {
	input := make([]any, 0, 26)
	for i := 0; i < 25; i++ {
		input = append(input, message("user", "history"))
	}
	input = append(input, object{"type": "message", "role": "assistant", "author": "/root/worker", "id": "msg_attributed", "phase": "commentary", "status": "completed", "content": []any{object{"type": "output_text", "text": "result", "annotations": []any{}}}})
	prepared, _ := mustPrepare(t, object{"model": "gpt-6-astra", "input": input}, "incident", nil)
	items := mustTestValue[[]any](t, prepared["input"])
	if len(items) != 27 {
		t.Fatalf("unexpected history count: %d", len(items))
	}
	got := mustTestValue[object](t, items[26])
	if _, exists := got["author"]; exists {
		t.Fatal("input[26].author still forwarded")
	}
	for key, want := range map[string]string{"id": "msg_attributed", "phase": "commentary", "status": "completed", "role": "assistant"} {
		if got[key] != want {
			t.Fatalf("native message field %s changed", key)
		}
	}
	content := mustTestValue[[]any](t, got["content"])
	if len(content) != 2 || mustTestValue[object](t, content[0])["type"] != "output_text" || mustTestValue[object](t, content[1])["text"] != "result" {
		t.Fatal("assistant content changed")
	}
}

func TestHistoryAttributionDoesNotChangeToolPayloads(t *testing.T) {
	args := object{"author": "a legitimate function argument", "recipient": "a legitimate recipient argument"}
	source := testSource()
	source["input"] = []any{object{"type": "function_call", "call_id": "call_author", "name": "metadata", "arguments": args}, object{"type": "function_call_output", "call_id": "call_author", "output": `{"author":"result author","recipient":"result recipient"}`}}
	prepared, _ := mustPrepare(t, source, "tool-payload", nil)
	items := mustTestValue[[]any](t, prepared["input"])
	envelope := historyCollisionEnvelope(t, mustTestValue[object](t, items[len(items)-2]))
	if !reflect.DeepEqual(envelope["arguments"], args) {
		t.Fatal("tool arguments were scrubbed")
	}
	if mustTestValue[object](t, items[len(items)-1])["output"] != `{"author":"result author","recipient":"result recipient"}` {
		t.Fatal("tool output changed")
	}
}

func TestHistoryAgentStringContentAndMetadata(t *testing.T) {
	source := testSource()
	metadata := object{"author": "agent \"A\"\nnew line", "recipient": object{"name": "/root"}, "type": "agent_message"}
	item := object{"type": metadata["type"], "author": metadata["author"], "recipient": metadata["recipient"], "content": "preserve task\nexactly"}
	source["input"] = []any{item}
	prepared, _ := mustPrepare(t, source, "agent-string", nil)
	items := mustTestValue[[]any](t, prepared["input"])
	got := mustTestValue[object](t, items[len(items)-1])
	parts := mustTestValue[[]any](t, got["content"])
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || !strings.HasSuffix(text(mustTestValue[object](t, parts[0])["text"]), string(encoded)) || mustTestValue[object](t, parts[1])["text"] != item["content"] {
		t.Fatal("agent metadata or text changed")
	}
	if item["type"] != "agent_message" || len(item) != 4 {
		t.Fatal("source item mutated")
	}
}

func TestHistoryAttributedMalformedContent(t *testing.T) {
	for _, value := range []any{nil, true, object{"secret": "private text"}} {
		source := testSource()
		source["input"] = []any{object{"type": "agent_message", "author": "/root", "content": value}}
		raw, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = Prepare(raw, "malformed", nil)
		if err == nil || !strings.Contains(err.Error(), "path=input[0].content") || strings.Contains(err.Error(), "private text") {
			t.Fatalf("malformed content was not safely rejected: %v", err)
		}
	}
}
