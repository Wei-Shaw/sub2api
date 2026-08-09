package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAgnesVideoResponsePreservesRawTaskID(t *testing.T) {
	input := []byte(`{"id":"task_abc123","status":"queued"}`)
	got := normalizeAgnesVideoResponse(input, "agnes-video-v2.0", "", true)

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("normalize response is not JSON: %v", err)
	}
	if payload["id"] != "task_abc123" {
		t.Fatalf("id = %v, want raw task ID", payload["id"])
	}
	if payload["video_id"] != "task_abc123" {
		t.Fatalf("video_id = %v, want raw task ID", payload["video_id"])
	}
	if payload["id"] == "video_task_abc123" {
		t.Fatal("normalization must not add a synthetic video_ prefix")
	}
}

func TestNormalizeAgnesVideoResponseAddsStatusFieldsForKnownTask(t *testing.T) {
	input := []byte(`{"result":{"video_url":"https://example.test/video.mp4"}}`)
	got := normalizeAgnesVideoResponse(input, "agnes-video-v2.0", "task_done", false)

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("normalize response is not JSON: %v", err)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result was not normalized as an object: %#v", payload["result"])
	}
	if result["video_id"] != "task_done" || result["status"] != "completed" {
		t.Fatalf("normalized result = %#v, want task_done/completed", result)
	}
}

func TestExtractAgnesVideoResponseIDSupportsNestedTaskIDs(t *testing.T) {
	got := extractAgnesVideoResponseID([]byte(`{"data":{"task_id":"task_nested"}}`))
	if got != "task_nested" {
		t.Fatalf("response id = %q, want task_nested", got)
	}
}

func TestIsVideoUsageResultRecognizesAgnesModels(t *testing.T) {
	if !isVideoUsageResult(&OpenAIForwardResult{VideoCount: 1, BillingModel: "agnes-video-v2.0"}, nil) {
		t.Fatal("Agnes video result should use video billing")
	}
	if isVideoUsageResult(&OpenAIForwardResult{VideoCount: 1, BillingModel: "text-model"}, nil) {
		t.Fatal("non-video result must not use video billing")
	}
}
