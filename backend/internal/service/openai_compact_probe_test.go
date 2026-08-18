package service

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeAccountTestMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: AccountTestModeDefault},
		{input: "default", want: AccountTestModeDefault},
		{input: " compact ", want: AccountTestModeCompact},
		{input: "COMPACT", want: AccountTestModeCompact},
		{input: "unknown", want: AccountTestModeDefault},
	}

	for _, tt := range tests {
		if got := normalizeAccountTestMode(tt.input); got != tt.want {
			t.Fatalf("normalizeAccountTestMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_SuccessMarksSupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, []byte(`{"id":"cmp_1"}`), nil, true, now)

	if got := updates[openAINativeCompactionV2SupportedKey]; got != true {
		t.Fatalf("%s = %v, want true", openAINativeCompactionV2SupportedKey, got)
	}
	if got := updates[openAINativeCompactionV2LastStatusKey]; got != http.StatusOK {
		t.Fatalf("%s = %v, want %d", openAINativeCompactionV2LastStatusKey, got, http.StatusOK)
	}
	if got := updates[openAINativeCompactionV2LastErrorKey]; got != "" {
		t.Fatalf("%s = %v, want empty string", openAINativeCompactionV2LastErrorKey, got)
	}
	if got := updates[openAINativeCompactionV2CheckedAtKey]; got != now.Format(time.RFC3339) {
		t.Fatalf("%s = %v, want %s", openAINativeCompactionV2CheckedAtKey, got, now.Format(time.RFC3339))
	}
	if _, exists := updates["openai_compact_supported"]; exists {
		t.Fatal("native v2 probe must not overwrite legacy compact capability")
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_404MarksUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`404 page not found`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusNotFound}, body, nil, false, now)

	if got := updates[openAINativeCompactionV2SupportedKey]; got != false {
		t.Fatalf("%s = %v, want false", openAINativeCompactionV2SupportedKey, got)
	}
	if got := updates[openAINativeCompactionV2LastStatusKey]; got != http.StatusNotFound {
		t.Fatalf("%s = %v, want %d", openAINativeCompactionV2LastStatusKey, got, http.StatusNotFound)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_502DoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadGateway}, []byte(`Upstream request failed`), nil, false, now)

	if _, exists := updates[openAINativeCompactionV2SupportedKey]; exists {
		t.Fatalf("did not expect %s for 502 response", openAINativeCompactionV2SupportedKey)
	}
	if got := updates[openAINativeCompactionV2LastStatusKey]; got != http.StatusBadGateway {
		t.Fatalf("%s = %v, want %d", openAINativeCompactionV2LastStatusKey, got, http.StatusBadGateway)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_RequestErrorDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, errors.New("dial tcp timeout"), false, now)

	if _, exists := updates[openAINativeCompactionV2SupportedKey]; exists {
		t.Fatalf("did not expect %s for request error", openAINativeCompactionV2SupportedKey)
	}
	if got, exists := updates[openAINativeCompactionV2LastStatusKey]; !exists || got != nil {
		t.Fatalf("%s = %v, want nil key", openAINativeCompactionV2LastStatusKey, got)
	}
	if got := updates[openAINativeCompactionV2LastErrorKey]; got == "" {
		t.Fatalf("expected %s to be populated", openAINativeCompactionV2LastErrorKey)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_NoResponseClearsLastStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(nil, nil, nil, false, now)

	if got, exists := updates[openAINativeCompactionV2LastStatusKey]; !exists || got != nil {
		t.Fatalf("%s = %v, want nil key", openAINativeCompactionV2LastStatusKey, got)
	}
	if got := updates[openAINativeCompactionV2LastErrorKey]; got != "compact probe failed" {
		t.Fatalf("%s = %v, want compact probe failed", openAINativeCompactionV2LastErrorKey, got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_UnknownModelDoesNotMarkUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	body := []byte(`{"error":{"message":"unknown model gpt-5.4-openai-compact"}}`)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusBadRequest}, body, nil, false, now)

	if _, exists := updates[openAINativeCompactionV2SupportedKey]; exists {
		t.Fatalf("did not expect %s for unknown-model diagnostics", openAINativeCompactionV2SupportedKey)
	}
	if got := updates[openAINativeCompactionV2LastStatusKey]; got != http.StatusBadRequest {
		t.Fatalf("%s = %v, want %d", openAINativeCompactionV2LastStatusKey, got, http.StatusBadRequest)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_EmptyFailureBodyFallsBackToHTTPStatus(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusServiceUnavailable}, nil, nil, false, now)

	if got := updates[openAINativeCompactionV2LastStatusKey]; got != http.StatusServiceUnavailable {
		t.Fatalf("%s = %v, want %d", openAINativeCompactionV2LastStatusKey, got, http.StatusServiceUnavailable)
	}
	if got := updates[openAINativeCompactionV2LastErrorKey]; got != "HTTP 503" {
		t.Fatalf("%s = %v, want HTTP 503", openAINativeCompactionV2LastErrorKey, got)
	}
}

func TestBuildOpenAICompactProbeExtraUpdates_2xxWithoutCompactionItemMarksUnsupported(t *testing.T) {
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	updates := buildOpenAICompactProbeExtraUpdates(&http.Response{StatusCode: http.StatusOK}, []byte(`{"id":"resp_1","output":[]}`), nil, false, now)

	if got := updates[openAINativeCompactionV2SupportedKey]; got != false {
		t.Fatalf("%s = %v, want false（2xx 无 compaction item = v2 不可用）", openAINativeCompactionV2SupportedKey, got)
	}
	if got := updates[openAINativeCompactionV2LastErrorKey]; got == "" {
		t.Fatalf("expected %s to explain the missing compaction item", openAINativeCompactionV2LastErrorKey)
	}
}

func TestCreateOpenAICompactProbePayload_NativeV2Shape(t *testing.T) {
	payload := createOpenAICompactProbePayload("gpt-5.6-sol", true)
	if payload["stream"] != true {
		t.Fatalf("v2 probe payload must be streaming")
	}
	if payload["store"] != false {
		t.Fatalf("OAuth probe payload must carry store:false")
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected 2 input items, got %v", payload["input"])
	}
	last, ok := input[len(input)-1].(map[string]any)
	if !ok || last["type"] != "compaction_trigger" {
		t.Fatalf("last input item must be compaction_trigger, got %v", input[len(input)-1])
	}

	apiKeyPayload := createOpenAICompactProbePayload("gpt-5.6-sol", false)
	if _, has := apiKeyPayload["store"]; has {
		t.Fatalf("API-key probe payload must not force store")
	}
}

func TestOpenAICompactProbeFoundCompactionItem(t *testing.T) {
	sseWithItem := []byte("event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction\",\"id\":\"cmp_1\",\"encrypted_content\":\"blob\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[]}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseWithItem) {
		t.Fatalf("SSE output_item.done 携带 compaction item 应判定为支持")
	}

	sseAlias := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"compaction_summary\",\"id\":\"cmp_2\"}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseAlias) {
		t.Fatalf("compaction_summary 别名应判定为支持")
	}

	sseNoItem := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"output\":[]}}\n\n")
	if openAICompactProbeFoundCompactionItem(sseNoItem) {
		t.Fatalf("无 compaction item 的流不应判定为支持")
	}

	jsonWithItem := []byte(`{"id":"resp_3","output":[{"type":"compaction","id":"cmp_3"}]}`)
	if !openAICompactProbeFoundCompactionItem(jsonWithItem) {
		t.Fatalf("JSON output[] 携带 compaction item 应判定为支持（降级链兜底）")
	}

	if openAICompactProbeFoundCompactionItem(nil) {
		t.Fatalf("空响应不应判定为支持")
	}
}

func TestOpenAICompactProbeFoundCompactionItem_TerminalResponseOutput(t *testing.T) {
	// 部分上游只在终态 response.completed 的 output[] 给出 compaction item，
	// 事件流里没有 output_item.done——探针同样必须判定为支持。
	sseTerminalOnly := []byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"id\":\"msg_1\"}}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_t\",\"output\":[{\"type\":\"compaction\",\"id\":\"cmp_t\"}]}}\n\n")
	if !openAICompactProbeFoundCompactionItem(sseTerminalOnly) {
		t.Fatalf("终态 response.output 中的 compaction item 应判定为支持")
	}

	sseTerminalEmpty := []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_e\",\"output\":[]}}\n\n")
	if openAICompactProbeFoundCompactionItem(sseTerminalEmpty) {
		t.Fatalf("终态 output 为空且无 item 事件时不应判定为支持")
	}
}
