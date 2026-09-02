package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateTemporaryBalanceGrantRejectsInvalidAmount(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	for _, amount := range []float64{0, -1, 1e12} {
		err := validateTemporaryBalanceGrant(amount, expiresAt, time.Now().UTC())
		require.Error(t, err, "amount=%v must be rejected", amount)
	}
}

func TestValidateTemporaryBalanceGrantRequiresFutureExpiry(t *testing.T) {
	now := time.Now().UTC()
	for _, expiresAt := range []time.Time{now, now.Add(-time.Second), now.AddDate(20, 0, 0)} {
		err := validateTemporaryBalanceGrant(10, expiresAt, now)
		require.Error(t, err, "expiry=%s must be rejected", expiresAt)
	}
}

func TestValidateTemporaryBalanceGrantNormalizesUTC(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(24 * time.Hour)
	require.NoError(t, validateTemporaryBalanceGrant(10, expiresAt, now))
}
