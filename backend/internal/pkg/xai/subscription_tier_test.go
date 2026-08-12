//go:build unit

package xai

import (
	"encoding/base64"
	"encoding/json"
	"testing"

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
REDACTED

func TestNormalizeSubscriptionTierAliases(t *testing.T) {
	t.Parallel()

	require.Equal(t, "free", NormalizeSubscriptionTier("Free"))
	require.Equal(t, "free", NormalizeSubscriptionTier(" FREE "))
	require.Equal(t, "supergrok", NormalizeSubscriptionTier("SuperGrok"))
	require.Equal(t, "supergrok_heavy", NormalizeSubscriptionTier("SuperGrok Heavy"))
	require.Equal(t, "supergrok_heavy", NormalizeSubscriptionTier("SuperGrokPro"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("SuperGrok Lite"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("SuperGrokLite"))
	require.Equal(t, "x_basic", NormalizeSubscriptionTier("X Basic"))
	require.Equal(t, "free", NormalizeSubscriptionTier("free-tier"))
	require.Equal(t, "free", NormalizeSubscriptionTier("free_tier"))
	require.Equal(t, "free", NormalizeSubscriptionTier("grok-basic"))
	require.Equal(t, "free", NormalizeSubscriptionTier("grok_basic"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("supergrok_lite"))
REDACTED

func TestSubscriptionTierFromJWTUsesNumericClaim(t *testing.T) {
	t.Parallel()

	require.Equal(t, "supergrok_heavy", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 5REDACTED)))
	require.Equal(t, "free", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 0REDACTED)))
	require.Equal(t, "supergrok_lite", SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"tier": 6REDACTED)))
	require.Empty(t, SubscriptionTierFromJWT(jwtWithClaims(t, map[string]any{"sub": "user"REDACTED)))
	require.Empty(t, SubscriptionTierFromJWT("not-a-jwt"))
REDACTED

func jwtWithClaims(t *testing.T, claims map[string]any) string {
REDACTED
	payload, err := json.Marshal(claims)
REDACTED
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
REDACTED
