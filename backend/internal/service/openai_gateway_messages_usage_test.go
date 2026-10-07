//go:build unit

package service

import (
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestCopyOpenAIUsageFromResponsesUsageTrustsCanonicalCacheCreationValue(t *testing.T) {
	usage := &apicompat.ResponsesUsage{
		InputTokens:              20,
		OutputTokens:             2,
		CacheCreationInputTokens: 0,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens:     3,
			CacheWriteTokens: 19,
		},
	}

	got := copyOpenAIUsageFromResponsesUsage(usage, nil)

	require.Equal(t, 20, got.InputTokens)
	require.Equal(t, 3, got.CacheReadInputTokens)
	require.Zero(t, got.CacheCreationInputTokens)
}

func TestOpenAIReasoningTokenObservation(t *testing.T) {
	for _, path := range []string{"output_tokens_details", "completion_tokens_details"} {
		for _, count := range []int{0, 17} {
			raw := fmt.Sprintf(`{"usage":{"input_tokens":100,"output_tokens":40,"total_tokens":140,"%s":{"reasoning_tokens":%d}}}`, path, count)
			usage, ok := extractOpenAIUsageFromJSONBytes([]byte(raw))
			require.True(t, ok)
			require.NotNil(t, usage.ReasoningTokens)
			require.Equal(t, count, *usage.ReasoningTokens)
			require.Equal(t, 40, usage.OutputTokens, "breakdown must not be charged twice")
			var typed struct {
				Usage apicompat.ResponsesUsage `json:"usage"`
			}
			require.NoError(t, json.Unmarshal([]byte(raw), &typed))
			converted := copyOpenAIUsageFromResponsesUsage(&typed.Usage, nil)
			require.Equal(t, usage.ReasoningTokens, converted.ReasoningTokens)
		}
	}
	absent, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"usage":{"input_tokens":100,"output_tokens":40,"output_tokens_details":{"audio_tokens":2}}}`))
	require.True(t, ok)
	require.Nil(t, absent.ReasoningTokens)
}

func TestOpenAIReasoningTokenStreamingSnapshots(t *testing.T) {
	svc := &OpenAIGatewayService{}
	var usage OpenAIUsage
	frame := `{"type":"response.in_progress","response":{"usage":{"input_tokens":100,"output_tokens":40,"output_tokens_details":{"reasoning_tokens":17}}}}`
	svc.parseSSEUsage(frame, &usage)
	svc.parseSSEUsage(frame, &usage)
	require.Equal(t, 17, *usage.ReasoningTokens)
	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":50}}}`, &usage)
	require.Equal(t, 17, *usage.ReasoningTokens, "omitted optional breakdown retains observed value")
	require.Equal(t, 50, usage.OutputTokens)
	svc.parseSSEUsage(`{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":50,"output_tokens_details":{"reasoning_tokens":0}}}}`, &usage)
	require.Equal(t, 0, *usage.ReasoningTokens)
}

func TestGeminiReasoningTokenObservation(t *testing.T) {
	for _, raw := range []string{`{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20}}`, `{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":0}}`, `{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"thoughtsTokenCount":5}}`} {
		usage := extractGeminiUsage([]byte(raw))
		require.NotNil(t, usage)
		count := gjson.Get(raw, "usageMetadata.thoughtsTokenCount")
		if !count.Exists() {
			require.Nil(t, usage.ReasoningTokens)
		} else {
			require.NotNil(t, usage.ReasoningTokens)
			require.EqualValues(t, count.Int(), *usage.ReasoningTokens)
			require.EqualValues(t, 20+count.Int(), usage.OutputTokens)
		}
	}
}

func TestGeminiStreamingRetainsObservedReasoningTokens(t *testing.T) {
	body := `data: {"candidates":[{"content":{"parts":[{"text":"first"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"thoughtsTokenCount":7}}` + "\n\n" +
		`data: {"candidates":[{"content":{"parts":[{"text":"last"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":30}}` + "\n\n"
	_, usage, err := collectGeminiSSE(strings.NewReader(body), false)
	require.NoError(t, err)
	require.NotNil(t, usage.ReasoningTokens)
	require.Equal(t, 7, *usage.ReasoningTokens)
	require.Equal(t, 30, usage.OutputTokens, "preserve the existing output/billing snapshot")
}
