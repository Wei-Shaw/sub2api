//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokTokenRefreshWindowWithJitter_StableAndBounded(t *testing.T) {
	base := grokTokenRefreshSkew
	w1 := grokTokenRefreshWindowWithJitter(42, base)
	w2 := grokTokenRefreshWindowWithJitter(42, base)
	require.Equal(t, w1, w2, "same account id must yield stable window")
	require.GreaterOrEqual(t, w1, grokTokenRefreshSkewMin)
	require.LessOrEqual(t, w1, base)

	// Different accounts should usually differ (not a hard guarantee for all pairs,
	// but for sequential ids the hash spread is good enough to assert inequality
	// across a small sample).
	seen := map[time.Duration]bool{}
	for id := int64(1); id <= 50; id++ {
		seen[grokTokenRefreshWindowWithJitter(id, base)] = true
	}
	require.Greater(t, len(seen), 1, "jitter should spread windows across accounts")
}

func TestGrokTokenRefresher_NeedsRefresh_UsesSkewFloor(t *testing.T) {
	refresher := NewGrokTokenRefresher(nil)
	// Expires in 50 minutes — within 1h skew, should need refresh.
	expires := time.Now().Add(50 * time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       7,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "at",
			"refresh_token": "rt",
			"expires_at":    expires,
		},
	}
	// Pass a tiny window; NeedsRefresh raises it to grokTokenRefreshSkew then jitter.
	require.True(t, refresher.NeedsRefresh(account, time.Minute))

	// Far future — no refresh.
	account.Credentials["expires_at"] = time.Now().Add(3 * time.Hour).UTC().Format(time.RFC3339)
	require.False(t, refresher.NeedsRefresh(account, time.Minute))
}
