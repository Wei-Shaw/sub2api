package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeKnownOpenAICodexModel_BareGPT56RoutesToSol(t *testing.T) {
	tests := map[string]string{
		"gpt-5.6":            "gpt-5.6-sol",
		"openai/gpt-5.6":     "gpt-5.6-sol",
		"gpt5.6":             "gpt-5.6-sol",
		"gpt-5.6-high":       "gpt-5.6-sol",
		"gpt-5.6-max":        "gpt-5.6-sol",
		"gpt-5.6-2026-07-09": "gpt-5.6-sol",
		"openai/gpt-5.6-max": "gpt-5.6-sol",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
}

func TestUsageBillingModelCandidates_BareGPT56IncludesSol(t *testing.T) {
	require.Equal(t,
		[]string{"gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("gpt-5.6"),
	)
	require.Equal(t,
		[]string{"openai/gpt-5.6", "gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("openai/gpt-5.6"),
	)
}

func TestSupportsOpenAICodexSamplingParametersMatchesOnlyGPT56Family(t *testing.T) {
	tests := map[string]bool{
		"gpt-5.6":                true,
		"gpt-5.6-sol":            true,
		"openai/GPT_5.6_TERRA":   true,
		"gpt-5.6-cyber":          true,
		"gpt-5.6-2026-07-09":     true,
		"gpt-5.5":                false,
		"gpt-5.60":               false,
		"gpt-4.1":                false,
		"third-party/gpt-5.6ish": false,
	}

	for model, expected := range tests {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, expected, supportsOpenAICodexSamplingParameters(model))
		})
	}
}
