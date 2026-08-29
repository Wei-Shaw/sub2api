package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestParseOpenAIVideoResult(t *testing.T) {
	result := parseOpenAIVideoResult([]byte(`{"id":"video_123","model":"sora-2-pro","status":"completed","seconds":"12","size":"1792x1024","created_at":1712697600,"completed_at":1712697725}`), "req_1", time.Second)
	if result.ResponseID != "video_123" || result.Model != "sora-2-pro" {
		t.Fatalf("unexpected identity: %+v", result)
	}
	if result.VideoCount != 1 || result.VideoDurationSeconds != 12 || result.VideoResolution != VideoBillingResolution1080P {
		t.Fatalf("unexpected billing fields: %+v", result)
	}
	if result.VideoCreatedAtUnix != 1712697600 || result.VideoCompletedAtUnix != 1712697725 {
		t.Fatalf("unexpected upstream timestamps: %+v", result)
	}
}

func TestParseOpenAIVideoResultReadsCompatibleMetadata(t *testing.T) {
	body := []byte(`{
		"id":"task_n4aDlP3s2FEVuOkWR4LrFUS8QRxOnBkk",
		"model":"minimax-h3-768p",
		"status":"completed",
		"metadata":{"duration":10,"height":768,"width":1376}
	}`)

	result := parseOpenAIVideoResult(body, "req_metadata", time.Second)

	if result.VideoDurationSeconds != 10 {
		t.Fatalf("expected metadata.duration=10, got %d", result.VideoDurationSeconds)
	}
	if result.VideoResolution != VideoBillingResolution720P {
		t.Fatalf("expected 768p output to use 720p billing tier, got %q", result.VideoResolution)
	}
	if result.VideoCount != 1 {
		t.Fatalf("expected completed video to be billable: %+v", result)
	}
}

func TestStableVideoTaskBillingRequestID(t *testing.T) {
	if got := StableVideoTaskBillingRequestID(PlatformOpenAI, "video_123"); got != "openai-video:video_123" {
		t.Fatalf("unexpected openai billing id: %q", got)
	}
	if got := StableVideoTaskBillingRequestID(PlatformGrok, "video_123"); got != "grok-video:video_123" {
		t.Fatalf("unexpected grok billing id: %q", got)
	}
}

func TestForwardOpenAIVideoBodyErrorDoesNotWriteDuplicateResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
	body := []byte(`{"error":{"message":"upstream database error"}}`)

	_, err := (&OpenAIGatewayService{}).forwardOpenAIVideoBodyError(c, resp, body, "req_1")
	if err == nil {
		t.Fatal("expected upstream failover error")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("upstream error helper wrote a response body: %s", recorder.Body.String())
	}
}
