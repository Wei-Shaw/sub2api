package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdaptAnthropicThinkingForUpstreamError_EnabledToAdaptive(t *testing.T) {
	body := []byte(`{"model":"claude-fable-5","max_tokens":16000,"thinking":{"type":"enabled","budget_tokens":10000,"display":"summarized"},"output_config":{"effort":"medium"}}`)
	response := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}}`)

	got, mode, applied := adaptAnthropicThinkingForUpstreamError(body, response)

	require.True(t, applied)
	require.Equal(t, anthropicThinkingModeAdaptive, mode)
	require.JSONEq(t, `{"model":"claude-fable-5","max_tokens":16000,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"medium"}}`, string(got))
}

func TestAdaptAnthropicThinkingForUpstreamError_AdaptiveToLegacy(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16000,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"high"}}`)
	response := []byte(`{"error":{"message":"adaptive thinking is not supported on this model; use thinking.type.enabled with budget_tokens instead"}}`)

	got, mode, applied := adaptAnthropicThinkingForUpstreamError(body, response)

	require.True(t, applied)
	require.Equal(t, anthropicThinkingModeLegacy, mode)
	var parsed struct {
		Thinking struct {
			Type    string `json:"type"`
			Budget  int64  `json:"budget_tokens"`
			Display string `json:"display"`
		} `json:"thinking"`
		OutputConfig map[string]any `json:"output_config"`
	}
	require.NoError(t, json.Unmarshal(got, &parsed))
	require.Equal(t, "enabled", parsed.Thinking.Type)
	require.GreaterOrEqual(t, parsed.Thinking.Budget, int64(1024))
	require.Less(t, parsed.Thinking.Budget, int64(16000))
	require.Equal(t, "summarized", parsed.Thinking.Display)
	require.NotContains(t, parsed.OutputConfig, "effort")
}

func TestAdaptAnthropicThinkingForUpstreamError_DisabledToAdaptive(t *testing.T) {
	body := []byte(`{"max_tokens":4096,"thinking":{"type":"disabled"}}`)
	response := []byte(`{"error":{"message":"\"thinking.type.disabled\" is not supported for this model; thinking defaults to adaptive mode"}}`)

	got, mode, applied := adaptAnthropicThinkingForUpstreamError(body, response)

	require.True(t, applied)
	require.Equal(t, anthropicThinkingModeAdaptive, mode)
	require.JSONEq(t, `{"max_tokens":4096,"thinking":{"type":"adaptive"}}`, string(got))
}

func TestAdaptAnthropicThinkingForUpstreamError_UnrelatedErrorIsUnchanged(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":10000}}`)
	response := []byte(`{"error":{"message":"invalid_request_error: max_tokens is too small"}}`)

	got, mode, applied := adaptAnthropicThinkingForUpstreamError(body, response)

	require.False(t, applied)
	require.Equal(t, anthropicThinkingModeUnknown, mode)
	require.Equal(t, body, got)
}

func TestAdaptAnthropicThinkingForUpstreamError_InvalidJSONIsFailSafe(t *testing.T) {
	response := []byte(`{"error":{"message":"\"thinking.type.enabled\" is not supported; use adaptive"}}`)

	got, mode, applied := adaptAnthropicThinkingForUpstreamError([]byte(`{not-json`), response)

	require.False(t, applied)
	require.Equal(t, anthropicThinkingModeUnknown, mode)
	require.Equal(t, []byte(`{not-json`), got)
}
