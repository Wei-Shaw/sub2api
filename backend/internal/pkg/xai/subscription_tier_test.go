//go:build unit

package xai

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMapJWTSubscriptionTierNumber(t *testing.T) {
	t.Parallel()

	require.Equal(t, "free", MapJWTSubscriptionTier(0))
	require.Equal(t, "supergrok", MapJWTSubscriptionTier(1))
	require.Equal(t, "x_basic", MapJWTSubscriptionTier(2))
	require.Equal(t, "x_premium", MapJWTSubscriptionTier(3))
	require.Equal(t, "x_premium_plus", MapJWTSubscriptionTier(4))
	require.Equal(t, "supergrok_heavy", MapJWTSubscriptionTier(5))
	require.Equal(t, "supergrok_lite", MapJWTSubscriptionTier(6))
	require.Equal(t, "supergrok_plus", MapJWTSubscriptionTier(7))
	require.Equal(t, "9", MapJWTSubscriptionTier(9))
}

func TestNormalizeSubscriptionTierAliases(t *testing.T) {
	t.Parallel()

	require.Equal(t, "free", NormalizeSubscriptionTier("Free"))
	require.Equal(t, "free", NormalizeSubscriptionTier(" FREE "))
	require.Equal(t, "supergrok", NormalizeSubscriptionTier("SuperGrok"))
	require.Equal(t, "supergrok_heavy", NormalizeSubscriptionTier("SuperGrok Heavy"))
	require.Equal(t, "supergrok_pro", NormalizeSubscriptionTier("SuperGrokPro"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("SuperGrok Lite"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("SuperGrokLite"))
	require.Equal(t, "x_basic", NormalizeSubscriptionTier("X Basic"))
	require.Equal(t, "free", NormalizeSubscriptionTier("free-tier"))
	require.Equal(t, "free", NormalizeSubscriptionTier("free_tier"))
	require.Equal(t, "free", NormalizeSubscriptionTier("grok-basic"))
	require.Equal(t, "free", NormalizeSubscriptionTier("grok_basic"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("supergrok_lite"))
}

func TestSubscriptionTierFromJWTUsesNumericClaim(t *testing.T) {
	t.Parallel()

	require.Equal(t, "supergrok_heavy", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 5})))
	require.Equal(t, "free", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 0})))
	require.Equal(t, "supergrok_lite", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 6})))
	require.Empty(t, SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"sub": "user"})))
	require.Empty(t, SubscriptionTierFromJWT("not-a-jwt"))
}

func TestCanonicalGrokPlanUsesOnlyGrok45ResponsesWindow(t *testing.T) {
	t.Parallel()

	zero := float64(0)
	heavyReq, heavyTok := int64(8300), int64(53_000_000)
	superReq, superTok := int64(900), int64(15_000_000)
	fresh := time.Now().UTC().Format(time.RFC3339)
	stale := time.Now().Add(-GrokQuotaSignalMaxAge - time.Hour).UTC().Format(time.RFC3339)

	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", nil))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrok", nil))
	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&zero, "SuperGrok Heavy", nil))
	require.Empty(t, CanonicalGrokPlan(&zero, "", nil))
	require.Equal(t, "free", CanonicalGrokPlan(&zero, "free", nil))

	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model:             "grok-4.5",
		Requests:          &QuotaWindow{Limit: &heavyReq},
		Tokens:            &QuotaWindow{Limit: &heavyTok},
		LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model:             "grok-4.6",
		Requests:          &QuotaWindow{Limit: &heavyReq},
		Tokens:            &QuotaWindow{Limit: &heavyTok},
		LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Requests:          &QuotaWindow{Limit: &heavyReq},
		Tokens:            &QuotaWindow{Limit: &heavyTok},
		LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model:             "grok-4.5",
		Requests:          &QuotaWindow{Limit: &superReq},
		Tokens:            &QuotaWindow{Limit: &superTok},
		LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model:             "grok-4.5",
		Requests:          &QuotaWindow{Limit: &heavyReq},
		LastHeadersSeenAt: stale,
	}))
	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model:                 "grok-4.6",
		Requests:              &QuotaWindow{Limit: &superReq},
		PlanFrom45Responses:   "supergrok_heavy",
		PlanFrom45ResponsesAt: fresh,
	}))
	require.Equal(t, "free", CanonicalGrokPlan(&zero, "free", &QuotaSnapshot{
		Model:             "grok-4.5",
		Requests:          &QuotaWindow{Limit: &heavyReq},
		LastHeadersSeenAt: fresh,
	}))
	heavyCents := float64(SuperGrokHeavyLimitCents)
	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&heavyCents, "SuperGrokPro", nil))
}

func TestApplyGrok45ResponsesPlanSignalCarriesHint(t *testing.T) {
	t.Parallel()

	heavyReq := int64(8300)
	fresh := time.Now().UTC().Format(time.RFC3339)
	prev := &QuotaSnapshot{
		Model:             "grok-4.5",
		Requests:          &QuotaWindow{Limit: &heavyReq},
		LastHeadersSeenAt: fresh,
	}
	prev.ApplyGrok45ResponsesPlanSignal(nil)
	require.Equal(t, "supergrok_heavy", prev.PlanFrom45Responses)

	next := &QuotaSnapshot{
		Model:             "grok-4.6",
		Requests:          &QuotaWindow{Limit: int64Ptr(100)},
		LastHeadersSeenAt: fresh,
	}
	next.ApplyGrok45ResponsesPlanSignal(prev)
	require.Equal(t, "supergrok_heavy", next.PlanFrom45Responses)
	require.Equal(t, prev.PlanFrom45ResponsesAt, next.PlanFrom45ResponsesAt)
}

func int64Ptr(v int64) *int64 { return &v }

func jwtWithClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
