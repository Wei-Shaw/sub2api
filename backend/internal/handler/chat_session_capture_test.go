package handler

import (
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

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

	messages, events := buildChatSessionMessages(&endpoint, body, "latest assistant response", nil)
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

	messages, _ := buildChatSessionMessages(&endpoint, body, "  ", nil)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1: %#v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[0].Direction != "inbound" || messages[0].ContentText != "hello" {
		t.Fatalf("message = %#v, want inbound user", messages[0])
	}
}

func TestBuildChatSessionMessagesKeepsToolAndImageJSON(t *testing.T) {
	t.Parallel()

	endpoint := "/v1/responses"
	body := []byte(`{
		"input": [
			{"role":"user","content":[{"type":"input_text","text":"make an image"}]},
			{"role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,abc123"},
				{"type":"function_call_output","call_id":"call_1","output":"tool result"}
			]}
		]
	}`)
	outputJSON := []byte(`{
		"type":"response",
		"output":[
			{"type":"function_call","name":"render","arguments":"{\"size\":\"1x1\"}"},
			{"type":"image_generation_call","result":"iVBORw0KGgo="}
		]
	}`)

	messages, _ := buildChatSessionMessages(&endpoint, body, "", outputJSON)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %#v", len(messages), messages)
	}
	inboundRaw := string(messages[0].ContentJSON)
	if !strings.Contains(inboundRaw, "data:image/png;base64,abc123") || !strings.Contains(inboundRaw, "function_call_output") {
		t.Fatalf("inbound content_json did not keep image/tool data: %s", inboundRaw)
	}
	outboundRaw := string(messages[1].ContentJSON)
	if !strings.Contains(outboundRaw, "image_generation_call") || !strings.Contains(outboundRaw, "iVBORw0KGgo=") {
		t.Fatalf("outbound content_json did not keep image data: %s", outboundRaw)
	}
	if got := gjson.GetBytes(messages[1].ContentJSON, "output.0.name").String(); got != "render" {
		t.Fatalf("tool name = %q, want render", got)
	}
}

func TestEnqueueChatSessionRecordDropsWhenQueueFull(t *testing.T) {
	oldQueue := chatSessionRecordQueue
	oldOnce := chatSessionRecordQueueOnce
	t.Cleanup(func() {
		chatSessionRecordQueue = oldQueue
		chatSessionRecordQueueOnce = oldOnce
	})

	chatSessionRecordQueue = make(chan chatSessionRecordTask, 1)
	chatSessionRecordQueue <- chatSessionRecordTask{}
	chatSessionRecordQueueOnce = sync.Once{}
	chatSessionRecordQueueOnce.Do(func() {})

	enqueueChatSessionRecord(
		service.NewChatSessionService(nil),
		&service.ChatSessionRecordInput{
			UserID:   1,
			APIKeyID: 2,
		},
		nil,
		"",
		nil,
	)
}

func TestRecordChatSessionWithTimeoutIgnoresNilInputs(t *testing.T) {
	recordChatSessionWithTimeout(nil, nil)
}
