package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRetryBillingEligibility_RetriesTransientAndSucceeds(t *testing.T) {
	attempts := 0
	err := retryBillingEligibility(context.Background(), func(context.Context) error {
		attempts++
		if attempts < billingEligibilityMaxAttempts {
			return service.ErrBillingServiceUnavailable.WithCause(errors.New("redis unavailable"))
		}
		return nil
	}, nil)

	require.NoError(t, err)
	require.Equal(t, billingEligibilityMaxAttempts, attempts)
}

func TestRetryBillingEligibility_StopsAfterMaxAttemptsForTransient(t *testing.T) {
	attempts := 0
	err := retryBillingEligibility(context.Background(), func(context.Context) error {
		attempts++
		return service.ErrBillingServiceUnavailable
	}, nil)

	require.ErrorIs(t, err, service.ErrBillingServiceUnavailable)
	require.Equal(t, billingEligibilityMaxAttempts, attempts)
}

func TestRetryBillingEligibility_DoesNotRetryNonTransientBillingError(t *testing.T) {
	attempts := 0
	err := retryBillingEligibility(context.Background(), func(context.Context) error {
		attempts++
		return service.ErrInsufficientBalance
	}, nil)

	require.ErrorIs(t, err, service.ErrInsufficientBalance)
	require.Equal(t, 1, attempts)
}

func TestRetryBillingEligibility_StopsDuringBackoffWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	err := retryBillingEligibility(ctx, func(context.Context) error {
		attempts++
		cancel()
		return service.ErrBillingServiceUnavailable
	}, []time.Duration{time.Hour, time.Hour})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestBillingEligibilityRetryDelay_UsesConfiguredBackoffsWithSmallJitter(t *testing.T) {
	backoffs := []time.Duration{100 * time.Millisecond, 300 * time.Millisecond}

	for attempt, base := range backoffs {
		for i := 0; i < 100; i++ {
			delay := billingEligibilityRetryDelay(attempt, backoffs)
			require.GreaterOrEqual(t, delay, base-base/10)
			require.LessOrEqual(t, delay, base+base/10)
		}
	}
}
