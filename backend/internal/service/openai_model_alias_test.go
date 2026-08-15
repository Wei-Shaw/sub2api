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

func TestNormalizeKnownOpenAICodexModel_WMSuffixPreserved(t *testing.T) {
	tests := map[string]string{
		// gpt-5.6-sol-wm 是独立模型 ID，不能被宽匹配归并成 gpt-5.6-sol。
		"gpt-5.6-sol-wm":      "gpt-5.6-sol-wm",
		"openai/gpt-5.6-sol-wm": "gpt-5.6-sol-wm",
		"GPT-5.6-SOL-WM":      "gpt-5.6-sol-wm",
		// 已知后缀和日期变体仍归并到基础模型。
		"gpt-5.6-sol-high":       "gpt-5.6-sol",
		"gpt-5.6-sol-2026-07-09": "gpt-5.6-sol",
		"gpt-5.6-sol":            "gpt-5.6-sol",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
}

func TestIsOpenAIGPT56Model_WMSuffixRecognized(t *testing.T) {
	require.True(t, isOpenAIGPT56Model("gpt-5.6-sol-wm"))
	require.True(t, isOpenAIGPT56Model("gpt-5.6-sol-wm-2026-08-14"))
	require.False(t, isOpenAIGPT56Model("gpt-5.6-unknown"))
}
