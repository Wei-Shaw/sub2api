package web3deposit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScannerCursorHasActiveLease(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC)
	owner := "scanner-01"
	token := "lease-token-01"
	expiresAt := now.Add(time.Minute)
	cursor := ScannerCursor{
		LeaseOwner:     &owner,
		LeaseToken:     &token,
		LeaseExpiresAt: &expiresAt,
	}

	require.True(t, cursor.HasActiveLease(token, now))
	require.False(t, cursor.HasActiveLease("stale-token", now))
	require.False(t, cursor.HasActiveLease(token, expiresAt))
}
