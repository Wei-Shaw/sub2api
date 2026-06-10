package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

	messages, events := buildChatSessionMessages(&endpoint, body, nil, "", "latest assistant response", nil, nil)
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

	messages, _ := buildChatSessionMessages(&endpoint, body, nil, "", "  ", nil, nil)
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

	messages, _ := buildChatSessionMessages(&endpoint, body, nil, "", "", outputJSON, nil)
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

func TestEnqueueChatSessionRecordExternalizesLargePayloads(t *testing.T) {
	oldQueue := chatSessionRecordQueue
	oldOnce := chatSessionRecordQueueOnce
	t.Cleanup(func() {
		chatSessionRecordQueue = oldQueue
		chatSessionRecordQueueOnce = oldOnce
	})
	payloadDir := t.TempDir()
	t.Setenv("CHAT_SESSION_RETENTION_PAYLOAD_DIR", payloadDir)

	chatSessionRecordQueue = make(chan chatSessionRecordTask, 1)
	chatSessionRecordQueueOnce = sync.Once{}
	chatSessionRecordQueueOnce.Do(func() {})

	endpoint := "/v1/responses"
	largeText := strings.Repeat("x", chatSessionInlineMaxBytes+1024)
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"` + largeText + `"}]}]}`)

	enqueueChatSessionRecord(
		service.NewChatSessionService(nil),
		&service.ChatSessionRecordInput{
			UserID:          1,
			APIKeyID:        2,
			InboundEndpoint: &endpoint,
			CreatedAt:       time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		},
		body,
		"",
		nil,
	)

	task := <-chatSessionRecordQueue
	if len(task.requestBody) != 0 {
		t.Fatalf("requestBody retained in queue: %d bytes", len(task.requestBody))
	}
	if len(task.requestBodyRef) == 0 {
		t.Fatalf("requestBodyRef is empty")
	}
	var ref chatSessionCapturePayloadRef
	if err := json.Unmarshal(task.requestBodyRef, &ref); err != nil {
		t.Fatalf("invalid requestBodyRef: %v", err)
	}
	if ref.Storage != "file" || ref.Compression != "gzip" || ref.Bytes <= int64(chatSessionInlineMaxBytes) {
		t.Fatalf("unexpected ref: %#v", ref)
	}
	if _, err := os.Stat(filepath.Join(payloadDir, filepath.FromSlash(ref.Path))); err != nil {
		t.Fatalf("payload file missing: %v", err)
	}
	if !strings.Contains(task.requestBodySummary, strings.Repeat("x", 16)) {
		t.Fatalf("summary does not include request text")
	}
}

func TestEnqueueChatSessionRecordDropsLargePayloadWhenExternalizeFails(t *testing.T) {
	oldQueue := chatSessionRecordQueue
	oldOnce := chatSessionRecordQueueOnce
	t.Cleanup(func() {
		chatSessionRecordQueue = oldQueue
		chatSessionRecordQueueOnce = oldOnce
	})
	payloadDir := filepath.Join(t.TempDir(), "payloads")
	if err := os.WriteFile(payloadDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("write payload dir placeholder: %v", err)
	}
	t.Setenv("CHAT_SESSION_RETENTION_PAYLOAD_DIR", payloadDir)

	chatSessionRecordQueue = make(chan chatSessionRecordTask, 1)
	chatSessionRecordQueueOnce = sync.Once{}
	chatSessionRecordQueueOnce.Do(func() {})

	endpoint := "/v1/responses"
	largeText := strings.Repeat("y", chatSessionInlineMaxBytes+1024)
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"` + largeText + `"}]}]}`)

	enqueueChatSessionRecord(
		service.NewChatSessionService(nil),
		&service.ChatSessionRecordInput{
			UserID:          1,
			APIKeyID:        2,
			InboundEndpoint: &endpoint,
			CreatedAt:       time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		},
		body,
		"",
		nil,
	)

	task := <-chatSessionRecordQueue
	if len(task.requestBody) != 0 {
		t.Fatalf("requestBody retained in queue after externalize failure: %d bytes", len(task.requestBody))
	}
	if len(task.requestBodyRef) != 0 {
		t.Fatalf("requestBodyRef = %s, want empty after externalize failure", task.requestBodyRef)
	}
	if !strings.Contains(task.requestBodySummary, strings.Repeat("y", 16)) {
		t.Fatalf("summary does not include request text")
	}

	messages, events := buildChatSessionMessages(
		task.payload.InboundEndpoint,
		task.requestBody,
		task.requestBodyRef,
		task.requestBodySummary,
		task.finalOutputText,
		task.finalOutputJSON,
		task.finalOutputJSONRef,
	)
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0", len(events))
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1: %#v", len(messages), messages)
	}
	if messages[0].Role != "user" || messages[0].Direction != "inbound" || !strings.Contains(messages[0].ContentText, strings.Repeat("y", 16)) {
		t.Fatalf("message = %#v, want inbound summary", messages[0])
	}
	if len(messages[0].ContentJSON) != 0 {
		t.Fatalf("ContentJSON retained after externalize failure: %d bytes", len(messages[0].ContentJSON))
	}
}

func TestRecordChatSessionWithTimeoutIgnoresNilInputs(t *testing.T) {
	recordChatSessionWithTimeout(nil, nil)
}
