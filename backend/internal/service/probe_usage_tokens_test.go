package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureProbeUsageOpenAIResponses(t *testing.T) {
	var usage ProbeUsageTokens
	captureProbeUsage(&usage, parseProbeJSONObject("{\"usage\":{\"input_tokens\":100,\"output_tokens\":20,\"input_tokens_details\":{\"cached_tokens\":30}}}"))
	require.Equal(t, 70, usage.Input, "OpenAI 口径 input_tokens 含缓存，存储时拆出去")
	require.Equal(t, 20, usage.Output)
	require.Equal(t, 0, usage.CacheCreation)
	require.Equal(t, 30, usage.CacheRead)
}

func TestCaptureProbeUsageOpenAIChat(t *testing.T) {
	var usage ProbeUsageTokens
	captureProbeUsage(&usage, parseProbeJSONObject("{\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":10,\"prompt_tokens_details\":{\"cached_tokens\":20}}}"))
	require.Equal(t, 30, usage.Input)
	require.Equal(t, 10, usage.Output)
	require.Equal(t, 20, usage.CacheRead)
}

func TestCaptureProbeUsageAnthropicMergesEvents(t *testing.T) {
	var usage ProbeUsageTokens
	captureProbeUsage(&usage, parseProbeJSONObject("{\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_creation_input_tokens\":40,\"cache_read_input_tokens\":25}}}"))
	captureProbeUsage(&usage, parseProbeJSONObject("{\"type\":\"message_delta\",\"usage\":{\"output_tokens\":12}}"))
	require.Equal(t, 100, usage.Input, "Anthropic input_tokens 不含缓存，直接取")
	require.Equal(t, 12, usage.Output)
	require.Equal(t, 40, usage.CacheCreation)
	require.Equal(t, 25, usage.CacheRead)
}

func TestCaptureProbeUsageGemini(t *testing.T) {
	var usage ProbeUsageTokens
	captureProbeUsage(&usage, parseProbeJSONObject("{\"usageMetadata\":{\"promptTokenCount\":90,\"candidatesTokenCount\":15,\"cachedContentTokenCount\":10}}"))
	require.Equal(t, 80, usage.Input)
	require.Equal(t, 15, usage.Output)
	require.Equal(t, 10, usage.CacheRead)
}

func TestCaptureProbeUsageDoesNotDoubleCount(t *testing.T) {
	var usage ProbeUsageTokens
	obj := parseProbeJSONObject("{\"usage\":{\"input_tokens\":100,\"output_tokens\":20}}")
	captureProbeUsage(&usage, obj)
	captureProbeUsage(&usage, obj)
	require.Equal(t, 100, usage.Input)
	require.Equal(t, 20, usage.Output)
}

func TestCaptureProbeUsageIgnoresEmpty(t *testing.T) {
	var usage ProbeUsageTokens
	captureProbeUsage(&usage, nil)
	captureProbeUsage(&usage, map[string]any{})
	require.True(t, usage.IsZero())
}
