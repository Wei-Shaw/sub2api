package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

// The Anthropic -> Chat Completions / Responses converters bill from
// ClaudeUsage, not from the raw upstream JSON. If the conversion layer drops
// usage.cache_creation, the aggregate survives but the TTL split does not, and
// billing then cannot tell a 1h cache write from a 5m one. These tests pin the
// split surviving the conversion and reaching the billed numbers.

func TestMergeAnthropicUsageKeepsCacheCreationTTLBreakdown(t *testing.T) {
	var src apicompat.AnthropicUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"input_tokens": 2,
		"output_tokens": 3651,
		"cache_read_input_tokens": 43965,
		"cache_creation_input_tokens": 338329,
		"cache_creation": {
			"ephemeral_5m_input_tokens": 21126,
			"ephemeral_1h_input_tokens": 317203
		}
	}`), &src))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, src)

	require.Equal(t, 338329, usage.CacheCreationInputTokens)
	require.Equal(t, 21126, usage.CacheCreation5mTokens)
	require.Equal(t, 317203, usage.CacheCreation1hTokens)
}

func TestMergeAnthropicUsageCacheCreationSplitSurvivesMessageStartThenDelta(t *testing.T) {
	// Real streams report the split on message_start; message_delta repeats the
	// aggregate without the nested object. The detail must not be erased.
	var start apicompat.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_start","message":{"usage":{
		"input_tokens": 2,
		"cache_creation_input_tokens": 338329,
		"cache_read_input_tokens": 43965,
		"cache_creation": {"ephemeral_5m_input_tokens": 21126, "ephemeral_1h_input_tokens": 317203}
	}}}`), &start))
	var delta apicompat.AnthropicStreamEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"message_delta","usage":{
		"output_tokens": 3651,
		"cache_creation_input_tokens": 338329
	}}`), &delta))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, start.Message.Usage)
	mergeAnthropicUsage(usage, *delta.Usage)

	require.Equal(t, 338329, usage.CacheCreationInputTokens)
	require.Equal(t, 21126, usage.CacheCreation5mTokens)
	require.Equal(t, 317203, usage.CacheCreation1hTokens)
	require.Equal(t, 3651, usage.OutputTokens)
}

func TestMergeAnthropicUsageCapsContradictorySplitAtBilling(t *testing.T) {
	// A provider that reports a split larger than its own aggregate must not be
	// able to inflate cost: computeCacheCreationCost caps it, so the conversion
	// layer passes the numbers through unchanged.
	var src apicompat.AnthropicUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"cache_creation_input_tokens": 100,
		"cache_creation": {"ephemeral_5m_input_tokens": 90, "ephemeral_1h_input_tokens": 60}
	}`), &src))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, src)
	require.Equal(t, 90, usage.CacheCreation5mTokens)
	require.Equal(t, 60, usage.CacheCreation1hTokens)

	svc := &BillingService{}
	pricing := &ModelPricing{
		SupportsCacheBreakdown: true,
		CacheCreation5mPrice:   1,
		CacheCreation1hPrice:   1,
	}
	cost := svc.computeCacheCreationCost(pricing, UsageTokens{
		CacheCreationTokens:   usage.CacheCreationInputTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
	}, 0, 1)
	require.Equal(t, float64(100), cost,
		"cache writes must never be billed above the reported aggregate")
}

func TestBilledCacheCreationUses1hPriceAfterConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pricing resolution in -short mode")
	}

	var src apicompat.AnthropicUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"cache_creation_input_tokens": 300000,
		"cache_creation": {"ephemeral_5m_input_tokens": 0, "ephemeral_1h_input_tokens": 300000}
	}`), &src))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, src)

	svc := &BillingService{}
	pricing := &ModelPricing{
		SupportsCacheBreakdown:     true,
		CacheCreation5mPrice:       6.25e-6,
		CacheCreation1hPrice:       10e-6,
		CacheCreationPricePerToken: 6.25e-6,
	}
	cost := svc.computeCacheCreationCost(pricing, UsageTokens{
		CacheCreationTokens:   usage.CacheCreationInputTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
	}, pricing.CacheCreationPricePerToken, 1)

	require.InDelta(t, 300000*10e-6, cost, 1e-9,
		"a pure-1h cache write must be billed at the 1h price, not the 5m fallback")
}

func TestMergeAnthropicUsageWithoutSplitLeavesBillingFallbackIntact(t *testing.T) {
	// Providers that really do not send the nested object keep the existing
	// all-5m fallback; this fix must not invent a split.
	var src apicompat.AnthropicUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"cache_creation_input_tokens": 5000
	}`), &src))

	usage := &ClaudeUsage{}
	mergeAnthropicUsage(usage, src)

	require.Equal(t, 5000, usage.CacheCreationInputTokens)
	require.Zero(t, usage.CacheCreation5mTokens)
	require.Zero(t, usage.CacheCreation1hTokens)

	svc := &BillingService{}
	pricing := &ModelPricing{
		SupportsCacheBreakdown: true,
		CacheCreation5mPrice:   4e-6,
		CacheCreation1hPrice:   8e-6,
	}
	cost := svc.computeCacheCreationCost(pricing, UsageTokens{
		CacheCreationTokens: usage.CacheCreationInputTokens,
	}, 0, 1)
	require.InDelta(t, 5000*4e-6, cost, 1e-12)
}
