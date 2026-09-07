package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScheduledTestHealthSampleOutcome(t *testing.T) {
	t.Run("success is eligible", func(t *testing.T) {
		success, eligible := scheduledTestHealthSampleOutcome(context.Background(), &ScheduledTestResult{Status: "success"})
		require.True(t, success)
		require.True(t, eligible)
	})

	t.Run("account-like failure is eligible", func(t *testing.T) {
		success, eligible := scheduledTestHealthSampleOutcome(context.Background(), &ScheduledTestResult{
			Status:       "failed",
			ErrorMessage: "upstream request timeout",
		})
		require.False(t, success)
		require.True(t, eligible)
	})

	t.Run("deterministic request failure is excluded", func(t *testing.T) {
		success, eligible := scheduledTestHealthSampleOutcome(context.Background(), &ScheduledTestResult{
			Status:       "failed",
			ErrorMessage: "upstream HTTP 400: unsupported model",
		})
		require.False(t, success)
		require.False(t, eligible)
	})

	t.Run("runner cancellation is excluded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, eligible := scheduledTestHealthSampleOutcome(ctx, &ScheduledTestResult{Status: "failed"})
		require.False(t, eligible)
	})
}
