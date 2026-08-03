//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractProbedPlanRaw_GrokFromBillingSnapshot(t *testing.T) {
	acc := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			// 无 subscription_tier
			"access_token": "x",
		},
		Extra: map[string]any{
			"grok_billing_snapshot": map[string]any{
				"plan": "SuperGrok",
			},
		},
	}
	raw := ExtractProbedPlanRaw(acc)
	require.Equal(t, "SuperGrok", raw)
	code := NormalizeUpstreamPlanFromProbe(PlatformGrok, raw)
	require.Equal(t, "supergrok", code)
}

func TestExtractProbedPlanRaw_GrokPrefersCredentialTier(t *testing.T) {
	acc := &Account{
		Platform: PlatformGrok,
		Credentials: map[string]any{
			"subscription_tier": "supergrokheavy",
		},
		Extra: map[string]any{
			"grok_billing_snapshot": map[string]any{"plan": "SuperGrok"},
		},
	}
	require.Equal(t, "supergrokheavy", ExtractProbedPlanRaw(acc))
}
