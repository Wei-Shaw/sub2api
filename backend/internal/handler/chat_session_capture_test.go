package handler

import "testing"

func TestBuildChatSessionMessagesRecordsUserAndAssistantTurn(t *testing.T) {
	t.Parallel()

	endpoint := "/v1/chat/completions"
	body := []byte(`{
		"model": "test-model",
		"messages": [
			{"role": "system", "content": "system text"},
			{"role": "user", "content": "first user turn"},
			{"role": "assistant", "content": "previous assistant turn"},
			{"role": "user", "content": "latest user turn"}
		]
	}`)

	messages, events := buildChatSessionMessages(&endpoint, body, "latest assistant response")
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0", len(events))
	}
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %#v", len(messages), messages)
	}

	if messages[0].Role != "user" || messages[0].Direction != "inbound" || messages[0].ContentText != "latest user turn" {
		t.Fatalf("first message = %#v, want latest inbound user", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Direction != "outbound" || messages[1].ContentText != "latest assistant response" {
		t.Fatalf("second message = %#v, want outbound assistant", messages[1])
	}
}

func TestBuildChatSessionMessagesSkipsEmptyAssistantTurn(t *testing.T) {
	t.Parallel()

	endpoint := "/v1/responses"
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)

	messages, _ := buildChatSessionMessages(&endpoint, body, "  ")
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1: %#v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[0].Direction != "inbound" || messages[0].ContentText != "hello" {
		t.Fatalf("message = %#v, want inbound user", messages[0])
	}
}
