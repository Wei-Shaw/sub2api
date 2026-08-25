package service

import (
	"testing"
	"time"
)

func TestParseOpenAIVideoResult(t *testing.T) {
	result := parseOpenAIVideoResult([]byte(`{"id":"video_123","model":"sora-2-pro","status":"completed","seconds":"12","size":"1792x1024"}`), "req_1", time.Second)
	if result.ResponseID != "video_123" || result.Model != "sora-2-pro" {
		t.Fatalf("unexpected identity: %+v", result)
	}
	if result.VideoCount != 1 || result.VideoDurationSeconds != 12 || result.VideoResolution != VideoBillingResolution1080P {
		t.Fatalf("unexpected billing fields: %+v", result)
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
