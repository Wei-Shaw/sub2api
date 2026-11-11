//go:build unit

package service

import (
	"context"
	"testing"
)

func TestGrokResponsesMonitorValidation(t *testing.T) {
	if err := validateAPIMode(MonitorProviderGrok, MonitorAPIModeResponses); err != nil {
		t.Fatalf("Grok Responses should be supported: %v", err)
	}
	if err := validateAPIMode(MonitorProviderAnthropic, MonitorAPIModeResponses); err == nil {
		t.Fatal("Anthropic must still reject Responses")
	}
	if err := validateTemplateCreateParams(ChannelMonitorRequestTemplateCreateParams{
		Name: "Grok Responses", Provider: MonitorProviderGrok, APIMode: MonitorAPIModeResponses,
	}); err != nil {
		t.Fatalf("Grok Responses templates should be valid: %v", err)
	}
}

func TestGrokResponsesMonitorUpdatePreservesMode(t *testing.T) {
	name := "renamed"
	existing := &ChannelMonitor{
		Provider: MonitorProviderGrok, APIMode: MonitorAPIModeResponses,
		Endpoint: "https://api.x.ai/v1", PrimaryModel: MonitorDefaultGrokModel, IntervalSeconds: 60,
	}
	if err := applyMonitorUpdate(existing, ChannelMonitorUpdateParams{Name: &name}); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if existing.APIMode != MonitorAPIModeResponses {
		t.Fatalf("rename changed protocol to %q", existing.APIMode)
	}
}

func TestGrokResponsesMonitorRequest(t *testing.T) {
	for _, suffix := range []string{"", "/v1"} {
		t.Run("endpoint"+suffix, func(t *testing.T) {
			h := &openAICaptureHandler{responsesLeadingReasoning: true}
			endpoint := setupFakeOpenAI(t, h) + suffix
			res := runCheckForModel(context.Background(), MonitorProviderGrok, endpoint, "xai-test-key", "grok-test", &CheckOptions{
				APIMode:          MonitorAPIModeResponses,
				BodyOverrideMode: MonitorBodyOverrideModeMerge,
				BodyOverride: map[string]any{
					"model": "wrong-model", "instructions": "wrong-instructions", "input": "wrong-input", "stream": true,
					"max_output_tokens": 64,
				},
			})
			if res.Status != MonitorStatusOperational {
				t.Fatalf("Responses challenge failed: status=%s message=%q", res.Status, res.Message)
			}
			if h.lastPath != providerOpenAIResponsesPath {
				t.Fatalf("wrong request path: %q", h.lastPath)
			}
			if h.lastBody["model"] != "grok-test" || h.lastBody["stream"] != false {
				t.Fatalf("merge overwrote protected request fields: %v", h.lastBody)
			}
			if h.lastBody["input"] == "wrong-input" || h.lastBody["instructions"] == "wrong-instructions" {
				t.Fatalf("merge overwrote the arithmetic challenge: %v", h.lastBody)
			}
			if _, ok := h.lastBody["messages"]; ok {
				t.Fatal("Responses request contains Chat messages")
			}
			if h.lastBody["max_output_tokens"] != float64(64) {
				t.Fatalf("merge lost the unprotected token override: %v", h.lastBody)
			}
			if h.lastHeaders.Get("Authorization") != "Bearer xai-test-key" {
				t.Fatal("missing bearer authorization")
			}
		})
	}
}
