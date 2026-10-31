//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIUsageDeepSeekPromptCacheHitTokensCountAsCacheRead(t *testing.T) {
	// DeepSeek usage 只有 prompt_cache_hit/miss_tokens
	usage, ok := openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":100,"completion_tokens":5,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20}`))
	require.True(t, ok)
	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 80, usage.CacheReadInputTokens)

	// 同时返回 prompt_tokens_details.cached_tokens 时以嵌套字段为准
	usage, ok = openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":100,"completion_tokens":5,"prompt_cache_hit_tokens":80,"prompt_tokens_details":{"cached_tokens":64}}`))
	require.True(t, ok)
	require.Equal(t, 64, usage.CacheReadInputTokens)

	// 顶层 cached_tokens 保持原有行为
	usage, ok = openAIUsageFromGJSON(gjson.Parse(`{"prompt_tokens":50,"completion_tokens":5,"cached_tokens":30}`))
	require.True(t, ok)
	require.Equal(t, 30, usage.CacheReadInputTokens)
}
