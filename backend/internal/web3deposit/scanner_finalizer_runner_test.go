package web3deposit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScannerFinalizerRunnerRunsFinalizerAfterScanner(t *testing.T) {
	order := make([]string, 0, 2)
	runner, err := NewScannerFinalizerRunner(
		scannerRunnerFunc(func(context.Context, string, time.Time) (ScannerResult, error) {
			order = append(order, "scanner")
			return ScannerResult{Advanced: true}, nil
		}),
		finalizerRunnerFunc(func(context.Context, string, time.Time) (FinalizerResult, error) {
			order = append(order, "finalizer")
			return FinalizerResult{Advanced: true}, nil
		}),
	)
	require.NoError(t, err)

	result, err := runner.ScanNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.True(t, result.Advanced)
	require.Equal(t, []string{"scanner", "finalizer"}, order)
}

func TestScannerFinalizerRunnerStillFinalizesAfterTransientScannerError(t *testing.T) {
	wantErr := errors.New("latest block unavailable")
	finalizerCalled := false
	runner, err := NewScannerFinalizerRunner(
		scannerRunnerFunc(func(context.Context, string, time.Time) (ScannerResult, error) {
			return ScannerResult{}, wantErr
		}),
		finalizerRunnerFunc(func(context.Context, string, time.Time) (FinalizerResult, error) {
			finalizerCalled = true
			return FinalizerResult{}, nil
		}),
	)
	require.NoError(t, err)

	_, err = runner.ScanNext(context.Background(), "lease-token", time.Now())

	require.ErrorIs(t, err, wantErr)
	require.True(t, finalizerCalled)
}

func TestScannerFinalizerRunnerStopsWhenScannerLostLease(t *testing.T) {
	finalizerCalled := false
	runner, err := NewScannerFinalizerRunner(
		scannerRunnerFunc(func(context.Context, string, time.Time) (ScannerResult, error) {
			return ScannerResult{}, ErrLeaseNotHeld
		}),
		finalizerRunnerFunc(func(context.Context, string, time.Time) (FinalizerResult, error) {
			finalizerCalled = true
			return FinalizerResult{}, nil
		}),
	)
	require.NoError(t, err)

	_, err = runner.ScanNext(context.Background(), "lease-token", time.Now())

	require.ErrorIs(t, err, ErrLeaseNotHeld)
	require.False(t, finalizerCalled)
}

type finalizerRunnerFunc func(context.Context, string, time.Time) (FinalizerResult, error)

func (f finalizerRunnerFunc) FinalizeNext(ctx context.Context, token string, now time.Time) (FinalizerResult, error) {
	return f(ctx, token, now)
}
