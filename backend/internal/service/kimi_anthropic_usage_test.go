package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestParseSSEUsagePassthroughNormalizesKimiPromptUsage(t *testing.T) {
	usage := &ClaudeUsage{}

	parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":173306,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0,"prompt_tokens":173306,"cached_tokens":0}}}`, usage)
	require.Equal(t, 173306, usage.InputTokens)
	require.Zero(t, usage.CacheReadInputTokens)

	parseSSEUsagePassthrough(`{"type":"message_delta","usage":{"input_tokens":250,"cache_creation_input_tokens":0,"cache_read_input_tokens":173056,"output_tokens":166,"prompt_tokens":173306,"cached_tokens":173056}}`, usage)
	require.Equal(t, 250, usage.InputTokens, "Kimi message_delta input_tokens is already the uncached bucket")
	require.Equal(t, 173056, usage.CacheReadInputTokens)
	require.Equal(t, 166, usage.OutputTokens)
}

func TestParseSSEUsagePassthroughKimiFullyCachedInputReplacesStartTotal(t *testing.T) {
	usage := &ClaudeUsage{}

	parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":{"input_tokens":173306,"prompt_tokens":173306}}}`, usage)
	parseSSEUsagePassthrough(`{"type":"message_delta","usage":{"input_tokens":0,"cache_read_input_tokens":173306,"output_tokens":8,"prompt_tokens":173306,"cached_tokens":173306}}`, usage)

	require.Zero(t, usage.InputTokens, "an explicit zero uncached bucket must not retain message_start's total")
	require.Equal(t, 173306, usage.CacheReadInputTokens)
}

func TestParseClaudeUsageFromResponseBodyNormalizesCNProviderAliases(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantInput     int
		wantCacheRead int
		wantOutput    int
	}{
		{
			name:          "Kimi top-level cached_tokens",
			body:          `{"usage":{"input_tokens":173306,"output_tokens":166,"cache_read_input_tokens":173056,"prompt_tokens":173306,"cached_tokens":173056}}`,
			wantInput:     250,
			wantCacheRead: 173056,
			wantOutput:    166,
		},
		{
			name:          "GLM nested prompt cache details",
			body:          `{"usage":{"input_tokens":1200,"output_tokens":300,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}}`,
			wantInput:     400,
			wantCacheRead: 800,
			wantOutput:    300,
		},
		{
			name:          "DeepSeek prompt cache hit and miss buckets",
			body:          `{"usage":{"input_tokens":1200,"output_tokens":300,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`,
			wantInput:     400,
			wantCacheRead: 800,
			wantOutput:    300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := parseClaudeUsageFromResponseBody([]byte(tt.body))
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantCacheRead, usage.CacheReadInputTokens)
			require.Equal(t, tt.wantOutput, usage.OutputTokens)
		})
	}
}

func TestParseSSEUsagePassthroughNormalizesGLMAndDeepSeekAliases(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		wantInput     int
		wantCacheRead int
	}{
		{
			name:          "GLM",
			data:          `{"type":"message_delta","usage":{"input_tokens":1200,"output_tokens":30,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}}`,
			wantInput:     400,
			wantCacheRead: 800,
		},
		{
			name:          "DeepSeek",
			data:          `{"type":"message_delta","usage":{"input_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`,
			wantInput:     400,
			wantCacheRead: 800,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &ClaudeUsage{}
			parseSSEUsagePassthrough(tt.data, usage)
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantCacheRead, usage.CacheReadInputTokens)
			require.Equal(t, 30, usage.OutputTokens)
		})
	}
}

func TestMergeAnthropicUsageNormalizesKimiStreamForOpenAIBilling(t *testing.T) {
	var start apicompat.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":173306,"prompt_tokens":173306,"cached_tokens":0}}}`), &start))
	var delta apicompat.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_delta","usage":{"input_tokens":250,"cache_read_input_tokens":173056,"output_tokens":166,"prompt_tokens":173306,"cached_tokens":173056}}`), &delta))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, start.Message.Usage)
	mergeAnthropicUsage(usage, *delta.Usage)
	require.Equal(t, 250, usage.InputTokens)
	require.Equal(t, 173056, usage.CacheReadInputTokens)

	openAIUsage := claudeUsageToOpenAIUsage(usage)
	require.Equal(t, 173306, openAIUsage.InputTokens, "OpenAI gateway expects an inclusive input total")
	require.Equal(t, 250, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
	require.Equal(t, 166, openAIUsage.OutputTokens)
}

func TestMergeAnthropicUsageNormalizesGLMAndDeepSeekAliases(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "GLM",
			raw:  `{"input_tokens":1200,"output_tokens":30,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}`,
		},
		{
			name: "DeepSeek",
			raw:  `{"input_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var src apicompat.AnthropicUsage
			require.NoError(t, json.Unmarshal([]byte(tt.raw), &src))

			usage := &ClaudeUsage{}
			mergeAnthropicUsage(usage, src)
			require.Equal(t, 400, usage.InputTokens)
			require.Equal(t, 800, usage.CacheReadInputTokens)

			openAIUsage := claudeUsageToOpenAIUsage(usage)
			require.Equal(t, 1200, openAIUsage.InputTokens)
			require.Equal(t, 400, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
		})
	}
}

func TestClaudeUsageToOpenAIUsagePreservesCNProviderNativeAnthropicBuckets(t *testing.T) {
	tests := []struct {
		name         string
		usage        ClaudeUsage
		wantTotal    int
		wantUncached int
	}{
		{
			name: "GLM",
			usage: ClaudeUsage{
				InputTokens:              2,
				OutputTokens:             302,
				CacheCreationInputTokens: 733,
				CacheReadInputTokens:     376156,
			},
			wantTotal:    376891,
			wantUncached: 2,
		},
		{
			name: "DeepSeek",
			usage: ClaudeUsage{
				InputTokens:          400,
				OutputTokens:         30,
				CacheReadInputTokens: 800,
			},
			wantTotal:    1200,
			wantUncached: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIUsage := claudeUsageToOpenAIUsage(&tt.usage)
			require.Equal(t, tt.wantTotal, openAIUsage.InputTokens)
			require.Equal(t, tt.wantUncached, openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens)
			require.Equal(t, tt.usage.CacheReadInputTokens, openAIUsage.CacheReadInputTokens)
			require.Equal(t, tt.usage.CacheCreationInputTokens, openAIUsage.CacheCreationInputTokens)
		})
	}
}

func TestCNProviderAnthropicUsageBillsUncachedInput(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		body      string
		wantInput int
	}{
		{
			name:      "Kimi",
			model:     "k3",
			body:      `{"usage":{"input_tokens":173306,"output_tokens":166,"prompt_tokens":173306,"cached_tokens":173056}}`,
			wantInput: 250,
		},
		{
			name:      "GLM",
			model:     "glm-5.2",
			body:      `{"usage":{"input_tokens":1200,"output_tokens":30,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}}`,
			wantInput: 400,
		},
		{
			name:      "DeepSeek",
			model:     "deepseek-v4-flash",
			body:      `{"usage":{"input_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}}`,
			wantInput: 400,
		},
	}

	billing := NewBillingService(&config.Config{}, nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeUsage := parseClaudeUsageFromResponseBody([]byte(tt.body))
			openAIUsage := claudeUsageToOpenAIUsage(claudeUsage)
			uncachedInput := max(openAIUsage.InputTokens-openAIUsage.CacheReadInputTokens-openAIUsage.CacheCreationInputTokens, 0)
			require.Equal(t, tt.wantInput, uncachedInput)

			cost, err := billing.CalculateCost(tt.model, UsageTokens{
				InputTokens:         uncachedInput,
				OutputTokens:        openAIUsage.OutputTokens,
				CacheCreationTokens: openAIUsage.CacheCreationInputTokens,
				CacheReadTokens:     openAIUsage.CacheReadInputTokens,
			}, 1)
			require.NoError(t, err)
			require.Positive(t, cost.InputCost, "uncached input must contribute to the final charge")

			pricing, err := billing.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, float64(tt.wantInput)*pricing.InputPricePerToken, cost.InputCost, 1e-12)
		})
	}
}
