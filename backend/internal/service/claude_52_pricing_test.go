//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaude52FallbackPricing(t *testing.T) {
	svc := newTestBillingService()
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2", "anthropic/claude-fable-5-2", "CLAUDE-MYTHOS-5-2", "claude-fable-5-2-20261109"} {
		t.Run(model, func(t *testing.T) {
			p, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 8e-6, p.InputPricePerToken, 1e-12)
			require.InDelta(t, 40e-6, p.OutputPricePerToken, 1e-12)
			require.InDelta(t, 10e-6, p.CacheCreationPricePerToken, 1e-12)
			require.InDelta(t, 10e-6, p.CacheCreation5mPrice, 1e-12)
			require.InDelta(t, 16e-6, p.CacheCreation1hPrice, 1e-12)
			require.InDelta(t, .2e-6, p.CacheReadPricePerToken, 1e-12)
			require.True(t, p.SupportsCacheBreakdown)
			require.Zero(t, p.LongContextInputThreshold)
			require.Empty(t, p.ReasoningEffortMultipliers)
			for _, tier := range []string{"", "fast"} {
				cost, err := svc.CalculateCostWithServiceTier(model, UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheCreationTokens: 2_000_000, CacheCreation5mTokens: 1_000_000, CacheCreation1hTokens: 1_000_000}, 1, tier)
				require.NoError(t, err)
				multiplier := 1.0
				if tier == "fast" {
					multiplier = 2
				}
				require.InDelta(t, 8*multiplier, cost.InputCost, 1e-10)
				require.InDelta(t, 40*multiplier, cost.OutputCost, 1e-10)
				require.InDelta(t, 26*multiplier, cost.CacheCreationCost, 1e-10)
				require.InDelta(t, .2*multiplier, cost.CacheReadCost, 1e-10)
			}
		})
	}
	// The older versions keep their own rates rather than inheriting 5.2 pricing.
	for _, model := range []string{"claude-fable-5", "claude-fable-5-1"} {
		p, err := svc.GetModelPricing(model)
		require.NoError(t, err)
		require.InDelta(t, 10e-6, p.InputPricePerToken, 1e-12)
	}
}

func TestClaude52FastTierEligibility(t *testing.T) {
	native := &Account{Platform: PlatformAnthropic}
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		require.NotNil(t, anthropicSpeedServiceTier(native, "fast", model))
		require.Nil(t, anthropicSpeedServiceTier(native, "standard", model))
		require.Nil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformAnthropic, Type: AccountTypeBedrock}, "fast", model))
		require.Nil(t, anthropicSpeedServiceTier(&Account{Platform: PlatformAntigravity}, "fast", model))
	}
	for _, model := range []string{"claude-fable-5", "claude-fable-5-1", "claude-fable-5-20", "claude-mythos-5-20"} {
		require.Nil(t, anthropicSpeedServiceTier(native, "fast", model))
	}
}

func TestClaude52RequestValidationAndCatalog(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		require.True(t, isClaude55SignedThinkingModel(model))
		for _, body := range []string{`{}`, `{"thinking":{"type":"adaptive"}}`, `{"temperature":1}`, `{"top_p":0.99}`} {
			require.NoError(t, validateClaude55Request([]byte(body), model))
		}
		for _, body := range []string{`{"thinking":{"type":"enabled","budget_tokens":1024}}`, `{"thinking":{"type":"disabled"}}`, `{"thinking":{"type":"between_tools"}}`, `{"tool_choice":{"type":"any"}}`, `{"temperature":0.5}`, `{"top_p":1}`, `{"top_k":40}`, `{"temperature":1,"top_p":0.99}`} {
			require.Error(t, validateClaude55Request([]byte(body), model), body)
		}
		descriptor := newConfiguredCodexModelDescriptor(model)
		require.Equal(t, int64(1_000_000), descriptor.ContextWindow)
		require.Equal(t, "high", *descriptor.DefaultReasoningLevel)
	}
}
