package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestCopyOpenAIUsageFromResponsesUsage_PreservesCacheWritePresence(t *testing.T) {
	t.Parallel()

	got := copyOpenAIUsageFromResponsesUsage(&apicompat.ResponsesUsage{
		InputTokens:                     100,
		OutputTokens:                    5,
		CacheCreationInputTokens:        0,
		CacheCreationInputTokensPresent: true,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens: 64,
		},
	})

	require.Equal(t, 100, got.InputTokens)
	require.Equal(t, 64, got.CacheReadInputTokens)
	require.Zero(t, got.CacheCreationInputTokens)
	require.True(t, got.CacheCreationInputTokensPresent)
}

func TestMergeOpenAIUsageNonZero_DoesNotErasePositiveCacheWriteWithProgressiveZero(t *testing.T) {
	t.Parallel()

	dst := OpenAIUsage{
		InputTokens:                     100,
		CacheCreationInputTokens:        32,
		CacheCreationInputTokensPresent: true,
	}
	src := OpenAIUsage{
		InputTokens:                     100,
		CacheCreationInputTokens:        0,
		CacheCreationInputTokensPresent: true,
	}

	mergeOpenAIUsageNonZero(&dst, src)

	require.Equal(t, 32, dst.CacheCreationInputTokens)
	require.True(t, dst.CacheCreationInputTokensPresent)
}
